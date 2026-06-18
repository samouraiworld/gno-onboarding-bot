package rowref

import "testing"

func TestEncodeDecodeRoundTrip(t *testing.T) {
	got := Encode(58, "123456789012345678")
	row, candidateID, err := Decode(got)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if row != 58 {
		t.Errorf("row = %d, want 58", row)
	}
	if candidateID != "123456789012345678" {
		t.Errorf("candidateID = %q, want %q", candidateID, "123456789012345678")
	}
}

func TestDecode_Invalid(t *testing.T) {
	cases := []string{"", "notanumber|123", "58|", "58"}
	for _, c := range cases {
		if _, _, err := Decode(c); err == nil {
			t.Errorf("Decode(%q): expected error, got nil", c)
		}
	}
}

func TestCustomIDRoundTrip(t *testing.T) {
	id := CustomID("approve", 12, "999")
	action, row, candidateID, err := DecodeCustomID(id)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if action != "approve" {
		t.Errorf("action = %q, want %q", action, "approve")
	}
	if row != 12 {
		t.Errorf("row = %d, want 12", row)
	}
	if candidateID != "999" {
		t.Errorf("candidateID = %q, want %q", candidateID, "999")
	}
}

func TestDecodeCustomID_Invalid(t *testing.T) {
	if _, _, _, err := DecodeCustomID("noseparator"); err == nil {
		t.Error("expected error for missing separator")
	}
}
