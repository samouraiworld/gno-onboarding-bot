package sheet

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"sync"
)

type Column int

const (
	ColumnCandidate Column = iota
	ColumnDiscord
	ColumnStatus
	ColumnChallengeSubmitted
	ColumnReviewers
	ColumnMissingCriteria
	ColumnDecisionDate
	ColumnValoperLink
	ColumnGovDAOStatus
	ColumnMoniker
	ColumnOperatorAddress
	ColumnIntroduction
	ColumnReviewMessageLink
	// Harvest assessment columns (N-Y), appended after the A-M intake columns.
	// The bot refreshes these on every harvest/import. Curating "good validators"
	// is done via the Status column + PR #4's "-approved" view, not a separate
	// Selected column.
	ColumnReadiness     // N
	ColumnSummary       // O
	ColumnSetup         // P ┐
	ColumnSync          // Q │
	ColumnTx            // R │ seven criterion checkboxes, in harvest.Criteria order
	ColumnValoper       // S │
	ColumnOps           // T │
	ColumnComms         // U │
	ColumnSafety        // V ┘
	ColumnRedFlags      // W
	ColumnEngagement    // X
	ColumnEvidenceLinks // Y
)

var columnLetters = []string{
	"A", "B", "C", "D", "E", "F", "G", "H", "I", "J", "K", "L", "M",
	"N", "O", "P", "Q", "R", "S", "T", "U", "V", "W", "X", "Y",
}

// criterionColumns are the seven checkbox columns (P-V), in harvest.Criteria order.
var criterionColumns = []Column{
	ColumnSetup, ColumnSync, ColumnTx, ColumnValoper, ColumnOps, ColumnComms, ColumnSafety,
}

// derivedColumns lists the harvest assessment columns N-Y in order.
var derivedColumns = []Column{
	ColumnReadiness, ColumnSummary,
	ColumnSetup, ColumnSync, ColumnTx, ColumnValoper, ColumnOps, ColumnComms, ColumnSafety,
	ColumnRedFlags, ColumnEngagement, ColumnEvidenceLinks,
}

// derivedHeaders labels the assessment columns (N-Y).
var derivedHeaders = map[Column]string{
	ColumnReadiness:     "Readiness",
	ColumnSummary:       "Summary",
	ColumnSetup:         "Setup",
	ColumnSync:          "Sync",
	ColumnTx:            "Tx",
	ColumnValoper:       "Valoper",
	ColumnOps:           "Ops",
	ColumnComms:         "Comms",
	ColumnSafety:        "Safety",
	ColumnRedFlags:      "Red flags",
	ColumnEngagement:    "Engagement",
	ColumnEvidenceLinks: "Evidence links",
}

func columnLetter(c Column) string {
	return columnLetters[c]
}

const (
	StatusCandidate           = "Candidate"
	StatusChallengeInProgress = "Challenge in progress"
	StatusNeedsRetry          = "Needs retry"
	StatusDeclined            = "Declined"
	StatusApproved            = "Approved"
	StatusGovDAOPending       = "GovDAO pending"
	StatusGovDAOSubmitted     = "GovDAO submitted"
)

type CandidateRow struct {
	Candidate          string
	Discord            string
	Status             string
	ChallengeSubmitted string
	Valoper            string
	Moniker            string
	OperatorAddress    string
	Introduction       string
}

func (r CandidateRow) toValues() []interface{} {
	return []interface{}{
		r.Candidate,          // A
		r.Discord,            // B
		r.Status,             // C
		r.ChallengeSubmitted, // D
		"",                   // E Reviewers
		"",                   // F MissingCriteria
		"",                   // G DecisionDate
		r.Valoper,            // H ValoperLink
		"",                   // I GovDAOStatus
		r.Moniker,            // J
		r.OperatorAddress,    // K
		r.Introduction,       // L
		"",                   // M ReviewMessageLink
	}
}

var updatedRangeRe = regexp.MustCompile(`![A-Z]+(\d+):[A-Z]+\d+$`)

func parseRowNumber(updatedRange string) (int, error) {
	m := updatedRangeRe.FindStringSubmatch(updatedRange)
	if m == nil {
		return 0, fmt.Errorf("unrecognized updated range %q", updatedRange)
	}
	return strconv.Atoi(m[1])
}

// Headers are the column titles written to row 1 by Ensure. Order matches the
// Column iota above; the slice is indexed by Column for safety.
var Headers = []string{
	"Candidate",
	"Discord",
	"Status",
	"Challenge submitted",
	"Reviewers",
	"Missing criteria",
	"Decision date",
	"Valoper link",
	"GovDAO status",
	"Moniker",
	"Operator address",
	"Introduction",
	"Review message link",
}

// allHeaders is the full A-Y header row: the intake headers (A-M) followed by the
// harvest assessment headers (N-Y) in column order.
func allHeaders() []interface{} {
	out := make([]interface{}, 0, len(Headers)+len(derivedColumns))
	for _, h := range Headers {
		out = append(out, h)
	}
	for _, c := range derivedColumns {
		out = append(out, derivedHeaders[c])
	}
	return out
}

type API interface {
	Append(ctx context.Context, spreadsheetID, rangeA1 string, values []interface{}) (updatedRange string, err error)
	Update(ctx context.Context, spreadsheetID, rangeA1, value string) error
	Get(ctx context.Context, spreadsheetID, rangeA1 string) ([][]interface{}, error)
	UpdateRow(ctx context.Context, spreadsheetID, rangeA1 string, values []interface{}) error
	SetFormula(ctx context.Context, spreadsheetID, rangeA1, formula string) error
	EnsureTab(ctx context.Context, spreadsheetID, sheetName string) (created bool, err error)
	SpreadsheetLocale(ctx context.Context, spreadsheetID string) (string, error)
	SetDropdown(ctx context.Context, spreadsheetID, sheetName string, col Column, startRow, endRow int, values []string) error
	SetLinkedText(ctx context.Context, spreadsheetID, sheetName string, row, col int, text, url string) error
	SetLinkedLines(ctx context.Context, spreadsheetID, sheetName string, row, col int, lines []LinkedLine) error
	SetStatusColors(ctx context.Context, spreadsheetID, sheetName string, statusCol Column, mapping map[string]string) error
	FreezeHeaderRow(ctx context.Context, spreadsheetID, sheetName string) error
	SetCheckbox(ctx context.Context, spreadsheetID, sheetName string, startCol, endCol Column) error
	ClearValues(ctx context.Context, spreadsheetID, rangeA1 string) error
	WriteRows(ctx context.Context, spreadsheetID, rangeA1 string, values [][]interface{}) error
}

// StatusColors maps each status to a light hex background color used by
// EnsureStatusColors.
var StatusColors = map[string]string{
	StatusCandidate:           "#e0e0e0",
	StatusChallengeInProgress: "#fff2a8",
	StatusNeedsRetry:          "#fcd5b4",
	StatusDeclined:            "#f4c7c3",
	StatusApproved:            "#c2eebc",
	StatusGovDAOPending:       "#b6d7f5",
	StatusGovDAOSubmitted:     "#d9c4ec",
}

// EnsureStatusColors installs the status-row coloring rules on sheetName,
// coloring the full A-Y schema width of each matching row.
func EnsureStatusColors(ctx context.Context, api API, spreadsheetID, sheetName string) error {
	return api.SetStatusColors(ctx, spreadsheetID, sheetName, ColumnStatus, StatusColors)
}

// EnsureFrozenHeader freezes the first row of sheetName so it stays visible
// while scrolling.
func EnsureFrozenHeader(ctx context.Context, api API, spreadsheetID, sheetName string) error {
	return api.FreezeHeaderRow(ctx, spreadsheetID, sheetName)
}

// AllStatuses lists the canonical status values used as the source for the
// column-C dropdown on the source tab.
var AllStatuses = []string{
	StatusCandidate,
	StatusChallengeInProgress,
	StatusNeedsRetry,
	StatusDeclined,
	StatusApproved,
	StatusGovDAOPending,
	StatusGovDAOSubmitted,
}

// EnsureStatusDropdown installs the dropdown on column C, sized to the
// current data band so empty rows don't show the dropdown arrow.
func EnsureStatusDropdown(ctx context.Context, api API, spreadsheetID, sheetName string) error {
	dataRange := fmt.Sprintf("%s!A2:A", sheetName)
	data, err := api.Get(ctx, spreadsheetID, dataRange)
	if err != nil {
		return fmt.Errorf("scan column A to size dropdown: %w", err)
	}
	if len(data) == 0 {
		return nil
	}
	endRow := 1 + len(data) // 1-based, inclusive
	return api.SetDropdown(ctx, spreadsheetID, sheetName, ColumnStatus, 2, endRow, AllStatuses)
}

// ApplyStatusDropdown extends the column-C dropdown so it covers rows 2..row
// inclusive. Call after writing a new row so the new row shows the dropdown
// without restarting the bot.
func ApplyStatusDropdown(ctx context.Context, api API, spreadsheetID, sheetName string, row int) error {
	if row < 2 {
		return nil
	}
	return api.SetDropdown(ctx, spreadsheetID, sheetName, ColumnStatus, 2, row, AllStatuses)
}

// formulaArgSep returns the function-argument separator the given spreadsheet
// locale expects. Locales whose decimal separator is "," (most of Europe and
// Latin America) require ";" between function args; comma-decimal locales
// (en_*, ja_*, ko_*, zh_*, etc.) use ",". Google Sheets does NOT auto-translate
// formulas written through the API, so we must format with the right separator.
func formulaArgSep(locale string) string {
	commaArgPrefixes := []string{"en", "ja", "ko", "zh", "th", "id", "ms", "vi", "tl", "hi"}
	for _, p := range commaArgPrefixes {
		if strings.HasPrefix(locale, p+"_") || locale == p {
			return ","
		}
	}
	return ";"
}

// ApprovedTabName returns the name of the live-filtered tab that mirrors the
// source tab's rows whose status is "GovDAO pending".
func ApprovedTabName(sourceSheetName string) string {
	return sourceSheetName + "-approved"
}

// Ensure makes sure the named tab exists in the spreadsheet and that row 1
// holds the column headers. Safe to call multiple times. Refuses to write
// headers if row 1 has any non-empty cell, so it cannot clobber unrelated data
// the operator may have placed there.
func Ensure(ctx context.Context, api API, spreadsheetID, sheetName string) error {
	if _, err := api.EnsureTab(ctx, spreadsheetID, sheetName); err != nil {
		return fmt.Errorf("ensure tab %q: %w", sheetName, err)
	}
	headerRange := fmt.Sprintf("%s!A1:%s1", sheetName, columnLetter(Column(len(Headers)-1)))
	row1, err := api.Get(ctx, spreadsheetID, headerRange)
	if err != nil {
		return fmt.Errorf("read header row: %w", err)
	}
	if len(row1) > 0 && !rowIsEmpty(row1[0]) {
		return nil
	}
	values := make([]interface{}, len(Headers))
	for i, h := range Headers {
		values[i] = h
	}
	if err := api.UpdateRow(ctx, spreadsheetID, headerRange, values); err != nil {
		return fmt.Errorf("write header row: %w", err)
	}
	return nil
}

// EnsureApprovedView creates the "{source}-approved" tab if missing and
// populates it with the full column headers (intake A-M plus the harvest
// assessment columns N-Y) and a live QUERY that mirrors every column of the
// source tab's GovDAO-progressing rows. Safe to call multiple times.
func EnsureApprovedView(ctx context.Context, api API, spreadsheetID, sourceSheetName string) error {
	tab := ApprovedTabName(sourceSheetName)
	if _, err := api.EnsureTab(ctx, spreadsheetID, tab); err != nil {
		return fmt.Errorf("ensure tab %q: %w", tab, err)
	}
	lastCol := columnLetter(ColumnEvidenceLinks) // mirror the full A-Y schema
	// Always (re)write the full header row: this tab is a bot-owned view, and an
	// older A-M header row from before the assessment columns existed must be
	// brought up to A-Y rather than skipped.
	headerRange := fmt.Sprintf("%s!A1:%s1", tab, lastCol)
	if err := api.UpdateRow(ctx, spreadsheetID, headerRange, allHeaders()); err != nil {
		return fmt.Errorf("write headers to %q: %w", tab, err)
	}
	// Always rewrite the QUERY too. Idempotent on the happy path; on the unhappy
	// path (cell holds a locale-broken formula whose rendered value reads as
	// "#ERROR!", i.e. non-empty), a skip check would prevent the fix from ever
	// applying.
	locale, err := api.SpreadsheetLocale(ctx, spreadsheetID)
	if err != nil {
		return fmt.Errorf("read spreadsheet locale: %w", err)
	}
	sep := formulaArgSep(locale)
	formulaCellRange := fmt.Sprintf("%s!A2", tab)
	formula := approvedViewFormula(sourceSheetName, lastCol, columnLetter(ColumnStatus), sep)
	if err := api.SetFormula(ctx, spreadsheetID, formulaCellRange, formula); err != nil {
		return fmt.Errorf("write approved-view formula to %q: %w", tab, err)
	}
	return nil
}

// approvedViewFormula builds the spilling array formula for the "-approved"
// tab: a single QUERY selecting both GovDAO statuses, ordered so "GovDAO
// submitted" rows sort above "GovDAO pending" rows ("submitted" > "pending",
// so `order by desc`). IFERROR renders "" when neither category has rows.
//
// A single self-spilling QUERY is used deliberately: IFS/VSTACK cannot return a
// multi-row array from a branch (Sheets raises "IFS range size inconsistent"),
// so the earlier LET/IFS form errored. headers=0 keeps QUERY from lifting the
// first data row into a header. sep is the locale's function-argument separator
// ("," or ";"); the SQL is a single string literal, so it is locale-independent.
func approvedViewFormula(sourceSheetName, lastCol, statusCol, sep string) string {
	src := fmt.Sprintf("'%s'!A2:%s", sourceSheetName, lastCol)
	sql := fmt.Sprintf(`"select * where %s = '%s' or %s = '%s' order by %s desc"`,
		statusCol, StatusGovDAOSubmitted, statusCol, StatusGovDAOPending, statusCol)
	return fmt.Sprintf(`=IFERROR(QUERY(%s%s %s%s 0)%s "")`, src, sep, sql, sep, sep)
}

func rowIsEmpty(r []interface{}) bool {
	for _, c := range r {
		if c == nil {
			continue
		}
		if s, ok := c.(string); ok && s == "" {
			continue
		}
		if fmt.Sprint(c) == "" {
			continue
		}
		return false
	}
	return true
}

// appendMu serializes the read-then-write inside AppendCandidateRow. discordgo
// runs each interaction in its own goroutine, so two near-simultaneous appends
// could otherwise both read the data band, compute the same target row, and have
// the second write silently clobber the first. A single process-wide lock here
// covers every caller (both /submit-request and /candidate-testnet) at once.
var appendMu sync.Mutex

// AppendCandidateRow places a candidate row in the first fully-empty row of the
// sheet's A:M data band (starting at row 2). It never overwrites existing data:
// rows with any non-empty cell are skipped, and if no gap exists the row is
// written one past the last row with any data.
func AppendCandidateRow(ctx context.Context, api API, spreadsheetID, sheetName string, row CandidateRow) (int, error) {
	appendMu.Lock()
	defer appendMu.Unlock()

	dataRange := fmt.Sprintf("%s!A2:%s", sheetName, columnLetter(Column(len(Headers)-1)))
	data, err := api.Get(ctx, spreadsheetID, dataRange)
	if err != nil {
		return 0, fmt.Errorf("scan data range %q: %w", dataRange, err)
	}
	target := len(data) + 2
	for i, r := range data {
		if rowIsEmpty(r) {
			target = i + 2
			break
		}
	}
	targetRange := fmt.Sprintf("%s!A%d:%s%d", sheetName, target, columnLetter(Column(len(Headers)-1)), target)
	if err := api.UpdateRow(ctx, spreadsheetID, targetRange, row.toValues()); err != nil {
		return 0, fmt.Errorf("write row at %s: %w", targetRange, err)
	}
	return target, nil
}

// ClearRow blanks every column (A:M) of the given 1-based row. Used to roll
// back a just-appended row when a later step of the same submission fails, so
// the candidate can resubmit cleanly instead of being blocked by the row they
// could not finish.
func ClearRow(ctx context.Context, api API, spreadsheetID, sheetName string, row int) error {
	rangeA1 := fmt.Sprintf("%s!A%d:%s%d", sheetName, row, columnLetter(Column(len(Headers)-1)), row)
	blank := make([]interface{}, len(Headers))
	for i := range blank {
		blank[i] = ""
	}
	return api.UpdateRow(ctx, spreadsheetID, rangeA1, blank)
}

// FindByOperatorAddress scans the data band of sheetName for a row whose
// operator-address column matches addr. Returns the row number (1-based) and
// the row's status (col C). If no match, returns 0 and "".
func FindByOperatorAddress(ctx context.Context, api API, spreadsheetID, sheetName, addr string) (int, string, error) {
	dataRange := fmt.Sprintf("%s!A2:%s", sheetName, columnLetter(Column(len(Headers)-1)))
	data, err := api.Get(ctx, spreadsheetID, dataRange)
	if err != nil {
		return 0, "", fmt.Errorf("scan for duplicate: %w", err)
	}
	want := strings.TrimSpace(addr)
	for i, row := range data {
		if int(ColumnOperatorAddress) >= len(row) {
			continue
		}
		if strings.TrimSpace(fmt.Sprint(row[ColumnOperatorAddress])) != want {
			continue
		}
		status := ""
		if int(ColumnStatus) < len(row) {
			status = strings.TrimSpace(fmt.Sprint(row[ColumnStatus]))
		}
		return i + 2, status, nil
	}
	return 0, "", nil
}

func UpdateFields(ctx context.Context, api API, spreadsheetID, sheetName string, row int, fields map[Column]string) error {
	for col, value := range fields {
		rangeA1 := fmt.Sprintf("%s!%s%d", sheetName, columnLetter(col), row)
		if err := api.Update(ctx, spreadsheetID, rangeA1, value); err != nil {
			return fmt.Errorf("update %s: %w", rangeA1, err)
		}
	}
	return nil
}

// --- Harvest assessment layer (columns N-Y, the Evidence tab) ---

// EvidenceTabName is the derived raw-evidence tab the harvest manages, named
// "{source}-evidence" (mirrors the "{source}-approved" naming of the view tab).
func EvidenceTabName(sourceSheetName string) string { return sourceSheetName + "-evidence" }

// IsValidated reports whether a candidate has already passed onboarding and so
// should be left untouched by the harvest pass. Case- and whitespace-tolerant.
func IsValidated(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case strings.ToLower(StatusApproved), strings.ToLower(StatusGovDAOPending), strings.ToLower(StatusGovDAOSubmitted):
		return true
	}
	return false
}

// IsReopenable reports whether an existing row for an operator address is in a
// terminal-for-now state that permits a fresh /submit-request to append a new
// row for the same address: "Needs retry" (reviewer asked for a resubmission)
// or "Declined" (candidate must restart from the beginning). Case- and
// whitespace-tolerant.
func IsReopenable(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case strings.ToLower(StatusNeedsRetry), strings.ToLower(StatusDeclined):
		return true
	}
	return false
}

// TrackerRow is one existing candidate row, read back for the harvest pass.
type TrackerRow struct {
	Row             int
	Candidate       string
	Discord         string
	Status          string
	Valoper         string
	Moniker         string
	OperatorAddress string
	Introduction    string
}

func cellAt(row []interface{}, idx int) string {
	if idx < 0 || idx >= len(row) {
		return ""
	}
	if s, ok := row[idx].(string); ok {
		return s
	}
	return fmt.Sprint(row[idx])
}

// ReadCandidates reads the intake rows (A2:M); header row 1 is skipped and rows
// with an empty Candidate cell are ignored.
func ReadCandidates(ctx context.Context, api API, spreadsheetID, sheetName string) ([]TrackerRow, error) {
	rows, err := api.Get(ctx, spreadsheetID, sheetName+"!A2:M")
	if err != nil {
		return nil, fmt.Errorf("read candidates: %w", err)
	}
	var out []TrackerRow
	for i, r := range rows {
		candidate := cellAt(r, int(ColumnCandidate))
		if strings.TrimSpace(candidate) == "" {
			continue
		}
		out = append(out, TrackerRow{
			Row:             i + 2,
			Candidate:       candidate,
			Discord:         cellAt(r, int(ColumnDiscord)),
			Status:          cellAt(r, int(ColumnStatus)),
			Valoper:         cellAt(r, int(ColumnValoperLink)),
			Moniker:         cellAt(r, int(ColumnMoniker)),
			OperatorAddress: cellAt(r, int(ColumnOperatorAddress)),
			Introduction:    cellAt(r, int(ColumnIntroduction)),
		})
	}
	return out, nil
}

// EnsureHarvestLayout provisions the assessment layer on startup: the N-Y header
// row + criterion checkboxes on the source tab, and the "{source}-evidence" tab.
func EnsureHarvestLayout(ctx context.Context, api API, spreadsheetID, sheetName string) error {
	header := make([]interface{}, len(derivedColumns))
	for i, c := range derivedColumns {
		header[i] = derivedHeaders[c]
	}
	hr := fmt.Sprintf("%s!%s1:%s1", sheetName, columnLetter(derivedColumns[0]), columnLetter(derivedColumns[len(derivedColumns)-1]))
	if err := api.UpdateRow(ctx, spreadsheetID, hr, header); err != nil {
		return fmt.Errorf("write assessment headers: %w", err)
	}
	if err := api.SetCheckbox(ctx, spreadsheetID, sheetName, criterionColumns[0], criterionColumns[len(criterionColumns)-1]+1); err != nil {
		return fmt.Errorf("set criterion checkboxes: %w", err)
	}
	if _, err := api.EnsureTab(ctx, spreadsheetID, EvidenceTabName(sheetName)); err != nil {
		return fmt.Errorf("ensure evidence tab: %w", err)
	}
	return nil
}

// WriteHarvestColumns writes the deterministic columns /harvest owns: Red flags
// (W) and Engagement (X). Refreshed each run.
func WriteHarvestColumns(ctx context.Context, api API, spreadsheetID, sheetName string, row int, redFlags, engagement string) error {
	return UpdateFields(ctx, api, spreadsheetID, sheetName, row, map[Column]string{
		ColumnRedFlags:   redFlags,
		ColumnEngagement: engagement,
	})
}

// LinkedLine is one line of a multi-link cell: Text is the visible label and
// URL the link it points to.
type LinkedLine struct {
	Text string
	URL  string
}

// WriteDigestColumns writes the columns /harvest-import owns: Readiness (N),
// Summary (O), Evidence links (Y) as titled clickable links, and the seven
// criterion checkboxes (P-V) as booleans in criterionColumns order.
func WriteDigestColumns(ctx context.Context, api API, spreadsheetID, sheetName string, row int, readiness, summary string, evidence []LinkedLine, criteria []bool) error {
	if err := UpdateFields(ctx, api, spreadsheetID, sheetName, row, map[Column]string{
		ColumnReadiness: readiness,
		ColumnSummary:   summary,
	}); err != nil {
		return err
	}
	if err := api.SetLinkedLines(ctx, spreadsheetID, sheetName, row, int(ColumnEvidenceLinks), evidence); err != nil {
		return fmt.Errorf("write evidence links: %w", err)
	}
	return writeCriteria(ctx, api, spreadsheetID, sheetName, row, criteria)
}

func writeCriteria(ctx context.Context, api API, spreadsheetID, sheetName string, row int, criteria []bool) error {
	vals := make([]interface{}, len(criterionColumns))
	for i := range criterionColumns {
		vals[i] = i < len(criteria) && criteria[i]
	}
	rangeA1 := fmt.Sprintf("%s!%s%d:%s%d", sheetName,
		columnLetter(criterionColumns[0]), row,
		columnLetter(criterionColumns[len(criterionColumns)-1]), row)
	if err := api.UpdateRow(ctx, spreadsheetID, rangeA1, vals); err != nil {
		return fmt.Errorf("write criterion checkboxes: %w", err)
	}
	return nil
}

// MarkDuplicateRow flags a superseded duplicate row: Readiness becomes
// "Duplicate of row N" and the other assessment cells are cleared, so a stale
// score does not linger. The human columns (A-M) are left untouched.
func MarkDuplicateRow(ctx context.Context, api API, spreadsheetID, sheetName string, row, keptRow int) error {
	if err := UpdateFields(ctx, api, spreadsheetID, sheetName, row, map[Column]string{
		ColumnReadiness:     fmt.Sprintf("Duplicate of row %d", keptRow),
		ColumnSummary:       "",
		ColumnRedFlags:      "",
		ColumnEngagement:    "",
		ColumnEvidenceLinks: "",
	}); err != nil {
		return err
	}
	return writeCriteria(ctx, api, spreadsheetID, sheetName, row, nil) // all false
}

var evidenceHeader = []interface{}{"Candidate", "Row", "Channel", "Source", "Timestamp", "Permalink", "Text"}

// WriteEvidence rewrites the evidence tab from scratch (ensure, clear A-G, write
// header + rows).
func WriteEvidence(ctx context.Context, api API, spreadsheetID, evidenceTab string, rows [][]interface{}) error {
	if _, err := api.EnsureTab(ctx, spreadsheetID, evidenceTab); err != nil {
		return fmt.Errorf("ensure evidence tab: %w", err)
	}
	if err := api.ClearValues(ctx, spreadsheetID, evidenceTab+"!A:G"); err != nil {
		return fmt.Errorf("clear evidence tab: %w", err)
	}
	matrix := append([][]interface{}{evidenceHeader}, rows...)
	if err := api.WriteRows(ctx, spreadsheetID, evidenceTab+"!A1", matrix); err != nil {
		return fmt.Errorf("write evidence tab: %w", err)
	}
	return nil
}
