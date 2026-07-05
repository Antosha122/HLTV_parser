package storage

import (
	"path/filepath"
	"testing"
	"time"

	"psr/internal/models"
)

func TestAggregateTeamMapStats(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "maps.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	t1 := models.Team{ID: 7020, Name: "Spirit"}
	t2 := models.Team{ID: 100, Name: "Other"}
	if err := db.UpsertTeam(t1); err != nil {
		t.Fatal(err)
	}
	if err := db.UpsertTeam(t2); err != nil {
		t.Fatal(err)
	}

	start := time.Date(2025, 12, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 6, 8, 0, 0, 0, 0, time.UTC)
	matchDate := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)

	detail := models.MatchDetail{
		Match: models.Match{
			ID:       9001,
			Team1:    t1,
			Team2:    t2,
			Format:   models.FormatBO3,
			Date:     matchDate,
			WinnerID: t1.ID,
		},
		Maps: []models.MatchMap{
			{MapName: "Mirage", Team1Score: 13, Team2Score: 9},
			{MapName: "Inferno", Team1Score: 10, Team2Score: 13},
		},
		Vetoes: []models.Veto{
			{Order: 1, Action: models.VetoBan, TeamID: t1.ID, MapName: "Nuke"},
			{Order: 2, Action: models.VetoPick, TeamID: t1.ID, MapName: "Mirage"},
		},
	}
	if err := db.UpsertMatch(detail); err != nil {
		t.Fatal(err)
	}

	stats, err := db.AggregateTeamMapStats(t1.ID, start, end)
	if err != nil {
		t.Fatal(err)
	}
	if len(stats) != 2 {
		t.Fatalf("stats: %d", len(stats))
	}
	byMap := make(map[string]models.TeamMapStat)
	for _, s := range stats {
		byMap[s.MapName] = s
	}
	if byMap["Mirage"].Wins != 1 || byMap["Mirage"].Losses != 0 {
		t.Fatalf("mirage: %+v", byMap["Mirage"])
	}
	if byMap["Inferno"].Wins != 0 || byMap["Inferno"].Losses != 1 {
		t.Fatalf("inferno: %+v", byMap["Inferno"])
	}
	if byMap["Mirage"].PickRate <= 0 {
		t.Fatalf("pick rate: %v", byMap["Mirage"].PickRate)
	}
}
