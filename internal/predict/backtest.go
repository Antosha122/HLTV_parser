package predict

import (
	"fmt"
	"time"

	"psr/internal/models"
)

type BacktestOptions struct {
	Limit      int
	MinHistory int
	Warmup     int
}

type BacktestMatch struct {
	MatchID   int       `json:"match_id"`
	Date      time.Time `json:"date"`
	Team1ID   int       `json:"team1_id"`
	Team1Name string    `json:"team1_name"`
	Team2ID   int       `json:"team2_id"`
	Team2Name string    `json:"team2_name"`
	Predicted int       `json:"predicted_winner_id"`
	Actual    int       `json:"actual_winner_id"`
	ProbTeam1 float64   `json:"prob_team1"`
	Correct   bool      `json:"correct"`
}

type BacktestResult struct {
	Total    int                    `json:"total"`
	Correct  int                    `json:"correct"`
	Accuracy float64                `json:"accuracy_pct"`
	LogLoss  float64                `json:"log_loss"`
	Brier    float64                `json:"brier_score"`
	Weights  Weights                `json:"weights"`
	Samples  []BacktestMatch        `json:"samples,omitempty"`
	ByFormat map[string]FormatStats `json:"by_format"`
}

type FormatStats struct {
	Total    int     `json:"total"`
	Correct  int     `json:"correct"`
	Accuracy float64 `json:"accuracy_pct"`
}

func (e *Engine) Backtest(opt BacktestOptions) (BacktestResult, error) {
	if opt.MinHistory <= 0 {
		opt.MinHistory = 20
	}
	if opt.Warmup <= 0 {
		opt.Warmup = 30
	}

	all, err := e.db.GetMatchesChronological(0)
	if err != nil {
		return BacktestResult{}, err
	}
	if len(all) <= opt.Warmup {
		return BacktestResult{}, fmt.Errorf("need at least %d matches in db, have %d", opt.Warmup+1, len(all))
	}

	result := BacktestResult{
		Weights:  e.Weights,
		ByFormat: make(map[string]FormatStats),
	}

	var logLossSum, brierSum float64
	samplesLeft := opt.Limit

	for i := opt.Warmup; i < len(all); i++ {
		m := all[i]
		if m.WinnerID == 0 || m.Date.IsZero() {
			continue
		}

		history := all[:i]
		if len(history) < opt.MinHistory {
			continue
		}

		format := m.Format
		if format == "" {
			format = models.FormatBO3
		}

		prob, err := e.predictProbSnapshot(history, m)
		if err != nil {
			continue
		}

		predicted := m.Team1ID
		if prob < 0.5 {
			predicted = m.Team2ID
		}
		correct := predicted == m.WinnerID
		actual := 0.0
		if m.WinnerID == m.Team1ID {
			actual = 1.0
		}

		result.Total++
		if correct {
			result.Correct++
		}
		logLossSum += logLoss(prob, actual)
		brierSum += brier(prob, actual)

		fs := result.ByFormat[string(format)]
		fs.Total++
		if correct {
			fs.Correct++
		}
		result.ByFormat[string(format)] = fs

		if samplesLeft > 0 {
			t1, _ := e.db.GetTeam(m.Team1ID)
			t2, _ := e.db.GetTeam(m.Team2ID)
			result.Samples = append(result.Samples, BacktestMatch{
				MatchID:   m.ID,
				Date:      m.Date,
				Team1ID:   m.Team1ID,
				Team1Name: t1.Name,
				Team2ID:   m.Team2ID,
				Team2Name: t2.Name,
				Predicted: predicted,
				Actual:    m.WinnerID,
				ProbTeam1: roundPct(prob),
				Correct:   correct,
			})
			samplesLeft--
		}
	}

	if result.Total == 0 {
		return result, fmt.Errorf("no matches could be backtested")
	}

	result.Accuracy = roundPct(float64(result.Correct) / float64(result.Total))
	result.LogLoss = roundPct(logLossSum / float64(result.Total))
	result.Brier = roundPct(brierSum / float64(result.Total))

	for k, fs := range result.ByFormat {
		if fs.Total > 0 {
			fs.Accuracy = roundPct(float64(fs.Correct) / float64(fs.Total))
		}
		result.ByFormat[k] = fs
	}

	return result, nil
}
