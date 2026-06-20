package handlers

import (
	"testing"

	"onboardingbot/internal/sheet"
)

func TestPartitionLatest(t *testing.T) {
	active := []sheet.TrackerRow{
		{Row: 2, Candidate: "alice", Discord: "@alice"},
		{Row: 3, Candidate: "bob", Discord: "@bob"},
		{Row: 5, Candidate: "alice-again", Discord: "@alice"}, // resubmission, higher row wins
		{Row: 6, Candidate: "no-handle-1", Discord: ""},
		{Row: 7, Candidate: "no-handle-2", Discord: ""},
	}
	records, superseded := partitionLatest(active)

	gotRows := map[int]bool{}
	for _, r := range records {
		gotRows[r.Row] = true
	}
	// Latest alice (5), bob (3), and both handle-less rows are evaluated; alice's
	// older row 2 is not.
	if len(records) != 4 || !gotRows[3] || !gotRows[5] || !gotRows[6] || !gotRows[7] {
		t.Errorf("records = %v, want rows {3,5,6,7}", gotRows)
	}
	if gotRows[2] {
		t.Error("superseded alice row 2 must not be evaluated")
	}
	if len(superseded) != 1 || superseded[0].row != 2 || superseded[0].keptRow != 5 {
		t.Errorf("superseded = %+v, want [{row:2 keptRow:5}]", superseded)
	}
}

func TestPartitionLatest_HandleCaseInsensitive(t *testing.T) {
	// "@Alice" and "@alice" normalize to the same handle, so the higher row wins.
	active := []sheet.TrackerRow{
		{Row: 2, Candidate: "alice", Discord: "@Alice"},
		{Row: 4, Candidate: "alice", Discord: "@alice"},
	}
	records, superseded := partitionLatest(active)
	if len(records) != 1 || records[0].Row != 4 {
		t.Errorf("records = %+v, want only row 4", records)
	}
	if len(superseded) != 1 || superseded[0].row != 2 || superseded[0].keptRow != 4 {
		t.Errorf("superseded = %+v, want [{row:2 keptRow:4}]", superseded)
	}
}

func TestJoinCapped(t *testing.T) {
	if got := joinCapped(nil, 10); got != "" {
		t.Errorf("empty = %q, want \"\"", got)
	}
	if got := joinCapped([]string{"a", "b"}, 10); got != "a; b" {
		t.Errorf("under cap = %q, want \"a; b\"", got)
	}
	if got := joinCapped([]string{"a", "b", "c"}, 3); got != "a; b; c" {
		t.Errorf("at cap = %q, want all three (no remainder)", got)
	}
	if got := joinCapped([]string{"a", "b", "c", "d"}, 3); got != "a; b; c; and 1 more" {
		t.Errorf("over cap = %q, want truncation + remainder", got)
	}
}
