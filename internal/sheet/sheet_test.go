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

type matrixWrite struct {
	rangeA1 string
	values  [][]interface{}
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
	ensureTabNames   []string

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

	linkedLinesRow   int
	linkedLinesCol   int
	linkedLinesLines []LinkedLine
	linkedLinesErr   error

	statusColorsCalled  bool
	statusColorsMapping map[string]string
	statusColorsErr     error

	freezeCalled bool
	freezeErr    error

	updateRowCalls  []matrixWrite
	checkboxes      [][2]Column
	cleared         []string
	writeRowsRange  string
	writeRowsValues [][]interface{}
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
	f.updateRowCalls = append(f.updateRowCalls, matrixWrite{rangeA1, [][]interface{}{values}})
	return f.updateRowErr
}

func (f *fakeAPI) SetCheckbox(ctx context.Context, spreadsheetID, sheetName string, startCol, endCol Column) error {
	f.checkboxes = append(f.checkboxes, [2]Column{startCol, endCol})
	return nil
}

func (f *fakeAPI) ClearValues(ctx context.Context, spreadsheetID, rangeA1 string) error {
	f.cleared = append(f.cleared, rangeA1)
	return nil
}

func (f *fakeAPI) WriteRows(ctx context.Context, spreadsheetID, rangeA1 string, values [][]interface{}) error {
	f.writeRowsRange = rangeA1
	f.writeRowsValues = values
	return nil
}

func (f *fakeAPI) EnsureTab(ctx context.Context, spreadsheetID, sheetName string) (bool, error) {
	f.ensureTabCalled = true
	f.ensureTabNames = append(f.ensureTabNames, sheetName)
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

func (f *fakeAPI) SetLinkedLines(ctx context.Context, spreadsheetID, sheetName string, row, col int, lines []LinkedLine) error {
	f.linkedLinesRow = row
	f.linkedLinesCol = col
	f.linkedLinesLines = lines
	return f.linkedLinesErr
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
	row := CandidateRow{Candidate: "alice", Discord: "@alice", Status: StatusCandidate, Architecture: "3 sentries", BackupPlan: "hot standby"}
	got, err := AppendCandidateRow(context.Background(), api, "sheet-id", "Sheet1", row)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != 2 {
		t.Errorf("got row %d, want 2", got)
	}
	if api.updateRowRange != "Sheet1!A2:O2" {
		t.Errorf("got range %q, want %q", api.updateRowRange, "Sheet1!A2:O2")
	}
	if len(api.updateRowValues) != len(Headers) {
		t.Fatalf("got %d values, want %d", len(api.updateRowValues), len(Headers))
	}
	if api.updateRowValues[0] != "alice" {
		t.Errorf("got candidate %v, want alice", api.updateRowValues[0])
	}
	if api.updateRowValues[ColumnArchitecture] != "3 sentries" {
		t.Errorf("architecture at col M = %v, want '3 sentries'", api.updateRowValues[ColumnArchitecture])
	}
	if api.updateRowValues[ColumnBackupPlan] != "hot standby" {
		t.Errorf("backup plan at col N = %v, want 'hot standby'", api.updateRowValues[ColumnBackupPlan])
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
	if api.updateRowRange != "Sheet1!A4:O4" {
		t.Errorf("got range %q, want %q", api.updateRowRange, "Sheet1!A4:O4")
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

func TestClearRow(t *testing.T) {
	api := &fakeAPI{}
	if err := ClearRow(context.Background(), api, "sheet-id", "Test", 7); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if api.updateRowRange != "Test!A7:O7" {
		t.Errorf("got range %q, want %q", api.updateRowRange, "Test!A7:O7")
	}
	if len(api.updateRowValues) != len(Headers) {
		t.Fatalf("got %d values, want %d", len(api.updateRowValues), len(Headers))
	}
	for i, v := range api.updateRowValues {
		if v != "" {
			t.Errorf("value[%d] = %v, want empty", i, v)
		}
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
	if api.updateRowRange != "Test!A1:O1" {
		t.Errorf("got header range %q, want %q", api.updateRowRange, "Test!A1:O1")
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
	if api.updateRowRange != "Test-approved!A1:AA1" {
		t.Errorf("got header range %q, want %q", api.updateRowRange, "Test-approved!A1:AA1")
	}
	if len(api.updateRowValues) != 27 {
		t.Fatalf("approved headers must span A-AA (27 cols), got %d", len(api.updateRowValues))
	}
	if api.updateRowValues[26] != "Evidence links" {
		t.Errorf("last approved header = %v, want \"Evidence links\"", api.updateRowValues[26])
	}
	if api.setFormulaRange != "Test-approved!A2" {
		t.Errorf("got formula range %q, want %q", api.setFormulaRange, "Test-approved!A2")
	}
	// A single self-spilling QUERY (IFS/VSTACK cannot return a multi-row array).
	if !strings.Contains(api.setFormulaFormula, "QUERY(") {
		t.Errorf("formula missing QUERY: %s", api.setFormulaFormula)
	}
	if strings.Contains(api.setFormulaFormula, "IFS(") || strings.Contains(api.setFormulaFormula, "VSTACK(") {
		t.Errorf("formula must not use IFS/VSTACK (they can't spill arrays): %s", api.setFormulaFormula)
	}
	if !strings.Contains(api.setFormulaFormula, "GovDAO submitted") || !strings.Contains(api.setFormulaFormula, "GovDAO pending") {
		t.Errorf("formula missing a GovDAO status: %s", api.setFormulaFormula)
	}
	if !strings.Contains(api.setFormulaFormula, "order by") {
		t.Errorf("formula missing 'order by' (submitted above pending): %s", api.setFormulaFormula)
	}
	subIdx := strings.Index(api.setFormulaFormula, "GovDAO submitted")
	penIdx := strings.Index(api.setFormulaFormula, "GovDAO pending")
	if subIdx < 0 || penIdx < 0 || subIdx >= penIdx {
		t.Errorf("submitted must appear before pending: %s", api.setFormulaFormula)
	}
	// QUERY must pass headers=0 so it never lifts the first data row into a header.
	if !strings.Contains(api.setFormulaFormula, ", 0)") {
		t.Errorf("QUERY missing explicit headers=0 argument: %s", api.setFormulaFormula)
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

func TestEnsureApprovedView_RewritesNarrowHeaderToFullWidth(t *testing.T) {
	// An older approved tab with only the A-O intake header must be brought up to
	// the full A-AA schema, not skipped.
	api := &fakeAPI{getResult: [][]interface{}{{"Candidate"}}}
	if err := EnsureApprovedView(context.Background(), api, "sheet-id", "Test"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if api.updateRowRange != "Test-approved!A1:AA1" {
		t.Errorf("headers must be rewritten to A1:AA1, got %q", api.updateRowRange)
	}
	if api.setFormulaRange != "Test-approved!A2" {
		t.Errorf("SetFormula not called or wrong range: %q", api.setFormulaRange)
	}
	if !strings.Contains(api.setFormulaFormula, "QUERY('Test'!A2:AA") {
		t.Errorf("formula must mirror the full A2:AA range: %s", api.setFormulaFormula)
	}
}

func TestEnsure_SkipsHeadersWhenPresent(t *testing.T) {
	// Ensure must not clobber a non-empty row 1; it writes the header only on an
	// empty tab.
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

func TestIsValidated(t *testing.T) {
	for _, s := range []string{StatusApproved, StatusGovDAOPending, StatusGovDAOSubmitted, " approved ", "GOVDAO SUBMITTED"} {
		if !IsValidated(s) {
			t.Errorf("IsValidated(%q) = false, want true", s)
		}
	}
	for _, s := range []string{StatusCandidate, StatusChallengeInProgress, StatusNeedsRetry, StatusDeclined, "", "rejected"} {
		if IsValidated(s) {
			t.Errorf("IsValidated(%q) = true, want false", s)
		}
	}
}

func TestIsReopenable(t *testing.T) {
	for _, s := range []string{StatusNeedsRetry, StatusDeclined, " needs retry ", "DECLINED"} {
		if !IsReopenable(s) {
			t.Errorf("IsReopenable(%q) = false, want true", s)
		}
	}
	for _, s := range []string{StatusCandidate, StatusChallengeInProgress, StatusApproved, StatusGovDAOPending, "", "rejected"} {
		if IsReopenable(s) {
			t.Errorf("IsReopenable(%q) = true, want false", s)
		}
	}
}

func TestReadCandidates(t *testing.T) {
	api := &fakeAPI{getResult: [][]interface{}{
		// A..M: Candidate, Discord, Status, Challenge, Reviewers, Missing, Decision, Valoper, GovDAO, Moniker, Operator, Intro, ReviewLink
		{"alice", "@alice", "Approved", "", "", "", "", "https://v/g1a", "", "alice-val", "g1alice", "intro a", ""},
		{"", "", ""}, // blank candidate -> skipped
		{"bob", "@bob", "Candidate"},
	}}
	got, err := ReadCandidates(context.Background(), api, "id", "Candidates")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d rows, want 2", len(got))
	}
	if got[0].Row != 2 || got[0].Candidate != "alice" || got[0].Status != "Approved" ||
		got[0].Moniker != "alice-val" || got[0].OperatorAddress != "g1alice" || got[0].Valoper != "https://v/g1a" {
		t.Errorf("alice = %+v", got[0])
	}
	if got[1].Row != 4 || got[1].Candidate != "bob" || got[1].OperatorAddress != "" {
		t.Errorf("bob = %+v", got[1])
	}
}

func TestWriteHarvestColumns(t *testing.T) {
	api := &fakeAPI{}
	if err := WriteHarvestColumns(context.Background(), api, "id", "Candidates", 2, "Secret leak: private_key", "12 msgs"); err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, u := range api.updates {
		got[u.rangeA1] = u.value
	}
	if got["Candidates!Y2"] != "Secret leak: private_key" || got["Candidates!Z2"] != "12 msgs" {
		t.Errorf("Red flags (Y2)/Engagement (Z2) = %v", got)
	}
}

func TestWriteDigestColumns(t *testing.T) {
	api := &fakeAPI{}
	criteria := []bool{true, true, false, false, true, false, true} // setup,sync,tx,valoper,ops,comms,safety
	evidence := []LinkedLine{{Text: "Submission", URL: "https://l"}}
	if err := WriteDigestColumns(context.Background(), api, "id", "Candidates", 2, "High (6/7)", "ok", evidence, criteria); err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, u := range api.updates {
		got[u.rangeA1] = u.value
	}
	if got["Candidates!P2"] != "High (6/7)" || got["Candidates!Q2"] != "ok" {
		t.Errorf("text columns = %v", got)
	}
	// Evidence links go through the rich-text path, not a plain value write.
	if got["Candidates!AA2"] != "" {
		t.Errorf("evidence links should not be written as a plain value, got %q", got["Candidates!AA2"])
	}
	if api.linkedLinesCol != int(ColumnEvidenceLinks) || api.linkedLinesRow != 2 {
		t.Errorf("linked lines target = row %d col %d, want row 2 col %d", api.linkedLinesRow, api.linkedLinesCol, int(ColumnEvidenceLinks))
	}
	if len(api.linkedLinesLines) != 1 || api.linkedLinesLines[0] != (LinkedLine{Text: "Submission", URL: "https://l"}) {
		t.Errorf("linked lines = %#v", api.linkedLinesLines)
	}
	if api.updateRowRange != "Candidates!R2:X2" {
		t.Errorf("criteria range = %q, want Candidates!R2:X2", api.updateRowRange)
	}
	// Assert all 7 positions so a reordering inside writeCriteria can't slip a
	// criterion into the wrong checkbox column.
	if len(api.updateRowValues) != len(criteria) {
		t.Fatalf("got %d criterion cells, want %d", len(api.updateRowValues), len(criteria))
	}
	for i, want := range criteria {
		if api.updateRowValues[i] != want {
			t.Errorf("criterion[%d] = %v, want %v", i, api.updateRowValues[i], want)
		}
	}
}

func TestLinkedCellRuns(t *testing.T) {
	text, runs := linkedCellRuns([]LinkedLine{
		{Text: "Submission", URL: "https://a"},
		{Text: "Sync proof", URL: "https://b"},
	})
	if text != "Submission\nSync proof" {
		t.Fatalf("text = %q", text)
	}
	// One run per link, plus a link-closing run on the separating newline so the
	// first link does not bleed into the second line.
	want := []linkRun{
		{start: 0, uri: "https://a"},
		{start: 10, uri: ""},
		{start: 11, uri: "https://b"},
	}
	if len(runs) != len(want) {
		t.Fatalf("got %d runs, want %d: %#v", len(runs), len(want), runs)
	}
	for i := range want {
		if runs[i] != want[i] {
			t.Errorf("run[%d] = %#v, want %#v", i, runs[i], want[i])
		}
	}

	if text, runs := linkedCellRuns(nil); text != "" || len(runs) != 0 {
		t.Errorf("empty input: text=%q runs=%#v", text, runs)
	}
}

func TestMarkDuplicateRow(t *testing.T) {
	api := &fakeAPI{}
	if err := MarkDuplicateRow(context.Background(), api, "id", "Candidates", 3, 5); err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, u := range api.updates {
		got[u.rangeA1] = u.value
	}
	if got["Candidates!P3"] != "Duplicate of row 5" {
		t.Errorf("Readiness = %q", got["Candidates!P3"])
	}
	for _, cell := range []string{"Q3", "Y3", "Z3", "AA3"} {
		if v, ok := got["Candidates!"+cell]; !ok || v != "" {
			t.Errorf("%s should be cleared (got %q, present=%v)", cell, v, ok)
		}
	}
	if api.updateRowRange != "Candidates!R3:X3" || len(api.updateRowValues) != 7 || api.updateRowValues[0] != false {
		t.Errorf("criteria not cleared: range=%q vals=%v", api.updateRowRange, api.updateRowValues)
	}
}

func TestEnsureHarvestLayout(t *testing.T) {
	api := &fakeAPI{}
	if err := EnsureHarvestLayout(context.Background(), api, "id", "Candidates"); err != nil {
		t.Fatal(err)
	}
	// one checkbox range: criteria R-X (ColumnSetup..ColumnSafety+1). No Selected.
	if len(api.checkboxes) != 1 {
		t.Fatalf("checkbox calls = %v, want 1 (criteria only)", api.checkboxes)
	}
	if api.checkboxes[0] != [2]Column{ColumnSetup, ColumnSafety + 1} {
		t.Errorf("criteria checkbox range = %v", api.checkboxes[0])
	}
	// assessment header row P1:AA1 written
	if !hasUpdateRow(api, "Candidates!P1:AA1") {
		t.Errorf("assessment header not written; calls=%v", api.updateRowCalls)
	}
	// evidence tab ensured, by its derived name
	foundEvidence := false
	for _, n := range api.ensureTabNames {
		if n == EvidenceTabName("Candidates") {
			foundEvidence = true
		}
	}
	if !foundEvidence {
		t.Errorf("evidence tab %q not ensured; ensured=%v", EvidenceTabName("Candidates"), api.ensureTabNames)
	}
}

func hasUpdateRow(api *fakeAPI, rangeA1 string) bool {
	for _, c := range api.updateRowCalls {
		if c.rangeA1 == rangeA1 {
			return true
		}
	}
	return false
}

func TestColumnLetters_ArchitectureAndBackup(t *testing.T) {
	if got := columnLetter(ColumnArchitecture); got != "M" {
		t.Errorf("ColumnArchitecture letter = %q, want M", got)
	}
	if got := columnLetter(ColumnBackupPlan); got != "N" {
		t.Errorf("ColumnBackupPlan letter = %q, want N", got)
	}
}
