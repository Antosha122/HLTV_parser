package storage

import (
	"database/sql"
	"strings"
	"time"
)

const TeamStaleAfter = 24 * time.Hour

type TeamMeta struct {
	ID             int    `json:"id"`
	UpdatedAt      string `json:"updated_at,omitempty"`
	LastSyncAt     string `json:"last_sync_at,omitempty"`
	LastSyncStatus string `json:"last_sync_status,omitempty"`
	MatchesCount   int    `json:"matches_count"`
	PlayersCount   int    `json:"players_count"`
	MapStatsCount  int    `json:"map_stats_count"`
	Stale          bool   `json:"stale"`
}

type TeamListItem struct {
	ID             int     `json:"id"`
	Name           string  `json:"name"`
	HLTVRating     float64 `json:"hltv_rating,omitempty"`
	WorldRank      int     `json:"world_rank,omitempty"`
	UpdatedAt      string  `json:"updated_at,omitempty"`
	LastSyncAt     string  `json:"last_sync_at,omitempty"`
	LastSyncStatus string  `json:"last_sync_status,omitempty"`
	MatchesCount   int     `json:"matches_count"`
	PlayersCount   int     `json:"players_count"`
	Stale          bool    `json:"stale"`
}

func (db *DB) GetTeamMeta(teamID int) (TeamMeta, error) {
	var meta TeamMeta
	meta.ID = teamID
	var updated sql.NullString
	err := db.sql.QueryRow(`
		SELECT COALESCE(updated_at, '')
		FROM teams WHERE id = ?`, teamID,
	).Scan(&updated)
	if err != nil {
		return meta, err
	}
	if updated.Valid {
		meta.UpdatedAt = updated.String
	}

	_ = db.sql.QueryRow(`
		SELECT COUNT(1) FROM matches WHERE team1_id = ? OR team2_id = ?`,
		teamID, teamID,
	).Scan(&meta.MatchesCount)

	_ = db.sql.QueryRow(`SELECT COUNT(1) FROM players WHERE team_id = ?`, teamID).Scan(&meta.PlayersCount)

	_ = db.sql.QueryRow(`
		SELECT COUNT(DISTINCT map_name) FROM team_map_stats
		WHERE team_id = ?
		  AND period_start = (
		    SELECT MAX(period_start) FROM team_map_stats WHERE team_id = ?
		  )`, teamID, teamID,
	).Scan(&meta.MapStatsCount)

	var syncAt, syncStatus sql.NullString
	err = db.sql.QueryRow(`
		SELECT synced_at, status FROM sync_log
		WHERE entity_type = 'team' AND entity_id = ?
		ORDER BY id DESC LIMIT 1`, teamID,
	).Scan(&syncAt, &syncStatus)
	if err == nil {
		if syncAt.Valid {
			meta.LastSyncAt = syncAt.String
		}
		if syncStatus.Valid {
			meta.LastSyncStatus = syncStatus.String
		}
	}

	meta.Stale = db.isTeamMetaStale(meta)
	return meta, nil
}

func (db *DB) IsTeamStale(teamID int) (bool, error) {
	meta, err := db.GetTeamMeta(teamID)
	if err != nil {
		return true, err
	}
	return meta.Stale, nil
}

func (db *DB) isTeamMetaStale(meta TeamMeta) bool {
	if meta.MatchesCount == 0 {
		return true
	}
	if meta.LastSyncStatus != "ok" {
		return true
	}
	if meta.LastSyncAt == "" {
		return true
	}
	t, err := time.Parse(time.RFC3339, meta.LastSyncAt)
	if err != nil || time.Since(t) > TeamStaleAfter {
		return true
	}
	return false
}

func (db *DB) ListTeamsWithMeta(search string, limit int) ([]TeamListItem, error) {
	if limit <= 0 {
		limit = 100
	}
	search = strings.TrimSpace(search)

	var rows *sql.Rows
	var err error
	if search == "" {
		rows, err = db.sql.Query(`
			SELECT t.id, t.name, t.hltv_rating, t.world_rank, COALESCE(t.updated_at, ''),
				(SELECT COUNT(1) FROM matches m WHERE m.team1_id = t.id OR m.team2_id = t.id),
				(SELECT COUNT(1) FROM players p WHERE p.team_id = t.id),
				(SELECT synced_at FROM sync_log sl
				 WHERE sl.entity_type = 'team' AND sl.entity_id = t.id
				 ORDER BY sl.id DESC LIMIT 1),
				(SELECT status FROM sync_log sl
				 WHERE sl.entity_type = 'team' AND sl.entity_id = t.id
				 ORDER BY sl.id DESC LIMIT 1),
				(SELECT COUNT(DISTINCT tms.map_name) FROM team_map_stats tms
				 WHERE tms.team_id = t.id
				   AND tms.period_start = (SELECT MAX(period_start) FROM team_map_stats WHERE team_id = t.id))
			FROM teams t
			ORDER BY COALESCE(t.world_rank, 9999), t.name
			LIMIT ?`, limit)
	} else {
		rows, err = db.sql.Query(`
			SELECT t.id, t.name, t.hltv_rating, t.world_rank, COALESCE(t.updated_at, ''),
				(SELECT COUNT(1) FROM matches m WHERE m.team1_id = t.id OR m.team2_id = t.id),
				(SELECT COUNT(1) FROM players p WHERE p.team_id = t.id),
				(SELECT synced_at FROM sync_log sl
				 WHERE sl.entity_type = 'team' AND sl.entity_id = t.id
				 ORDER BY sl.id DESC LIMIT 1),
				(SELECT status FROM sync_log sl
				 WHERE sl.entity_type = 'team' AND sl.entity_id = t.id
				 ORDER BY sl.id DESC LIMIT 1),
				(SELECT COUNT(DISTINCT tms.map_name) FROM team_map_stats tms
				 WHERE tms.team_id = t.id
				   AND tms.period_start = (SELECT MAX(period_start) FROM team_map_stats WHERE team_id = t.id))
			FROM teams t
			WHERE LOWER(t.name) LIKE '%' || LOWER(?) || '%'
			ORDER BY COALESCE(t.world_rank, 9999), t.name
			LIMIT ?`, search, limit)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []TeamListItem
	for rows.Next() {
		var item TeamListItem
		var rating sql.NullFloat64
		var rank sql.NullInt64
		var updated, syncAt, syncStatus sql.NullString
		var mapStatsCount int
		if err := rows.Scan(
			&item.ID, &item.Name, &rating, &rank, &updated,
			&item.MatchesCount, &item.PlayersCount, &syncAt, &syncStatus,
			&mapStatsCount,
		); err != nil {
			return nil, err
		}
		if rating.Valid {
			item.HLTVRating = rating.Float64
		}
		if rank.Valid {
			item.WorldRank = int(rank.Int64)
		}
		if updated.Valid {
			item.UpdatedAt = updated.String
		}
		if syncAt.Valid {
			item.LastSyncAt = syncAt.String
		}
		if syncStatus.Valid {
			item.LastSyncStatus = syncStatus.String
		}
		meta := TeamMeta{
			LastSyncAt:     item.LastSyncAt,
			LastSyncStatus: item.LastSyncStatus,
			MatchesCount:   item.MatchesCount,
			PlayersCount:   item.PlayersCount,
			MapStatsCount:  mapStatsCount,
		}
		item.Stale = db.isTeamMetaStale(meta)
		out = append(out, item)
	}
	if out == nil {
		out = []TeamListItem{}
	}
	return out, rows.Err()
}

func (db *DB) ListAllTeamIDs() ([]int, error) {
	rows, err := db.sql.Query(`SELECT id FROM teams ORDER BY id`)
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
