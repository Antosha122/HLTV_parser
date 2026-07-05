package predict

import (
	"math"

	"psr/internal/models"
	"psr/internal/storage"
)

const defaultElo = 1500.0

// PriorEloFromTeam uses HLTV world rank / rating when match history is thin.
func PriorEloFromTeam(t models.Team) float64 {
	if t.WorldRank > 0 {
		return clamp(2050-float64(t.WorldRank)*6.5, 1250, 2000)
	}
	if t.HLTVRating > 0 {
		return clamp(1150+t.HLTVRating*6, 1250, 2000)
	}
	return defaultElo
}

func BlendedRating(computed map[int]float64, t models.Team) float64 {
	fromMatches := ratingOf(computed, t.ID)
	prior := PriorEloFromTeam(t)
	if fromMatches == defaultElo && prior != defaultElo {
		return prior
	}
	if fromMatches != defaultElo && prior != defaultElo {
		return 0.65*fromMatches + 0.35*prior
	}
	return fromMatches
}

func ComputeElo(matches []storage.MatchRecord) map[int]float64 {
	ratings := make(map[int]float64)

	for _, m := range matches {
		if m.WinnerID == 0 {
			continue
		}
		r1 := ratingOf(ratings, m.Team1ID)
		r2 := ratingOf(ratings, m.Team2ID)

		e1 := expectedScore(r1, r2)
		e2 := 1 - e1

		k := kFactor(m.Format)
		s1, s2 := 0.0, 1.0
		if m.WinnerID == m.Team1ID {
			s1, s2 = 1.0, 0.0
		}

		ratings[m.Team1ID] = r1 + k*(s1-e1)
		ratings[m.Team2ID] = r2 + k*(s2-e2)
	}
	return ratings
}

func ratingOf(ratings map[int]float64, teamID int) float64 {
	if r, ok := ratings[teamID]; ok {
		return r
	}
	return defaultElo
}

func expectedScore(rA, rB float64) float64 {
	return 1.0 / (1.0 + math.Pow(10, (rB-rA)/400.0))
}

func EloWinProbability(rA, rB float64) float64 {
	return expectedScore(rA, rB)
}

func kFactor(format models.MatchFormat) float64 {
	switch format {
	case models.FormatBO5:
		return 32
	case models.FormatBO3:
		return 24
	case models.FormatBO1:
		return 16
	default:
		return 20
	}
}
