package hltv

import (
	"context"
	"fmt"
	"net/url"
	"strconv"

	"psr/internal/models"
)

func (c *Client) GetEventResults(ctx context.Context, eventID int) ([]models.MatchSummary, []models.Team, error) {
	q := url.Values{"event": {strconv.Itoa(eventID)}}

	var all []models.MatchSummary
	seen := make(map[int]struct{})

	maxOffset := 300
	if c.session != nil && c.session.IsUserChrome() && !c.session.prefersHTTP() {
		maxOffset = 0
	}
	for offset := 0; offset <= maxOffset; offset += 100 {
		q.Set("offset", strconv.Itoa(offset))
		html, err := c.Fetch(ctx, "/results?"+q.Encode())
		if err != nil {
			return nil, nil, fmt.Errorf("fetch event results: %w", err)
		}

		batch, err := ParseResultsPage(html)
		if err != nil {
			return nil, nil, err
		}
		if len(batch) == 0 {
			break
		}

		added := 0
		for _, m := range batch {
			if _, dup := seen[m.ID]; dup {
				continue
			}
			seen[m.ID] = struct{}{}
			all = append(all, m)
			added++
		}
		if added == 0 || len(batch) < 50 {
			break
		}
	}

	teamSeen := make(map[int]models.Team)
	for _, m := range all {
		if m.Team1.ID > 0 && m.Team1.Name != "" {
			teamSeen[m.Team1.ID] = m.Team1
		}
		if m.Team2.ID > 0 && m.Team2.Name != "" {
			teamSeen[m.Team2.ID] = m.Team2
		}
	}

	teams := make([]models.Team, 0, len(teamSeen))
	for _, t := range teamSeen {
		teams = append(teams, t)
	}

	return all, teams, nil
}
