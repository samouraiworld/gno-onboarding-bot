package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/bwmarrin/discordgo"

	"onboardingbot/internal/config"
	"onboardingbot/internal/harvest"
	"onboardingbot/internal/notify"
	"onboardingbot/internal/sheet"
	"onboardingbot/internal/templates"
)

const (
	cmdHarvest       = "harvest"
	cmdHarvestImport = "harvest-import"
	maxDigestBytes   = 5 << 20 // 5 MiB cap on the uploaded digest.json
)

// RegisterHarvest registers the two reviewer-only end-of-window commands:
// `/harvest` (collect evidence, write deterministic columns, return harvest.json)
// and `/harvest-import` (write the judgment columns from a digest.json upload).
func RegisterHarvest(s *discordgo.Session, cfg *config.Config, api sheet.API, _ *templates.Templates) error {
	run := &discordgo.ApplicationCommand{
		Name:        cmdHarvest,
		Description: "Collect end-of-window Discord evidence and update the tracker (reviewers only)",
		Type:        discordgo.ChatApplicationCommand,
	}
	// Command channel/role restriction is configured manually by a guild admin
	// (Discord rejects the permissions endpoint for bot tokens); the reviewer
	// gate is enforced at runtime by hasRole in each handler.
	if _, err := s.ApplicationCommandCreate(s.State.User.ID, cfg.GuildID, run); err != nil {
		return fmt.Errorf("create harvest command: %w", err)
	}

	imp := &discordgo.ApplicationCommand{
		Name:        cmdHarvestImport,
		Description: "Import a competency digest.json and fill the readiness columns (reviewers only)",
		Type:        discordgo.ChatApplicationCommand,
		Options: []*discordgo.ApplicationCommandOption{{
			Type:        discordgo.ApplicationCommandOptionAttachment,
			Name:        "file",
			Description: "The digest.json produced by the competency-digest skill",
			Required:    true,
		}},
	}
	if _, err := s.ApplicationCommandCreate(s.State.User.ID, cfg.GuildID, imp); err != nil {
		return fmt.Errorf("create harvest-import command: %w", err)
	}

	s.AddHandler(func(s *discordgo.Session, i *discordgo.InteractionCreate) {
		if i.Type != discordgo.InteractionApplicationCommand {
			return
		}
		switch i.ApplicationCommandData().Name {
		case cmdHarvest:
			handleHarvest(s, i, cfg, api)
		case cmdHarvestImport:
			handleHarvestImport(s, i, cfg, api)
		}
	})
	return nil
}

func handleHarvest(s *discordgo.Session, i *discordgo.InteractionCreate, cfg *config.Config, api sheet.API) {
	if !hasRole(i.Member, cfg.ReviewerRoleID) {
		respondError(s, i.Interaction, "You need the reviewer role to run the harvest.")
		return
	}
	if err := deferEphemeral(s, i.Interaction); err != nil {
		return
	}
	ctx := context.Background()

	// Self-heal the layout (headers, checkboxes, evidence tab) in case it was
	// removed, or the sheet was shared only after the bot started.
	if err := sheet.EnsureHarvestLayout(ctx, api, cfg.SheetID, cfg.SheetName); err != nil {
		editEphemeral(s, i.Interaction, "Could not prepare the sheet layout: "+err.Error())
		return
	}

	all, err := sheet.ReadCandidates(ctx, api, cfg.SheetID, cfg.SheetName)
	if err != nil {
		editEphemeral(s, i.Interaction, "Could not read the candidate tracker. Check the bot's Sheet access.")
		return
	}
	if len(all) == 0 {
		editEphemeral(s, i.Interaction, "No candidates in the tracker yet, nothing to harvest.")
		return
	}
	// Leave already-validated candidates untouched: skip them so their existing
	// columns and Evidence are preserved across re-runs.
	var active []sheet.TrackerRow
	validated := 0
	for _, r := range all {
		if sheet.IsValidated(r.Status) {
			validated++
			continue
		}
		active = append(active, r)
	}
	// Collapse duplicate handles (a resubmission appends a new row): keep the
	// latest row per handle for evaluation, mark the older ones as duplicates.
	records, superseded := partitionLatest(active)
	if len(records) == 0 {
		editEphemeral(s, i.Interaction, fmt.Sprintf("Nothing to harvest: all %d candidate(s) are already validated and were left untouched.", validated))
		return
	}

	since := cfg.HarvestSinceParsed // validated at config load; zero = no bound

	channels := []struct{ id, key string }{
		{cfg.GeneralChatChannelID, harvest.ChannelGeneral},
		{cfg.OnboardingChannelID, harvest.ChannelOnboarding},
		{cfg.ValidatorReviewChannelID, harvest.ChannelReview},
	}

	var messages []harvest.Message
	idByRow := map[int]string{}
	for _, ch := range channels {
		raw, ferr := fetchRaw(s, ch.id, since, cfg.HarvestMaxMessages)
		if ferr != nil {
			editEphemeral(s, i.Interaction, fmt.Sprintf("Could not read <#%s>: %v.\nCheck the Message Content intent and the bot's Read Message History permission.", ch.id, ferr))
			return
		}
		// Per-channel counts so an operator can spot a mis-set channel ID (returns
		// 0) versus a quiet channel from the logs.
		log.Printf("harvest: %s channel (%s) returned %d messages", ch.key, ch.id, len(raw))
		// The submission embeds in #validator-review carry the candidate's Discord
		// ID in their footer (rowref). Decode them so reviewer @mentions can be
		// attributed to the right candidate even when usernames differ.
		if ch.key == harvest.ChannelReview {
			for _, m := range raw {
				if row, candidateID, _, perr := notify.ParseSubmissionEmbed(m); perr == nil {
					idByRow[row] = candidateID
				}
			}
		}
		messages = append(messages, toHarvestMessages(raw, cfg.GuildID, ch.id, ch.key)...)
	}

	crecords := make([]harvest.CandidateRecord, len(records))
	for idx, r := range records {
		// PR #4 verifies the valoper on-chain at /submit and writes the operator
		// address (col K) only on success, so its presence is the valoper verdict.
		valState, valDetail := harvest.StateNotFound, "no operator address on the tracker (not registered at submit)"
		if strings.TrimSpace(r.OperatorAddress) != "" {
			valState, valDetail = harvest.StateFound, "operator address recorded (verified on-chain at submit)"
		}
		crecords[idx] = harvest.CandidateRecord{
			Row:            r.Row,
			Candidate:      r.Candidate,
			Discord:        r.Discord,
			ResolvedUserID: idByRow[r.Row],
			Moniker:        strings.TrimSpace(r.Moniker + " " + r.OperatorAddress),
			Valoper:        r.Valoper,
			Introduction:   r.Introduction,
			ValoperState:   valState,
			ValoperDetail:  valDetail,
		}
	}

	hf := harvest.Build(cfg.GuildID, time.Now(), crecords, messages)

	if err := sheet.WriteEvidence(ctx, api, cfg.SheetID, sheet.EvidenceTabName(cfg.SheetName), buildEvidenceRows(hf)); err != nil {
		editEphemeral(s, i.Interaction, "Harvest computed, but writing the Evidence tab failed: "+err.Error())
		return
	}
	writeFailures := 0
	for _, c := range hf.Candidates {
		if err := sheet.WriteHarvestColumns(ctx, api, cfg.SheetID, cfg.SheetName, c.Row, c.Signals.RedFlagsCell(), c.Signals.EngagementCell()); err != nil {
			log.Printf("harvest: write deterministic columns for row %d: %v", c.Row, err)
			writeFailures++
		}
	}
	for _, sup := range superseded {
		if err := sheet.MarkDuplicateRow(ctx, api, cfg.SheetID, cfg.SheetName, sup.row, sup.keptRow); err != nil {
			log.Printf("harvest: mark duplicate row %d: %v", sup.row, err)
			writeFailures++
		}
	}

	data, err := json.MarshalIndent(hf, "", "  ")
	if err != nil {
		editEphemeral(s, i.Interaction, "Harvest written to the Sheet, but encoding harvest.json failed.")
		return
	}

	var notes []string
	if validated > 0 {
		notes = append(notes, fmt.Sprintf("%d already-validated", validated))
	}
	if len(superseded) > 0 {
		notes = append(notes, fmt.Sprintf("%d duplicate rows collapsed", len(superseded)))
	}
	skipped := ""
	if len(notes) > 0 {
		skipped = " (" + strings.Join(notes, ", ") + ")"
	}
	warning := ""
	if writeFailures > 0 {
		warning = fmt.Sprintf("\n\nWarning: %d row(s) failed to write to the tracker (see logs); re-run to retry.", writeFailures)
	}
	content := fmt.Sprintf(
		"Harvest complete: %d candidates%s, %d messages. The Evidence tab and the Red flags / Engagement columns are updated.%s\n\nNext: run the `competency-digest` skill on the attached `harvest.json`, then `/harvest-import` the resulting `digest.json`.",
		len(hf.Candidates), skipped, len(messages), warning,
	)
	if _, err := s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
		Content: &content,
		Files:   []*discordgo.File{{Name: "harvest.json", ContentType: "application/json", Reader: bytes.NewReader(data)}},
	}); err != nil {
		log.Printf("harvest: attach harvest.json: %v", err)
		editEphemeral(s, i.Interaction, "Harvest written to the Sheet, but attaching harvest.json failed. Re-run to retry the download.")
	}
}

func handleHarvestImport(s *discordgo.Session, i *discordgo.InteractionCreate, cfg *config.Config, api sheet.API) {
	if !hasRole(i.Member, cfg.ReviewerRoleID) {
		respondError(s, i.Interaction, "You need the reviewer role to import a digest.")
		return
	}
	if err := deferEphemeral(s, i.Interaction); err != nil {
		return
	}

	data := i.ApplicationCommandData()
	if len(data.Options) == 0 || data.Resolved == nil {
		editEphemeral(s, i.Interaction, "No file attached.")
		return
	}
	attachID, ok := data.Options[0].Value.(string)
	if !ok {
		editEphemeral(s, i.Interaction, "Could not read the attached file option.")
		return
	}
	att, ok := data.Resolved.Attachments[attachID]
	if !ok {
		editEphemeral(s, i.Interaction, "Could not resolve the attached file.")
		return
	}

	body, err := download(att.URL, maxDigestBytes)
	if err != nil {
		editEphemeral(s, i.Interaction, "Could not download the attached file: "+err.Error())
		return
	}
	digest, err := harvest.ParseDigest(body)
	if err != nil {
		editEphemeral(s, i.Interaction, "The attached file is not a valid digest.json: "+err.Error())
		return
	}

	ctx := context.Background()
	if err := sheet.EnsureHarvestLayout(ctx, api, cfg.SheetID, cfg.SheetName); err != nil {
		editEphemeral(s, i.Interaction, "Could not prepare the sheet layout: "+err.Error())
		return
	}
	// Validate each digest row against the live tracker before writing. A digest
	// produced from a stale harvest.json (rows shifted since) would otherwise
	// stamp a readiness verdict onto an unrelated candidate.
	all, err := sheet.ReadCandidates(ctx, api, cfg.SheetID, cfg.SheetName)
	if err != nil {
		editEphemeral(s, i.Interaction, "Could not read the candidate tracker to validate the digest. Check the bot's Sheet access.")
		return
	}
	nameByRow := make(map[int]string, len(all))
	for _, r := range all {
		nameByRow[r.Row] = r.Candidate
	}
	normName := func(s string) string { return strings.ToLower(strings.TrimSpace(s)) }

	written, failed, skipped := 0, 0, 0
	var skipNotes []string
	for _, c := range digest.Candidates {
		trackerName, known := nameByRow[c.Row]
		switch {
		case !known:
			skipped++
			skipNotes = append(skipNotes, fmt.Sprintf("row %d (no candidate on the tracker)", c.Row))
			continue
		case normName(c.Candidate) != "" && normName(c.Candidate) != normName(trackerName):
			skipped++
			skipNotes = append(skipNotes, fmt.Sprintf("row %d (digest %q vs tracker %q)", c.Row, c.Candidate, trackerName))
			continue
		}
		if err := sheet.WriteDigestColumns(ctx, api, cfg.SheetID, cfg.SheetName, c.Row, c.ReadinessCell(), c.Summary, strings.Join(c.EvidenceLinks, "\n"), c.CriteriaBools()); err != nil {
			log.Printf("import: write row %d: %v", c.Row, err)
			failed++
			continue
		}
		written++
	}

	msg := fmt.Sprintf("Imported %d candidate(s).", written)
	if failed > 0 {
		msg += fmt.Sprintf(" %d failed (see logs).", failed)
	}
	if skipped > 0 {
		msg += fmt.Sprintf(" %d skipped: %s.", skipped, joinCapped(skipNotes, 10))
	}
	if failed == 0 && skipped == 0 {
		msg += " Sort the tracker by Readiness to surface the most complete candidates."
	}
	editEphemeral(s, i.Interaction, msg)
}

// joinCapped joins notes with "; ", showing at most max entries followed by a
// count of the remainder, so a digest full of mismatches cannot overflow the
// Discord message limit.
func joinCapped(notes []string, max int) string {
	if len(notes) <= max {
		return strings.Join(notes, "; ")
	}
	return strings.Join(notes[:max], "; ") + fmt.Sprintf("; and %d more", len(notes)-max)
}

type supersededRow struct{ row, keptRow int }

// partitionLatest splits candidates into the rows to evaluate and the superseded
// duplicates. Within a normalized handle the highest row number wins (a
// resubmission appends a new row below the old one); rows with no handle are all
// kept, since there is nothing to dedup them on.
func partitionLatest(active []sheet.TrackerRow) (records []sheet.TrackerRow, superseded []supersededRow) {
	latestRow := map[string]int{}
	for _, r := range active {
		if h := harvest.NormalizeHandle(r.Discord); h != "" && r.Row > latestRow[h] {
			latestRow[h] = r.Row
		}
	}
	seen := map[string]bool{}
	for _, r := range active {
		h := harvest.NormalizeHandle(r.Discord)
		switch {
		case h == "":
			records = append(records, r)
		case r.Row == latestRow[h] && !seen[h]:
			records = append(records, r)
			seen[h] = true
		case r.Row != latestRow[h]:
			superseded = append(superseded, supersededRow{r.Row, latestRow[h]})
		}
	}
	return records, superseded
}

// fetchRaw pages through a channel's messages newest-first, stopping at `since`
// (zero = no lower bound) or after `max` messages.
func fetchRaw(s *discordgo.Session, channelID string, since time.Time, max int) ([]*discordgo.Message, error) {
	var out []*discordgo.Message
	beforeID := ""
	for len(out) < max {
		batch, err := s.ChannelMessages(channelID, 100, beforeID, "", "")
		if err != nil {
			return nil, err
		}
		if len(batch) == 0 {
			break
		}
		stop := false
		// Discord returns each before-paginated batch newest-first, so the first
		// message older than `since` means every later one is older too: stop.
		for _, m := range batch {
			if !since.IsZero() && m.Timestamp.Before(since) {
				stop = true
				break
			}
			out = append(out, m)
			if len(out) >= max {
				break
			}
		}
		if stop {
			break
		}
		beforeID = batch[len(batch)-1].ID
	}
	return out, nil
}

func toHarvestMessages(raw []*discordgo.Message, guildID, channelID, channelKey string) []harvest.Message {
	out := make([]harvest.Message, 0, len(raw))
	for _, m := range raw {
		if m.Author == nil || m.Author.Bot {
			continue
		}
		out = append(out, harvest.Message{
			ChannelKey:     channelKey,
			AuthorID:       m.Author.ID,
			AuthorUsername: m.Author.Username,
			Content:        m.Content,
			Timestamp:      m.Timestamp,
			Permalink:      notify.MessagePermalink(guildID, channelID, m.ID),
		})
	}
	return out
}

func buildEvidenceRows(hf harvest.HarvestFile) [][]interface{} {
	var rows [][]interface{}
	for _, c := range hf.Candidates {
		for _, m := range c.Messages {
			rows = append(rows, []interface{}{c.Candidate, c.Row, m.Channel, "candidate", m.Timestamp, m.Permalink, m.Text})
		}
		for _, m := range c.ReviewerCtx {
			rows = append(rows, []interface{}{c.Candidate, c.Row, harvest.ChannelReview, m.Author, m.Timestamp, m.Permalink, m.Text})
		}
	}
	return rows
}

var downloadClient = &http.Client{Timeout: 30 * time.Second}

func download(url string, limit int64) ([]byte, error) {
	resp, err := downloadClient.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("status %d", resp.StatusCode)
	}
	return io.ReadAll(io.LimitReader(resp.Body, limit))
}
