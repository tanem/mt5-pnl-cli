package render

import (
	"io"
	"strings"
)

type tone int

const (
	toneNone tone = iota
	tonePos
	toneNeg
)

const (
	ansiGreen = "\x1b[32m"
	ansiRed   = "\x1b[31m"
	ansiReset = "\x1b[0m"
)

type colSpec struct {
	header string
	right  bool
}

type cell struct {
	text string
	tone tone
}

// writeTable renders a header and body rows as a fixed-width table with
// per-column alignment. Widths are computed from the plain cell text, so ANSI
// colour (applied after padding when color is true) never skews alignment.
// Columns are separated by two spaces (matching the old tabwriter gutter); a
// left-aligned final column is not right-padded, so there is no trailing space.
// Each row must have exactly len(cols) cells.
func writeTable(w io.Writer, cols []colSpec, rows [][]cell, color bool) error {
	widths := make([]int, len(cols))
	for i, c := range cols {
		widths[i] = len(c.header)
	}
	for _, row := range rows {
		for i, cl := range row {
			if len(cl.text) > widths[i] {
				widths[i] = len(cl.text)
			}
		}
	}
	var b strings.Builder
	writeRow := func(get func(i int) (string, tone)) {
		for i := range cols {
			if i > 0 {
				b.WriteString("  ")
			}
			text, tn := get(i)
			last := i == len(cols)-1
			if !(last && !cols[i].right) {
				text = pad(text, widths[i], cols[i].right)
			}
			b.WriteString(colorise(text, tn, color))
		}
		b.WriteByte('\n')
	}
	writeRow(func(i int) (string, tone) { return cols[i].header, toneNone })
	for _, row := range rows {
		writeRow(func(i int) (string, tone) { return row[i].text, row[i].tone })
	}
	_, err := io.WriteString(w, b.String())
	return err
}

func pad(s string, width int, right bool) string {
	gap := width - len(s)
	if gap <= 0 {
		return s
	}
	if right {
		return strings.Repeat(" ", gap) + s
	}
	return s + strings.Repeat(" ", gap)
}

func colorise(s string, t tone, color bool) string {
	if !color || t == toneNone {
		return s
	}
	code := ansiGreen
	if t == toneNeg {
		code = ansiRed
	}
	return code + s + ansiReset
}
