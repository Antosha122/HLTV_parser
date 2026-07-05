package hltv

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"

	"psr/internal/models"
)

func (c *Client) GetRankingTeams(ctx context.Context) ([]models.Team, error) {
	now := time.Now().UTC()
	month := strings.ToLower(now.Format("January"))
	paths := []string{
		fmt.Sprintf("/ranking/teams/%d/%s/%d", now.Year(), month, now.Day()),
		"/ranking/teams",
	}

	var lastErr error
	for _, path := range paths {
		html, err := c.Fetch(ctx, path)
		if err != nil {
			lastErr = err
			continue
		}
		teams, err := ParseRankingPage(html)
		if err != nil {
			lastErr = err
			continue
		}
		if len(teams) > 0 {
			return teams, nil
		}
	}
	if lastErr != nil {
		return nil, fmt.Errorf("fetch ranking: %w", lastErr)
	}
	return nil, nil
}

func ParseRankingPage(html string) ([]models.Team, error) {
	doc, err := parseDoc(html)
	if err != nil {
		return nil, fmtErr("ranking page", err)
	}

	seen := make(map[int]struct{})
	var teams []models.Team

	add := func(id int, name string, rank int, points float64) {
		if id <= 0 || name == "" {
			return
		}
		if _, dup := seen[id]; dup {
			return
		}
		seen[id] = struct{}{}
		teams = append(teams, models.Team{
			ID:         id,
			Name:       name,
			WorldRank:  rank,
			HLTVRating: points,
		})
	}

	doc.Find(".ranked-team").Each(func(_ int, row *goquery.Selection) {
		link := row.Find("a[href^='/team/']").First()
		if link.Length() == 0 {
			link = row.Closest("a[href^='/team/']")
		}
		href, _ := link.Attr("href")
		id := idAt(href, 1)
		name := strings.TrimSpace(row.Find(".name, .teamLine .name").First().Text())
		if name == "" {
			name = strings.TrimSpace(link.Text())
		}
		if name == "" {
			name = strings.TrimSpace(row.Find("img[alt]").First().AttrOr("alt", ""))
		}
		rank := parseInt(strings.TrimLeft(strings.TrimSpace(row.Find(".position, .ranking-header-rank").First().Text()), "#"))
		points := parseRankingPoints(row.Find(".points, .teamLine .points").First().Text())
		add(id, name, rank, points)
	})

	if len(teams) == 0 {
		doc.Find("a[href^='/team/']").Each(func(_ int, link *goquery.Selection) {
			href, _ := link.Attr("href")
			id := idAt(href, 1)
			block := link.Closest(".ranked-team, .ranking-header, .standard-box")
			name := strings.TrimSpace(block.Find(".name").First().Text())
			if name == "" {
				name = strings.TrimSpace(link.Find("img[alt]").First().AttrOr("alt", ""))
			}
			rank := parseInt(strings.TrimLeft(strings.TrimSpace(block.Find(".position").First().Text()), "#"))
			points := parseRankingPoints(block.Find(".points").First().Text())
			add(id, name, rank, points)
		})
	}

	if len(teams) == 0 {
		return nil, fmt.Errorf("no ranked teams found")
	}
	return teams, nil
}

func parseRankingPoints(raw string) float64 {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0
	}
	raw = strings.TrimPrefix(raw, "(")
	raw = strings.TrimSuffix(raw, ")")
	raw = strings.Fields(raw)[0]
	raw = strings.TrimSuffix(raw, ",")
	return parseFloat(raw)
}
