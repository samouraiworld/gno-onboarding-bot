package forms

import (
	"reflect"
	"testing"
)

func TestSplitLines(t *testing.T) {
	got := SplitLines("a\n  b  \n\nc\n")
	want := []string{"a", "b", "c"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("SplitLines() = %v, want %v", got, want)
	}
}

func TestSplitLines_Empty(t *testing.T) {
	got := SplitLines("   \n\n  ")
	if len(got) != 0 {
		t.Errorf("SplitLines() = %v, want empty", got)
	}
}

func TestMissingRequired(t *testing.T) {
	got := MissingRequired([]Field{
		{Label: "Moniker", Value: "alice"},
		{Label: "Valoper link", Value: "   "},
		{Label: "Intro", Value: ""},
	})
	want := []string{"Valoper link", "Intro"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("MissingRequired() = %v, want %v", got, want)
	}
}

func TestMissingRequired_NoneMissing(t *testing.T) {
	got := MissingRequired([]Field{{Label: "Moniker", Value: "alice"}})
	if len(got) != 0 {
		t.Errorf("MissingRequired() = %v, want empty", got)
	}
}
