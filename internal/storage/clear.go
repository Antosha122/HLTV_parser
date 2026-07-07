package storage

import (
	"fmt"
	"strings"
)

func (db *DB) ClearAll() error {
	tables := []string{
		"sync_log",
		"vetoes",
		"match_maps",
		"team_map_stats",
		"elo_ratings",
		"event_teams",
		"matches",
		"players",
		"events",
		"teams",
	}
	for _, table := range tables {
		if err := db.deleteTableRows(table); err != nil {
			return fmt.Errorf("очистка %s: %w", table, err)
		}
	}
	return nil
}

func (db *DB) deleteTableRows(table string) error {
	_, err := db.sql.Exec(`DELETE FROM ` + table)
	if err == nil {
		return nil
	}
	if strings.Contains(strings.ToLower(err.Error()), "no such table") {
		return nil
	}
	return err
}
