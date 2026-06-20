package sheet

import (
	"context"
	"strings"
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

	setFormulaRange   string
	setFormulaFormula string
	setFormulaErr     error

	locale    string
	localeErr error

	dropdownCol      Column
	dropdownStartRow int
	dropdownEndRow   int
	dropdownValues   []string
	dropdownErr      error

	linkedTextCalled bool
	linkedTextText   string
	linkedTextURL    string
	linkedTextErr    error

	statusColorsCalled  bool
	statusColorsMapping map[string]string
	statusColorsErr     error

	freezeCalled bool
	freezeErr    error
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

func (f *fakeAPI) SetFormula(ctx context.Context, spreadsheetID, rangeA1, formula string) error {
	f.setFormulaRange = rangeA1
	f.setFormulaFormula = formula
	return f.setFormulaErr
}

func (f *fakeAPI) SpreadsheetLocale(ctx context.Context, spreadsheetID string) (string, error) {
	if f.locale == "" {
		return "en_US", f.localeErr
	}
	return f.locale, f.localeErr
}

func (f *fakeAPI) SetDropdown(ctx context.Context, spreadsheetID, sheetName string, col Column, startRow, endRow int, values []string) error {
	f.dropdownCol = col
	f.dropdownStartRow = startRow
	f.dropdownEndRow = endRow
	f.dropdownValues = values
	return f.dropdownErr
}

func (f *fakeAPI) SetLinkedText(ctx context.Context, spreadsheetID, sheetName string, row, col int, text, url string) error {
	f.linkedTextCalled = true
	f.linkedTextText = text
	f.linkedTextURL = url
	return f.linkedTextErr
}

func (f *fakeAPI) SetStatusColors(ctx context.Context, spreadsheetID, sheetName string, statusCol Column, mapping map[string]string) error {
	f.statusColorsCalled = true
	f.statusColorsMapping = mapping
	return f.statusColorsErr
}

func (f *fakeAPI) FreezeHeaderRow(ctx context.Context, spreadsheetID, sheetName string) error {
	f.freezeCalled = true
	return f.freezeErr
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

func TestEnsureApprovedView_WritesHeadersAndFormula(t *testing.T) {
	api := &fakeAPI{getResult: nil} // both header row and A2 read return empty
	if err := EnsureApprovedView(context.Background(), api, "sheet-id", "Test"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !api.ensureTabCalled {
		t.Error("EnsureTab not called")
	}
	if api.updateRowRange != "Test-approved!A1:M1" {
		t.Errorf("got header range %q, want %q", api.updateRowRange, "Test-approved!A1:M1")
	}
	if api.setFormulaRange != "Test-approved!A2" {
		t.Errorf("got formula range %q, want %q", api.setFormulaRange, "Test-approved!A2")
	}
	if !strings.Contains(api.setFormulaFormula, "VSTACK(") {
		t.Errorf("formula missing VSTACK: %s", api.setFormulaFormula)
	}
	if !strings.Contains(api.setFormulaFormula, "GovDAO submitted") {
		t.Errorf("formula missing GovDAO submitted: %s", api.setFormulaFormula)
	}
	if !strings.Contains(api.setFormulaFormula, "GovDAO pending") {
		t.Errorf("formula missing GovDAO pending: %s", api.setFormulaFormula)
	}
	if !strings.Contains(api.setFormulaFormula, "MAKEARRAY(1, 13") {
		t.Errorf("en_US: formula missing MAKEARRAY(1, 13 ...) divider row: %s", api.setFormulaFormula)
	}
	subIdx := strings.Index(api.setFormulaFormula, "GovDAO submitted")
	penIdx := strings.Index(api.setFormulaFormula, "GovDAO pending")
	if subIdx < 0 || penIdx < 0 || subIdx >= penIdx {
		t.Errorf("submitted block must appear before pending block: %s", api.setFormulaFormula)
	}
}

func TestEnsureApprovedView_UsesSemicolonInFrenchLocale(t *testing.T) {
	api := &fakeAPI{locale: "fr_FR"}
	if err := EnsureApprovedView(context.Background(), api, "sheet-id", "Test"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// QUERY inner string literal still has commas inside the SQL; we only
	// care that the OUTER call separator is ";" — check that ", \"\"" is not
	// present (that would be the en-US "), """ tail of the formula).
	if strings.Contains(api.setFormulaFormula, `, "")`) {
		t.Errorf("fr_FR locale: outer formula must not use comma separator: %s", api.setFormulaFormula)
	}
	if !strings.Contains(api.setFormulaFormula, `; "")`) {
		t.Errorf("fr_FR locale: outer formula must use semicolon separator: %s", api.setFormulaFormula)
	}
}

func TestEnsureStatusDropdown_SizesToFilledRows(t *testing.T) {
	api := &fakeAPI{getResult: [][]interface{}{
		{"alice"}, {"bob"}, {"carol"},
	}}
	if err := EnsureStatusDropdown(context.Background(), api, "sheet-id", "Test"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if api.dropdownStartRow != 2 || api.dropdownEndRow != 4 {
		t.Errorf("dropdown rows = [%d, %d], want [2, 4]", api.dropdownStartRow, api.dropdownEndRow)
	}
}

func TestEnsureStatusDropdown_SkipsEmptyTab(t *testing.T) {
	api := &fakeAPI{getResult: nil}
	if err := EnsureStatusDropdown(context.Background(), api, "sheet-id", "Test"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if api.dropdownEndRow != 0 {
		t.Errorf("dropdown applied to empty tab: endRow=%d", api.dropdownEndRow)
	}
}

func TestApplyStatusDropdown(t *testing.T) {
	api := &fakeAPI{}
	if err := ApplyStatusDropdown(context.Background(), api, "sheet-id", "Test", 7); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if api.dropdownStartRow != 2 || api.dropdownEndRow != 7 {
		t.Errorf("dropdown rows = [%d, %d], want [2, 7]", api.dropdownStartRow, api.dropdownEndRow)
	}
}

func TestFormulaArgSep(t *testing.T) {
	tests := []struct {
		locale string
		want   string
	}{
		{"en_US", ","},
		{"en_GB", ","},
		{"ja_JP", ","},
		{"ko_KR", ","},
		{"zh_CN", ","},
		{"fr_FR", ";"},
		{"de_DE", ";"},
		{"it_IT", ";"},
		{"es_ES", ";"},
		{"pt_BR", ";"},
		{"", ";"},
	}
	for _, tt := range tests {
		if got := formulaArgSep(tt.locale); got != tt.want {
			t.Errorf("formulaArgSep(%q) = %q, want %q", tt.locale, got, tt.want)
		}
	}
}

func TestEnsureApprovedView_SkipsHeadersButAlwaysRewritesFormula(t *testing.T) {
	api := &fakeAPI{getResult: [][]interface{}{{"Candidate"}}}
	if err := EnsureApprovedView(context.Background(), api, "sheet-id", "Test"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if api.updateRowRange != "" {
		t.Errorf("UpdateRow called but headers already exist: %q", api.updateRowRange)
	}
	if api.setFormulaRange != "Test-approved!A2" {
		t.Errorf("SetFormula not called or wrong range: %q", api.setFormulaRange)
	}
	if !strings.Contains(api.setFormulaFormula, "QUERY('Test'!A2:M") {
		t.Errorf("formula body wrong: %s", api.setFormulaFormula)
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
