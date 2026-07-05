package hltv

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"

	"psr/internal/logx"
	"psr/internal/models"
)

func (c *Client) GetTeamResults(ctx context.Context, teamID int) ([]models.MatchSummary, error) {
	q := url.Values{"team": {strconv.Itoa(teamID)}}
	return c.fetchResultsPages(ctx, q)
}

func (c *Client) GetTeamResultsPlain(ctx context.Context, teamID int) ([]models.MatchSummary, error) {
	return c.GetTeamResults(ctx, teamID)
}

func (c *Client) fetchResultsPages(ctx context.Context, q url.Values) ([]models.MatchSummary, error) {
	var all []models.MatchSummary
	seen := make(map[int]struct{})

	for offset := 0; offset <= 500; offset += 100 {
		q.Set("offset", strconv.Itoa(offset))
		html, err := c.Fetch(ctx, "/results?"+q.Encode())
		if err != nil {
			if len(all) > 0 {
				return all, nil
			}
			return nil, fmt.Errorf("fetch results offset %d: %w", offset, err)
		}

		batch, err := ParseResultsPage(html)
		if err != nil {
			return all, err
		}
		if len(batch) == 0 {
			break
		}

		minDate := minDateFromQuery(q)

		added := 0
		for _, m := range batch {
			if _, dup := seen[m.ID]; dup {
				continue
			}
			if !minDate.IsZero() && !m.Date.IsZero() && m.Date.Before(minDate) {
				continue
			}
			seen[m.ID] = struct{}{}
			all = append(all, m)
			added++
		}
		if added == 0 {
			break
		}
		// Меньше 100 результатов — последняя страница.
		if len(batch) < 100 {
			break
		}
	}

	return all, nil
}

func minDateFromQuery(q url.Values) time.Time {
	raw := q.Get("startDate")
	if raw == "" {
		return time.Time{}
	}
	t, err := time.Parse("2006-01-02", raw)
	if err != nil {
		return time.Time{}
	}
	return t.UTC()
}

func mergeMatchSummaries(batches ...[]models.MatchSummary) []models.MatchSummary {
	seen := make(map[int]struct{})
	var out []models.MatchSummary
	for _, batch := range batches {
		for _, m := range batch {
			if m.ID <= 0 {
				continue
			}
			if _, dup := seen[m.ID]; dup {
				continue
			}
			seen[m.ID] = struct{}{}
			out = append(out, m)
		}
	}
	return out
}

// GetTeamResultsWithFallback loads all played matches from /results?team=ID.
func (c *Client) GetTeamResultsWithFallback(ctx context.Context, teamID int, teamName string) ([]models.MatchSummary, error) {
	logx.Info("hltv", "загрузка матчей: /results?team=%d", teamID)

	results, err := c.GetTeamResults(ctx, teamID)
	if err == nil && len(results) > 0 {
		logx.Info("hltv", "команда %d: %d матчей из /results?team=", teamID, len(results))
		return results, nil
	}
	if err != nil {
		logx.Warn("hltv", "results?team=%d: %v", teamID, err)
	}

	pageMatches, err3 := c.GetTeamPageMatches(ctx, teamID, teamName)
	if len(pageMatches) > 0 {
		logx.Info("hltv", "команда %d: %d матчей со страницы команды (fallback)", teamID, len(pageMatches))
		return pageMatches, nil
	}
	if err != nil {
		return nil, err
	}
	return nil, err3
}

func ParseResultsPage(html string) ([]models.MatchSummary, error) {
	doc, err := parseDoc(html)
	if err != nil {
		return nil, fmtErr("results page", err)
	}

	var matches []models.MatchSummary
	seen := make(map[int]struct{})

	parseRow := func(row *goquery.Selection) {
		link := row.Find("a.a-reset[href*='/matches/']").First()
		if link.Length() == 0 {
			link = row.Find("a[href*='/matches/']").First()
		}
		href, ok := link.Attr("href")
		if !ok {
			return
		}
		id := extractMatchID(href)
		if id == 0 {
			return
		}
		if _, dup := seen[id]; dup {
			return
		}
		seen[id] = struct{}{}

		dateScope := row.Closest(".result-con")
		if dateScope.Length() == 0 {
			dateScope = row
		}

		parseScope := row
		if link.Length() > 0 && link.Find("table").Length() > 0 {
			parseScope = link
		} else if row.Is("a") && row.Find("table").Length() > 0 {
			parseScope = row
		} else if row.Find("table").Length() > 0 {
			parseScope = row
		}

		summary := models.MatchSummary{
			ID:   id,
			Date: parseUnixFromRow(dateScope),
		}

		teamList := parseResultTeams(parseScope, href)
		if len(teamList) >= 1 {
			summary.Team1 = teamList[0]
		}
		if len(teamList) >= 2 {
			summary.Team2 = teamList[1]
		}
		summary.Score = parseResultScore(parseScope)
		summary.WinnerName, summary.WinnerID = parseResultWinner(parseScope, summary.Team1, summary.Team2)
		summary.Event = parseResultEvent(parseScope)
		summary.Format = parseResultFormat(parseScope)

		matches = append(matches, summary)
	}

	doc.Find(".result-con").Each(func(_ int, row *goquery.Selection) {
		parseRow(row)
	})

	doc.Find("a.a-reset[href*='/matches/']").Each(func(_ int, s *goquery.Selection) {
		href, _ := s.Attr("href")
		id := extractMatchID(href)
		if id == 0 {
			return
		}
		if _, dup := seen[id]; dup {
			return
		}
		row := s.Closest(".result-con")
		if row.Length() == 0 {
			row = s
		}
		parseRow(row)
	})

	// Older/alternate layout.
	if len(matches) == 0 {
		doc.Find(".results-sublist .a-reset").Each(func(_ int, row *goquery.Selection) {
			parseRow(row)
		})
	}

	if len(matches) == 0 {
		doc.Find("a[href*='/matches/']").Each(func(_ int, s *goquery.Selection) {
			href, _ := s.Attr("href")
			id := extractMatchID(href)
			if id == 0 {
				return
			}
			if _, dup := seen[id]; dup {
				return
			}
			seen[id] = struct{}{}
			row := s.Closest(".result-con")
			if row.Length() == 0 {
				row = s.Parent()
			}
			summary := models.MatchSummary{ID: id}
			teamList := parseResultTeams(row, href)
			if len(teamList) >= 1 {
				summary.Team1 = teamList[0]
			}
			if len(teamList) >= 2 {
				summary.Team2 = teamList[1]
			}
			matches = append(matches, summary)
		})
	}

	return matches, nil
}

func parseTeamFromResultCell(cell *goquery.Selection) models.Team {
	var t models.Team
	if link := cell.Find("a[href^='/team/']").First(); link.Length() > 0 {
		href, _ := link.Attr("href")
		t.ID = idAt(href, 1)
		t.Name = strings.TrimSpace(link.Text())
	}
	if t.Name == "" {
		t.Name = strings.TrimSpace(cell.Find(".team").First().Text())
	}
	if t.Name == "" {
		t.Name = strings.TrimSpace(cell.Find("img[alt]").First().AttrOr("alt", ""))
	}
	return t
}

func parseResultTeams(row *goquery.Selection, matchHref string) []models.Team {
	cells := row.Find("table tr td.team-cell")
	if cells.Length() >= 2 {
		return []models.Team{
			parseTeamFromResultCell(cells.First()),
			parseTeamFromResultCell(cells.Eq(1)),
		}
	}

	teamList := collectTeamLinks(row, 2)
	if len(teamList) >= 2 {
		return teamList
	}

	var names []string
	row.Find("td.team-cell .team").Each(func(_ int, s *goquery.Selection) {
		n := strings.TrimSpace(s.Text())
		if n != "" {
			names = append(names, n)
		}
	})
	if len(names) < 2 {
		row.Find(".team").Each(func(_ int, s *goquery.Selection) {
			n := strings.TrimSpace(s.Text())
			if n != "" {
				names = append(names, n)
			}
		})
	}
	if len(names) < 2 {
		row.Find(".team-cell img[alt]").Each(func(_ int, s *goquery.Selection) {
			n := strings.TrimSpace(s.AttrOr("alt", ""))
			if n != "" {
				names = append(names, n)
			}
		})
	}
	if len(names) < 2 {
		n1, n2 := teamNamesFromMatchHref(matchHref)
		if n1 != "" {
			names = append(names, n1)
		}
		if n2 != "" {
			names = append(names, n2)
		}
	}

	seen := make(map[string]struct{})
	var teams []models.Team
	for _, n := range names {
		key := strings.ToLower(n)
		if _, dup := seen[key]; dup {
			continue
		}
		seen[key] = struct{}{}
		teams = append(teams, models.Team{Name: n})
		if len(teams) >= 2 {
			break
		}
	}

	if len(teams) >= 2 {
		return teams
	}
	if len(teamList) > 0 {
		return append(teamList, teams...)
	}
	return teams
}

func parseResultWinner(row *goquery.Selection, team1, team2 models.Team) (winnerName string, winnerID int) {
	cells := row.Find("table tr td.team-cell")
	if cells.Length() >= 2 {
		if cells.First().Find(".team-won").Length() > 0 {
			winnerName = team1.Name
			winnerID = team1.ID
			return winnerName, winnerID
		}
		if cells.Eq(1).Find(".team-won").Length() > 0 {
			winnerName = team2.Name
			winnerID = team2.ID
			return winnerName, winnerID
		}
	}

	wonName := strings.TrimSpace(row.Find(".team-cell .team.team-won").First().Text())
	if wonName == "" {
		wonName = strings.TrimSpace(row.Find(".team-won").First().Text())
	}
	if wonName != "" {
		winnerName = wonName
		if team1.ID > 0 && strings.EqualFold(team1.Name, wonName) {
			return winnerName, team1.ID
		}
		if team2.ID > 0 && strings.EqualFold(team2.Name, wonName) {
			return winnerName, team2.ID
		}
		return winnerName, 0
	}

	winnerID = winnerFromScore(parseResultScore(row), team1, team2)
	if winnerID == team1.ID {
		winnerName = team1.Name
	} else if winnerID == team2.ID {
		winnerName = team2.Name
	}
	return winnerName, winnerID
}

// parseResultScore returns team1 - team2 score in table order (not score-won first).
func parseResultScore(row *goquery.Selection) string {
	cell := row.Find("table tr td.result-score").First()
	if cell.Length() == 0 {
		cell = row.Find("td.result-score").First()
	}
	if cell.Length() == 0 {
		cell = row.Find(".result-score").First()
	}
	var parts []string
	cell.Children().Each(func(_ int, s *goquery.Selection) {
		t := strings.TrimSpace(s.Text())
		if t != "" {
			parts = append(parts, t)
		}
	})
	if len(parts) >= 2 {
		return parts[0] + " - " + parts[1]
	}
	text := strings.TrimSpace(cell.Text())
	text = strings.ReplaceAll(text, "\u00a0", " ")
	return text
}

func parseResultEvent(row *goquery.Selection) string {
	if e := strings.TrimSpace(row.Find("td.event .event-name").First().Text()); e != "" {
		return e
	}
	if e := strings.TrimSpace(row.Find(".event-name").First().Text()); e != "" {
		return e
	}
	if e := strings.TrimSpace(row.Find("td.event img[alt]").First().AttrOr("alt", "")); e != "" {
		return e
	}
	return strings.TrimSpace(row.Find(".event a").First().Text())
}

func parseResultFormat(row *goquery.Selection) models.MatchFormat {
	mapText := strings.ToLower(strings.TrimSpace(row.Find(".map-text").First().Text()))
	switch mapText {
	case "bo1", "bo3", "bo5":
		return models.MatchFormat(mapText)
	}
	if f := models.MatchFormat(parseFormat(row.Text())); f != "" {
		return f
	}
	return ""
}

func winnerFromScore(score string, team1, team2 models.Team) int {
	score = strings.ReplaceAll(score, "\u00a0", " ")
	parts := strings.FieldsFunc(score, func(r rune) bool {
		return r == '-' || r == ':' || r == ' '
	})
	if len(parts) < 2 {
		return 0
	}
	s1 := parseInt(parts[0])
	s2 := parseInt(parts[1])
	if s1 == s2 {
		return 0
	}
	if s1 > s2 {
		return team1.ID
	}
	return team2.ID
}
