package hltv

import "testing"

func TestUrlsMatch(t *testing.T) {
	tests := []struct {
		current string
		target  string
		want    bool
	}{
		{
			"https://www.hltv.org/events",
			"https://www.hltv.org/events",
			true,
		},
		{
			"https://www.hltv.org/results?event=9029&offset=0",
			"https://www.hltv.org/results?event=8301&offset=0",
			false,
		},
		{
			"https://www.hltv.org/results?event=9029&offset=0",
			"https://www.hltv.org/results?event=9029&offset=0",
			true,
		},
	}
	for _, tc := range tests {
		got := urlsMatch(tc.current, tc.target)
		if got != tc.want {
			t.Fatalf("%s vs %s: got %v want %v", tc.current, tc.target, got, tc.want)
		}
	}
}
