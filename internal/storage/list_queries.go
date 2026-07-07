package storage

import (
	"database/sql"
	"strings"
	"time"

	"psr/internal/models"
)

func (db *DB) ListTeams(search string, limit int) ([]models.Team, error) {
	return db.ListTeamsPage(search, limit, 0)
}

// ListTeamsPage returns a page of teams. limit is the page size (clamped to
// [1, 500]); offset is the number of rows to skip (>= 0). Pass offset=0 for
// the first page.
func (db *DB) ListTeamsPage(search string, limit, offset int) ([]models.Team, error) {
	if limit <= 0 || limit > 500 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}

	var rows *sql.Rows
	var err error
	search = strings.TrimSpace(search)
	if search == "" {
		rows, err = db.sql.Query(`
			SELECT id, name, hltv_rating, world_rank
			FROM teams
			ORDER BY COALESCE(world_rank, 9999), name
			LIMIT ? OFFSET ?`, limit, offset)
	} else {
		rows, err = db.sql.Query(`
			SELECT id, name, hltv_rating, world_rank
			FROM teams
			WHERE LOWER(name) LIKE '%' || LOWER(?) || '%'
			ORDER BY COALESCE(world_rank, 9999), name
			LIMIT ? OFFSET ?`, search, limit, offset)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanTeams(rows)
}

func (db *DB) GetEvent(id int) (models.Event, error) {
	var e models.Event
	var status sql.NullString
	err := db.sql.QueryRow(`SELECT id, name, status FROM events WHERE id = ?`, id).
		Scan(&e.ID, &e.Name, &status)
	if err != nil {
		return models.Event{}, err
	}
	if status.Valid {
		e.Status = models.EventStatus(status.String)
	}
	return e, nil
}

func (db *DB) ListEvents(search string, limit int) ([]models.Event, error) {
	return db.ListEventsPage(search, limit, 0)
}

// ListEventsPage returns a page of events. limit is the page size (clamped to
// [1, 500]); offset is the number of rows to skip (>= 0).
func (db *DB) ListEventsPage(search string, limit, offset int) ([]models.Event, error) {
	if limit <= 0 || limit > 500 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}
	search = strings.TrimSpace(search)
	var rows *sql.Rows
	var err error
	if search == "" {
		rows, err = db.sql.Query(`
			SELECT id, name, status FROM events
			WHERE status IS NULL OR status = '' OR status IN ('ongoing', 'past', 'upcoming')
			ORDER BY CASE status
				WHEN 'ongoing' THEN 0
				WHEN 'upcoming' THEN 1
				WHEN '' THEN 2
				ELSE 3
			END, updated_at DESC LIMIT ? OFFSET ?`, limit, offset)
	} else {
		rows, err = db.sql.Query(`
			SELECT id, name, status FROM events
			WHERE (status IS NULL OR status = '' OR status IN ('ongoing', 'past', 'upcoming'))
			  AND LOWER(name) LIKE '%' || LOWER(?) || '%'
			ORDER BY CASE status
				WHEN 'ongoing' THEN 0
				WHEN 'upcoming' THEN 1
				WHEN '' THEN 2
				ELSE 3
			END, updated_at DESC LIMIT ? OFFSET ?`, search, limit, offset)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var events []models.Event
	for rows.Next() {
		var e models.Event
		var status sql.NullString
		if err := rows.Scan(&e.ID, &e.Name, &status); err != nil {
			return nil, err
		}
		if status.Valid {
			e.Status = models.EventStatus(status.String)
		}
		events = append(events, e)
	}
	if events == nil {
		events = []models.Event{}
	}
	return events, rows.Err()
}

func scanTeams(rows *sql.Rows) ([]models.Team, error) {
	var teams []models.Team
	for rows.Next() {
		var t models.Team
		var rating sql.NullFloat64
		var rank sql.NullInt64
		if err := rows.Scan(&t.ID, &t.Name, &rating, &rank); err != nil {
			return nil, err
		}
		if rating.Valid {
			t.HLTVRating = rating.Float64
		}
		if rank.Valid {
			t.WorldRank = int(rank.Int64)
		}
		teams = append(teams, t)
	}
	if teams == nil {
		teams = []models.Team{}
	}
	return teams, rows.Err()
}

func (db *DB) GetMatchesBefore(before time.Time, limit int) ([]MatchRecord, error) {
	q := `
		SELECT id, team1_id, team2_id, winner_id, format, match_date
		FROM matches
		WHERE winner_id IS NOT NULL AND match_date IS NOT NULL AND match_date < ?
		ORDER BY match_date ASC`
	args := []any{before.UTC().Format(time.RFC3339)}
	if limit > 0 {
		q += ` LIMIT ?`
		args = append(args, limit)
	}

	rows, err := db.sql.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanMatchRecords(rows)
}

func (db *DB) GetTeamFormBefore(teamID, limit int, before time.Time) (wins, total int, err error) {
	rows, err := db.sql.Query(`
		SELECT winner_id
		FROM matches
		WHERE (team1_id = ? OR team2_id = ?) AND winner_id IS NOT NULL AND match_date < ?
		ORDER BY match_date DESC
		LIMIT ?`, teamID, teamID, before.UTC().Format(time.RFC3339), limit)
	if err != nil {
		return 0, 0, err
	}
	defer rows.Close()

	for rows.Next() {
		var winnerID int
		if err := rows.Scan(&winnerID); err != nil {
			return 0, 0, err
		}
		total++
		if winnerID == teamID {
			wins++
		}
	}
	return wins, total, rows.Err()
}

func (db *DB) GetH2HBefore(team1ID, team2ID, limit int, before time.Time) ([]models.MatchSummary, error) {
	rows, err := db.sql.Query(`
		SELECT m.id, m.team1_id, t1.name, m.team2_id, t2.name, m.match_date, m.format, m.winner_id, m.event_name
		FROM matches m
		JOIN teams t1 ON t1.id = m.team1_id
		JOIN teams t2 ON t2.id = m.team2_id
		WHERE m.match_date < ?
		  AND m.winner_id IS NOT NULL
		  AND ((m.team1_id = ? AND m.team2_id = ?) OR (m.team1_id = ? AND m.team2_id = ?))
		ORDER BY m.match_date DESC
		LIMIT ?`,
		before.UTC().Format(time.RFC3339),
		team1ID, team2ID, team2ID, team1ID, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanMatchSummaries(rows)
}

func scanMatchRecords(rows *sql.Rows) ([]MatchRecord, error) {
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
