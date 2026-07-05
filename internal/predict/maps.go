package predict

import (
	"psr/internal/models"
	"psr/internal/storage"
)

// ActiveDuty is the current competitive map pool used for veto simulation.
var ActiveDuty = []string{
	"Ancient", "Anubis", "Dust2", "Inferno", "Mirage", "Nuke", "Overpass",
}

// vetoMapPool returns maps available for Bo3 veto: active duty maps both teams
// actually play. Falls back to full ActiveDuty when sample is too thin.
func vetoMapPool(p1, p2 map[string]models.MapProfile) []string {
	duty := make(map[string]struct{}, len(ActiveDuty))
	for _, m := range ActiveDuty {
		duty[m] = struct{}{}
	}

	seen := make(map[string]struct{})
	for _, profiles := range []map[string]models.MapProfile{p1, p2} {
		for name, prof := range profiles {
			if prof.Wins+prof.Losses == 0 {
				continue
			}
			if _, ok := duty[name]; !ok {
				continue
			}
			seen[name] = struct{}{}
		}
	}

	var pool []string
	for _, m := range ActiveDuty {
		if _, ok := seen[m]; ok {
			pool = append(pool, m)
		}
	}
	if len(pool) >= len(ActiveDuty) {
		return pool
	}
	return append([]string(nil), ActiveDuty...)
}

func BuildMapProfiles(stats []models.TeamMapStat, vetoes []storage.VetoRecord) map[string]models.MapProfile {
	profiles := make(map[string]models.MapProfile)

	for _, s := range stats {
		total := s.Wins + s.Losses
		wr := 0.5
		if total > 0 {
			wr = float64(s.Wins) / float64(total)
		}
		profiles[s.MapName] = models.MapProfile{
			MapName:  s.MapName,
			WinRate:  wr,
			Wins:     s.Wins,
			Losses:   s.Losses,
			PickRate: s.PickRate,
			BanRate:  s.BanRate,
		}
	}

	pickCount := make(map[string]int)
	banCount := make(map[string]int)
	vetoTotal := 0
	for _, v := range vetoes {
		vetoTotal++
		switch v.Action {
		case models.VetoPick:
			pickCount[v.MapName]++
		case models.VetoBan:
			banCount[v.MapName]++
		}
	}

	for name, p := range profiles {
		if vetoTotal > 0 {
			if p.PickRate == 0 {
				p.PickRate = float64(pickCount[name]) / float64(vetoTotal)
			}
			if p.BanRate == 0 {
				p.BanRate = float64(banCount[name]) / float64(vetoTotal)
			}
		}
		profiles[name] = p
	}

	for _, m := range ActiveDuty {
		if _, ok := profiles[m]; !ok {
			profiles[m] = models.MapProfile{MapName: m, WinRate: 0.5}
		}
	}

	return profiles
}

func MapWinProbability(teamA, teamB models.MapProfile) float64 {
	// Laplace smoothing for thin samples.
	aWR := smoothedWR(teamA.Wins, teamA.Losses)
	bWR := smoothedWR(teamB.Wins, teamB.Losses)
	diff := aWR - bWR

	p := 0.5 + diff*0.9
	return clamp(p, 0.12, 0.88)
}

func smoothedWR(wins, losses int) float64 {
	return (float64(wins) + 3.0) / (float64(wins+losses) + 6.0)
}

func clamp(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
