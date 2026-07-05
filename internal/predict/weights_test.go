package predict

import (
	"path/filepath"
	"testing"
)

func TestWeightsSaveLoad(t *testing.T) {
	path := filepath.Join(t.TempDir(), "w.json")
	w := Weights{Elo: 0.3, Form: 0.2, H2H: 0.1, Maps: 0.4}
	if err := SaveWeights(path, w); err != nil {
		t.Fatal(err)
	}
	loaded := LoadWeights(path)
	if loaded.Maps < 0.35 || loaded.Maps > 0.45 {
		t.Fatalf("loaded: %+v", loaded)
	}
}
