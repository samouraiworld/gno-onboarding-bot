package sheet

import (
	"context"

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
	return resp.Updates.UpdatedRange, nil
}

func (c *GoogleSheetsClient) Update(ctx context.Context, spreadsheetID, rangeA1, value string) error {
	_, err := c.svc.Spreadsheets.Values.Update(spreadsheetID, rangeA1, &sheets.ValueRange{
		Values: [][]interface{}{{value}},
	}).ValueInputOption("RAW").Context(ctx).Do()
	return err
}
