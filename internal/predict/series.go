package predict

func Bo3SeriesProbability(mapWinProbs []float64) (team1Win float64, scores map[string]float64) {
	scores = map[string]float64{"2-0": 0, "2-1": 0, "1-2": 0, "0-2": 0}

	if len(mapWinProbs) < 2 {
		p := 0.5
		if len(mapWinProbs) == 1 {
			p = mapWinProbs[0]
		}
		scores["2-0"] = p * p
		scores["0-2"] = (1 - p) * (1 - p)
		scores["2-1"] = p * (1 - p) * p
		scores["1-2"] = (1 - p) * p * (1 - p)
		team1Win = scores["2-0"] + scores["2-1"]
		return team1Win, scores
	}

	p1, p2 := mapWinProbs[0], mapWinProbs[1]
	p3 := 0.5
	if len(mapWinProbs) >= 3 {
		p3 = mapWinProbs[2]
	}

	scores["2-0"] = p1 * p2
	scores["0-2"] = (1 - p1) * (1 - p2)
	scores["2-1"] = p1*(1-p2)*p3 + (1-p1)*p2*p3
	scores["1-2"] = p1*(1-p2)*(1-p3) + (1-p1)*p2*(1-p3)

	team1Win = scores["2-0"] + scores["2-1"]
	return team1Win, scores
}

func Bo1Probability(mapWinProbs []float64) float64 {
	if len(mapWinProbs) == 0 {
		return 0.5
	}
	return mapWinProbs[0]
}

func Ensemble(team1ProbElo, team1ProbForm, team1ProbH2H, team1ProbMaps float64) float64 {
	return EnsembleWeighted(team1ProbElo, team1ProbForm, team1ProbH2H, team1ProbMaps, DefaultWeights)
}

func EnsembleWeighted(team1ProbElo, team1ProbForm, team1ProbH2H, team1ProbMaps float64, w Weights) float64 {
	w = w.Normalize()
	p := w.Elo*team1ProbElo + w.Form*team1ProbForm + w.H2H*team1ProbH2H + w.Maps*team1ProbMaps
	return clamp(p, 0.05, 0.95)
}
