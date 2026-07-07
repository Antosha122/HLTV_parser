package storage

import (
	"path/filepath"
	"testing"
	"time"

	"psr/internal/models"
)

func TestMigrationsIdempotent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.db")
	db, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	// Running migrations again on an already-migrated DB must not error.
	if err := db.runMigrations(); err != nil {
		t.Fatalf("re-run migrations: %v", err)
	}

	v, err := db.currentSchemaVersion()
	if err != nil {
		t.Fatal(err)
	}
	if v != latestSchemaVersion() {
		t.Fatalf("schema version = %d, want %d", v, latestSchemaVersion())
	}
}

func TestLegacyDBUpgrade(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy.db")
	db, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	// Simulate a legacy DB by removing the schema_version rows and re-running.
	if _, err := db.sql.Exec(`DELETE FROM schema_version`); err != nil {
		t.Fatal(err)
	}
	if err := db.runMigrations(); err != nil {
		t.Fatalf("migrate legacy: %v", err)
	}

	v, err := db.currentSchemaVersion()
	if err != nil {
		t.Fatal(err)
	}
	if v != latestSchemaVersion() {
		t.Fatalf("legacy upgrade version = %d, want %d", v, latestSchemaVersion())
	}
}

func TestEloCacheRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "elo.db")
	db, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	// Empty DB: cache is fresh (no matches to recompute from).
	fresh, err := db.EloCacheFresh()
	if err != nil {
		t.Fatal(err)
	}
	if !fresh {
		t.Fatal("expected fresh cache when no matches exist")
	}

	// Insert teams + a finished match.
	team1 := models.Team{ID: 1, Name: "A"}
	team2 := models.Team{ID: 2, Name: "B"}
	if err := db.UpsertTeam(team1); err != nil {
		t.Fatal(err)
	}
	if err := db.UpsertTeam(team2); err != nil {
		t.Fatal(err)
	}
	if err := db.UpsertMatch(models.MatchDetail{
		Match: models.Match{
			ID: 10, Team1: team1, Team2: team2,
			Format: models.FormatBO3, WinnerID: 1,
			Date: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
		},
	}); err != nil {
		t.Fatal(err)
	}

	// Now cache should be stale (match exists, no elo_ratings row).
	fresh, _ = db.EloCacheFresh()
	if fresh {
		t.Fatal("expected stale cache after adding a match")
	}

	ratings := map[int]float64{1: 1600, 2: 1400}
	if err := db.SaveEloRatings(ratings); err != nil {
		t.Fatal(err)
	}

	// SaveEloRatings bumps updated_at, so cache should now be fresh.
	fresh, _ = db.EloCacheFresh()
	if !fresh {
		t.Fatal("expected fresh cache after SaveEloRatings")
	}

	got, err := db.GetEloRatings()
	if err != nil {
		t.Fatal(err)
	}
	if got[1] != 1600 || got[2] != 1400 {
		t.Fatalf("elo ratings = %+v, want {1:1600, 2:1400}", got)
	}

	r, err := db.GetEloRating(1)
	if err != nil || r != 1600 {
		t.Fatalf("GetEloRating(1) = %v %v, want 1600 nil", r, err)
	}

	// Unknown team returns 0, nil.
	r, err = db.GetEloRating(9999)
	if err != nil || r != 0 {
		t.Fatalf("GetEloRating(9999) = %v %v, want 0 nil", r, err)
	}
}

func TestPaginationOffset(t *testing.T) {
	path := filepath.Join(t.TempDir(), "page.db")
	db, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	// Insert 5 teams.
	for i := 1; i <= 5; i++ {
		if err := db.UpsertTeam(models.Team{ID: i, Name: "Team" + string(rune('A'+i-1))}); err != nil {
			t.Fatal(err)
		}
	}

	page1, err := db.ListTeamsPage("", 2, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(page1) != 2 {
		t.Fatalf("page1 len = %d, want 2", len(page1))
	}

	page2, err := db.ListTeamsPage("", 2, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(page2) != 2 {
		t.Fatalf("page2 len = %d, want 2", len(page2))
	}

	// Pages must not overlap.
	if page1[0].ID == page2[0].ID || page1[0].ID == page2[1].ID ||
		page1[1].ID == page2[0].ID || page1[1].ID == page2[1].ID {
		t.Fatal("pages overlap - offset not working")
	}

	// Last page has the remaining 1 team.
	page3, err := db.ListTeamsPage("", 2, 4)
	if err != nil {
		t.Fatal(err)
	}
	if len(page3) != 1 {
		t.Fatalf("page3 len = %d, want 1", len(page3))
	}
}
