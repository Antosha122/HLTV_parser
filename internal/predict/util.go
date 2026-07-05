package predict

import (
	"math"

	"psr/internal/models"
)

func buildH2HMatches(h2h []models.MatchSummary, predictTeam1ID int) []models.H2HMatch {
	out := make([]models.H2HMatch, 0, len(h2h))
	for _, m := range h2h {
		row := models.H2HMatch{
			ID:        m.ID,
			Event:     m.Event,
			Format:    m.Format,
			Team1Name: m.Team1.Name,
			Team2Name: m.Team2.Name,
		}
		if !m.Date.IsZero() {
			row.Date = m.Date.UTC().Format("02.01.2006")
		}
		if m.WinnerID > 0 {
			row.Played = true
			switch m.WinnerID {
			case m.Team1.ID:
				row.WinnerName = m.Team1.Name
			case m.Team2.ID:
				row.WinnerName = m.Team2.Name
			}
			if m.WinnerID == predictTeam1ID {
				row.Team1Won = true
			} else {
				row.Team1Won = false
			}
		}
		out = append(out, row)
	}
	return out
}

func bestMap(preds []models.MapPrediction) models.MapPrediction {
	best := preds[0]
	for _, p := range preds[1:] {
		if p.Team1WinPct > best.Team1WinPct {
			best = p
		}
	}
	return best
}

func assessConfidence(n1, n2, h2h, maps1, maps2 int) string {
	score := 0
	if n1 >= 10 {
		score++
	}
	if n2 >= 10 {
		score++
	}
	if h2h >= 3 {
		score++
	}
	if maps1 >= 5 && maps2 >= 5 {
		score++
	}
	switch {
	case score >= 3:
		return "high"
	case score >= 2:
		return "medium"
	default:
		return "low"
	}
}

func roundPct(p float64) float64 {
	return math.Round(p*1000) / 10
}

func roundScoreMap(m map[string]float64) map[string]float64 {
	out := make(map[string]float64, len(m))
	for k, v := range m {
		out[k] = roundPct(v)
	}
	return out
}
