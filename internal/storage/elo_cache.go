package storage

import (
	"database/sql"
	"fmt"
	"time"
)

// EloRating is the cached Elo rating for a team.
type EloRating struct {
	TeamID    int     `json:"team_id"`
	Rating    float64 `json:"rating"`
	UpdatedAt string  `json:"updated_at"`
}

// GetEloRatings returns all cached Elo ratings keyed by team id.
// Teams without a cached entry are absent from the map; callers should
// fall back to the default Elo via predict.ratingOf.
func (db *DB) GetEloRatings() (map[int]float64, error) {
	rows, err := db.sql.Query(`SELECT team_id, rating FROM elo_ratings`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make(map[int]float64)
	for rows.Next() {
		var id int
		var rating float64
		if err := rows.Scan(&id, &rating); err != nil {
			return nil, err
		}
		out[id] = rating
	}
	return out, rows.Err()
}

// GetEloRating returns a cached Elo rating for a single team.
// When no cache row exists it returns 0 (predict.ratingOf maps that to default).
func (db *DB) GetEloRating(teamID int) (float64, error) {
	var rating float64
	err := db.sql.QueryRow(`SELECT rating FROM elo_ratings WHERE team_id = ?`, teamID).Scan(&rating)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	return rating, err
}

// EloCacheFresh reports whether the Elo cache was updated at or after the
// timestamp of the most recent finished match. When false the caller should
// recompute ratings and call SaveEloRatings.
func (db *DB) EloCacheFresh() (bool, error) {
	var lastMatch sql.NullString
	err := db.sql.QueryRow(`
		SELECT MAX(match_date) FROM matches
		WHERE winner_id IS NOT NULL AND match_date IS NOT NULL`,
	).Scan(&lastMatch)
	if err != nil {
		return false, err
	}
	if !lastMatch.Valid {
		// No finished matches yet: an empty cache is perfectly fresh.
		return true, nil
	}

	var cacheUpdated sql.NullString
	err = db.sql.QueryRow(`SELECT MAX(updated_at) FROM elo_ratings`).Scan(&cacheUpdated)
	if err != nil {
		return false, err
	}
	if !cacheUpdated.Valid {
		return false, nil
	}

	matchTS, err := time.Parse(time.RFC3339, lastMatch.String)
	if err != nil {
		return false, fmt.Errorf("parse match_date: %w", err)
	}
	cacheTS, err := time.Parse(time.RFC3339, cacheUpdated.String)
	if err != nil {
		return false, fmt.Errorf("parse elo_ratings.updated_at: %w", err)
	}
	return !cacheTS.Before(matchTS), nil
}

// SaveEloRatings upserts the given ratings into elo_ratings in a single
// transaction. Pass the full recomputed map; teams that are absent keep their
// stale rows (no implicit reset) so incremental updates are safe.
func (db *DB) SaveEloRatings(ratings map[int]float64) error {
	if len(ratings) == 0 {
		return nil
	}
	tx, err := db.sql.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	now := time.Now().UTC().Format(time.RFC3339)
	stmt, err := tx.Prepare(`
		INSERT INTO elo_ratings (team_id, rating, updated_at)
		VALUES (?, ?, ?)
		ON CONFLICT(team_id) DO UPDATE SET
			rating = excluded.rating,
			updated_at = excluded.updated_at`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for teamID, rating := range ratings {
		if _, err := stmt.Exec(teamID, rating, now); err != nil {
			return err
		}
	}
	return tx.Commit()
}
