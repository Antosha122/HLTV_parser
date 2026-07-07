package predict

import (
	"fmt"
	"math"
	"time"

	"psr/internal/models"
	"psr/internal/storage"
)

type Engine struct {
	db      DataProvider
	Weights Weights
}

func NewEngine(db DataProvider, weights Weights) *Engine {
	if weights == (Weights{}) {
		weights = DefaultWeights
	}
	return &Engine{db: db, Weights: weights.Normalize()}
}

func New(db DataProvider) *Engine {
	return NewEngine(db, DefaultWeights)
}

func NewWithWeightsPath(db DataProvider, path string) *Engine {
	return NewEngine(db, LoadWeights(path))
}

type Options struct {
	Team1ID int
	Team2ID int
	Format  models.MatchFormat
	AsOf    time.Time
}

func (e *Engine) Predict(opt Options) (models.Prediction, error) {
	if opt.Team1ID <= 0 || opt.Team2ID <= 0 {
		return models.Prediction{}, fmt.Errorf("team1 and team2 ids required")
	}
	if opt.Format == "" {
		opt.Format = models.FormatBO3
	}

	team1, err := e.db.GetTeam(opt.Team1ID)
	if err != nil {
		return models.Prediction{}, fmt.Errorf("team1: %w", err)
	}
	team2, err := e.db.GetTeam(opt.Team2ID)
	if err != nil {
		return models.Prediction{}, fmt.Errorf("team2: %w", err)
	}

	elo, err := e.eloRatings(opt.AsOf)
	if err != nil {
		return models.Prediction{}, err
	}
	pElo := EloWinProbability(
		BlendedRating(elo, team1.Team),
		BlendedRating(elo, team2.Team),
	)

	var w1, n1, w2, n2 int
	if opt.AsOf.IsZero() {
		w1, n1, err = e.db.GetTeamForm(opt.Team1ID, 15)
		if err != nil {
			return models.Prediction{}, err
		}
		w2, n2, err = e.db.GetTeamForm(opt.Team2ID, 15)
	} else {
		w1, n1, err = e.db.GetTeamFormBefore(opt.Team1ID, 15, opt.AsOf)
		if err != nil {
			return models.Prediction{}, err
		}
		w2, n2, err = e.db.GetTeamFormBefore(opt.Team2ID, 15, opt.AsOf)
	}
	if err != nil {
		return models.Prediction{}, err
	}
	pForm1 := FormWinProbability(w1, n1)
	pForm2 := FormWinProbability(w2, n2)
	pForm := clamp(0.5+(pForm1-pForm2)*0.85, 0.10, 0.90)

	var h2h []models.MatchSummary
	if opt.AsOf.IsZero() {
		h2h, err = e.db.GetH2H(opt.Team1ID, opt.Team2ID, 50)
	} else {
		h2h, err = e.db.GetH2HBefore(opt.Team1ID, opt.Team2ID, 50, opt.AsOf)
	}
	if err != nil {
		return models.Prediction{}, err
	}
	h2hWins1 := 0
	for _, m := range h2h {
		if m.WinnerID == opt.Team1ID {
			h2hWins1++
		}
	}
	pH2H := H2HWinProbability(h2hWins1, len(h2h))

	stats1, _ := e.db.GetTeamMapStatsForTeam(opt.Team1ID)
	stats2, _ := e.db.GetTeamMapStatsForTeam(opt.Team2ID)
	vetoes1, _ := e.db.GetTeamVetoes(opt.Team1ID, 80)
	vetoes2, _ := e.db.GetTeamVetoes(opt.Team2ID, 80)
	profiles1 := BuildMapProfiles(stats1, vetoes1)
	profiles2 := BuildMapProfiles(stats2, vetoes2)

	var mapPreds []models.MapPrediction
	for _, mapName := range ActiveDuty {
		mp := MapWinProbability(profiles1[mapName], profiles2[mapName])
		mapPreds = append(mapPreds, models.MapPrediction{
			MapName:     mapName,
			Team1WinPct: roundPct(mp),
		})
	}

	veto := SimulateBo3Veto(team1.Team, team2.Team, profiles1, profiles2)
	seriesMaps := SeriesMapsFromVeto(veto)

	var mapWinProbs []float64
	for _, mapName := range seriesMaps {
		mp := MapWinProbability(profiles1[mapName], profiles2[mapName])
		mapWinProbs = append(mapWinProbs, mp)
	}

	var pMaps float64
	var seriesScores map[string]float64
	switch opt.Format {
	case models.FormatBO1:
		if len(mapWinProbs) > 0 {
			pMaps = Bo1Probability(mapWinProbs[:1])
		} else if len(mapPreds) > 0 {
			pMaps = bestMap(mapPreds).Team1WinPct / 100
		} else {
			pMaps = 0.5
		}
		seriesScores = map[string]float64{"1-0": pMaps, "0-1": 1 - pMaps}
	default:
		pMaps, seriesScores = Bo3SeriesProbability(mapWinProbs)
	}

	for i := range mapPreds {
		for _, sm := range seriesMaps {
			if mapPreds[i].MapName == sm {
				mapPreds[i].InSeries = true
			}
		}
	}
	for _, step := range veto.Steps {
		for i := range mapPreds {
			if mapPreds[i].MapName == step.MapName {
				mapPreds[i].Role = string(step.Action)
			}
		}
	}

	pFinal := EnsembleWeighted(pElo, pForm, pH2H, pMaps, e.Weights)

	return models.Prediction{
		Team1:  team1.Team,
		Team2:  team2.Team,
		Format: opt.Format,
		WinProb: models.WinProbability{
			Team1: roundPct(pFinal),
			Team2: roundPct(1 - pFinal),
		},
		SeriesScores: roundScoreMap(seriesScores),
		Veto:         veto,
		Maps:         mapPreds,
		Breakdown: models.ModelBreakdown{
			Elo:   roundPct(pElo),
			Form:  roundPct(pForm),
			H2H:   roundPct(pH2H),
			Maps:  roundPct(pMaps),
			Final: roundPct(pFinal),
		},
		H2H: models.H2HSummary{
			Total:       len(h2h),
			Team1Wins:   h2hWins1,
			Team2Wins:   len(h2h) - h2hWins1,
			Team1WinPct: roundPct(pH2H),
			Matches:     buildH2HMatches(h2h, opt.Team1ID),
		},
		Form: models.FormSummary{
			Team1WinPct: roundPct(pForm1),
			Team2WinPct: roundPct(pForm2),
			Team1Sample: n1,
			Team2Sample: n2,
		},
		Confidence:    assessConfidence(n1, n2, len(h2h), len(stats1), len(stats2)),
		Team1MapStats: mapStatSummaries(stats1),
		Team2MapStats: mapStatSummaries(stats2),
	}, nil
}

func mapStatSummaries(stats []models.TeamMapStat) []models.MapStatSummary {
	out := make([]models.MapStatSummary, 0, len(stats))
	for _, s := range stats {
		total := s.Wins + s.Losses
		if total == 0 {
			continue
		}
		out = append(out, models.MapStatSummary{
			MapName:  s.MapName,
			Wins:     s.Wins,
			Losses:   s.Losses,
			WinRate:  roundPct(float64(s.Wins) / float64(total)),
			PickRate: s.PickRate,
			BanRate:  s.BanRate,
		})
	}
	return out
}

// eloRatings returns the Elo ratings map for the given moment.
//
// For live predictions (AsOf zero) it uses the persisted cache when it is fresh;
// otherwise it recomputes over the full match history and stores the result so
// subsequent predictions are O(1) until new matches arrive.
//
// For historical snapshots (AsOf set, used by backtest) the cache must not be
// used because it reflects the latest state, so we recompute on the fly.
func (e *Engine) eloRatings(asOf time.Time) (map[int]float64, error) {
	if asOf.IsZero() {
		if fresh, err := e.db.EloCacheFresh(); err == nil && fresh {
			if cached, err := e.db.GetEloRatings(); err == nil && len(cached) > 0 {
				return cached, nil
			}
		}
		matches, err := e.db.GetMatchesChronological(0)
		if err != nil {
			return nil, err
		}
		ratings := ComputeElo(matches)
		if saveErr := e.db.SaveEloRatings(ratings); saveErr != nil {
			// Cache write failure is non-fatal; we still return ratings.
			_ = saveErr
		}
		return ratings, nil
	}

	matches, err := e.db.GetMatchesBefore(asOf, 0)
	if err != nil {
		return nil, err
	}
	return ComputeElo(matches), nil
}

func (e *Engine) PredictProb(opt Options) (float64, error) {
	p, err := e.Predict(opt)
	if err != nil {
		return 0, err
	}
	return p.WinProb.Team1 / 100, nil
}

func (e *Engine) predictProbSnapshot(history []storage.MatchRecord, m storage.MatchRecord) (float64, error) {
	format := m.Format
	if format == "" {
		format = models.FormatBO3
	}

	elo := ComputeElo(history)
	pElo := EloWinProbability(ratingOf(elo, m.Team1ID), ratingOf(elo, m.Team2ID))

	w1, n1, err := e.db.GetTeamFormBefore(m.Team1ID, 15, m.Date)
	if err != nil {
		return 0, err
	}
	w2, n2, err := e.db.GetTeamFormBefore(m.Team2ID, 15, m.Date)
	if err != nil {
		return 0, err
	}
	pForm1 := FormWinProbability(w1, n1)
	pForm2 := FormWinProbability(w2, n2)
	pForm := clamp(0.5+(pForm1-pForm2)*0.85, 0.10, 0.90)

	h2h, err := e.db.GetH2HBefore(m.Team1ID, m.Team2ID, 50, m.Date)
	if err != nil {
		return 0, err
	}
	h2hWins1 := 0
	for _, h := range h2h {
		if h.WinnerID == m.Team1ID {
			h2hWins1++
		}
	}
	pH2H := H2HWinProbability(h2hWins1, len(h2h))

	stats1, _ := e.db.GetTeamMapStatsForTeam(m.Team1ID)
	stats2, _ := e.db.GetTeamMapStatsForTeam(m.Team2ID)
	vetoes1, _ := e.db.GetTeamVetoes(m.Team1ID, 80)
	vetoes2, _ := e.db.GetTeamVetoes(m.Team2ID, 80)
	profiles1 := BuildMapProfiles(stats1, vetoes1)
	profiles2 := BuildMapProfiles(stats2, vetoes2)

	veto := SimulateBo3Veto(
		models.Team{ID: m.Team1ID},
		models.Team{ID: m.Team2ID},
		profiles1, profiles2,
	)
	seriesMaps := SeriesMapsFromVeto(veto)

	var mapWinProbs []float64
	for _, mapName := range seriesMaps {
		mapWinProbs = append(mapWinProbs, MapWinProbability(profiles1[mapName], profiles2[mapName]))
	}

	var pMaps float64
	switch format {
	case models.FormatBO1:
		if len(mapWinProbs) > 0 {
			pMaps = Bo1Probability(mapWinProbs[:1])
		} else {
			pMaps = 0.5
		}
	default:
		pMaps, _ = Bo3SeriesProbability(mapWinProbs)
	}

	return EnsembleWeighted(pElo, pForm, pH2H, pMaps, e.Weights), nil
}

func logLoss(prob, actual float64) float64 {
	prob = clamp(prob, 1e-6, 1-1e-6)
	return -(actual*math.Log(prob) + (1-actual)*math.Log(1-prob))
}

func brier(prob, actual float64) float64 {
	return (prob - actual) * (prob - actual)
}
