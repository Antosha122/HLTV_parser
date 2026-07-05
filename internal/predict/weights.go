package predict

import (
	"encoding/json"
	"os"
	"path/filepath"
)

type Weights struct {
	Elo  float64 `json:"elo"`
	Form float64 `json:"form"`
	H2H  float64 `json:"h2h"`
	Maps float64 `json:"maps"`
}

var DefaultWeights = Weights{Elo: 0.25, Form: 0.20, H2H: 0.15, Maps: 0.40}

func (w Weights) Normalize() Weights {
	sum := w.Elo + w.Form + w.H2H + w.Maps
	if sum <= 0 {
		return DefaultWeights
	}
	return Weights{
		Elo:  w.Elo / sum,
		Form: w.Form / sum,
		H2H:  w.H2H / sum,
		Maps: w.Maps / sum,
	}
}

func LoadWeights(path string) Weights {
	if path == "" {
		return DefaultWeights
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return DefaultWeights
	}
	var w Weights
	if err := json.Unmarshal(data, &w); err != nil {
		return DefaultWeights
	}
	return w.Normalize()
}

func SaveWeights(path string, w Weights) error {
	if dir := filepath.Dir(path); dir != "." && dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	w = w.Normalize()
	data, err := json.MarshalIndent(w, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}
