package sheet

import (
	"context"
	"log"

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

func (c *GoogleSheetsClient) Get(ctx context.Context, spreadsheetID, rangeA1 string) ([][]interface{}, error) {
	resp, err := c.svc.Spreadsheets.Values.Get(spreadsheetID, rangeA1).Context(ctx).Do()
	if err != nil {
		return nil, err
	}
	return resp.Values, nil
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
