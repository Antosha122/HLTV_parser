package hltv

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/PuerkitoBio/goquery"

	"psr/internal/models"
)

var eventSlugRe = regexp.MustCompile(`[^a-z0-9]+`)

func eventPagePath(e models.Event) string {
	slug := eventSlugRe.ReplaceAllString(strings.ToLower(strings.TrimSpace(e.Name)), "-")
	slug = strings.Trim(slug, "-")
	if slug == "" {
		slug = "event"
	}
	return fmt.Sprintf("/events/%d/%s", e.ID, slug)
}

func (c *Client) GetEventPage(ctx context.Context, event models.Event) ([]models.Team, []models.MatchSummary, error) {
	html, err := c.Fetch(ctx, eventPagePath(event))
	if err != nil {
		return nil, nil, fmt.Errorf("fetch event page: %w", err)
	}
	return ParseEventPage(html)
}

func ParseEventPage(html string) ([]models.Team, []models.MatchSummary, error) {
	matches, err := ParseResultsPage(html)
	if err != nil {
		return nil, nil, err
	}

	teamSeen := make(map[int]models.Team)
	addTeam := func(t models.Team) {
		if t.ID <= 0 {
			return // только команды с HLTV id — без /search?term=
		}
		if cur, ok := teamSeen[t.ID]; !ok || cur.Name == "" {
			teamSeen[t.ID] = t
		}
	}

	for _, m := range matches {
		addTeam(m.Team1)
		addTeam(m.Team2)
	}
	for _, t := range parseEventParticipantTeams(html) {
		addTeam(t)
	}

	teams := make([]models.Team, 0, len(teamSeen))
	for _, t := range teamSeen {
		teams = append(teams, t)
	}

	return teams, matches, nil
}

func (c *Client) GetEventParticipants(ctx context.Context, event models.Event) ([]models.Team, error) {
	html, err := c.Fetch(ctx, eventPagePath(event))
	if err != nil {
		return nil, fmt.Errorf("fetch event page: %w", err)
	}
	return parseEventParticipantTeams(html), nil
}

func parseEventParticipantTeams(html string) []models.Team {
	doc, err := parseDoc(html)
	if err != nil {
		return nil
	}

	seen := make(map[int]struct{})
	var teams []models.Team
	addLink := func(s *goquery.Selection) {
		href, _ := s.Attr("href")
		id := idAt(href, 1)
		if id <= 0 {
			return
		}
		if _, dup := seen[id]; dup {
			return
		}
		seen[id] = struct{}{}
		name := strings.TrimSpace(s.Text())
		if name == "" {
			name = strings.TrimSpace(s.AttrOr("title", ""))
		}
		if name == "" {
			href, _ := s.Attr("href")
			name = teamNameFromHref(href)
		}
		if name == "" {
			return
		}
		teams = append(teams, models.Team{ID: id, Name: name})
	}

	// Участники турнира — не все /team/ ссылки на странице (навигация, матчи и т.д.).
	selectors := []string{
		".team-row a[href^='/team/']",
		".teamsbox a[href^='/team/']",
		".standings a[href^='/team/']",
		".placement a[href^='/team/']",
		".teamName a[href^='/team/']",
		".teamPoolPlaceHolder a[href^='/team/']",
		".event-participant a[href^='/team/']",
	}
	for _, sel := range selectors {
		doc.Find(sel).Each(func(_ int, s *goquery.Selection) {
			addLink(s)
		})
	}
	if len(teams) > 0 {
		return teams
	}

	doc.Find("a[href^='/team/']").Each(func(_ int, s *goquery.Selection) {
		addLink(s)
	})
	return teams
}
