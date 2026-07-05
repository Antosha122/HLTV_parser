package hltv

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/PuerkitoBio/goquery"

	"psr/internal/models"
)

var mapScoreLineRe = regexp.MustCompile(`(\d{1,2})\s*[-–:]\s*(\d{1,2})`)

func (c *Client) GetMatch(ctx context.Context, matchID int) (models.MatchDetail, error) {
	html, err := c.Fetch(ctx, fmt.Sprintf("/matches/%d/_", matchID))
	if err != nil {
		return models.MatchDetail{}, fmt.Errorf("fetch match: %w", err)
	}
	return ParseMatchPage(html, matchID)
}

func ParseMatchPage(html string, matchID int) (models.MatchDetail, error) {
	doc, err := parseDoc(html)
	if err != nil {
		return models.MatchDetail{}, fmtErr("match page", err)
	}

	detail := models.MatchDetail{Match: models.Match{ID: matchID}}

	detail.Date = parseUnixAttr(doc.Find(".time[data-unix], .date[data-unix]").First())
	detail.EventName = strings.TrimSpace(doc.Find(".timeAndEvent .event a, .match-info-row .event a").First().Text())

	if href, ok := doc.Find(".timeAndEvent .event a, .match-info-row .event a").Attr("href"); ok {
		detail.EventID = idAt(href, 1)
	}

	formatText := firstText(doc, ".preformatted-text", ".match-info-row .padding")
	if formatText == "" {
		formatText = doc.Find(".match-meta").Text()
	}
	detail.Format = models.MatchFormat(parseFormat(formatText))

	detail.Team1, detail.Team2 = parseMatchTeams(doc)

	// Winner from won/lost class.
	doc.Find(".team").Each(func(_ int, s *goquery.Selection) {
		if !strings.Contains(s.AttrOr("class", ""), "won") {
			return
		}
		if id, _ := teamFromLink(s.Find("a[href^='/team/']").First()); id != 0 {
			detail.WinnerID = id
		}
	})

	detail.Vetoes = parseVetoes(doc, detail.Team1, detail.Team2)
	detail.Maps = parseMatchMaps(doc)

	// Stars from match page (importance).
	starsText := doc.Find(".match-page .stars i, .match-info .matchRating i").Length()
	if starsText > 0 {
		detail.Stars = starsText
	}

	detail.H2H = parseH2H(doc)

	return detail, nil
}

func parseMatchTeams(doc *goquery.Document) (models.Team, models.Team) {
	root := doc.Find(".match-page, .match-page-content, .standard-box.match-page").First()
	if root.Length() == 0 {
		root = doc.Find("body")
	}
	teams := collectTeamLinks(root, 4)
	if len(teams) >= 1 {
		if len(teams) >= 2 {
			return teams[0], teams[1]
		}
		return teams[0], models.Team{}
	}
	return models.Team{}, models.Team{}
}

func parseVetoes(doc *goquery.Document, team1, team2 models.Team) []models.Veto {
	var vetoes []models.Veto
	order := 1

	doc.Find(".standard-box.veto-box .padding, .veto-box .vetoinfo, .veto-box div.padding").Each(func(_ int, s *goquery.Selection) {
		text := strings.TrimSpace(s.Text())
		if text == "" {
			return
		}
		v := models.Veto{Order: order}
		lower := strings.ToLower(text)

		switch {
		case strings.Contains(lower, "left over"), strings.Contains(lower, "was left"):
			v.Action = models.VetoDecider
		case strings.Contains(lower, "removed"), strings.Contains(lower, "banned"), strings.Contains(lower, "ban"):
			v.Action = models.VetoBan
		case strings.Contains(lower, "picked"), strings.Contains(lower, "pick"), strings.Contains(lower, "selected"):
			v.Action = models.VetoPick
		default:
			return
		}

		v.TeamName, v.MapName = extractVetoParts(text)
		v.TeamID = resolveTeamID(v.TeamName, team1, team2)
		v.MapName = normalizeMap(v.MapName)
		if v.MapName == "" {
			return
		}

		vetoes = append(vetoes, v)
		order++
	})

	return vetoes
}

func extractVetoParts(text string) (team, mapName string) {
	lower := strings.ToLower(text)
	if strings.Contains(lower, "left over") {
		mapName = strings.TrimSpace(strings.Split(text, " was left")[0])
		mapName = strings.TrimSpace(strings.Split(mapName, " left")[0])
		return "", mapName
	}

	// Examples:
	// "Natus Vincere removed Nuke"
	// "1. FaZe picked Mirage"
	if len(strings.Fields(text)) < 3 {
		return "", ""
	}
	for _, verb := range []string{" removed ", " banned ", " picked ", " selected ", " was left over", " left over"} {
		if idx := strings.Index(lower, verb); idx > 0 {
			team = strings.TrimSpace(text[:idx])
			team = strings.TrimLeft(team, "0123456789. ")
			mapPart := strings.TrimSpace(text[idx+len(verb):])
			mapPart = strings.TrimSuffix(mapPart, ".")
			if strings.Contains(strings.ToLower(mapPart), "was left") {
				mapName = strings.Fields(mapPart)[0]
			} else {
				mapName = mapPart
			}
			return team, mapName
		}
	}
	return "", ""
}

func resolveTeamID(name string, team1, team2 models.Team) int {
	n := strings.ToLower(name)
	if strings.Contains(strings.ToLower(team1.Name), n) || strings.Contains(n, strings.ToLower(team1.Name)) {
		return team1.ID
	}
	if strings.Contains(strings.ToLower(team2.Name), n) || strings.Contains(n, strings.ToLower(team2.Name)) {
		return team2.ID
	}
	return 0
}

func parseMatchMaps(doc *goquery.Document) []models.MatchMap {
	var maps []models.MatchMap

	doc.Find(".mapholder, .mapholder-bg").Each(func(_ int, s *goquery.Selection) {
		name := strings.TrimSpace(s.Find(".mapname, .mapname-holder").Text())
		if name == "" {
			name = strings.TrimSpace(s.Find(".map-name").Text())
		}
		name = normalizeMap(name)
		if name == "" {
			return
		}

		mp := models.MatchMap{MapName: name}
		mp.Team1Score, mp.Team2Score = parseMapHolderScores(s)
		maps = append(maps, mp)
	})

	return maps
}

func parseMapHolderScores(s *goquery.Selection) (int, int) {
	selectors := []string{
		".results-team-score",
		".results-center-half-score",
		".team-row .score",
		".score",
	}
	for _, sel := range selectors {
		scores := s.Find(sel)
		if scores.Length() >= 2 {
			s1 := parseInt(scores.Eq(0).Text())
			s2 := parseInt(scores.Eq(1).Text())
			if s1 > 0 || s2 > 0 {
				return s1, s2
			}
		}
	}
	text := strings.ReplaceAll(s.Text(), "\n", " ")
	if m := mapScoreLineRe.FindStringSubmatch(text); len(m) == 3 {
		return parseInt(m[1]), parseInt(m[2])
	}
	return 0, 0
}

func parseH2H(doc *goquery.Document) []models.MatchSummary {
	var out []models.MatchSummary
	seen := make(map[int]struct{})

	doc.Find(".head-to-head-list a[href*='/matches/'], .h2h a[href*='/matches/']").Each(func(_ int, s *goquery.Selection) {
		href, _ := s.Attr("href")
		id := extractMatchID(href)
		if id == 0 {
			return
		}
		if _, dup := seen[id]; dup {
			return
		}
		seen[id] = struct{}{}
		out = append(out, models.MatchSummary{ID: id})
	})

	return out
}
