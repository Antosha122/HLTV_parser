package predict

import (
	"testing"

	"psr/internal/models"
)

func TestBuildH2HMatchesSkipsUnplayedWinner(t *testing.T) {
	h2h := []models.MatchSummary{
		{
			ID: 1, Team1: models.Team{ID: 10, Name: "A"}, Team2: models.Team{ID: 20, Name: "B"},
			WinnerID: 20,
		},
		{
			ID: 2, Team1: models.Team{ID: 10, Name: "A"}, Team2: models.Team{ID: 20, Name: "B"},
			WinnerID: 0,
		},
	}
	out := buildH2HMatches(h2h, 10)
	if len(out) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(out))
	}
	if !out[0].Played || out[0].WinnerName != "B" {
		t.Fatalf("played match: %+v", out[0])
	}
	if out[1].Played || out[1].WinnerName != "" {
		t.Fatalf("scheduled match must not look played: %+v", out[1])
	}
}
