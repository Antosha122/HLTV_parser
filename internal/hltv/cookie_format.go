package hltv

import "strings"

// EssentialCookie keeps only Cloudflare cookies needed for HLTV requests.
func EssentialCookie(cookie string) string {
	cf, bm := ParseCookieParts(NormalizeCookie(cookie))
	if cf == "" {
		return ""
	}
	return BuildCookieFromParts(cf, bm)
}

// BuildCookieFromParts joins cf_clearance and __cf_bm into a request Cookie header value.
func BuildCookieFromParts(cfClearance, cfBm string) string {
	cf := NormalizeCookieValue("cf_clearance", cfClearance)
	bm := NormalizeCookieValue("__cf_bm", cfBm)
	if cf == "" {
		return ""
	}
	if bm == "" {
		return "cf_clearance=" + cf
	}
	return "cf_clearance=" + cf + "; __cf_bm=" + bm
}

// ParseCookieParts extracts cf_clearance and __cf_bm from a combined cookie string.
func ParseCookieParts(cookie string) (cfClearance, cfBm string) {
	for _, chunk := range splitCookieChunks(NormalizeCookie(cookie)) {
		name, value, ok := strings.Cut(chunk, "=")
		if !ok {
			continue
		}
		name = strings.TrimSpace(name)
		value = strings.TrimSpace(value)
		switch name {
		case "cf_clearance":
			cfClearance = value
		case "__cf_bm":
			cfBm = value
		}
	}
	return cfClearance, cfBm
}

// NormalizeCookieValue strips a name prefix and returns the raw cookie value.
func NormalizeCookieValue(name, input string) string {
	input = strings.TrimSpace(input)
	if input == "" {
		return ""
	}
	lowerName := strings.ToLower(name)
	lowerInput := strings.ToLower(input)
	for _, sep := range []string{"=", ":"} {
		prefix := lowerName + sep
		if strings.HasPrefix(lowerInput, prefix) {
			return strings.TrimSpace(input[len(name)+1:])
		}
	}
	if strings.HasPrefix(strings.ToLower(input), "cookie:") {
		cf, bm := ParseCookieParts(input[7:])
		if name == "cf_clearance" && cf != "" {
			return cf
		}
		if name == "__cf_bm" && bm != "" {
			return bm
		}
	}
	return input
}

// NormalizeCookie fixes common paste mistakes (colon instead of =, bare value, multiple lines).
func NormalizeCookie(input string) string {
	input = strings.TrimSpace(input)
	if input == "" {
		return ""
	}

	var parts []string
	for _, chunk := range splitCookieChunks(input) {
		if pair := parseCookiePair(chunk); pair != "" {
			parts = append(parts, pair)
		}
	}
	if len(parts) > 0 {
		return strings.Join(parts, "; ")
	}

	one := strings.ReplaceAll(input, "\n", "")
	one = strings.TrimSpace(one)
	if !strings.Contains(one, "=") && !strings.Contains(one, ":") && len(one) > 20 {
		return "cf_clearance=" + one
	}
	return one
}

func splitCookieChunks(s string) []string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	var out []string
	for _, line := range strings.Split(s, "\n") {
		for _, p := range strings.Split(line, ";") {
			p = strings.TrimSpace(p)
			if p != "" {
				out = append(out, p)
			}
		}
	}
	return out
}

func parseCookiePair(chunk string) string {
	chunk = strings.TrimSpace(chunk)
	if strings.HasPrefix(strings.ToLower(chunk), "cookie:") {
		chunk = strings.TrimSpace(chunk[7:])
	}
	if chunk == "" {
		return ""
	}

	for _, sep := range []string{":", "="} {
		idx := strings.Index(chunk, sep)
		if idx <= 0 {
			continue
		}
		name := strings.TrimSpace(chunk[:idx])
		value := strings.TrimSpace(chunk[idx+1:])
		if name == "" || value == "" {
			continue
		}
		if isCookieName(name) {
			return name + "=" + value
		}
	}
	return ""
}

func isCookieName(name string) bool {
	if name == "cf_clearance" || name == "__cf_bm" || name == "__cflb" {
		return true
	}
	return strings.HasPrefix(name, "cf_") || strings.HasPrefix(name, "__cf") || strings.HasPrefix(name, "_")
}
