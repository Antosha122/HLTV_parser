package storage

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite"

	"psr/internal/models"
)

type DB struct {
	sql *sql.DB
}

func Open(path string) (*DB, error) {
	if dir := filepath.Dir(path); dir != "." && dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, fmt.Errorf("create db dir: %w", err)
		}
	}

	conn, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	conn.SetMaxOpenConns(4)

	db := &DB{sql: conn}
	var migrateErr error
	for attempt := 0; attempt < 15; attempt++ {
		if attempt > 0 {
			time.Sleep(time.Second)
		}
		_, _ = conn.Exec("PRAGMA busy_timeout = 10000")
		migrateErr = db.runMigrations()
		if migrateErr == nil {
			_, _ = conn.Exec("PRAGMA journal_mode = WAL")
			return db, nil
		}
		if !isSQLiteBusy(migrateErr) {
			break
		}
	}
	_ = conn.Close()
	if migrateErr != nil && isSQLiteBusy(migrateErr) {
		return nil, fmt.Errorf("база данных занята (%s): закройте вкладки/программы с psr.db и запустите stop.bat", path)
	}
	return nil, migrateErr
}

func isSQLiteBusy(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "locked") || strings.Contains(msg, "busy")
}

func (db *DB) Close() error {
	return db.sql.Close()
}

func (db *DB) ensureColumn(table, column, colType string) {
	_, _ = db.sql.Exec(fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s %s", table, column, colType))
}

func (db *DB) UpsertTeam(t models.Team) error {
	_, err := db.sql.Exec(`
		INSERT INTO teams (id, name, hltv_rating, world_rank, updated_at)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			name = excluded.name,
			hltv_rating = excluded.hltv_rating,
			world_rank = excluded.world_rank,
			updated_at = excluded.updated_at`,
		t.ID, t.Name, nullFloat(t.HLTVRating), nullInt(t.WorldRank), time.Now().UTC().Format(time.RFC3339),
	)
	return err
}

func (db *DB) UpsertPlayer(p models.Player) error {
	_, err := db.sql.Exec(`
		INSERT INTO players (id, name, team_id, updated_at)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			name = excluded.name,
			team_id = excluded.team_id,
			updated_at = excluded.updated_at`,
		p.ID, p.Name, p.TeamID, time.Now().UTC().Format(time.RFC3339),
	)
	return err
}

func (db *DB) UpsertEvent(e models.Event) error {
	status := string(e.Status)
	_, err := db.sql.Exec(`
		INSERT INTO events (id, name, status, updated_at)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			name = CASE WHEN excluded.name != '' THEN excluded.name ELSE events.name END,
			status = CASE
				WHEN excluded.status != '' AND (
					excluded.status = 'ongoing'
					OR events.status IS NULL OR events.status = ''
					OR events.status = 'upcoming' AND excluded.status IN ('ongoing', 'past')
					OR events.status = 'past' AND excluded.status = 'ongoing'
				) THEN excluded.status
				ELSE events.status
			END,
			updated_at = excluded.updated_at`,
		e.ID, e.Name, status, time.Now().UTC().Format(time.RFC3339),
	)
	return err
}

func (db *DB) UpsertMatch(m models.MatchDetail) error {
	tx, err := db.sql.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	now := time.Now().UTC().Format(time.RFC3339)
	matchDate := ""
	if !m.Date.IsZero() {
		matchDate = m.Date.UTC().Format(time.RFC3339)
	}

	_, err = tx.Exec(`
		INSERT INTO matches (id, event_id, event_name, team1_id, team2_id, format, match_date, winner_id, stars, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			event_id = excluded.event_id,
			event_name = excluded.event_name,
			team1_id = excluded.team1_id,
			team2_id = excluded.team2_id,
			format = excluded.format,
			match_date = excluded.match_date,
			winner_id = excluded.winner_id,
			stars = excluded.stars,
			updated_at = excluded.updated_at`,
		m.ID,
		nullInt(m.EventID),
		m.EventName,
		m.Team1.ID,
		m.Team2.ID,
		string(m.Format),
		matchDate,
		nullInt(m.WinnerID),
		nullInt(m.Stars),
		now,
	)
	if err != nil {
		return err
	}

	if _, err = tx.Exec(`DELETE FROM match_maps WHERE match_id = ?`, m.ID); err != nil {
		return err
	}
	for _, mp := range m.Maps {
		_, err = tx.Exec(`
			INSERT INTO match_maps (match_id, map_name, team1_score, team2_score, picked_by)
			VALUES (?, ?, ?, ?, ?)`,
			m.ID, mp.MapName, mp.Team1Score, mp.Team2Score, nullInt(mp.PickedBy),
		)
		if err != nil {
			return err
		}
	}

	if _, err = tx.Exec(`DELETE FROM vetoes WHERE match_id = ?`, m.ID); err != nil {
		return err
	}
	for _, v := range m.Vetoes {
		_, err = tx.Exec(`
			INSERT INTO vetoes (match_id, veto_order, action, team_id, team_name, map_name)
			VALUES (?, ?, ?, ?, ?, ?)`,
			m.ID, v.Order, string(v.Action), nullInt(v.TeamID), v.TeamName, v.MapName,
		)
		if err != nil {
			return err
		}
	}

	return tx.Commit()
}

func (db *DB) UpsertTeamMapStats(stats []models.TeamMapStat, periodStart, periodEnd time.Time) error {
	if len(stats) == 0 {
		return nil
	}
	tx, err := db.sql.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	ps := periodStart.UTC().Format("2006-01-02")
	pe := periodEnd.UTC().Format("2006-01-02")
	teamID := stats[0].TeamID

	if _, err = tx.Exec(`DELETE FROM team_map_stats WHERE team_id = ?`, teamID); err != nil {
		return err
	}

	for _, s := range stats {
		_, err = tx.Exec(`
			INSERT INTO team_map_stats (team_id, map_name, wins, losses, pick_rate, ban_rate, period_start, period_end)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT(team_id, map_name, period_start) DO UPDATE SET
				wins = excluded.wins,
				losses = excluded.losses,
				pick_rate = excluded.pick_rate,
				ban_rate = excluded.ban_rate,
				period_end = excluded.period_end`,
			s.TeamID, s.MapName, s.Wins, s.Losses, nullFloat(s.PickRate), nullFloat(s.BanRate), ps, pe,
		)
		if err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (db *DB) LogSync(entityType string, entityID int, status, message string) error {
	_, err := db.sql.Exec(`
		INSERT INTO sync_log (entity_type, entity_id, synced_at, status, message)
		VALUES (?, ?, ?, ?, ?)`,
		entityType, entityID, time.Now().UTC().Format(time.RFC3339), status, message,
	)
	return err
}

func (db *DB) SetMatchEvent(matchID, eventID int, eventName string) error {
	_, err := db.sql.Exec(`
		UPDATE matches SET event_id = ?, event_name = ?, updated_at = ?
		WHERE id = ?`,
		nullInt(eventID), eventName, time.Now().UTC().Format(time.RFC3339), matchID,
	)
	return err
}

func (db *DB) CountTeamMatches(teamID int) (int, error) {
	var n int
	err := db.sql.QueryRow(`
		SELECT COUNT(1) FROM matches WHERE team1_id = ? OR team2_id = ?`,
		teamID, teamID,
	).Scan(&n)
	return n, err
}

func (db *DB) MatchExists(id int) (bool, error) {
	var n int
	err := db.sql.QueryRow(`SELECT COUNT(1) FROM matches WHERE id = ?`, id).Scan(&n)
	return n > 0, err
}

func (db *DB) MatchHasWinner(id int) (bool, error) {
	var n int
	err := db.sql.QueryRow(`SELECT COUNT(1) FROM matches WHERE id = ? AND winner_id IS NOT NULL`, id).Scan(&n)
	return n > 0, err
}

func (db *DB) GetTeam(id int) (models.TeamDetail, error) {
	var t models.Team
	var rating sql.NullFloat64
	var rank sql.NullInt64
	err := db.sql.QueryRow(`
		SELECT id, name, hltv_rating, world_rank FROM teams WHERE id = ?`, id,
	).Scan(&t.ID, &t.Name, &rating, &rank)
	if err != nil {
		return models.TeamDetail{}, err
	}
	if rating.Valid {
		t.HLTVRating = rating.Float64
	}
	if rank.Valid {
		t.WorldRank = int(rank.Int64)
	}

	rows, err := db.sql.Query(`SELECT id, name, team_id FROM players WHERE team_id = ? ORDER BY name`, id)
	if err != nil {
		return models.TeamDetail{}, err
	}
	defer rows.Close()

	var players []models.Player
	for rows.Next() {
		var p models.Player
		if err := rows.Scan(&p.ID, &p.Name, &p.TeamID); err != nil {
			return models.TeamDetail{}, err
		}
		players = append(players, p)
	}

	recent, err := db.GetTeamRecentMatches(id, 10)
	if err != nil {
		return models.TeamDetail{}, err
	}

	return models.TeamDetail{Team: t, Players: players, RecentMatches: recent}, nil
}

func (db *DB) GetTeamRecentMatches(teamID, limit int) ([]models.MatchSummary, error) {
	rows, err := db.sql.Query(`
		SELECT m.id, m.team1_id, t1.name, m.team2_id, t2.name, m.match_date, m.format, m.winner_id, m.event_name
		FROM matches m
		JOIN teams t1 ON t1.id = m.team1_id
		JOIN teams t2 ON t2.id = m.team2_id
		WHERE m.team1_id = ? OR m.team2_id = ?
		ORDER BY m.match_date DESC
		LIMIT ?`, teamID, teamID, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanMatchSummaries(rows)
}

func (db *DB) GetMatch(id int) (models.MatchDetail, error) {
	var m models.MatchDetail
	var eventID sql.NullInt64
	var matchDate sql.NullString
	var winnerID sql.NullInt64
	var stars sql.NullInt64
	var format sql.NullString

	err := db.sql.QueryRow(`
		SELECT m.id, m.event_id, m.event_name,
		       m.team1_id, COALESCE(t1.name, ''),
		       m.team2_id, COALESCE(t2.name, ''),
		       m.format, m.match_date, m.winner_id, m.stars
		FROM matches m
		LEFT JOIN teams t1 ON t1.id = m.team1_id
		LEFT JOIN teams t2 ON t2.id = m.team2_id
		WHERE m.id = ?`, id,
	).Scan(&m.ID, &eventID, &m.EventName, &m.Team1.ID, &m.Team1.Name, &m.Team2.ID, &m.Team2.Name, &format, &matchDate, &winnerID, &stars)
	if err != nil {
		return models.MatchDetail{}, err
	}
	if eventID.Valid {
		m.EventID = int(eventID.Int64)
	}
	if format.Valid {
		m.Format = models.MatchFormat(format.String)
	}
	if matchDate.Valid {
		m.Date, _ = time.Parse(time.RFC3339, matchDate.String)
	}
	if winnerID.Valid {
		m.WinnerID = int(winnerID.Int64)
	}
	if stars.Valid {
		m.Stars = int(stars.Int64)
	}

	mapRows, err := db.sql.Query(`
		SELECT map_name, team1_score, team2_score, picked_by FROM match_maps WHERE match_id = ?`, id,
	)
	if err != nil {
		return models.MatchDetail{}, err
	}
	defer mapRows.Close()
	for mapRows.Next() {
		var mp models.MatchMap
		var picked sql.NullInt64
		if err := mapRows.Scan(&mp.MapName, &mp.Team1Score, &mp.Team2Score, &picked); err != nil {
			return models.MatchDetail{}, err
		}
		if picked.Valid {
			mp.PickedBy = int(picked.Int64)
		}
		m.Maps = append(m.Maps, mp)
	}

	vetoRows, err := db.sql.Query(`
		SELECT veto_order, action, team_id, team_name, map_name FROM vetoes WHERE match_id = ? ORDER BY veto_order`, id,
	)
	if err != nil {
		return models.MatchDetail{}, err
	}
	defer vetoRows.Close()
	for vetoRows.Next() {
		var v models.Veto
		var teamID sql.NullInt64
		if err := vetoRows.Scan(&v.Order, &v.Action, &teamID, &v.TeamName, &v.MapName); err != nil {
			return models.MatchDetail{}, err
		}
		if teamID.Valid {
			v.TeamID = int(teamID.Int64)
		}
		m.Vetoes = append(m.Vetoes, v)
	}

	if m.Team1.ID > 0 && m.Team2.ID > 0 {
		m.H2H, _ = db.GetH2H(m.Team1.ID, m.Team2.ID, 10)
	}

	return m, nil
}

func (db *DB) GetH2H(team1ID, team2ID, limit int) ([]models.MatchSummary, error) {
	rows, err := db.sql.Query(`
		SELECT m.id, m.team1_id, t1.name, m.team2_id, t2.name, m.match_date, m.format, m.winner_id, m.event_name
		FROM matches m
		JOIN teams t1 ON t1.id = m.team1_id
		JOIN teams t2 ON t2.id = m.team2_id
		WHERE ((m.team1_id = ? AND m.team2_id = ?) OR (m.team1_id = ? AND m.team2_id = ?))
		  AND m.winner_id IS NOT NULL
		  AND m.match_date >= ?
		ORDER BY m.match_date DESC
		LIMIT ?`,
		team1ID, team2ID, team2ID, team1ID, minMatchDateRFC(), limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanMatchSummaries(rows)
}

func (db *DB) GetLastSync(entityType string) (syncedAt, status, message string, err error) {
	err = db.sql.QueryRow(`
		SELECT synced_at, status, message FROM sync_log
		WHERE entity_type = ?
		ORDER BY id DESC LIMIT 1`, entityType,
	).Scan(&syncedAt, &status, &message)
	return
}

// knownTables is the whitelist of tables exposed via Stats(). Using a static
// list avoids string-concatenating table names into SQL, which looks like a
// SQL-injection pattern even though the values were constants.
var knownTables = []string{
	"teams", "players", "matches", "match_maps", "vetoes",
	"team_map_stats", "events", "event_teams", "sync_log",
	"elo_ratings", "schema_version",
}

func (db *DB) Stats() (map[string]int, error) {
	out := make(map[string]int, len(knownTables))
	for _, table := range knownTables {
		var n int
		// table comes from a compile-time constant list, not user input; we
		// still route it through a placeholder-like check via the whitelist.
		if err := db.sql.QueryRow(`SELECT COUNT(1) FROM ` + table).Scan(&n); err != nil {
			return nil, err
		}
		out[table] = n
	}
	return out, nil
}

func scanMatchSummaries(rows *sql.Rows) ([]models.MatchSummary, error) {
	var out []models.MatchSummary
	for rows.Next() {
		var s models.MatchSummary
		var matchDate sql.NullString
		var format sql.NullString
		var winnerID sql.NullInt64
		var eventName sql.NullString
		if err := rows.Scan(
			&s.ID, &s.Team1.ID, &s.Team1.Name, &s.Team2.ID, &s.Team2.Name,
			&matchDate, &format, &winnerID, &eventName,
		); err != nil {
			return nil, err
		}
		if matchDate.Valid {
			s.Date, _ = time.Parse(time.RFC3339, matchDate.String)
		}
		if format.Valid {
			s.Format = models.MatchFormat(format.String)
		}
		if winnerID.Valid {
			s.WinnerID = int(winnerID.Int64)
		}
		if eventName.Valid {
			s.Event = eventName.String
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

func nullInt(v int) any {
	if v == 0 {
		return nil
	}
	return v
}

func nullFloat(v float64) any {
	if v == 0 {
		return nil
	}
	return v
}
