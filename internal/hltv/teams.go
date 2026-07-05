package hltv

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/PuerkitoBio/goquery"

	"psr/internal/models"
)

func (c *Client) SearchTeams(ctx context.Context, query string) ([]models.Team, error) {
	q := url.Values{"term": {query}}
	html, err := c.Fetch(ctx, "/search?"+q.Encode())
	if err != nil {
		return nil, fmt.Errorf("fetch search: %w", err)
	}
	return ParseTeamSearch(html)
}

func ParseTeamSearch(html string) ([]models.Team, error) {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		return nil, fmt.Errorf("parse html: %w", err)
	}

	seen := make(map[int]struct{})
	var teams []models.Team

	doc.Find("a").Each(func(_ int, s *goquery.Selection) {
		href, ok := s.Attr("href")
		if !ok || !strings.HasPrefix(href, "/team/") {
			return
		}
		parts := strings.Split(strings.Trim(href, "/"), "/")
		if len(parts) < 2 || parts[0] != "team" {
			return
		}
		id, err := strconv.Atoi(parts[1])
		if err != nil || id <= 0 {
			return
		}
		if _, dup := seen[id]; dup {
			return
		}

		name := strings.TrimSpace(s.Text())
		if name == "" {
			return
		}

		seen[id] = struct{}{}
		teams = append(teams, models.Team{ID: id, Name: name})
	})

	return teams, nil
}
