package hltv

import "testing"

func TestIsRateLimitedPath(t *testing.T) {
	base := "https://www.hltv.org"
	if !isRateLimitedPath(base + "/results?team=6665&offset=0") {
		t.Fatal("/results?team= should be rate limited")
	}
	if isRateLimitedPath(base + "/results?event=9029&offset=0") {
		t.Fatal("/results?event= must NOT be rate limited")
	}
	if isRateLimitedPath(base + "/stats/teams/maps/6665/_?startDate=2025-01-01") {
		t.Fatal("map stats use Chrome, not HTTP rate limit")
	}
	if isRateLimitedPath(base + "/team/6665/astralis") {
		t.Fatal("team page should not be rate limited")
	}
}
