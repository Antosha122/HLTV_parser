package hltv

import "strings"

// isRateLimitedPath marks HLTV endpoints that Cloudflare rate-limits under heavy scraping.
// /results?event= is the primary match source for events and must NOT be blocked.
// /stats/teams/maps/ is loaded via Chrome, not HTTP — do not apply HTTP cooldown here.
func isRateLimitedPath(fullURL string) bool {
	low := strings.ToLower(fullURL)
	if strings.Contains(low, "/results?") {
		// only team results are rate-limited; event results are not
		return strings.Contains(low, "team=") && !strings.Contains(low, "event=")
	}
	return false
}
