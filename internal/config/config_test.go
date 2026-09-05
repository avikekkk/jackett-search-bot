package config

import "testing"

func TestParseJackettURL(t *testing.T) {
	cases := []struct {
		in, want string
		wantErr  bool
	}{
		{"http://localhost:9117", "http://localhost:9117", false},
		{"http://localhost:9117/", "http://localhost:9117", false},
		// A pasted torznab path must not be doubled up on every lookup.
		{"http://host:9117/api/v2.0/indexers/all/results/torznab/", "http://host:9117", false},
		{"https://jackett.example.com/?apikey=x", "https://jackett.example.com", false},
		{"localhost:9117", "", true},
		{"", "", true},
	}
	for _, tc := range cases {
		got, err := parseJackettURL(tc.in)
		if (err != nil) != tc.wantErr {
			t.Errorf("parseJackettURL(%q) err = %v, wantErr %v", tc.in, err, tc.wantErr)
			continue
		}
		if got != tc.want {
			t.Errorf("parseJackettURL(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
