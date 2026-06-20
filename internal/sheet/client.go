package sheet

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"strings"

	"google.golang.org/api/option"
	"google.golang.org/api/sheets/v4"
)

type GoogleSheetsClient struct {
	svc *sheets.Service
}

func NewGoogleSheetsClient(ctx context.Context, credentialsFile string) (*GoogleSheetsClient, error) {
	svc, err := sheets.NewService(ctx, option.WithCredentialsFile(credentialsFile))
	if err != nil {
		return nil, err
	}
	return &GoogleSheetsClient{svc: svc}, nil
}

func (c *GoogleSheetsClient) Append(ctx context.Context, spreadsheetID, rangeA1 string, values []interface{}) (string, error) {
	resp, err := c.svc.Spreadsheets.Values.Append(spreadsheetID, rangeA1, &sheets.ValueRange{
		Values: [][]interface{}{values},
	}).ValueInputOption("RAW").Context(ctx).Do()
	if err != nil {
		return "", err
	}
	log.Printf("sheets.Append OK -> %s", resp.Updates.UpdatedRange)
	return resp.Updates.UpdatedRange, nil
}

func (c *GoogleSheetsClient) Update(ctx context.Context, spreadsheetID, rangeA1, value string) error {
	_, err := c.svc.Spreadsheets.Values.Update(spreadsheetID, rangeA1, &sheets.ValueRange{
		Values: [][]interface{}{{value}},
	}).ValueInputOption("RAW").Context(ctx).Do()
	return err
}

func (c *GoogleSheetsClient) UpdateRow(ctx context.Context, spreadsheetID, rangeA1 string, values []interface{}) error {
	_, err := c.svc.Spreadsheets.Values.Update(spreadsheetID, rangeA1, &sheets.ValueRange{
		Values: [][]interface{}{values},
	}).ValueInputOption("RAW").Context(ctx).Do()
	return err
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
	meta, err := c.svc.Spreadsheets.Get(spreadsheetID).Context(ctx).Do()
	if err != nil {
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
	_, err = c.svc.Spreadsheets.BatchUpdate(spreadsheetID, &sheets.BatchUpdateSpreadsheetRequest{
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
	resp, err := c.svc.Spreadsheets.Values.Get(spreadsheetID, rangeA1).Context(ctx).Do()
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
	_, err = c.svc.Spreadsheets.BatchUpdate(spreadsheetID, &sheets.BatchUpdateSpreadsheetRequest{
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
}

// SetLinkedText writes a single cell with text and a hyperlink (rich text),
// locale-independent — no formula involved.
func (c *GoogleSheetsClient) SetLinkedText(ctx context.Context, spreadsheetID, sheetName string, row, col int, text, url string) error {
	sheetID, err := c.sheetIDByName(ctx, spreadsheetID, sheetName)
	if err != nil {
		return err
	}
	_, err = c.svc.Spreadsheets.BatchUpdate(spreadsheetID, &sheets.BatchUpdateSpreadsheetRequest{
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
						TextFormatRuns: []*sheets.TextFormatRun{{
							StartIndex: 0,
							Format:     &sheets.TextFormat{Link: &sheets.Link{Uri: url}},
						}},
					}},
				}},
				Fields: "userEnteredValue,textFormatRuns",
			},
		}},
	}).Context(ctx).Do()
	return err
}

// SetStatusColors replaces this sheet's conditional formatting rules with one
// background-color rule per (status, hex) entry in mapping. The rule colors
// the whole row of headers when the status column matches the status.
func (c *GoogleSheetsClient) SetStatusColors(ctx context.Context, spreadsheetID, sheetName string, statusCol Column, mapping map[string]string) error {
	meta, err := c.svc.Spreadsheets.Get(spreadsheetID).Context(ctx).Do()
	if err != nil {
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
	for status, hex := range mapping {
		r, g, b := hexToRGB(hex)
		requests = append(requests, &sheets.Request{
			AddConditionalFormatRule: &sheets.AddConditionalFormatRuleRequest{
				Rule: &sheets.ConditionalFormatRule{
					Ranges: []*sheets.GridRange{{
						SheetId:          sheetID,
						StartRowIndex:    1,
						StartColumnIndex: 0,
						EndColumnIndex:   int64(len(Headers)),
					}},
					BooleanRule: &sheets.BooleanRule{
						Condition: &sheets.BooleanCondition{
							Type: "CUSTOM_FORMULA",
							Values: []*sheets.ConditionValue{{
								UserEnteredValue: fmt.Sprintf(`=$%s2=%q`, colLetter(int(statusCol)), status),
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
	_, err = c.svc.Spreadsheets.BatchUpdate(spreadsheetID, &sheets.BatchUpdateSpreadsheetRequest{Requests: requests}).Context(ctx).Do()
	return err
}

// FreezeHeaderRow freezes the first row of sheetName.
func (c *GoogleSheetsClient) FreezeHeaderRow(ctx context.Context, spreadsheetID, sheetName string) error {
	sheetID, err := c.sheetIDByName(ctx, spreadsheetID, sheetName)
	if err != nil {
		return err
	}
	_, err = c.svc.Spreadsheets.BatchUpdate(spreadsheetID, &sheets.BatchUpdateSpreadsheetRequest{
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
}

func (c *GoogleSheetsClient) sheetIDByName(ctx context.Context, spreadsheetID, sheetName string) (int64, error) {
	meta, err := c.svc.Spreadsheets.Get(spreadsheetID).Fields("sheets(properties(sheetId,title))").Context(ctx).Do()
	if err != nil {
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

func colLetter(zeroBased int) string {
	letters := []string{"A", "B", "C", "D", "E", "F", "G", "H", "I", "J", "K", "L", "M"}
	if zeroBased < 0 || zeroBased >= len(letters) {
		return "A"
	}
	return letters[zeroBased]
}

// SpreadsheetLocale returns the spreadsheet's locale (e.g. "fr_FR", "en_US").
// Used to pick the formula-argument separator since Sheets does NOT translate
// API-written formulas across locales.
func (c *GoogleSheetsClient) SpreadsheetLocale(ctx context.Context, spreadsheetID string) (string, error) {
	meta, err := c.svc.Spreadsheets.Get(spreadsheetID).Fields("properties.locale").Context(ctx).Do()
	if err != nil {
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
	meta, err := c.svc.Spreadsheets.Get(spreadsheetID).Context(ctx).Do()
	if err != nil {
		return false, err
	}
	for _, sh := range meta.Sheets {
		if sh.Properties != nil && sh.Properties.Title == sheetName {
			return false, nil
		}
	}
	_, err = c.svc.Spreadsheets.BatchUpdate(spreadsheetID, &sheets.BatchUpdateSpreadsheetRequest{
		Requests: []*sheets.Request{{
			AddSheet: &sheets.AddSheetRequest{
				Properties: &sheets.SheetProperties{Title: sheetName},
			},
		}},
	}).Context(ctx).Do()
	if err != nil {
		return false, err
	}
	log.Printf("sheets: created tab %q", sheetName)
	return true, nil
}
