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

func TestWriteKVAligns(t *testing.T) {
	var buf bytes.Buffer
	err := writeKV(&buf, []kvGroup{
		{"Performance", []kv{
			{"Trades", "4", toneNone},
			{"Profit factor", "3.50", toneNone},
		}},
		{"P&L breakdown", []kv{
			{"Net P&L", "10.00", tonePos},
		}},
	}, false)
	if err != nil {
		t.Fatal(err)
	}
	want := "Summary\n" +
		"  Performance\n" +
		"    Trades         4\n" +
		"    Profit factor  3.50\n" +
		"  P&L breakdown\n" +
		"    Net P&L        10.00\n"
	if buf.String() != want {
		t.Errorf("writeKV output:\ngot:\n%q\nwant:\n%q", buf.String(), want)
	}
}
