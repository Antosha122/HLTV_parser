package api

import (
	"context"
	"errors"
	"net/http"

	"psr/internal/logx"
	psync "psr/internal/sync"
)

func (s *Server) handleRefresh(w http.ResponseWriter, r *http.Request) {
	logx.Info("api", "refresh requested from UI")

	tr := s.syncTracker()
	if tr != nil && tr.IsRefreshRunning() {
		st := tr.CurrentStatus()
		logx.Warn("api", "refresh already running: %s", st.Detail)
		writeJSON(w, map[string]any{"started": false, "status": st})
		return
	}
	if tr != nil {
		tr.ClearStaleRunning()
	}

	reqCtx := s.requestCtx(r)
	if err := s.ensureHLTV(reqCtx); err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			writeError(w, http.StatusRequestTimeout, errors.New("operation interrupted"))
			return
		}
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
		logx.Info("api", "background refresh started")
		res, err := syncer.Refresh(bgCtx, psync.RefreshOptions{})
		if err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				logx.Info("api", "refresh interrupted")
				return
			}
			logx.Error("api", "refresh failed: %v", err)
			return
		}
		logx.Info("api", "refresh done: events=%d matches=%d history=%d",
			res.EventsSynced, res.MatchesSynced, res.HistoryMatches)
	}()

	st := psync.Status{}
	if tr != nil {
		st = tr.CurrentStatus()
	}
	writeJSON(w, map[string]any{"started": true, "status": st})
}

func (s *Server) handleRefreshEvents(w http.ResponseWriter, r *http.Request) {
	logx.Info("api", "events-only refresh requested from UI")

	tr := s.syncTracker()
	if tr != nil && tr.IsBusy() {
		writeJSON(w, map[string]any{"started": false, "status": tr.CurrentStatus()})
		return
	}

	reqCtx := s.requestCtx(r)
	if err := s.ensureHLTV(reqCtx); err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			writeError(w, http.StatusRequestTimeout, errors.New("operation interrupted"))
			return
		}
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
	go func() {
		if _, err := syncer.RefreshEventsList(parent); err != nil {
			if !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
				logx.Error("api", "events refresh failed: %v", err)
			}
		}
	}()

	st := psync.Status{}
	if tr != nil {
		st = tr.CurrentStatus()
	}
	writeJSON(w, map[string]any{"started": true, "status": st})
}

func (s *Server) handleSyncStop(w http.ResponseWriter, r *http.Request) {
	tr := s.syncTracker()
	if tr == nil {
		writeJSON(w, map[string]any{"stopped": false, "status": psync.Status{}})
		return
	}
	if !tr.IsCancellableRefresh() {
		writeJSON(w, map[string]any{"stopped": false, "status": tr.CurrentStatus()})
		return
	}
	s.mu.Lock()
	syncer := s.syncer
	s.mu.Unlock()
	if syncer != nil && syncer.Cancel().Cancel() {
		logx.Info("api", "refresh stop requested from UI")
		writeJSON(w, map[string]any{"stopped": true, "status": tr.CurrentStatus()})
		return
	}
	writeJSON(w, map[string]any{"stopped": false, "status": tr.CurrentStatus()})
}

func (s *Server) handleSyncStatus(w http.ResponseWriter, r *http.Request) {
	st := psync.Status{}
	if tr := s.syncTracker(); tr != nil {
		st = tr.CurrentStatus()
	}
	out := map[string]any{
		"running":          st.Running,
		"phase":            st.Phase,
		"detail":           st.Detail,
		"started_at":       st.StartedAt,
		"last_finished_at": st.LastFinishedAt,
		"last_result":      st.LastResult,
	}

	if stats, err := s.db.Stats(); err == nil {
		out["db"] = stats
		out["teams"] = stats["teams"]
		out["matches"] = stats["matches"]
		out["events"] = stats["events"]
	}
	if at, status, msg, err := s.db.GetLastSync("refresh"); err == nil {
		out["last_refresh_at"] = at
		out["last_refresh_status"] = status
		out["last_refresh_message"] = msg
	}
	writeJSON(w, out)
}
