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
	ColumnMonikerAddress
	ColumnIntroduction
	ColumnReviewMessageLink
)

var columnLetters = []string{"A", "B", "C", "D", "E", "F", "G", "H", "I", "J", "K", "L"}

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
	MonikerAddress     string
	Introduction       string
}

func (r CandidateRow) toValues() []interface{} {
	return []interface{}{
		r.Candidate,
		r.Discord,
		r.Status,
		r.ChallengeSubmitted,
		"",
		"",
		"",
		r.Valoper,
		"",
		r.MonikerAddress,
		r.Introduction,
		"",
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

type API interface {
	Append(ctx context.Context, spreadsheetID, rangeA1 string, values []interface{}) (updatedRange string, err error)
	Update(ctx context.Context, spreadsheetID, rangeA1, value string) error
}

func AppendCandidateRow(ctx context.Context, api API, spreadsheetID, sheetName string, row CandidateRow) (int, error) {
	updatedRange, err := api.Append(ctx, spreadsheetID, sheetName+"!A:L", row.toValues())
	if err != nil {
		return 0, fmt.Errorf("append candidate row: %w", err)
	}
	return parseRowNumber(updatedRange)
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
