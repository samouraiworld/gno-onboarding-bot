package sheet

import "testing"

func TestParseA1Cell(t *testing.T) {
	tests := []struct {
		in       string
		row, col int
		wantErr  bool
	}{
		{"A1", 0, 0, false},
		{"A2", 1, 0, false},
		{"B2", 1, 1, false},
		{"M2", 1, 12, false},
		{"AA1", 0, 26, false},
		{"AB10", 9, 27, false},
		{"", 0, 0, true},
		{"A", 0, 0, true},
		{"1", 0, 0, true},
		{"A0", 0, 0, true},
	}
	for _, tt := range tests {
		row, col, err := parseA1Cell(tt.in)
		if tt.wantErr {
			if err == nil {
				t.Errorf("parseA1Cell(%q): expected error, got (%d,%d)", tt.in, row, col)
			}
			continue
		}
		if err != nil {
			t.Errorf("parseA1Cell(%q): unexpected error: %v", tt.in, err)
		}
		if row != tt.row || col != tt.col {
			t.Errorf("parseA1Cell(%q) = (%d,%d), want (%d,%d)", tt.in, row, col, tt.row, tt.col)
		}
	}
}
