package storage

import (
	"path/filepath"
	"testing"
	"time"

	"psr/internal/models"
)

func TestDBRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.db")
	db, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	team := models.Team{ID: 4608, Name: "Natus Vincere", HLTVRating: 892, WorldRank: 3}
	team2 := models.Team{ID: 6667, Name: "FaZe"}
	if err := db.UpsertTeam(team); err != nil {
		t.Fatal(err)
	}
	if err := db.UpsertTeam(team2); err != nil {
		t.Fatal(err)
	}
	if err := db.UpsertPlayer(models.Player{ID: 1, Name: "s1mple", TeamID: 4608}); err != nil {
		t.Fatal(err)
	}
	if err := db.UpsertEvent(models.Event{ID: 50, Name: "BLAST", Status: models.EventStatusOngoing}); err != nil {
		t.Fatal(err)
	}
	if err := db.UpsertEvent(models.Event{ID: 50, Name: "BLAST"}); err != nil {
		t.Fatal(err)
	}
	gotEvent, err := db.GetEvent(50)
	if err != nil {
		t.Fatal(err)
	}
	if gotEvent.Status != models.EventStatusOngoing {
		t.Fatalf("status must be preserved, got %q", gotEvent.Status)
	}

	match := models.MatchDetail{
		Match: models.Match{
			ID:       100,
			EventID:  50,
			Team1:    team,
			Team2:    team2,
			Format:   models.FormatBO3,
			Date:     time.Date(2025, 3, 1, 12, 0, 0, 0, time.UTC),
			WinnerID: 4608,
			Stars:    3,
		},
		EventName: "BLAST",
		Maps: []models.MatchMap{
			{MapName: "Mirage", Team1Score: 13, Team2Score: 9},
		},
		Vetoes: []models.Veto{
			{Order: 1, Action: models.VetoBan, TeamID: 4608, MapName: "Nuke"},
		},
	}
	if err := db.UpsertMatch(match); err != nil {
		t.Fatal(err)
	}

	got, err := db.GetMatch(100)
	if err != nil {
		t.Fatal(err)
	}
	if got.Team1.Name != "Natus Vincere" || len(got.Maps) != 1 || len(got.Vetoes) != 1 {
		t.Fatalf("match: %+v", got)
	}

	teamDetail, err := db.GetTeam(4608)
	if err != nil {
		t.Fatal(err)
	}
	if len(teamDetail.Players) != 1 || len(teamDetail.RecentMatches) != 1 {
		t.Fatalf("team detail: %+v", teamDetail)
	}
}
