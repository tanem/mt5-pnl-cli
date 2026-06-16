package main

import (
	"bytes"
	"testing"
)

func TestResolveColor(t *testing.T) {
	env := func(m map[string]string) func(string) string {
		return func(k string) string { return m[k] }
	}
	var buf bytes.Buffer // not an *os.File, so isatty is false

	cases := []struct {
		name string
		mode string
		env  map[string]string
		want bool
	}{
		{"always overrides everything", "always", map[string]string{"NO_COLOR": "1"}, true},
		{"never overrides everything", "never", nil, false},
		{"auto to a buffer is off", "auto", nil, false},
		{"auto honours NO_COLOR", "auto", map[string]string{"NO_COLOR": "1"}, false},
		{"auto honours TERM=dumb", "auto", map[string]string{"TERM": "dumb"}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := resolveColor(c.mode, &buf, env(c.env)); got != c.want {
				t.Errorf("resolveColor(%q) = %v, want %v", c.mode, got, c.want)
			}
		})
	}
}
