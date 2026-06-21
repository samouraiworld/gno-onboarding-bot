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
)

var columnLetters = []string{"A", "B", "C", "D", "E", "F", "G", "H", "I", "J", "K", "L", "M"}

func columnLetter(c Column) string {
	return columnLetters[c]
}

const (
	StatusCandidate           = "Candidate"
	StatusChallengeInProgress = "Challenge in progress"
	StatusNeedsRetry          = "Needs retry"
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
	SetStatusColors(ctx context.Context, spreadsheetID, sheetName string, statusCol Column, mapping map[string]string) error
	FreezeHeaderRow(ctx context.Context, spreadsheetID, sheetName string) error
}

// StatusColors maps each status to a light hex background color used by
// EnsureStatusColors.
var StatusColors = map[string]string{
	StatusCandidate:           "#e0e0e0",
	StatusChallengeInProgress: "#fff2a8",
	StatusNeedsRetry:          "#fcd5b4",
	StatusApproved:            "#c2eebc",
	StatusGovDAOPending:       "#b6d7f5",
	StatusGovDAOSubmitted:     "#d9c4ec",
}

// EnsureStatusColors installs the status-row coloring rules on sheetName.
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
// populates it with the column headers plus a live FILTER formula that mirrors
// the source tab's rows whose status is StatusGovDAOPending. Safe to call
// multiple times; only writes when cells are empty.
func EnsureApprovedView(ctx context.Context, api API, spreadsheetID, sourceSheetName string) error {
	tab := ApprovedTabName(sourceSheetName)
	if _, err := api.EnsureTab(ctx, spreadsheetID, tab); err != nil {
		return fmt.Errorf("ensure tab %q: %w", tab, err)
	}
	lastCol := columnLetter(Column(len(Headers) - 1))
	headerRange := fmt.Sprintf("%s!A1:%s1", tab, lastCol)
	row1, err := api.Get(ctx, spreadsheetID, headerRange)
	if err != nil {
		return fmt.Errorf("read header row of %q: %w", tab, err)
	}
	if len(row1) == 0 || rowIsEmpty(row1[0]) {
		values := make([]interface{}, len(Headers))
		for i, h := range Headers {
			values[i] = h
		}
		if err := api.UpdateRow(ctx, spreadsheetID, headerRange, values); err != nil {
			return fmt.Errorf("write headers to %q: %w", tab, err)
		}
	}
	// Always rewrite the FILTER formula. Idempotent on the happy path; on the
	// unhappy path (cell holds a locale-broken formula whose rendered value
	// reads as "#ERROR!", i.e. non-empty), a skip check would prevent the fix
	// from ever applying.
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
// tab. It lists "GovDAO submitted" rows above a divider row above "GovDAO
// pending" rows, and is robust to either category being empty:
//
//   - QUERY is given headers=0 so it never lifts the first data row into a
//     header (data-loss bug otherwise, since the source range is data-only).
//   - COUNTIF gives scalar category counts, so the divider row is shown ONLY
//     when both categories are non-empty (no stray divider, no #N/A padding).
//   - When a single category is non-empty its QUERY block is returned as-is;
//     when both are empty the cell renders "".
//
// sep is the locale's function-argument separator ("," or ";"). The QUERY SQL
// is a single string literal, so it is locale-independent.
func approvedViewFormula(sourceSheetName, lastCol, statusCol, sep string) string {
	join := func(args ...string) string { return strings.Join(args, sep+" ") }
	quote := func(s string) string { return `"` + s + `"` }

	src := fmt.Sprintf("'%s'!A2:%s", sourceSheetName, lastCol)
	statusRange := fmt.Sprintf("'%s'!%s2:%s", sourceSheetName, statusCol, statusCol)

	query := func(statusVal string) string {
		sql := quote(fmt.Sprintf("select * where %s = '%s'", statusCol, statusVal))
		return "IFERROR(QUERY(" + join(src, sql, "0") + ")" + sep + ` "")`
	}
	count := func(statusVal string) string {
		return "COUNTIF(" + join(statusRange, quote(statusVal)) + ")"
	}
	divider := "MAKEARRAY(" + join("1", strconv.Itoa(len(Headers)),
		"LAMBDA("+join("r", "c", quote("───"))+")") + ")"

	ifs := "IFS(" + join(
		"AND("+join("nS>0", "nP>0")+")", "VSTACK("+join("qs", divider, "qp")+")",
		"nS>0", "qs",
		"nP>0", "qp",
		"TRUE", quote(""),
	) + ")"
	return "=LET(" + join(
		"nS", count(StatusGovDAOSubmitted),
		"nP", count(StatusGovDAOPending),
		"qs", query(StatusGovDAOSubmitted),
		"qp", query(StatusGovDAOPending),
		ifs,
	) + ")"
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
