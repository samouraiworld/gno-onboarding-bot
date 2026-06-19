package sheet

import (
	"context"
	"testing"
)

type fakeUpdate struct {
	rangeA1 string
	value   string
}

type fakeAPI struct {
	appendRange  string
	appendValues []interface{}
	appendResult string
	appendErr    error

	updates   []fakeUpdate
	updateErr error
}

func (f *fakeAPI) Append(ctx context.Context, spreadsheetID, rangeA1 string, values []interface{}) (string, error) {
	f.appendRange = rangeA1
	f.appendValues = values
	return f.appendResult, f.appendErr
}

func (f *fakeAPI) Update(ctx context.Context, spreadsheetID, rangeA1, value string) error {
	f.updates = append(f.updates, fakeUpdate{rangeA1, value})
	return f.updateErr
}

func TestParseRowNumber(t *testing.T) {
	cases := []struct {
		in      string
		want    int
		wantErr bool
	}{
		{"Sheet1!A58:L58", 58, false},
		{"Candidates!A2:L2", 2, false},
		{"garbage", 0, true},
		{"", 0, true},
	}
	for _, c := range cases {
		got, err := parseRowNumber(c.in)
		if c.wantErr {
			if err == nil {
				t.Errorf("parseRowNumber(%q): expected error, got nil", c.in)
			}
			continue
		}
		if err != nil {
			t.Errorf("parseRowNumber(%q): unexpected error: %v", c.in, err)
		}
		if got != c.want {
			t.Errorf("parseRowNumber(%q) = %d, want %d", c.in, got, c.want)
		}
	}
}

func TestAppendCandidateRow(t *testing.T) {
	api := &fakeAPI{appendResult: "Sheet1!A58:L58"}
	row := CandidateRow{Candidate: "alice", Discord: "@alice", Status: StatusCandidate}
	got, err := AppendCandidateRow(context.Background(), api, "sheet-id", "Sheet1", row)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != 58 {
		t.Errorf("got row %d, want 58", got)
	}
	if api.appendRange != "Sheet1!A:L" {
		t.Errorf("got range %q, want %q", api.appendRange, "Sheet1!A:L")
	}
	if len(api.appendValues) != 12 {
		t.Fatalf("got %d values, want 12", len(api.appendValues))
	}
	if api.appendValues[0] != "alice" {
		t.Errorf("got candidate %v, want alice", api.appendValues[0])
	}
}

func TestAppendCandidateRow_Error(t *testing.T) {
	api := &fakeAPI{appendErr: context.DeadlineExceeded}
	_, err := AppendCandidateRow(context.Background(), api, "sheet-id", "Sheet1", CandidateRow{})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestUpdateFields(t *testing.T) {
	api := &fakeAPI{}
	err := UpdateFields(context.Background(), api, "sheet-id", "Sheet1", 58, map[Column]string{
		ColumnStatus:    StatusNeedsRetry,
		ColumnReviewers: "bob",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(api.updates) != 2 {
		t.Fatalf("got %d updates, want 2", len(api.updates))
	}
	got := map[string]string{}
	for _, u := range api.updates {
		got[u.rangeA1] = u.value
	}
	if got["Sheet1!C58"] != StatusNeedsRetry {
		t.Errorf("status update = %q, want %q", got["Sheet1!C58"], StatusNeedsRetry)
	}
	if got["Sheet1!E58"] != "bob" {
		t.Errorf("reviewers update = %q, want %q", got["Sheet1!E58"], "bob")
	}
}

func TestUpdateFields_Error(t *testing.T) {
	api := &fakeAPI{updateErr: context.DeadlineExceeded}
	err := UpdateFields(context.Background(), api, "sheet-id", "Sheet1", 1, map[Column]string{ColumnStatus: StatusApproved})
	if err == nil {
		t.Fatal("expected error")
	}
}
