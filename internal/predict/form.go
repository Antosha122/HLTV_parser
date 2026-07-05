package predict

func FormWinProbability(wins, total int) float64 {
	if total == 0 {
		return 0.5
	}
	return clamp((float64(wins)+2.0)/(float64(total)+4.0), 0.15, 0.85)
}

func H2HWinProbability(team1Wins, total int) float64 {
	if total == 0 {
		return 0.5
	}
	return clamp((float64(team1Wins)+1.0)/(float64(total)+2.0), 0.10, 0.90)
}
