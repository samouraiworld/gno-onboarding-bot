package sheet

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"strings"
	"unicode/utf16"

	"google.golang.org/api/option"
	"google.golang.org/api/sheets/v4"
)

type GoogleSheetsClient struct {
	svc   *sheets.Service
	retry RetryPolicy
}

// NewGoogleSheetsClient builds a client that retries failed requests per policy (zero fields fall back to DefaultRetryPolicy).
func NewGoogleSheetsClient(ctx context.Context, credentialsFile string, policy RetryPolicy) (*GoogleSheetsClient, error) {
	svc, err := sheets.NewService(ctx, option.WithCredentialsFile(credentialsFile))
	if err != nil {
		return nil, err
	}
	return &GoogleSheetsClient{svc: svc, retry: policy.normalized()}, nil
}

func (c *GoogleSheetsClient) Append(ctx context.Context, spreadsheetID, rangeA1 string, values []interface{}) (string, error) {
	var resp *sheets.AppendValuesResponse
	err := c.do(ctx, func() error {
		var err error
		resp, err = c.svc.Spreadsheets.Values.Append(spreadsheetID, rangeA1, &sheets.ValueRange{
			Values: [][]interface{}{values},
		}).ValueInputOption("RAW").Context(ctx).Do()
		return err
	})
	if err != nil {
		return "", err
	}
	log.Printf("sheets.Append OK -> %s", resp.Updates.UpdatedRange)
	return resp.Updates.UpdatedRange, nil
}

func (c *GoogleSheetsClient) Update(ctx context.Context, spreadsheetID, rangeA1, value string) error {
	return c.do(ctx, func() error {
		_, err := c.svc.Spreadsheets.Values.Update(spreadsheetID, rangeA1, &sheets.ValueRange{
			Values: [][]interface{}{{value}},
		}).ValueInputOption("RAW").Context(ctx).Do()
		return err
	})
}

func (c *GoogleSheetsClient) UpdateRow(ctx context.Context, spreadsheetID, rangeA1 string, values []interface{}) error {
	return c.do(ctx, func() error {
		_, err := c.svc.Spreadsheets.Values.Update(spreadsheetID, rangeA1, &sheets.ValueRange{
			Values: [][]interface{}{values},
		}).ValueInputOption("RAW").Context(ctx).Do()
		return err
	})
}

// SetFormula writes a formula to a single cell, using batchUpdate's
// formulaValue so the formula is stored in canonical (en-US, comma-separated)
// form regardless of the spreadsheet's locale. The values.update +
// USER_ENTERED path is locale-sensitive and breaks on non-en locales whose
// separator is ";".
func (c *GoogleSheetsClient) SetFormula(ctx context.Context, spreadsheetID, rangeA1, formula string) error {
	parts := strings.SplitN(rangeA1, "!", 2)
	if len(parts) != 2 {
		return fmt.Errorf("invalid range %q", rangeA1)
	}
	sheetName := strings.Trim(parts[0], "'")
	row, col, err := parseA1Cell(parts[1])
	if err != nil {
		return fmt.Errorf("parse cell %q: %w", parts[1], err)
	}
	var meta *sheets.Spreadsheet
	if err := c.do(ctx, func() error {
		var err error
		meta, err = c.svc.Spreadsheets.Get(spreadsheetID).Context(ctx).Do()
		return err
	}); err != nil {
		return fmt.Errorf("get spreadsheet meta: %w", err)
	}
	var sheetID int64 = -1
	for _, sh := range meta.Sheets {
		if sh.Properties != nil && sh.Properties.Title == sheetName {
			sheetID = sh.Properties.SheetId
			break
		}
	}
	if sheetID == -1 {
		return fmt.Errorf("sheet %q not found", sheetName)
	}
	return c.do(ctx, func() error {
		_, err := c.svc.Spreadsheets.BatchUpdate(spreadsheetID, &sheets.BatchUpdateSpreadsheetRequest{
			Requests: []*sheets.Request{{
				UpdateCells: &sheets.UpdateCellsRequest{
					Range: &sheets.GridRange{
						SheetId:          sheetID,
						StartRowIndex:    int64(row),
						EndRowIndex:      int64(row + 1),
						StartColumnIndex: int64(col),
						EndColumnIndex:   int64(col + 1),
					},
					Rows: []*sheets.RowData{{
						Values: []*sheets.CellData{{
							UserEnteredValue: &sheets.ExtendedValue{FormulaValue: &formula},
						}},
					}},
					Fields: "userEnteredValue",
				},
			}},
		}).Context(ctx).Do()
		return err
	})
}

// parseA1Cell parses a single-cell A1 reference like "A2" or "AB10" into
// zero-indexed (row, col).
func parseA1Cell(ref string) (row, col int, err error) {
	i := 0
	for i < len(ref) {
		ch := ref[i]
		if ch < 'A' || ch > 'Z' {
			break
		}
		i++
	}
	if i == 0 || i == len(ref) {
		return 0, 0, fmt.Errorf("not a single-cell A1 ref")
	}
	for j := 0; j < i; j++ {
		col = col*26 + int(ref[j]-'A') + 1
	}
	col--
	r, err := strconv.Atoi(ref[i:])
	if err != nil || r < 1 {
		return 0, 0, fmt.Errorf("bad row number")
	}
	return r - 1, col, nil
}

func (c *GoogleSheetsClient) Get(ctx context.Context, spreadsheetID, rangeA1 string) ([][]interface{}, error) {
	var resp *sheets.ValueRange
	err := c.do(ctx, func() error {
		var err error
		resp, err = c.svc.Spreadsheets.Values.Get(spreadsheetID, rangeA1).Context(ctx).Do()
		return err
	})
	if err != nil {
		return nil, err
	}
	return resp.Values, nil
}

// SetDropdown installs a "one of" data-validation rule on column col of
// sheetName, covering rows [startRow, endRow] (1-based, inclusive). values
// are the dropdown options.
func (c *GoogleSheetsClient) SetDropdown(ctx context.Context, spreadsheetID, sheetName string, col Column, startRow, endRow int, values []string) error {
	if endRow < startRow {
		return nil
	}
	sheetID, err := c.sheetIDByName(ctx, spreadsheetID, sheetName)
	if err != nil {
		return err
	}
	conds := make([]*sheets.ConditionValue, len(values))
	for i, v := range values {
		conds[i] = &sheets.ConditionValue{UserEnteredValue: v}
	}
	return c.do(ctx, func() error {
		_, err := c.svc.Spreadsheets.BatchUpdate(spreadsheetID, &sheets.BatchUpdateSpreadsheetRequest{
			Requests: []*sheets.Request{{
				SetDataValidation: &sheets.SetDataValidationRequest{
					Range: &sheets.GridRange{
						SheetId:          sheetID,
						StartRowIndex:    int64(startRow - 1),
						EndRowIndex:      int64(endRow),
						StartColumnIndex: int64(col),
						EndColumnIndex:   int64(col) + 1,
					},
					Rule: &sheets.DataValidationRule{
						Condition: &sheets.BooleanCondition{
							Type:   "ONE_OF_LIST",
							Values: conds,
						},
						ShowCustomUi: true,
						Strict:       true,
					},
				},
			}},
		}).Context(ctx).Do()
		return err
	})
}

// linkRun is a resolved formatting run for a multi-link cell. An empty uri ends
// the previous link so the separator and any later text stay unlinked.
type linkRun struct {
	start int
	uri   string
}

// linkedCellRuns renders lines into one cell string (one line per link,
// newline-separated) plus the formatting runs that confine each link to its own
// line. Offsets are UTF-16 code units, as the Sheets API expects.
func linkedCellRuns(lines []LinkedLine) (string, []linkRun) {
	var b strings.Builder
	var runs []linkRun
	offset := 0 // running UTF-16 length of what's been written
	for i, ln := range lines {
		if i > 0 {
			runs = append(runs, linkRun{start: offset}) // close previous link
			b.WriteString("\n")
			offset++ // newline is one UTF-16 unit
		}
		runs = append(runs, linkRun{start: offset, uri: ln.URL})
		b.WriteString(ln.Text)
		offset += utf16Len(ln.Text)
	}
	return b.String(), runs
}

func utf16Len(s string) int { return len(utf16.Encode([]rune(s))) }

// SetLinkedLines writes one cell containing one hyperlink per line: each line's
// text links to its URL, confined to that line. Replaces the cell's prior
// content and formatting; an empty lines slice clears the cell.
func (c *GoogleSheetsClient) SetLinkedLines(ctx context.Context, spreadsheetID, sheetName string, row, col int, lines []LinkedLine) error {
	sheetID, err := c.sheetIDByName(ctx, spreadsheetID, sheetName)
	if err != nil {
		return err
	}
	text, runs := linkedCellRuns(lines)
	formatRuns := make([]*sheets.TextFormatRun, len(runs))
	for i, r := range runs {
		f := &sheets.TextFormat{}
		if r.uri != "" {
			f.Link = &sheets.Link{Uri: r.uri}
		}
		formatRuns[i] = &sheets.TextFormatRun{StartIndex: int64(r.start), Format: f}
	}
	return c.do(ctx, func() error {
		_, err := c.svc.Spreadsheets.BatchUpdate(spreadsheetID, &sheets.BatchUpdateSpreadsheetRequest{
			Requests: []*sheets.Request{{
				UpdateCells: &sheets.UpdateCellsRequest{
					Range: &sheets.GridRange{
						SheetId:          sheetID,
						StartRowIndex:    int64(row - 1),
						EndRowIndex:      int64(row),
						StartColumnIndex: int64(col),
						EndColumnIndex:   int64(col) + 1,
					},
					Rows: []*sheets.RowData{{
						Values: []*sheets.CellData{{
							UserEnteredValue: &sheets.ExtendedValue{StringValue: &text},
							TextFormatRuns:   formatRuns,
						}},
					}},
					Fields: "userEnteredValue,textFormatRuns",
				},
			}},
		}).Context(ctx).Do()
		return err
	})
}

// SetLinkedText writes a single cell with text and a hyperlink (rich text),
// locale-independent — no formula involved. It is the single-link case of
// SetLinkedLines.
func (c *GoogleSheetsClient) SetLinkedText(ctx context.Context, spreadsheetID, sheetName string, row, col int, text, url string) error {
	return c.SetLinkedLines(ctx, spreadsheetID, sheetName, row, col, []LinkedLine{{Text: text, URL: url}})
}

// SetStatusColors replaces this sheet's conditional formatting rules with one
// background-color rule per (status, hex) entry in mapping. The rule colors
// the whole row of headers when the status column matches the status.
func (c *GoogleSheetsClient) SetStatusColors(ctx context.Context, spreadsheetID, sheetName string, statusCol Column, mapping map[string]string) error {
	var meta *sheets.Spreadsheet
	if err := c.do(ctx, func() error {
		var err error
		meta, err = c.svc.Spreadsheets.Get(spreadsheetID).Context(ctx).Do()
		return err
	}); err != nil {
		return err
	}
	var sheetID int64 = -1
	var existing []*sheets.ConditionalFormatRule
	for _, sh := range meta.Sheets {
		if sh.Properties != nil && sh.Properties.Title == sheetName {
			sheetID = sh.Properties.SheetId
			existing = sh.ConditionalFormats
			break
		}
	}
	if sheetID == -1 {
		return fmt.Errorf("sheet %q not found", sheetName)
	}
	var requests []*sheets.Request
	for i := len(existing) - 1; i >= 0; i-- {
		requests = append(requests, &sheets.Request{
			DeleteConditionalFormatRule: &sheets.DeleteConditionalFormatRuleRequest{
				SheetId: sheetID,
				Index:   int64(i),
			},
		})
	}
	// Iterate AllStatuses (a stable ordered slice) rather than ranging the map,
	// so the conditional-format rule priority is deterministic across runs.
	for _, status := range AllStatuses {
		hex, ok := mapping[status]
		if !ok {
			continue
		}
		r, g, b := hexToRGB(hex)
		requests = append(requests, &sheets.Request{
			AddConditionalFormatRule: &sheets.AddConditionalFormatRuleRequest{
				Rule: &sheets.ConditionalFormatRule{
					Ranges: []*sheets.GridRange{{
						SheetId:          sheetID,
						StartRowIndex:    1,
						StartColumnIndex: 0,
						EndColumnIndex:   int64(len(columnLetters)), // full schema width A-AA
					}},
					BooleanRule: &sheets.BooleanRule{
						Condition: &sheets.BooleanCondition{
							Type: "CUSTOM_FORMULA",
							Values: []*sheets.ConditionValue{{
								UserEnteredValue: fmt.Sprintf(`=$%s2=%q`, columnLetter(statusCol), status),
							}},
						},
						Format: &sheets.CellFormat{
							BackgroundColor: &sheets.Color{Red: r, Green: g, Blue: b, Alpha: 1},
						},
					},
				},
			},
		})
	}
	if len(requests) == 0 {
		return nil
	}
	return c.do(ctx, func() error {
		_, err := c.svc.Spreadsheets.BatchUpdate(spreadsheetID, &sheets.BatchUpdateSpreadsheetRequest{Requests: requests}).Context(ctx).Do()
		return err
	})
}

// FreezeHeaderRow freezes the first row of sheetName.
func (c *GoogleSheetsClient) FreezeHeaderRow(ctx context.Context, spreadsheetID, sheetName string) error {
	sheetID, err := c.sheetIDByName(ctx, spreadsheetID, sheetName)
	if err != nil {
		return err
	}
	return c.do(ctx, func() error {
		_, err := c.svc.Spreadsheets.BatchUpdate(spreadsheetID, &sheets.BatchUpdateSpreadsheetRequest{
			Requests: []*sheets.Request{{
				UpdateSheetProperties: &sheets.UpdateSheetPropertiesRequest{
					Properties: &sheets.SheetProperties{
						SheetId:        sheetID,
						GridProperties: &sheets.GridProperties{FrozenRowCount: 1},
					},
					Fields: "gridProperties.frozenRowCount",
				},
			}},
		}).Context(ctx).Do()
		return err
	})
}

func (c *GoogleSheetsClient) sheetIDByName(ctx context.Context, spreadsheetID, sheetName string) (int64, error) {
	var meta *sheets.Spreadsheet
	if err := c.do(ctx, func() error {
		var err error
		meta, err = c.svc.Spreadsheets.Get(spreadsheetID).Fields("sheets(properties(sheetId,title))").Context(ctx).Do()
		return err
	}); err != nil {
		return 0, err
	}
	for _, sh := range meta.Sheets {
		if sh.Properties != nil && sh.Properties.Title == sheetName {
			return sh.Properties.SheetId, nil
		}
	}
	return 0, fmt.Errorf("sheet %q not found", sheetName)
}

func hexToRGB(hex string) (float64, float64, float64) {
	hex = strings.TrimPrefix(hex, "#")
	if len(hex) != 6 {
		return 1, 1, 1
	}
	r, _ := strconv.ParseInt(hex[0:2], 16, 0)
	g, _ := strconv.ParseInt(hex[2:4], 16, 0)
	b, _ := strconv.ParseInt(hex[4:6], 16, 0)
	return float64(r) / 255, float64(g) / 255, float64(b) / 255
}

// SpreadsheetLocale returns the spreadsheet's locale (e.g. "fr_FR", "en_US").
// Used to pick the formula-argument separator since Sheets does NOT translate
// API-written formulas across locales.
func (c *GoogleSheetsClient) SpreadsheetLocale(ctx context.Context, spreadsheetID string) (string, error) {
	var meta *sheets.Spreadsheet
	if err := c.do(ctx, func() error {
		var err error
		meta, err = c.svc.Spreadsheets.Get(spreadsheetID).Fields("properties.locale").Context(ctx).Do()
		return err
	}); err != nil {
		return "", err
	}
	if meta.Properties == nil {
		return "en_US", nil
	}
	return meta.Properties.Locale, nil
}

// EnsureTab adds the tab to the spreadsheet if it does not exist. Reports
// whether it created the tab.
func (c *GoogleSheetsClient) EnsureTab(ctx context.Context, spreadsheetID, sheetName string) (bool, error) {
	var meta *sheets.Spreadsheet
	if err := c.do(ctx, func() error {
		var err error
		meta, err = c.svc.Spreadsheets.Get(spreadsheetID).Context(ctx).Do()
		return err
	}); err != nil {
		return false, err
	}
	for _, sh := range meta.Sheets {
		if sh.Properties != nil && sh.Properties.Title == sheetName {
			return false, nil
		}
	}
	if err := c.do(ctx, func() error {
		_, err := c.svc.Spreadsheets.BatchUpdate(spreadsheetID, &sheets.BatchUpdateSpreadsheetRequest{
			Requests: []*sheets.Request{{
				AddSheet: &sheets.AddSheetRequest{
					Properties: &sheets.SheetProperties{Title: sheetName},
				},
			}},
		}).Context(ctx).Do()
		return err
	}); err != nil {
		return false, err
	}
	log.Printf("sheets: created tab %q", sheetName)
	return true, nil
}

// SetCheckbox installs BOOLEAN data validation (a checkbox) on columns
// [startCol, endCol) of sheetName, from row 2 down.
func (c *GoogleSheetsClient) SetCheckbox(ctx context.Context, spreadsheetID, sheetName string, startCol, endCol Column) error {
	sheetID, err := c.sheetIDByName(ctx, spreadsheetID, sheetName)
	if err != nil {
		return err
	}
	return c.do(ctx, func() error {
		_, err := c.svc.Spreadsheets.BatchUpdate(spreadsheetID, &sheets.BatchUpdateSpreadsheetRequest{
			Requests: []*sheets.Request{{
				SetDataValidation: &sheets.SetDataValidationRequest{
					Range: &sheets.GridRange{
						SheetId:          sheetID,
						StartRowIndex:    1, // skip the header row
						StartColumnIndex: int64(startCol),
						EndColumnIndex:   int64(endCol),
						ForceSendFields:  []string{"SheetId"},
					},
					Rule: &sheets.DataValidationRule{
						Condition:    &sheets.BooleanCondition{Type: "BOOLEAN"},
						ShowCustomUi: true,
					},
				},
			}},
		}).Context(ctx).Do()
		return err
	})
}

// ClearValues clears the values in the given A1 range (used to rebuild the
// evidence tab from scratch).
func (c *GoogleSheetsClient) ClearValues(ctx context.Context, spreadsheetID, rangeA1 string) error {
	return c.do(ctx, func() error {
		_, err := c.svc.Spreadsheets.Values.Clear(spreadsheetID, rangeA1, &sheets.ClearValuesRequest{}).Context(ctx).Do()
		return err
	})
}

// WriteRows writes a matrix of values starting at rangeA1 (RAW), so Go bool
// values land as real booleans that render as checked checkboxes.
func (c *GoogleSheetsClient) WriteRows(ctx context.Context, spreadsheetID, rangeA1 string, values [][]interface{}) error {
	return c.do(ctx, func() error {
		_, err := c.svc.Spreadsheets.Values.Update(spreadsheetID, rangeA1, &sheets.ValueRange{
			Values: values,
		}).ValueInputOption("RAW").Context(ctx).Do()
		return err
	})
}

// CellLink returns the hyperlink URI attached to a single cell, or "" if the
// cell has no link. row is 1-based; col is a 0-based Column index. The link is
// read from the cell's textFormatRuns (how SetLinkedText writes it) with the
// cell-level hyperlink field as a fallback.
func (c *GoogleSheetsClient) CellLink(ctx context.Context, spreadsheetID, sheetName string, row, col int) (string, error) {
	cell := fmt.Sprintf("%s!%s%d", sheetName, columnLetter(Column(col)), row)
	var resp *sheets.Spreadsheet
	err := c.do(ctx, func() error {
		var err error
		resp, err = c.svc.Spreadsheets.Get(spreadsheetID).
			Ranges(cell).
			Fields("sheets.data.rowData.values(hyperlink,textFormatRuns.format.link.uri)").
			IncludeGridData(true).
			Context(ctx).Do()
		return err
	})
	if err != nil {
		return "", err
	}
	for _, sh := range resp.Sheets {
		for _, d := range sh.Data {
			for _, rd := range d.RowData {
				for _, v := range rd.Values {
					for _, run := range v.TextFormatRuns {
						if run.Format != nil && run.Format.Link != nil && run.Format.Link.Uri != "" {
							return run.Format.Link.Uri, nil
						}
					}
					if v.Hyperlink != "" {
						return v.Hyperlink, nil
					}
				}
			}
		}
	}
	return "", nil
}
