package storage

import (
	"time"

	"psr/internal/models"
)

// ListTeamMatchesNeedingMaps returns played matches without per-map rows in the period.
func (db *DB) ListTeamMatchesNeedingMaps(teamID int, since time.Time) ([]int, error) {
	sinceStr := since.UTC().Format(time.RFC3339)
	rows, err := db.sql.Query(`
		SELECT m.id
		FROM matches m
		WHERE (m.team1_id = ? OR m.team2_id = ?)
		  AND m.winner_id IS NOT NULL
		  AND m.match_date >= ?
		  AND NOT EXISTS (SELECT 1 FROM match_maps mm WHERE mm.match_id = m.id)
		ORDER BY m.match_date DESC`, teamID, teamID, sinceStr,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var ids []int
	for rows.Next() {
		var id int
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// AggregateTeamMapStats builds W/L and pick/ban rates from synced match pages.
func (db *DB) AggregateTeamMapStats(teamID int, start, end time.Time) ([]models.TeamMapStat, error) {
	startStr := start.UTC().Format(time.RFC3339)
	endStr := end.UTC().Format(time.RFC3339)

	type mapWL struct {
		wins, losses int
	}
	wl := make(map[string]*mapWL)

	rows, err := db.sql.Query(`
		SELECT m.team1_id, m.team2_id, mm.map_name, mm.team1_score, mm.team2_score
		FROM matches m
		JOIN match_maps mm ON mm.match_id = m.id
		WHERE (m.team1_id = ? OR m.team2_id = ?)
		  AND m.match_date >= ? AND m.match_date <= ?`,
		teamID, teamID, startStr, endStr,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var team1ID, team2ID, s1, s2 int
		var mapName string
		if err := rows.Scan(&team1ID, &team2ID, &mapName, &s1, &s2); err != nil {
			return nil, err
		}
		if mapName == "" || s1 == s2 {
			continue
		}
		won := false
		switch teamID {
		case team1ID:
			won = s1 > s2
		case team2ID:
			won = s2 > s1
		default:
			continue
		}
		entry := wl[mapName]
		if entry == nil {
			entry = &mapWL{}
			wl[mapName] = entry
		}
		if won {
			entry.wins++
		} else {
			entry.losses++
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	pickCount := make(map[string]int)
	banCount := make(map[string]int)
	vetoTotal := 0

	vrows, err := db.sql.Query(`
		SELECT v.map_name, v.action, COUNT(1)
		FROM vetoes v
		JOIN matches m ON m.id = v.match_id
		WHERE v.team_id = ?
		  AND m.match_date >= ? AND m.match_date <= ?
		  AND v.action IN ('pick', 'ban')
		GROUP BY v.map_name, v.action`,
		teamID, startStr, endStr,
	)
	if err != nil {
		return nil, err
	}
	defer vrows.Close()

	for vrows.Next() {
		var mapName, action string
		var count int
		if err := vrows.Scan(&mapName, &action, &count); err != nil {
			return nil, err
		}
		if mapName == "" || count <= 0 {
			continue
		}
		vetoTotal += count
		switch models.VetoAction(action) {
		case models.VetoPick:
			pickCount[mapName] += count
		case models.VetoBan:
			banCount[mapName] += count
		}
	}
	if err := vrows.Err(); err != nil {
		return nil, err
	}

	stats := make([]models.TeamMapStat, 0, len(wl))
	for mapName, entry := range wl {
		if entry.wins == 0 && entry.losses == 0 {
			continue
		}
		s := models.TeamMapStat{
			TeamID:  teamID,
			MapName: mapName,
			Wins:    entry.wins,
			Losses:  entry.losses,
		}
		if vetoTotal > 0 {
			s.PickRate = float64(pickCount[mapName]) / float64(vetoTotal)
			s.BanRate = float64(banCount[mapName]) / float64(vetoTotal)
		}
		stats = append(stats, s)
	}

	return stats, nil
}
