package render

import (
	"bytes"
	"testing"
)

func TestWriteTableRightAligns(t *testing.T) {
	cols := []colSpec{{"NAME", false}, {"AMOUNT", true}}
	rows := [][]cell{
		{{"a", toneNone}, {"5.00", toneNone}},
		{{"bb", toneNone}, {"100.00", toneNone}},
	}
	var buf bytes.Buffer
	if err := writeTable(&buf, cols, rows, false); err != nil {
		t.Fatal(err)
	}
	got := buf.String()
	// AMOUNT column right-aligned to width 6 ("100.00"); "5.00" gets two
	// leading spaces. Two-space gutter after the NAME column (width 4).
	want := "NAME  AMOUNT\n" +
		"a       5.00\n" +
		"bb    100.00\n"
	if got != want {
		t.Errorf("got:\n%q\nwant:\n%q", got, want)
	}
}
