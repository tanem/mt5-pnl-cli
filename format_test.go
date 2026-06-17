package main

import "testing"

func TestResolveFormat(t *testing.T) {
	cases := []struct {
		name    string
		format  string
		want    string
		wantErr bool
	}{
		{"default table", "table", "table", false},
		{"json", "json", "json", false},
		{"csv", "csv", "csv", false},
		{"invalid value", "bogus", "", true},
	}
	for _, c := range cases {
		got, err := resolveFormat(c.format)
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
