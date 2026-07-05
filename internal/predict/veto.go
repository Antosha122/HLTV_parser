package predict

import (
	"sort"

	"psr/internal/models"
)

// Bo3VetoOrder: A ban, B ban, A pick, B pick, A ban, B ban, decider.
var Bo3VetoOrder = []struct {
	Action   models.VetoAction
	TeamSlot int // 0 = team1, 1 = team2
}{
	{models.VetoBan, 0},
	{models.VetoBan, 1},
	{models.VetoPick, 0},
	{models.VetoPick, 1},
	{models.VetoBan, 0},
	{models.VetoBan, 1},
}

type vetoState struct {
	remaining []string
	steps     []models.VetoStep
}

func SimulateBo3Veto(
	team1, team2 models.Team,
	p1, p2 map[string]models.MapProfile,
) models.VetoPrediction {
	pool := vetoMapPool(p1, p2)
	state := vetoState{remaining: pool}

	order := 0
	for _, step := range Bo3VetoOrder {
		order++
		slot := step.TeamSlot

		var team models.Team
		var self, opp map[string]models.MapProfile
		if slot == 0 {
			team, self, opp = team1, p1, p2
		} else {
			team, self, opp = team2, p2, p1
		}

		mapName, conf := chooseMap(step.Action, self, opp, state.remaining)
		state.steps = append(state.steps, models.VetoStep{
			Order:      order,
			Action:     step.Action,
			TeamID:     team.ID,
			TeamName:   team.Name,
			MapName:    mapName,
			Confidence: conf,
		})
		state.remaining = removeMap(state.remaining, mapName)
	}

	if len(state.remaining) == 1 {
		order++
		state.steps = append(state.steps, models.VetoStep{
			Order:      order,
			Action:     models.VetoDecider,
			MapName:    state.remaining[0],
			Confidence: 0.95,
		})
	}

	return models.VetoPrediction{Steps: state.steps}
}

func chooseMap(action models.VetoAction, self, opp map[string]models.MapProfile, remaining []string) (string, float64) {
	type scored struct {
		name  string
		score float64
	}
	var candidates []scored

	for _, m := range remaining {
		sp := self[m]
		op := opp[m]
		adv := smoothedWR(sp.Wins, sp.Losses) - smoothedWR(op.Wins, op.Losses)
		pickBias := sp.PickRate * 0.08
		banBias := sp.BanRate * 0.08

		var score float64
		switch action {
		case models.VetoBan:
			score = -adv + banBias
		case models.VetoPick:
			score = adv + pickBias
		}
		candidates = append(candidates, scored{name: m, score: score})
	}

	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].score > candidates[j].score
	})

	if len(candidates) == 0 {
		return "", 0
	}
	top := candidates[0]
	conf := 0.55
	if len(candidates) > 1 {
		gap := top.score - candidates[1].score
		conf = clamp(0.5+gap*2, 0.45, 0.92)
	}
	return top.name, conf
}

func removeMap(pool []string, name string) []string {
	out := make([]string, 0, len(pool)-1)
	for _, m := range pool {
		if m != name {
			out = append(out, m)
		}
	}
	return out
}

func SeriesMapsFromVeto(v models.VetoPrediction) []string {
	var maps []string
	for _, s := range v.Steps {
		if s.Action == models.VetoPick || s.Action == models.VetoDecider {
			maps = append(maps, s.MapName)
		}
	}
	return maps
}
