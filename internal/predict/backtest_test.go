package predict

import (
	"path/filepath"
	"testing"
	"time"

	"psr/internal/models"
	"psr/internal/storage"
)

func TestBacktest(t *testing.T) {
	db, err := storage.Open(filepath.Join(t.TempDir(), "bt.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	t1 := models.Team{ID: 1, Name: "A"}
	t2 := models.Team{ID: 2, Name: "B"}
	_ = db.UpsertTeam(t1)
	_ = db.UpsertTeam(t2)

	base := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	for i := 0; i < 40; i++ {
		winner := t1.ID
		if i%3 == 0 {
			winner = t2.ID
		}
		_ = db.UpsertMatch(models.MatchDetail{
			Match: models.Match{
				ID:       100 + i,
				Team1:    t1,
				Team2:    t2,
				Format:   models.FormatBO3,
				Date:     base.AddDate(0, 0, i),
				WinnerID: winner,
			},
		})
	}

	res, err := New(db).Backtest(BacktestOptions{Limit: 5, Warmup: 10, MinHistory: 5})
	if err != nil {
		t.Fatal(err)
	}
	if res.Total == 0 {
		t.Fatal("expected backtest results")
	}
	if res.Accuracy <= 0 {
		t.Fatalf("accuracy: %f", res.Accuracy)
	}
}
