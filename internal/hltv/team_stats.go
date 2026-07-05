package hltv

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"

	"psr/internal/models"
)

func teamMapStatsPath(teamID int, name string, start, end time.Time) string {
	slug := slugifyName(name)
	if slug == "" {
		slug = "_"
	}
	return fmt.Sprintf("/stats/teams/maps/%d/%s?csVersion=CS2&startDate=%s&endDate=%s",
		teamID, slug, start.Format("2006-01-02"), end.Format("2006-01-02"))
}

func applyMapStatsPeriod(path string, start, end time.Time) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	rel, err := url.Parse(path)
	if err != nil {
		return path
	}
	q := rel.Query()
	q.Set("csVersion", "CS2")
	q.Set("startDate", start.Format("2006-01-02"))
	q.Set("endDate", end.Format("2006-01-02"))
	rel.RawQuery = q.Encode()
	if rel.Path == "" {
		return path
	}
	if rel.RawQuery != "" {
		return rel.Path + "?" + rel.RawQuery
	}
	return rel.Path
}

func (c *Client) GetTeamMapStats(ctx context.Context, teamID int, teamName string, start, end time.Time, mapsPagePath string) ([]models.TeamMapStat, error) {
	paths := []string{}
	if mapsPagePath != "" {
		paths = append(paths, applyMapStatsPeriod(mapsPagePath, start, end))
	}
	if teamName != "" {
		paths = append(paths, teamMapStatsPath(teamID, teamName, start, end))
	}
	paths = append(paths, teamMapStatsPath(teamID, "", start, end))

	var lastErr error
	for _, path := range paths {
		html, err := c.Fetch(ctx, path)
		if err != nil {
			lastErr = err
			continue
		}
		stats, err := ParseTeamMapStats(html, teamID)
		if err != nil {
			lastErr = err
			continue
		}
		if len(stats) > 0 {
			return stats, nil
		}
	}
	if lastErr != nil {
		return nil, fmt.Errorf("fetch team map stats: %w", lastErr)
	}
	return nil, nil
}

func ParseTeamMapStats(html string, teamID int) ([]models.TeamMapStat, error) {
	doc, err := parseDoc(html)
	if err != nil {
		return nil, fmtErr("team map stats", err)
	}

	stats := parseMapPoolLayout(doc, teamID)
	if len(stats) > 0 {
		return stats, nil
	}
	return parseMapStatsTable(doc, teamID)
}

func parseMapPoolLayout(doc *goquery.Document, teamID int) []models.TeamMapStat {
	var stats []models.TeamMapStat
	seen := make(map[string]struct{})

	doc.Find(".col").Each(func(_ int, col *goquery.Selection) {
		mapName := strings.TrimSpace(col.Find(".map-pool-map-name").First().Text())
		if mapName == "" {
			mapName = strings.TrimSpace(col.Find(".map-pool img[alt]").First().AttrOr("alt", ""))
		}
		mapName = normalizeMap(mapName)
		if mapName == "" {
			return
		}
		key := strings.ToLower(mapName)
		if _, dup := seen[key]; dup {
			return
		}

		var wins, losses int
		var pickRate, banRate float64
		col.Find(".stats-row").Each(func(_ int, row *goquery.Selection) {
			label := strings.ToLower(strings.TrimSpace(row.Find("span.strong").First().Text()))
			value := statsRowValue(row)
			switch {
			case strings.Contains(label, "wins") && strings.Contains(label, "loss"):
				wins, losses = parseWDL(value)
			case strings.Contains(label, "pick"):
				pickRate = parsePercent(value)
			case strings.Contains(label, "ban"):
				banRate = parsePercent(value)
			}
		})

		if wins == 0 && losses == 0 {
			return
		}
		seen[key] = struct{}{}
		stats = append(stats, models.TeamMapStat{
			TeamID:   teamID,
			MapName:  mapName,
			Wins:     wins,
			Losses:   losses,
			PickRate: pickRate,
			BanRate:  banRate,
		})
	})

	return stats
}

func statsRowValue(row *goquery.Selection) string {
	var val string
	row.Find("span").Each(func(_ int, s *goquery.Selection) {
		if s.HasClass("strong") {
			return
		}
		if t := strings.TrimSpace(s.Text()); t != "" {
			val = t
		}
	})
	return val
}

func parseWDL(raw string) (wins, losses int) {
	raw = strings.ReplaceAll(raw, "\u00a0", " ")
	parts := strings.Split(raw, "/")
	if len(parts) < 3 {
		return 0, 0
	}
	return parseInt(strings.TrimSpace(parts[0])), parseInt(strings.TrimSpace(parts[2]))
}

func parsePercent(raw string) float64 {
	raw = strings.TrimSpace(strings.TrimSuffix(raw, "%"))
	if raw == "" {
		return 0
	}
	v := parseFloat(raw)
	if v > 1 {
		return v / 100
	}
	return v
}

func parseMapStatsTable(doc *goquery.Document, teamID int) ([]models.TeamMapStat, error) {
	var stats []models.TeamMapStat

	doc.Find("table.stats-table tbody tr").Each(func(_ int, s *goquery.Selection) {
		cells := s.Find("td")
		if cells.Length() < 3 {
			return
		}

		mapName := normalizeMap(strings.TrimSpace(cells.First().Text()))
		if mapName == "" {
			return
		}

		wins := parseInt(cells.Eq(1).Text())
		losses := parseInt(cells.Eq(2).Text())

		if wins == 0 && losses == 0 {
			wl := strings.TrimSpace(cells.Eq(1).Text())
			if parts := strings.Split(wl, "/"); len(parts) >= 2 {
				wins = parseInt(parts[0])
				losses = parseInt(parts[1])
			}
		}

		if wins == 0 && losses == 0 {
			return
		}

		stats = append(stats, models.TeamMapStat{
			TeamID:  teamID,
			MapName: mapName,
			Wins:    wins,
			Losses:  losses,
		})
	})

	return stats, nil
}
