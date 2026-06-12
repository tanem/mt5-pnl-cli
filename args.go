package main

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var lastRe = regexp.MustCompile(`^(\d+)([dwmy])$`)

func civilDate(t time.Time) time.Time {
	u := t.UTC()
	return time.Date(u.Year(), u.Month(), u.Day(), 0, 0, 0, 0, time.UTC)
}

// parseLast turns "30d" / "2w" / "6m" / "1y" into an inclusive (from, to)
// date pair ending today. Months and years are calendar-accurate (AddDate).
func parseLast(s string, now time.Time) (time.Time, time.Time, error) {
	m := lastRe.FindStringSubmatch(strings.ToLower(strings.TrimSpace(s)))
	if m == nil {
		return time.Time{}, time.Time{}, fmt.Errorf("invalid --last %q: use e.g. 30d, 2w, 6m, 1y", s)
	}
	n, _ := strconv.Atoi(m[1])
	today := civilDate(now)
	var from time.Time
	switch m[2] {
	case "d":
		from = today.AddDate(0, 0, -n)
	case "w":
		from = today.AddDate(0, 0, -7*n)
	case "m":
		from = today.AddDate(0, -n, 0)
	case "y":
		from = today.AddDate(-n, 0, 0)
	}
	return from, today, nil
}

func resolveRange(last, from, to string, now time.Time) (time.Time, time.Time, error) {
	var zero time.Time
	if last != "" && (from != "" || to != "") {
		return zero, zero, errors.New("--last cannot be combined with --from/--to")
	}
	if to != "" && from == "" {
		return zero, zero, errors.New("--to requires --from")
	}
	if from != "" {
		f, err := time.ParseInLocation("2006-01-02", from, time.UTC)
		if err != nil {
			return zero, zero, fmt.Errorf("invalid --from %q: use YYYY-MM-DD", from)
		}
		t := civilDate(now)
		if to != "" {
			t, err = time.ParseInLocation("2006-01-02", to, time.UTC)
			if err != nil {
				return zero, zero, fmt.Errorf("invalid --to %q: use YYYY-MM-DD", to)
			}
		}
		if t.Before(f) {
			return zero, zero, errors.New("--to is before --from")
		}
		return f, t, nil
	}
	if last == "" {
		last = "30d"
	}
	return parseLast(last, now)
}
