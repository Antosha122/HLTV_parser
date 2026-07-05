package storage

import (
	"time"

	"psr/internal/models"
)

func (db *DB) UpsertEventTeam(eventID, teamID int) error {
	if eventID <= 0 || teamID <= 0 {
		return nil
	}
	_, err := db.sql.Exec(`
		INSERT INTO event_teams (event_id, team_id, updated_at)
		VALUES (?, ?, ?)
		ON CONFLICT(event_id, team_id) DO UPDATE SET updated_at = excluded.updated_at`,
		eventID, teamID, time.Now().UTC().Format(time.RFC3339),
	)
	return err
}

func (db *DB) ClearEventTeams(eventID int) error {
	_, err := db.sql.Exec(`DELETE FROM event_teams WHERE event_id = ?`, eventID)
	return err
}

func (db *DB) GetEventTeams(eventID int) ([]models.Team, error) {
	rows, err := db.sql.Query(`
		SELECT DISTINCT t.id, t.name, t.hltv_rating, t.world_rank
		FROM teams t
		WHERE t.id IN (
			SELECT team_id FROM event_teams WHERE event_id = ?
			UNION
			SELECT team1_id FROM matches WHERE event_id = ?
			UNION
			SELECT team2_id FROM matches WHERE event_id = ?
		)
		ORDER BY COALESCE(t.world_rank, 9999), t.name`, eventID, eventID, eventID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	teams, err := scanTeams(rows)
	if err != nil {
		return nil, err
	}
	if teams == nil {
		teams = []models.Team{}
	}
	return teams, nil
}
