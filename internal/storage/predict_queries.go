package storage

import (
	"database/sql"
	"time"

	"psr/internal/models"
)

type MatchRecord struct {
	ID       int
	Team1ID  int
	Team2ID  int
	WinnerID int
	Format   models.MatchFormat
	Date     time.Time
}

func (db *DB) GetMatchesChronological(limit int) ([]MatchRecord, error) {
	q := `
		SELECT id, team1_id, team2_id, winner_id, format, match_date
		FROM matches
		WHERE winner_id IS NOT NULL AND match_date IS NOT NULL AND match_date >= ?
		ORDER BY match_date ASC`
	if limit > 0 {
		q += ` LIMIT ?`
	}

	var rows *sql.Rows
	var err error
	minDate := minMatchDateRFC()
	if limit > 0 {
		rows, err = db.sql.Query(q, minDate, limit)
	} else {
		rows, err = db.sql.Query(q, minDate)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []MatchRecord
	for rows.Next() {
		var m MatchRecord
		var winnerID sql.NullInt64
		var format sql.NullString
		var matchDate sql.NullString
		if err := rows.Scan(&m.ID, &m.Team1ID, &m.Team2ID, &winnerID, &format, &matchDate); err != nil {
			return nil, err
		}
		if winnerID.Valid {
			m.WinnerID = int(winnerID.Int64)
		}
		if format.Valid {
			m.Format = models.MatchFormat(format.String)
		}
		if matchDate.Valid {
			m.Date, _ = time.Parse(time.RFC3339, matchDate.String)
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func (db *DB) GetTeamMapStatsForTeam(teamID int) ([]models.TeamMapStat, error) {
	rows, err := db.sql.Query(`
		SELECT team_id, map_name, wins, losses, pick_rate, ban_rate
		FROM team_map_stats
		WHERE team_id = ?
		  AND period_start = (
		    SELECT MAX(period_start) FROM team_map_stats WHERE team_id = ?
		  )
		ORDER BY (wins + losses) DESC`, teamID, teamID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var stats []models.TeamMapStat
	for rows.Next() {
		var s models.TeamMapStat
		var pick, ban sql.NullFloat64
		if err := rows.Scan(&s.TeamID, &s.MapName, &s.Wins, &s.Losses, &pick, &ban); err != nil {
			return nil, err
		}
		if pick.Valid {
			s.PickRate = pick.Float64
		}
		if ban.Valid {
			s.BanRate = ban.Float64
		}
		stats = append(stats, s)
	}
	return stats, rows.Err()
}

type VetoRecord struct {
	TeamID  int
	MapName string
	Action  models.VetoAction
}

func (db *DB) GetTeamVetoes(teamID int, limit int) ([]VetoRecord, error) {
	rows, err := db.sql.Query(`
		SELECT v.team_id, v.map_name, v.action
		FROM vetoes v
		JOIN matches m ON m.id = v.match_id
		WHERE v.team_id = ? AND v.action IN ('ban', 'pick')
		ORDER BY m.match_date DESC
		LIMIT ?`, teamID, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []VetoRecord
	for rows.Next() {
		var r VetoRecord
		var action string
		if err := rows.Scan(&r.TeamID, &r.MapName, &action); err != nil {
			return nil, err
		}
		r.Action = models.VetoAction(action)
		out = append(out, r)
	}
	return out, rows.Err()
}

func (db *DB) GetTeamForm(teamID, limit int) (wins, total int, err error) {
	rows, err := db.sql.Query(`
		SELECT winner_id, team1_id, team2_id
		FROM matches
		WHERE (team1_id = ? OR team2_id = ?) AND winner_id IS NOT NULL AND match_date >= ?
		ORDER BY match_date DESC
		LIMIT ?`, teamID, teamID, minMatchDateRFC(), limit,
	)
	if err != nil {
		return 0, 0, err
	}
	defer rows.Close()

	for rows.Next() {
		var winnerID, t1, t2 int
		if err := rows.Scan(&winnerID, &t1, &t2); err != nil {
			return 0, 0, err
		}
		total++
		if winnerID == teamID {
			wins++
		}
	}
	return wins, total, rows.Err()
}
