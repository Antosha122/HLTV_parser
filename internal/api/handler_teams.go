package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"psr/internal/logx"
	psync "psr/internal/sync"
)

func (s *Server) handleTeams(w http.ResponseWriter, r *http.Request) {
	search := strings.TrimSpace(r.URL.Query().Get("search"))
	limit := queryInt(r, "limit", 100)

	teams, err := s.db.ListTeamsWithMeta(search, limit)
	if err != nil {
		logx.Warn("api", "ListTeamsWithMeta: %v - fallback ListTeams", err)
		basic, err2 := s.db.ListTeams(search, limit)
		if err2 != nil {
			writeError(w, http.StatusInternalServerError, err2)
			return
		}
		writeJSON(w, basic)
		return
	}
	writeJSON(w, teams)
}

func (s *Server) handleTeamProfile(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil || id <= 0 {
		writeError(w, http.StatusBadRequest, fmt.Errorf("invalid team id"))
		return
	}

	forceSync := r.URL.Query().Get("sync") == "1"
	var syncErr string
	if forceSync {
		if psync.IsBusy() {
			syncErr = "another operation is running - wait for completion"
		} else {
			psync.BeginOperation("team", fmt.Sprintf("Team %d: loading from HLTV...", id))
			syncErr = s.runTeamSync(s.detachedCtx(), id)
			if syncErr != "" {
				psync.FailRefresh(fmt.Errorf("%s", syncErr))
			} else {
				psync.EndOperation("Team load complete")
			}
		}
	}

	profile, err := s.db.GetTeamProfile(id)
	if err != nil {
		writeError(w, http.StatusNotFound, fmt.Errorf("team not found - sync data first"))
		return
	}
	meta, _ := s.db.GetTeamMeta(id)
	out := teamProfileResponse(profile, meta, forceSync, syncErr)
	writeJSON(w, out)
}

func (s *Server) handleTeamSync(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil || id <= 0 {
		writeError(w, http.StatusBadRequest, fmt.Errorf("invalid team id"))
		return
	}
	if psync.IsBusy() {
		writeJSON(w, map[string]any{"started": false, "status": psync.CurrentStatus()})
		return
	}

	ctx := s.detachedCtx()
	go func() {
		psync.BeginOperation("team", fmt.Sprintf("Team %d: loading from HLTV...", id))
		if syncErr := s.runTeamSync(ctx, id); syncErr != "" {
			psync.FailRefresh(fmt.Errorf("%s", syncErr))
			return
		}
		psync.EndOperation("Team load complete")
	}()

	writeJSON(w, map[string]any{"started": true, "status": psync.CurrentStatus()})
}

func (s *Server) handleTeamsSyncAll(w http.ResponseWriter, r *http.Request) {
	if psync.IsBusy() {
		logx.Warn("api", "teams sync: another operation already running")
		writeJSON(w, map[string]any{"started": false, "status": psync.CurrentStatus()})
		return
	}
	psync.ClearStaleRunning()
	reqCtx := s.requestCtx(r)
	if err := s.ensureHLTV(reqCtx); err != nil {
		writeError(w, http.StatusBadRequest, s.hltvError(err))
		return
	}
	s.mu.Lock()
	syncer := s.syncer
	s.mu.Unlock()
	if syncer == nil {
		writeError(w, http.StatusServiceUnavailable, errors.New("sync service not ready"))
		return
	}

	parent := s.appCtx
	if parent == nil {
		parent = context.Background()
	}
	bgCtx := psync.NewRefreshContext(parent)
	go func() {
		psync.BeginRefresh()
		var res psync.RefreshResult
		var syncErr error
		defer func() {
			if syncErr != nil {
				if errors.Is(syncErr, context.Canceled) {
					psync.EndRefreshCancelled()
				} else {
					psync.FailRefresh(syncErr)
				}
			}
		}()
		teamsSynced, matchesAdded, err := syncer.SyncAllTeams(bgCtx)
		if err != nil {
			syncErr = err
			return
		}
		res.TeamsSaved = teamsSynced
		res.HistoryMatches = matchesAdded
		if logErr := s.db.LogSync("refresh", 0, "ok", fmt.Sprintf("teams=%d matches=%d", teamsSynced, matchesAdded)); logErr != nil {
			logx.Warn("api", "log sync write failed: %v", logErr)
		}
		psync.EndRefresh(res)
		logx.Info("api", "teams sync: %d teams, +%d matches", teamsSynced, matchesAdded)
	}()

	writeJSON(w, map[string]any{"started": true, "status": psync.CurrentStatus()})
}
