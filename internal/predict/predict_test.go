package predict

import (
	"testing"

	"psr/internal/models"
	"psr/internal/storage"
)

func TestEloWinProbability(t *testing.T) {
	p := EloWinProbability(1600, 1500)
	if p < 0.6 || p > 0.7 {
		t.Fatalf("expected ~64%%, got %f", p*100)
	}
	if EloWinProbability(1500, 1500) != 0.5 {
		t.Fatal("equal elo should be 50%")
	}
}

func TestComputeElo(t *testing.T) {
	matches := []storage.MatchRecord{
		{Team1ID: 1, Team2ID: 2, WinnerID: 1, Format: models.FormatBO3},
		{Team1ID: 1, Team2ID: 2, WinnerID: 1, Format: models.FormatBO3},
	}
	ratings := ComputeElo(matches)
	if ratings[1] <= ratings[2] {
		t.Fatalf("team1 should be higher: %v", ratings)
	}
}

func TestBo3SeriesProbability(t *testing.T) {
	win, scores := Bo3SeriesProbability([]float64{0.6, 0.6, 0.55})
	if win <= 0.5 {
		t.Fatalf("expected team1 favorite, got %f", win)
	}
	if scores["2-0"] <= 0 {
		t.Fatalf("expected 2-0 prob, got %v", scores)
	}
}

func TestSimulateBo3Veto(t *testing.T) {
	p1 := map[string]models.MapProfile{
		"Mirage":  {MapName: "Mirage", Wins: 10, Losses: 2, PickRate: 0.4},
		"Inferno": {MapName: "Inferno", Wins: 3, Losses: 8},
		"Nuke":    {MapName: "Nuke", Wins: 2, Losses: 9, BanRate: 0.5},
	}
	p2 := map[string]models.MapProfile{
		"Inferno": {MapName: "Inferno", Wins: 9, Losses: 3, PickRate: 0.35},
		"Mirage":  {MapName: "Mirage", Wins: 4, Losses: 7},
		"Ancient": {MapName: "Ancient", Wins: 6, Losses: 5},
	}
	for _, m := range ActiveDuty {
		if _, ok := p1[m]; !ok {
			p1[m] = models.MapProfile{MapName: m, Wins: 5, Losses: 5}
		}
		if _, ok := p2[m]; !ok {
			p2[m] = models.MapProfile{MapName: m, Wins: 5, Losses: 5}
		}
	}

	v := SimulateBo3Veto(
		models.Team{ID: 1, Name: "TeamA"},
		models.Team{ID: 2, Name: "TeamB"},
		p1, p2,
	)
	if len(v.Steps) < 7 {
		t.Fatalf("expected veto steps, got %d", len(v.Steps))
	}
	maps := SeriesMapsFromVeto(v)
	if len(maps) != 3 {
		t.Fatalf("expected 3 series maps, got %v", maps)
	}
}

func TestEnsemble(t *testing.T) {
	p := Ensemble(0.6, 0.55, 0.5, 0.65)
	if p < 0.55 || p > 0.65 {
		t.Fatalf("unexpected ensemble: %f", p)
	}
}

func TestVetoMapPoolExcludesUnplayedMaps(t *testing.T) {
	maps := []string{"Ancient", "Anubis", "Dust2", "Inferno", "Mirage", "Nuke", "Overpass"}
	p1 := make(map[string]models.MapProfile)
	p2 := make(map[string]models.MapProfile)
	for _, m := range maps {
		p1[m] = models.MapProfile{MapName: m, Wins: 5, Losses: 3}
		p2[m] = models.MapProfile{MapName: m, Wins: 4, Losses: 4}
	}
	p1["Train"] = models.MapProfile{MapName: "Train", WinRate: 0.5}
	p2["Train"] = models.MapProfile{MapName: "Train", WinRate: 0.5}

	pool := vetoMapPool(p1, p2)
	if len(pool) != 7 {
		t.Fatalf("expected 7 maps, got %v", pool)
	}
	for _, m := range pool {
		if m == "Train" {
			t.Fatal("Train must not be in veto pool")
		}
	}
}

func TestMapWinProbability(t *testing.T) {
	a := models.MapProfile{Wins: 12, Losses: 3}
	b := models.MapProfile{Wins: 4, Losses: 10}
	p := MapWinProbability(a, b)
	if p < 0.55 {
		t.Fatalf("team A should be favored on map, got %f", p)
	}
}
