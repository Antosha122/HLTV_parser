package predict

import (
	"path/filepath"
	"testing"
	"time"

	"psr/internal/models"
	"psr/internal/storage"
)

func TestPredictorIntegration(t *testing.T) {
	db, err := storage.Open(filepath.Join(t.TempDir(), "predict.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	team1 := models.Team{ID: 1, Name: "Alpha", HLTVRating: 900, WorldRank: 1}
	team2 := models.Team{ID: 2, Name: "Beta", HLTVRating: 850, WorldRank: 5}
	if err := db.UpsertTeam(team1); err != nil {
		t.Fatal(err)
	}
	if err := db.UpsertTeam(team2); err != nil {
		t.Fatal(err)
	}

	date := time.Date(2025, 3, 1, 0, 0, 0, 0, time.UTC)
	for i, winner := range []int{1, 1, 2, 1} {
		m := models.MatchDetail{
			Match: models.Match{
				ID:       100 + i,
				Team1:    team1,
				Team2:    team2,
				Format:   models.FormatBO3,
				Date:     date.AddDate(0, 0, i),
				WinnerID: winner,
			},
			EventName: "Test",
		}
		if err := db.UpsertMatch(m); err != nil {
			t.Fatal(err)
		}
	}

	if err := db.UpsertTeamMapStats([]models.TeamMapStat{
		{TeamID: 1, MapName: "Mirage", Wins: 10, Losses: 2},
		{TeamID: 1, MapName: "Inferno", Wins: 5, Losses: 5},
		{TeamID: 2, MapName: "Mirage", Wins: 4, Losses: 8},
		{TeamID: 2, MapName: "Inferno", Wins: 9, Losses: 3},
	}, date.AddDate(0, -3, 0), date); err != nil {
		t.Fatal(err)
	}

	pred, err := New(db).Predict(Options{Team1ID: 1, Team2ID: 2, Format: models.FormatBO3})
	if err != nil {
		t.Fatal(err)
	}
	if pred.WinProb.Team1 <= 50 {
		t.Fatalf("alpha should be favored: %+v", pred.WinProb)
	}
	if len(pred.Maps) != len(ActiveDuty) {
		t.Fatalf("map preds: %d", len(pred.Maps))
	}
	if pred.Breakdown.Final == 0 {
		t.Fatal("breakdown missing")
	}
}
