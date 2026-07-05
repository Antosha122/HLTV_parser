package hltv

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"

	"psr/internal/models"
)

var mapNames = map[string]string{
	"ancient": "Ancient", "anubis": "Anubis", "dust2": "Dust2", "dust ii": "Dust2",
	"inferno": "Inferno", "mirage": "Mirage", "nuke": "Nuke",
	"overpass": "Overpass", "vertigo": "Vertigo", "train": "Train", "cache": "Cache",
}

func parseDoc(html string) (*goquery.Document, error) {
	return goquery.NewDocumentFromReader(strings.NewReader(html))
}

func idAt(path string, idx int) int {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if idx < 0 || idx >= len(parts) {
		return 0
	}
	id, _ := strconv.Atoi(parts[idx])
	return id
}

func normalizeMap(name string) string {
	key := strings.ToLower(strings.TrimSpace(name))
	if v, ok := mapNames[key]; ok {
		return v
	}
	if name == "" {
		return ""
	}
	return strings.ToUpper(name[:1]) + strings.ToLower(name[1:])
}

func parseUnixAttr(s *goquery.Selection) time.Time {
	if s.Length() == 0 {
		return time.Time{}
	}
	for _, attr := range []string{"data-unix", "data-zonedgrouping-entry-unix"} {
		raw, ok := s.Attr(attr)
		if !ok || raw == "" {
			continue
		}
		ms, err := strconv.ParseInt(raw, 10, 64)
		if err != nil {
			continue
		}
		if ms > 1_000_000_000_000 {
			return time.UnixMilli(ms).UTC()
		}
		return time.Unix(ms, 0).UTC()
	}
	return time.Time{}
}

func parseUnixFromRow(row *goquery.Selection) time.Time {
	if t := parseUnixAttr(row); !t.IsZero() {
		return t
	}
	con := row.Closest(".result-con")
	if t := parseUnixAttr(con); !t.IsZero() {
		return t
	}
	return time.Time{}
}

func parseFormat(text string) string {
	lower := strings.ToLower(text)
	switch {
	case strings.Contains(lower, "bo5"), strings.Contains(lower, "best of 5"):
		return "bo5"
	case strings.Contains(lower, "bo3"), strings.Contains(lower, "best of 3"):
		return "bo3"
	case strings.Contains(lower, "bo1"), strings.Contains(lower, "best of 1"):
		return "bo1"
	default:
		return ""
	}
}

func parseFloat(text string) float64 {
	text = strings.TrimSpace(text)
	text = strings.TrimSuffix(text, "%")
	text = strings.ReplaceAll(text, ",", ".")
	v, _ := strconv.ParseFloat(text, 64)
	return v
}

func parseInt(text string) int {
	text = strings.TrimSpace(text)
	text = strings.TrimPrefix(text, "#")
	v, _ := strconv.Atoi(text)
	return v
}

func teamNameFromHref(href string) string {
	parts := strings.Split(strings.Trim(href, "/"), "/")
	if len(parts) < 3 || parts[0] != "team" {
		return ""
	}
	return strings.ReplaceAll(parts[2], "-", " ")
}

func teamFromLink(s *goquery.Selection) (int, string) {
	href, ok := s.Attr("href")
	if !ok || !strings.HasPrefix(href, "/team/") {
		return 0, ""
	}
	id := idAt(href, 1)
	name := strings.TrimSpace(s.Text())
	if name == "" {
		name = strings.TrimSpace(s.Find("img").AttrOr("alt", ""))
	}
	if name == "" {
		name = teamNameFromHref(href)
	}
	return id, name
}

func collectTeamLinks(s *goquery.Selection, max int) []models.Team {
	seen := make(map[int]struct{})
	var teams []models.Team
	s.Find("a[href^='/team/']").Each(func(_ int, link *goquery.Selection) {
		if len(teams) >= max {
			return
		}
		id, name := teamFromLink(link)
		if id <= 0 {
			return
		}
		if _, dup := seen[id]; dup {
			return
		}
		seen[id] = struct{}{}
		teams = append(teams, models.Team{ID: id, Name: name})
	})
	return teams
}

func teamNamesFromMatchHref(href string) (string, string) {
	parts := strings.Split(strings.Trim(href, "/"), "/")
	if len(parts) < 3 || parts[0] != "matches" {
		return "", ""
	}
	slug := parts[2]
	const marker = "-vs-"
	idx := strings.Index(slug, marker)
	if idx <= 0 {
		return "", ""
	}
	t1 := strings.ReplaceAll(slug[:idx], "-", " ")
	rest := slug[idx+len(marker):]
	t2slug := rest
	lower := strings.ToLower(rest)
	for _, sep := range []string{"-iem-", "-blast-", "-esl-", "-major-", "-stage-", "-open-", "-cup-"} {
		if i := strings.Index(lower, sep); i > 0 {
			t2slug = rest[:i]
			break
		}
	}
	t2 := strings.ReplaceAll(strings.Trim(t2slug, "-"), "-", " ")
	return strings.TrimSpace(t1), strings.TrimSpace(t2)
}

func extractMatchID(href string) int {
	parts := strings.Split(strings.Trim(href, "/"), "/")
	for i, p := range parts {
		if p == "matches" && i+1 < len(parts) {
			id, _ := strconv.Atoi(parts[i+1])
			return id
		}
	}
	return 0
}

func firstText(doc *goquery.Document, selectors ...string) string {
	for _, sel := range selectors {
		t := strings.TrimSpace(doc.Find(sel).First().Text())
		if t != "" {
			return t
		}
	}
	return ""
}

func fmtErr(page string, err error) error {
	return fmt.Errorf("%s: %w", page, err)
}
