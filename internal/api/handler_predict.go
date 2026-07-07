package api

import (
	"fmt"
	"net/http"
	"strings"

	"psr/internal/logx"
	"psr/internal/models"
	"psr/internal/predict"
)

func (s *Server) handlePredict(w http.ResponseWriter, r *http.Request) {
	var req predictRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if req.Team1ID <= 0 || req.Team2ID <= 0 {
		writeError(w, http.StatusBadRequest, fmt.Errorf("team1_id and team2_id required"))
		return
	}
	format := models.MatchFormat(strings.ToLower(req.Format))
	if format == "" {
		format = models.FormatBO3
	}

	logx.Info("api", "predict: team %d vs %d (%s)", req.Team1ID, req.Team2ID, format)
	workCtx := s.detachedCtx()

	s.mu.Lock()
	syncer := s.syncer
	s.mu.Unlock()
	needSync := false
	for _, id := range []int{req.Team1ID, req.Team2ID} {
		stale, err := s.db.IsTeamStale(id)
		if err != nil || stale {
			needSync = true
			break
		}
	}
	if needSync {
		tr := s.syncTracker()
		if tr != nil {
			tr.BeginOperation("predict", "Preparing team data for prediction...")
			defer tr.EndOperation("Prediction ready")
		}
		if err := s.ensureHLTV(workCtx); err != nil {
			writeError(w, http.StatusBadRequest, s.hltvError(err))
			return
		}
		if syncer != nil {
			if err := syncer.EnsureTeamsData(workCtx, req.Team1ID, req.Team2ID); err != nil {
				logx.Warn("api", "predict prep: %v", err)
			}
		}
	}

	pred, err := s.engine.Predict(predict.Options{
		Team1ID: req.Team1ID,
		Team2ID: req.Team2ID,
		Format:  format,
	})
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, pred)
}

func (s *Server) handleBacktest(w http.ResponseWriter, r *http.Request) {
	res, err := s.engine.Backtest(predict.BacktestOptions{
		Limit:      queryInt(r, "samples", 20),
		MinHistory: queryInt(r, "min_history", 20),
		Warmup:     queryInt(r, "warmup", 30),
	})
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, res)
}

func (s *Server) handleWeights(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, s.engine.Weights)
}
