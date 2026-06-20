package sheet

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
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
	EnsureTab(ctx context.Context, spreadsheetID, sheetName string) (created bool, err error)
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

// AppendCandidateRow places a candidate row in the first fully-empty row of the
// sheet's A:M data band (starting at row 2). It never overwrites existing data:
// rows with any non-empty cell are skipped, and if no gap exists the row is
// written one past the last row with any data.
func AppendCandidateRow(ctx context.Context, api API, spreadsheetID, sheetName string, row CandidateRow) (int, error) {
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

func UpdateFields(ctx context.Context, api API, spreadsheetID, sheetName string, row int, fields map[Column]string) error {
	for col, value := range fields {
		rangeA1 := fmt.Sprintf("%s!%s%d", sheetName, columnLetter(col), row)
		if err := api.Update(ctx, spreadsheetID, rangeA1, value); err != nil {
			return fmt.Errorf("update %s: %w", rangeA1, err)
		}
	}
	return nil
}
