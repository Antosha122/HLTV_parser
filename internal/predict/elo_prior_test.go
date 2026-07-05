package predict

import (
	"testing"

	"psr/internal/models"
)

func TestPriorEloFromTeam(t *testing.T) {
	spirit := models.Team{ID: 7020, Name: "Spirit", WorldRank: 3}
	z9 := models.Team{ID: 9996, Name: "9z", WorldRank: 45}

	spiritElo := PriorEloFromTeam(spirit)
	z9Elo := PriorEloFromTeam(z9)
	if spiritElo <= z9Elo {
		t.Fatalf("spirit %f should be > 9z %f", spiritElo, z9Elo)
	}

	elo := map[int]float64{}
	p := EloWinProbability(BlendedRating(elo, spirit), BlendedRating(elo, z9))
	if p < 0.6 {
		t.Fatalf("expected spirit favorite, got %f", p)
	}
}
