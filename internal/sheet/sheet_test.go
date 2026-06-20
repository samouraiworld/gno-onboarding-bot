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

	getResult [][]interface{}
	getErr    error

	updateRowRange  string
	updateRowValues []interface{}
	updateRowErr    error

	ensureTabCalled  bool
	ensureTabCreated bool
	ensureTabErr     error
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

func (f *fakeAPI) Get(ctx context.Context, spreadsheetID, rangeA1 string) ([][]interface{}, error) {
	return f.getResult, f.getErr
}

func (f *fakeAPI) UpdateRow(ctx context.Context, spreadsheetID, rangeA1 string, values []interface{}) error {
	f.updateRowRange = rangeA1
	f.updateRowValues = values
	return f.updateRowErr
}

func (f *fakeAPI) EnsureTab(ctx context.Context, spreadsheetID, sheetName string) (bool, error) {
	f.ensureTabCalled = true
	return f.ensureTabCreated, f.ensureTabErr
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

func TestAppendCandidateRow_EmptyTab(t *testing.T) {
	api := &fakeAPI{getResult: nil}
	row := CandidateRow{Candidate: "alice", Discord: "@alice", Status: StatusCandidate}
	got, err := AppendCandidateRow(context.Background(), api, "sheet-id", "Sheet1", row)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != 2 {
		t.Errorf("got row %d, want 2", got)
	}
	if api.updateRowRange != "Sheet1!A2:M2" {
		t.Errorf("got range %q, want %q", api.updateRowRange, "Sheet1!A2:M2")
	}
	if len(api.updateRowValues) != 13 {
		t.Fatalf("got %d values, want 13", len(api.updateRowValues))
	}
	if api.updateRowValues[0] != "alice" {
		t.Errorf("got candidate %v, want alice", api.updateRowValues[0])
	}
}

func TestAppendCandidateRow_AppendsAfterExisting(t *testing.T) {
	api := &fakeAPI{getResult: [][]interface{}{
		{"alice", "@alice", "Approved"},
		{"bob", "@bob", "Candidate"},
	}}
	row := CandidateRow{Candidate: "carol", Discord: "@carol", Status: StatusCandidate}
	got, err := AppendCandidateRow(context.Background(), api, "sheet-id", "Sheet1", row)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != 4 {
		t.Errorf("got row %d, want 4", got)
	}
	if api.updateRowRange != "Sheet1!A4:M4" {
		t.Errorf("got range %q, want %q", api.updateRowRange, "Sheet1!A4:M4")
	}
}

func TestAppendCandidateRow_FillsTrueGap(t *testing.T) {
	api := &fakeAPI{getResult: [][]interface{}{
		{"alice", "@alice"},
		{"", "", "", "", "", "", "", "", "", "", "", "", ""},
		{"bob", "@bob"},
	}}
	row := CandidateRow{Candidate: "carol"}
	got, err := AppendCandidateRow(context.Background(), api, "sheet-id", "Sheet1", row)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != 3 {
		t.Errorf("got row %d, want 3 (the all-empty gap)", got)
	}
}

func TestAppendCandidateRow_SkipsPartiallyEmptyRow(t *testing.T) {
	api := &fakeAPI{getResult: [][]interface{}{
		{"alice", "@alice"},
		{"", "", "stray note in col C"},
		{"bob", "@bob"},
	}}
	row := CandidateRow{Candidate: "carol"}
	got, err := AppendCandidateRow(context.Background(), api, "sheet-id", "Sheet1", row)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != 5 {
		t.Errorf("got row %d, want 5 (skip past partial-empty row 3)", got)
	}
}

func TestAppendCandidateRow_Error(t *testing.T) {
	api := &fakeAPI{getErr: context.DeadlineExceeded}
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

func TestEnsure_WritesHeadersWhenEmpty(t *testing.T) {
	api := &fakeAPI{getResult: nil} // empty row 1
	if err := Ensure(context.Background(), api, "sheet-id", "Test"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !api.ensureTabCalled {
		t.Error("EnsureTab not called")
	}
	if api.updateRowRange != "Test!A1:M1" {
		t.Errorf("got header range %q, want %q", api.updateRowRange, "Test!A1:M1")
	}
	if len(api.updateRowValues) != len(Headers) {
		t.Fatalf("got %d header values, want %d", len(api.updateRowValues), len(Headers))
	}
	if api.updateRowValues[0] != "Candidate" {
		t.Errorf("first header = %v, want Candidate", api.updateRowValues[0])
	}
}

func TestEnsure_SkipsHeadersWhenPresent(t *testing.T) {
	api := &fakeAPI{getResult: [][]interface{}{{"Candidate"}}}
	if err := Ensure(context.Background(), api, "sheet-id", "Test"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if api.updateRowRange != "" {
		t.Errorf("UpdateRow was called but headers already exist: range=%q", api.updateRowRange)
	}
}

func TestUpdateFields_Error(t *testing.T) {
	api := &fakeAPI{updateErr: context.DeadlineExceeded}
	err := UpdateFields(context.Background(), api, "sheet-id", "Sheet1", 1, map[Column]string{ColumnStatus: StatusApproved})
	if err == nil {
		t.Fatal("expected error")
	}
}
