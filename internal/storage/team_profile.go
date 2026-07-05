package storage

import (
	"database/sql"
	"time"

	"psr/internal/models"
)

type TeamEventStat struct {
	EventID   int    `json:"event_id"`
	EventName string `json:"event_name"`
	Matches   int    `json:"matches"`
	Wins      int    `json:"wins"`
}

type TeamMatchRow struct {
	ID        int                `json:"id"`
	Opponent  string             `json:"opponent"`
	Score     string             `json:"score,omitempty"`
	EventName string             `json:"event_name,omitempty"`
	Format    models.MatchFormat `json:"format,omitempty"`
	Date      string             `json:"date,omitempty"`
	Won       bool               `json:"won"`
	Played    bool               `json:"played"`
}

type TeamProfile struct {
	Team          models.Team       `json:"team"`
	Players       []models.Player   `json:"players"`
	MatchesTotal  int               `json:"matches_total"`
	Wins          int               `json:"wins"`
	Losses        int               `json:"losses"`
	WinRate       float64           `json:"win_rate"`
	RecentWins    int               `json:"recent_wins"`
	RecentTotal   int               `json:"recent_total"`
	RecentWinRate float64           `json:"recent_win_rate"`
	MapStats      []MapStat         `json:"map_stats"`
	Events        []TeamEventStat   `json:"events"`
	RecentMatches []TeamMatchRow    `json:"recent_matches"`
}

type MapStat struct {
	MapName  string  `json:"map_name"`
	Wins     int     `json:"wins"`
	Losses   int     `json:"losses"`
	WinRate  float64 `json:"win_rate"`
	PickRate float64 `json:"pick_rate,omitempty"`
	BanRate  float64 `json:"ban_rate,omitempty"`
}

func (db *DB) GetTeamProfile(teamID int) (TeamProfile, error) {
	var out TeamProfile
	var rating sql.NullFloat64
	var rank sql.NullInt64
	err := db.sql.QueryRow(`
		SELECT id, name, hltv_rating, world_rank FROM teams WHERE id = ?`, teamID,
	).Scan(&out.Team.ID, &out.Team.Name, &rating, &rank)
	if err != nil {
		return out, err
	}
	if rating.Valid {
		out.Team.HLTVRating = rating.Float64
	}
	if rank.Valid {
		out.Team.WorldRank = int(rank.Int64)
	}

	rows, err := db.sql.Query(`SELECT id, name, team_id FROM players WHERE team_id = ? ORDER BY name`, teamID)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		var p models.Player
		if err := rows.Scan(&p.ID, &p.Name, &p.TeamID); err != nil {
			return out, err
		}
		out.Players = append(out.Players, p)
	}

	var winsNull sql.NullInt64
	_ = db.sql.QueryRow(`
		SELECT
			COUNT(1),
			COALESCE(SUM(CASE WHEN winner_id = ? THEN 1 ELSE 0 END), 0)
		FROM matches
		WHERE (team1_id = ? OR team2_id = ?) AND winner_id IS NOT NULL AND match_date >= ?`,
		teamID, teamID, teamID, minMatchDateRFC(),
	).Scan(&out.MatchesTotal, &winsNull)
	if winsNull.Valid {
		out.Wins = int(winsNull.Int64)
	}
	out.Losses = out.MatchesTotal - out.Wins
	if out.MatchesTotal > 0 {
		out.WinRate = float64(out.Wins) / float64(out.MatchesTotal) * 100
	}

	out.RecentWins, out.RecentTotal, _ = db.GetTeamForm(teamID, 10)
	if out.RecentTotal > 0 {
		out.RecentWinRate = float64(out.RecentWins) / float64(out.RecentTotal) * 100
	}

	rawMapStats, _ := db.GetTeamMapStatsForTeam(teamID)
	for _, s := range rawMapStats {
		total := s.Wins + s.Losses
		ms := MapStat{
			MapName:  s.MapName,
			Wins:     s.Wins,
			Losses:   s.Losses,
			PickRate: s.PickRate,
			BanRate:  s.BanRate,
		}
		if total > 0 {
			ms.WinRate = float64(s.Wins) / float64(total) * 100
		}
		out.MapStats = append(out.MapStats, ms)
	}

	eventRows, err := db.sql.Query(`
		SELECT COALESCE(event_id, 0), COALESCE(event_name, ''), COUNT(1),
			SUM(CASE WHEN winner_id = ? THEN 1 ELSE 0 END)
		FROM matches
		WHERE (team1_id = ? OR team2_id = ?) AND winner_id IS NOT NULL AND match_date >= ?
		GROUP BY COALESCE(event_id, 0), COALESCE(event_name, '')
		ORDER BY COUNT(1) DESC
		LIMIT 20`, teamID, teamID, teamID, minMatchDateRFC())
	if err == nil {
		defer eventRows.Close()
		for eventRows.Next() {
			var es TeamEventStat
			if err := eventRows.Scan(&es.EventID, &es.EventName, &es.Matches, &es.Wins); err != nil {
				break
			}
			out.Events = append(out.Events, es)
		}
	}

	matchRows, err := db.sql.Query(`
		SELECT m.id, m.team1_id, COALESCE(t1.name, ''), m.team2_id, COALESCE(t2.name, ''),
			m.winner_id, m.event_name, m.format, m.match_date
		FROM matches m
		LEFT JOIN teams t1 ON t1.id = m.team1_id
		LEFT JOIN teams t2 ON t2.id = m.team2_id
		WHERE (m.team1_id = ? OR m.team2_id = ?) AND m.match_date >= ?
		ORDER BY m.match_date DESC
		LIMIT 50`, teamID, teamID, minMatchDateRFC())
	if err == nil {
		defer matchRows.Close()
		for matchRows.Next() {
			var row TeamMatchRow
			var t1ID, t2ID int
			var winnerID sql.NullInt64
			var t1Name, t2Name string
			var eventName, format, matchDate sql.NullString
			if err := matchRows.Scan(&row.ID, &t1ID, &t1Name, &t2ID, &t2Name, &winnerID, &eventName, &format, &matchDate); err != nil {
				break
			}
			if t1ID == teamID {
				row.Opponent = t2Name
				if row.Opponent == "" {
					row.Opponent = "?"
				}
			} else {
				row.Opponent = t1Name
				if row.Opponent == "" {
					row.Opponent = "?"
				}
			}
			if winnerID.Valid {
				row.Played = true
				row.Won = int(winnerID.Int64) == teamID
			}
			if eventName.Valid {
				row.EventName = eventName.String
			}
			if format.Valid {
				row.Format = models.MatchFormat(format.String)
			}
			if matchDate.Valid {
				if d, err := time.Parse(time.RFC3339, matchDate.String); err == nil {
					row.Date = d.Format("02.01.2006")
				}
			}
			out.RecentMatches = append(out.RecentMatches, row)
		}
	}

	return out, nil
}
