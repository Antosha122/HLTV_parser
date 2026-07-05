package predict

import (
	"fmt"
)

type CalibrateResult struct {
	BestWeights Weights        `json:"best_weights"`
	Before      BacktestResult `json:"before"`
	After       BacktestResult `json:"after"`
	Iterations  int            `json:"iterations"`
}

func (e *Engine) Calibrate(opt BacktestOptions) (CalibrateResult, error) {
	before, err := e.Backtest(opt)
	if err != nil {
		return CalibrateResult{}, err
	}

	bestW := e.Weights
	bestLoss := before.LogLoss
	iterations := 0

	steps := []float64{0.15, 0.25, 0.35, 0.45}

	for _, elo := range steps {
		for _, form := range steps {
			for _, h2h := range steps {
				for _, maps := range steps {
					w := Weights{Elo: elo, Form: form, H2H: h2h, Maps: maps}.Normalize()
					trial := NewEngine(e.db, w)
					trial.Weights = w
					res, err := trial.Backtest(BacktestOptions{
						Limit:      0,
						MinHistory: opt.MinHistory,
						Warmup:     opt.Warmup,
					})
					if err != nil {
						continue
					}
					iterations++
					if res.LogLoss < bestLoss {
						bestLoss = res.LogLoss
						bestW = w
					}
				}
			}
		}
	}

	e.Weights = bestW
	after, err := e.Backtest(opt)
	if err != nil {
		return CalibrateResult{}, err
	}

	return CalibrateResult{
		BestWeights: bestW,
		Before:      before,
		After:       after,
		Iterations:  iterations,
	}, nil
}

func CalibrateAndSave(db DataProvider, weightsPath string, opt BacktestOptions) (CalibrateResult, error) {
	engine := NewWithWeightsPath(db, weightsPath)
	res, err := engine.Calibrate(opt)
	if err != nil {
		return CalibrateResult{}, err
	}
	if err := SaveWeights(weightsPath, res.BestWeights); err != nil {
		return res, fmt.Errorf("save weights: %w", err)
	}
	return res, nil
}
