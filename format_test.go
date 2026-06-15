package main

import "testing"

func TestResolveFormat(t *testing.T) {
	cases := []struct {
		name                        string
		format                      string
		formatSet, jsonSet, jsonVal bool
		want                        string
		wantErr                     bool
	}{
		{"default table", "table", false, false, false, "table", false},
		{"json alias alone", "table", false, true, true, "json", false},
		{"json=false ignored", "table", false, true, false, "table", false},
		{"csv explicit", "csv", true, false, false, "csv", false},
		{"json and --format json agree", "json", true, true, true, "json", false},
		{"json conflicts with csv", "csv", true, true, true, "", true},
		{"invalid value", "bogus", true, false, false, "", true},
	}
	for _, c := range cases {
		got, err := resolveFormat(c.format, c.formatSet, c.jsonSet, c.jsonVal)
		if c.wantErr {
			if err == nil {
				t.Errorf("%s: want error, got %q", c.name, got)
			}
			continue
		}
		if err != nil || got != c.want {
			t.Errorf("%s: got %q (err %v), want %q", c.name, got, err, c.want)
		}
	}
}
