package hltv

import "testing"

func TestResolveHLTVURL(t *testing.T) {
	tests := []struct {
		path string
		want string
	}{
		{"/events", "https://www.hltv.org/events"},
		{"/results?event=9148&offset=0", "https://www.hltv.org/results?event=9148&offset=0"},
	}
	for _, tc := range tests {
		got, err := resolveHLTVURL("https://www.hltv.org", tc.path)
		if err != nil {
			t.Fatalf("path %q: %v", tc.path, err)
		}
		if got != tc.want {
			t.Fatalf("path %q: got %q want %q", tc.path, got, tc.want)
		}
	}
}
