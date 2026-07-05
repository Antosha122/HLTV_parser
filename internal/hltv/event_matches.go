package hltv

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/PuerkitoBio/goquery"

	"psr/internal/models"
)

func eventMatchesPaths(event models.Event) []string {
	slugPath := eventPagePath(event) + "/matches"
	return []string{
		fmt.Sprintf("/events/%d/matches", event.ID),
		slugPath,
	}
}

func (c *Client) GetEventMatches(ctx context.Context, event models.Event) ([]models.MatchSummary, error) {
	var lastErr error
	for _, path := range eventMatchesPaths(event) {
		html, err := c.Fetch(ctx, path)
		if err != nil {
			lastErr = err
			continue
		}
		matches, err := ParseEventMatchesPage(html, event)
		if err != nil {
			lastErr = err
			continue
		}
		if len(matches) > 0 {
			return matches, nil
		}
	}
	if lastErr != nil {
		return nil, fmt.Errorf("fetch event matches: %w", lastErr)
	}
	return nil, nil
}

func ParseEventMatchesPage(html string, event models.Event) ([]models.MatchSummary, error) {
	doc, err := parseDoc(html)
	if err != nil {
		return nil, fmtErr("event matches", err)
	}

	var matches []models.MatchSummary
	seen := make(map[int]struct{})

	parseWrapper := func(wrapper *goquery.Selection) {
		matchID := attrInt(wrapper, "data-match-id")
		if matchID == 0 {
			href, _ := wrapper.Find("a[href*='/matches/']").First().Attr("href")
			matchID = extractMatchID(href)
		}
		if matchID == 0 {
			return
		}
		if _, dup := seen[matchID]; dup {
			return
		}
		seen[matchID] = struct{}{}

		t1ID := attrInt(wrapper, "team1")
		if t1ID == 0 {
			t1ID = attrInt(wrapper, "data-team1")
		}
		t2ID := attrInt(wrapper, "team2")
		if t2ID == 0 {
			t2ID = attrInt(wrapper, "data-team2")
		}

		teamNames := wrapper.Find(".match-teamname")
		t1Name := strings.TrimSpace(teamNames.Eq(0).Text())
		t2Name := strings.TrimSpace(teamNames.Eq(1).Text())
		if t1Name == "" {
			t1Name = strings.TrimSpace(wrapper.Find(".match-team.team1 img[alt]").First().AttrOr("alt", ""))
		}
		if t2Name == "" {
			t2Name = strings.TrimSpace(wrapper.Find(".match-team.team2 img[alt]").First().AttrOr("alt", ""))
		}

		summary := models.MatchSummary{
			ID:     matchID,
			Team1:  models.Team{ID: t1ID, Name: t1Name},
			Team2:  models.Team{ID: t2ID, Name: t2Name},
			Event:  event.Name,
			Format: models.MatchFormat(parseFormat(wrapper.Find(".match-meta").First().Text())),
			Date:   parseUnixAttr(wrapper.Find(".match-time, [data-unix]").First()),
		}
		if summary.Format == "" {
			summary.Format = models.MatchFormat(parseFormat(wrapper.Text()))
		}
		matches = append(matches, summary)
	}

	doc.Find(".match-wrapper").Each(func(_ int, s *goquery.Selection) {
		parseWrapper(s)
	})

	if len(matches) == 0 {
		doc.Find(".match").Each(func(_ int, s *goquery.Selection) {
			if s.Find("a[href*='/matches/']").Length() == 0 {
				return
			}
			parseWrapper(s)
		})
	}

	return matches, nil
}

func attrInt(s *goquery.Selection, name string) int {
	raw, ok := s.Attr(name)
	if !ok || raw == "" {
		return 0
	}
	n, _ := strconv.Atoi(strings.TrimSpace(raw))
	return n
}
