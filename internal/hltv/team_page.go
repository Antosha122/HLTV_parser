package hltv

import (
	"context"
	"fmt"
	"strings"

	"github.com/PuerkitoBio/goquery"

	"psr/internal/models"
)

func teamPagePath(teamID int, name string) string {
	slug := slugifyName(name)
	if slug == "" {
		return fmt.Sprintf("/team/%d/_", teamID)
	}
	return fmt.Sprintf("/team/%d/%s", teamID, slug)
}

func slugifyName(name string) string {
	slug := eventSlugRe.ReplaceAllString(strings.ToLower(strings.TrimSpace(name)), "-")
	return strings.Trim(slug, "-")
}

func (c *Client) GetTeam(ctx context.Context, teamID int) (models.TeamDetail, error) {
	return c.GetTeamByID(ctx, teamID, "")
}

// TeamPageData is loaded from a single /team/ request (profile + embedded match list).
type TeamPageData struct {
	Detail        models.TeamDetail
	Matches       []models.MatchSummary
	MapsStatsPath string // e.g. /stats/teams/maps/4608/bench
}

func (c *Client) GetTeamByID(ctx context.Context, teamID int, name string) (models.TeamDetail, error) {
	data, err := c.LoadTeamPage(ctx, teamID, name)
	if err != nil {
		return models.TeamDetail{}, err
	}
	return data.Detail, nil
}

func (c *Client) LoadTeamPage(ctx context.Context, teamID int, name string) (TeamPageData, error) {
	html, err := c.fetchTeamPageHTML(ctx, teamID, name)
	if err != nil {
		return TeamPageData{}, fmt.Errorf("fetch team %d: %w", teamID, err)
	}
	detail, err := ParseTeamPage(html, teamID)
	if err != nil {
		return TeamPageData{}, err
	}
	matches, _ := ParseTeamPageMatches(html)
	return TeamPageData{
		Detail:        detail,
		Matches:       matches,
		MapsStatsPath: ParseTeamMapsStatsPath(html, teamID),
	}, nil
}

// ParseTeamMapsStatsPath finds the overview maps stats link on a team page.
func ParseTeamMapsStatsPath(html string, teamID int) string {
	doc, err := parseDoc(html)
	if err != nil {
		return ""
	}
	prefix := fmt.Sprintf("/stats/teams/maps/%d/", teamID)
	var found string
	doc.Find("a[href]").Each(func(_ int, a *goquery.Selection) {
		href, ok := a.Attr("href")
		if !ok {
			return
		}
		href = strings.Split(href, "#")[0]
		if !strings.Contains(href, prefix) {
			return
		}
		if strings.Contains(href, "/stats/teams/map/") {
			return
		}
		found = href
	})
	return found
}

func (c *Client) GetTeamPageMatches(ctx context.Context, teamID int, name string) ([]models.MatchSummary, error) {
	data, err := c.LoadTeamPage(ctx, teamID, name)
	if err != nil {
		return nil, fmt.Errorf("team page matches: %w", err)
	}
	return data.Matches, nil
}

func (c *Client) fetchTeamPageHTML(ctx context.Context, teamID int, name string) (string, error) {
	paths := []string{}
	if name != "" {
		paths = append(paths, teamPagePath(teamID, name))
	}
	paths = append(paths, fmt.Sprintf("/team/%d/_", teamID))

	var lastErr error
	for _, path := range paths {
		html, err := c.Fetch(ctx, path)
		if err != nil {
			lastErr = err
			continue
		}
		if looksBlocked(html) || !looksLikeHLTV(html) {
			lastErr = fmt.Errorf("team page blocked")
			continue
		}
		detail, err := ParseTeamPage(html, teamID)
		if err != nil {
			lastErr = err
			continue
		}
		matches, _ := ParseTeamPageMatches(html)
		if detail.Name != "" || len(detail.Players) > 0 || len(matches) > 0 {
			return html, nil
		}
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("empty team page")
	}
	return "", lastErr
}

func ParseTeamPage(html string, teamID int) (models.TeamDetail, error) {
	doc, err := parseDoc(html)
	if err != nil {
		return models.TeamDetail{}, fmtErr("team page", err)
	}

	detail := models.TeamDetail{Team: models.Team{ID: teamID}}

	name := firstText(doc, ".profile-team-name", ".teamName", "h1.name")
	if name == "" {
		name = firstText(doc, "h1")
	}
	detail.Name = name

	doc.Find(".profile-team-stat, .team-stat").Each(func(_ int, s *goquery.Selection) {
		label := strings.ToLower(strings.TrimSpace(s.Find(".description, .stat-description").Text()))
		value := strings.TrimSpace(s.Find(".right, .stat-value, .value").Text())
		switch {
		case strings.Contains(label, "world rank"), strings.Contains(label, "ranking"):
			detail.WorldRank = parseInt(value)
		case strings.Contains(label, "rating"), strings.Contains(label, "points"):
			detail.HLTVRating = parseFloat(value)
		}
	})

	if detail.WorldRank == 0 {
		doc.Find(".ranking-header-rank, .world-rank").Each(func(_ int, s *goquery.Selection) {
			if detail.WorldRank == 0 {
				detail.WorldRank = parseInt(s.Text())
			}
		})
	}

	seenPlayers := make(map[int]struct{})
	addPlayer := func(id int, pname string) {
		if id == 0 || pname == "" {
			return
		}
		if _, ok := seenPlayers[id]; ok {
			return
		}
		seenPlayers[id] = struct{}{}
		detail.Players = append(detail.Players, models.Player{ID: id, Name: pname, TeamID: teamID})
	}

	doc.Find("a[href^='/player/']").Each(func(_ int, s *goquery.Selection) {
		href, _ := s.Attr("href")
		addPlayer(idAt(href, 1), strings.TrimSpace(s.Text()))
	})
	doc.Find(".bodyshot-team a[href*='/player/'], .players-table a[href*='/player/'], .lineup a[href*='/player/']").Each(func(_ int, s *goquery.Selection) {
		href, _ := s.Attr("href")
		addPlayer(idAt(href, 1), strings.TrimSpace(s.Text()))
	})

	return detail, nil
}

func ParseTeamPageMatches(html string) ([]models.MatchSummary, error) {
	doc, err := parseDoc(html)
	if err != nil {
		return nil, fmtErr("team matches", err)
	}

	selectors := []string{
		"#tab-matchesBox .result-con",
		"#matches .result-con",
		".results-holder .result-con",
		".past-matches-box .result-con",
		".teamPastMatches .result-con",
		".stats-matchlist .result-con",
		".match-history .result-con",
		".team-page .result-con",
	}
	for _, sel := range selectors {
		rows := doc.Find(sel)
		if rows.Length() == 0 {
			continue
		}
		var buf strings.Builder
		buf.WriteString("<html><body>")
		rows.Each(func(_ int, row *goquery.Selection) {
			inner, _ := row.Html()
			buf.WriteString(`<div class="result-con">`)
			buf.WriteString(inner)
			buf.WriteString("</div>")
		})
		buf.WriteString("</body></html>")
		if buf.Len() <= 26 {
			continue
		}
		matches, err := ParseResultsPage(buf.String())
		if err != nil {
			return nil, err
		}
		if len(matches) > 0 {
			return matches, nil
		}
	}

	// История матчей может быть в теле страницы, не только в #tab-matchesBox.
	if matches, err := ParseResultsPage(html); err == nil && len(matches) > 0 && len(matches) <= 120 {
		return matches, nil
	}

	// Last resort: parse any result rows on the team page (usually recent matches block).
	rows := doc.Find(".result-con")
	if rows.Length() > 0 && rows.Length() <= 80 {
		var buf strings.Builder
		buf.WriteString("<html><body>")
		rows.Each(func(_ int, row *goquery.Selection) {
			inner, _ := row.Html()
			buf.WriteString(`<div class="result-con">`)
			buf.WriteString(inner)
			buf.WriteString("</div>")
		})
		buf.WriteString("</body></html>")
		if matches, err := ParseResultsPage(buf.String()); err == nil && len(matches) > 0 {
			return matches, nil
		}
	}

	return nil, nil
}
