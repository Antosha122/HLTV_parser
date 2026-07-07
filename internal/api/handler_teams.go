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
	offset := queryInt(r, "offset", 0)

	teams, err := s.db.ListTeamsWithMeta(search, limit)
	if err != nil {
		logx.Warn("api", "ListTeamsWithMeta: %v - fallback ListTeams", err)
		basic, err2 := s.db.ListTeamsPage(search, limit, offset)
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
		tr := s.syncTracker()
		if tr != nil && tr.IsBusy() {
			syncErr = "another operation is running - wait for completion"
		} else {
			if tr != nil {
				tr.BeginOperation("team", fmt.Sprintf("Team %d: loading from HLTV...", id))
			}
			syncErr = s.runTeamSync(s.detachedCtx(), id)
			if tr != nil {
				if syncErr != "" {
					tr.FailRefresh(fmt.Errorf("%s", syncErr))
				} else {
					tr.EndOperation("Team load complete")
				}
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
	tr := s.syncTracker()
	if tr != nil && tr.IsBusy() {
		writeJSON(w, map[string]any{"started": false, "status": tr.CurrentStatus()})
		return
	}

	ctx := s.detachedCtx()
	go func() {
		if tr != nil {
			tr.BeginOperation("team", fmt.Sprintf("Team %d: loading from HLTV...", id))
		}
		if syncErr := s.runTeamSync(ctx, id); syncErr != "" {
			if tr != nil {
				tr.FailRefresh(fmt.Errorf("%s", syncErr))
			}
			return
		}
		if tr != nil {
			tr.EndOperation("Team load complete")
		}
	}()

	st := psync.Status{}
	if tr != nil {
		st = tr.CurrentStatus()
	}
	writeJSON(w, map[string]any{"started": true, "status": st})
}

func (s *Server) handleTeamsSyncAll(w http.ResponseWriter, r *http.Request) {
	tr := s.syncTracker()
	if tr != nil && tr.IsBusy() {
		logx.Warn("api", "teams sync: another operation already running")
		writeJSON(w, map[string]any{"started": false, "status": tr.CurrentStatus()})
		return
	}
	if tr != nil {
		tr.ClearStaleRunning()
	}
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
	bgCtx := syncer.Cancel().NewContext(parent)
	go func() {
		if tr != nil {
			tr.BeginRefresh()
		}
		var res psync.RefreshResult
		var syncErr error
		defer func() {
			if syncErr != nil {
				if tr != nil {
					if errors.Is(syncErr, context.Canceled) {
						tr.EndRefreshCancelled()
					} else {
						tr.FailRefresh(syncErr)
					}
					syncer.Cancel().Clear()
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
		if tr != nil {
			tr.EndRefresh(res)
		}
		syncer.Cancel().Clear()
		logx.Info("api", "teams sync: %d teams, +%d matches", teamsSynced, matchesAdded)
	}()

	st := psync.Status{}
	if tr != nil {
		st = tr.CurrentStatus()
	}
	writeJSON(w, map[string]any{"started": true, "status": st})
}
