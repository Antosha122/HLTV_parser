package storage

import (
	"database/sql"
	"fmt"
	"time"
)

// schemaVersion records applied migrations so upgrades are deterministic and
// reproducible. Each migration is an idempotent SQL block; together with the
// initial schema they move a database from any historical state to the latest.
type schemaVersion struct {
	version int
	name    string
	stmt    string
}

// migrations is the ordered list of schema migrations. v1 is the initial
// schema; subsequent entries add indexes, columns and tables.
//
// NOTE: every statement must be idempotent (CREATE ... IF NOT EXISTS,
// ALTER handled via ensureColumn) so re-running on a partially migrated DB
// is safe.
var migrations = []schemaVersion{
	{
		version: 1,
		name:    "initial_schema",
		stmt:    schema,
	},
	{
		version: 2,
		name:    "event_teams_table",
		stmt: `CREATE TABLE IF NOT EXISTS event_teams (
			event_id   INTEGER NOT NULL REFERENCES events(id) ON DELETE CASCADE,
			team_id    INTEGER NOT NULL REFERENCES teams(id) ON DELETE CASCADE,
			updated_at TEXT NOT NULL,
			PRIMARY KEY (event_id, team_id)
		)`,
	},
	{
		version: 3,
		name:    "performance_indexes",
		stmt: `CREATE INDEX IF NOT EXISTS idx_matches_team1      ON matches(team1_id);
		CREATE INDEX IF NOT EXISTS idx_matches_team2      ON matches(team2_id);
		CREATE INDEX IF NOT EXISTS idx_matches_date       ON matches(match_date);
		CREATE INDEX IF NOT EXISTS idx_matches_winner     ON matches(winner_id);
		CREATE INDEX IF NOT EXISTS idx_matches_event      ON matches(event_id);
		CREATE INDEX IF NOT EXISTS idx_match_maps_match   ON match_maps(match_id);
		CREATE INDEX IF NOT EXISTS idx_vetoes_match       ON vetoes(match_id);
		CREATE INDEX IF NOT EXISTS idx_vetoes_team        ON vetoes(team_id);
		CREATE INDEX IF NOT EXISTS idx_team_map_stats_team ON team_map_stats(team_id);
		CREATE INDEX IF NOT EXISTS idx_team_map_stats_period ON team_map_stats(team_id, period_start);
		CREATE INDEX IF NOT EXISTS idx_players_team       ON players(team_id);
		CREATE INDEX IF NOT EXISTS idx_event_teams_event  ON event_teams(event_id);
		CREATE INDEX IF NOT EXISTS idx_event_teams_team   ON event_teams(team_id);
		CREATE INDEX IF NOT EXISTS idx_sync_log_entity    ON sync_log(entity_type, entity_id);`,
	},
	{
		version: 4,
		name:    "elo_ratings_cache",
		stmt: `CREATE TABLE IF NOT EXISTS elo_ratings (
			team_id    INTEGER PRIMARY KEY REFERENCES teams(id) ON DELETE CASCADE,
			rating     REAL NOT NULL,
			updated_at TEXT NOT NULL
		);
		CREATE INDEX IF NOT EXISTS idx_elo_ratings_team ON elo_ratings(team_id);`,
	},
}

// latestSchemaVersion returns the highest migration version available.
func latestSchemaVersion() int {
	if len(migrations) == 0 {
		return 0
	}
	return migrations[len(migrations)-1].version
}

// runMigrations applies pending migrations in order and records them in
// schema_version. For legacy databases without the version table the initial
// schema is assumed to already exist, so we fast-forward to v1 and only apply
// later migrations.
func (db *DB) runMigrations() error {
	if _, err := db.sql.Exec(`CREATE TABLE IF NOT EXISTS schema_version (
		version    INTEGER PRIMARY KEY,
		name       TEXT NOT NULL,
		applied_at TEXT NOT NULL
	)`); err != nil {
		return fmt.Errorf("create schema_version: %w", err)
	}

	current, err := db.currentSchemaVersion()
	if err != nil {
		return err
	}

	// Legacy DB: tables exist but version tracking does not. Mark v1 as applied
	// so we only run additive migrations (indexes, new tables, columns).
	if current == 0 && db.legacySchemaExists() {
		if err := db.recordMigration(1, "initial_schema_legacy"); err != nil {
			return err
		}
		current = 1
	}

	for _, m := range migrations {
		if m.version <= current {
			continue
		}
		if _, err := db.sql.Exec(m.stmt); err != nil {
			return fmt.Errorf("migration %d (%s): %w", m.version, m.name, err)
		}
		if err := db.recordMigration(m.version, m.name); err != nil {
			return err
		}
	}

	// Column additions that can't be expressed as CREATE IF NOT EXISTS.
	db.ensureColumn("team_map_stats", "pick_rate", "REAL")
	db.ensureColumn("team_map_stats", "ban_rate", "REAL")

	return nil
}

func (db *DB) currentSchemaVersion() (int, error) {
	var v sql.NullInt64
	if err := db.sql.QueryRow(`SELECT MAX(version) FROM schema_version`).Scan(&v); err != nil {
		return 0, err
	}
	if !v.Valid {
		return 0, nil
	}
	return int(v.Int64), nil
}

func (db *DB) recordMigration(version int, name string) error {
	_, err := db.sql.Exec(
		`INSERT INTO schema_version (version, name, applied_at) VALUES (?, ?, ?)`,
		version, name, time.Now().UTC().Format(time.RFC3339),
	)
	return err
}

// legacySchemaExists reports whether the core tables already exist, i.e. the
// database predates the migration system.
func (db *DB) legacySchemaExists() bool {
	var n int
	err := db.sql.QueryRow(
		`SELECT COUNT(1) FROM sqlite_master WHERE type='table' AND name='teams'`,
	).Scan(&n)
	return err == nil && n > 0
}
