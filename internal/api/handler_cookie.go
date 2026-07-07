package api

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	"psr/internal/hltv"
	"psr/internal/logx"
	"psr/internal/models"
	"psr/internal/platform"
)

func (s *Server) handleCookieStatus(w http.ResponseWriter, r *http.Request) {
	has, valid := false, false
	s.mu.Lock()
	client := s.client
	s.mu.Unlock()
	if client != nil {
		has, valid = client.CookieStatus(s.requestCtx(r))
	}
	writeJSON(w, map[string]any{
		"has_cookie": has,
		"valid":      valid,
	})
}

func (s *Server) handleCookie(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Cookie string `json:"cookie"`
	}
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	logx.Info("api", "cookie save from UI")
	if err := s.ensureHLTVClient(); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	s.mu.Lock()
	client := s.client
	s.mu.Unlock()
	if err := client.SetCookie(s.requestCtx(r), req.Cookie); err != nil {
		writeError(w, http.StatusBadRequest, errors.New(hltv.UserError(err)))
		return
	}
	logx.Info("api", "cookie accepted, HLTV available")
	writeJSON(w, map[string]string{"status": "ok"})
}

func (s *Server) handleOpenHLTV(w http.ResponseWriter, r *http.Request) {
	url := strings.TrimRight(s.cfg.BaseURL, "/")
	logx.Info("api", "opening HLTV in Chrome PSR: %s (profile: %s)", url, s.cfg.ChromeProfileDir)
	if err := platform.OpenChromeWithProfile(url, s.cfg.ChromeProfileDir, s.cfg.ChromeDebugPort); err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Errorf("failed to open Chrome: %w", err))
		return
	}
	writeJSON(w, map[string]string{"status": "opened"})
}

func (s *Server) handleConnect(w http.ResponseWriter, r *http.Request) {
	logx.Info("api", "connecting to HLTV via Chrome profile")
	if err := s.ensureHLTVClient(); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	s.mu.Lock()
	client := s.client
	s.mu.Unlock()
	if err := client.Connect(s.requestCtx(r)); err != nil {
		writeError(w, http.StatusBadRequest, errors.New(hltv.UserError(err)))
		return
	}
	logx.Info("api", "HLTV connection successful")
	writeJSON(w, map[string]string{"status": "ok"})
}

func (s *Server) ensureHLTVClient() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.client != nil {
		return nil
	}
	return s.initHLTV()
}

func (s *Server) handleDBClear(w http.ResponseWriter, r *http.Request) {
	logx.Warn("api", "DB clear requested from UI")
	if err := s.db.ClearAll(); err != nil {
		logx.Error("api", "DB clear failed: %v", err)
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	stats, _ := s.db.Stats()
	logx.Info("api", "DB cleared: teams=%d matches=%d events=%d",
		stats["teams"], stats["matches"], stats["events"])
	writeJSON(w, map[string]any{"status": "ok", "db": stats})
}

func (s *Server) handleEvents(w http.ResponseWriter, r *http.Request) {
	search := strings.TrimSpace(r.URL.Query().Get("search"))
	events, err := s.db.ListEventsPage(search, queryInt(r, "limit", 50), queryInt(r, "offset", 0))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, events)
}

func (s *Server) handleEventTeams(w http.ResponseWriter, r *http.Request) {
	id, err := parseIntPath(r, "id")
	if err != nil || id <= 0 {
		writeError(w, http.StatusBadRequest, fmt.Errorf("invalid event id"))
		return
	}

	teams, err := s.db.GetEventTeams(id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	if len(teams) == 0 {
		reqCtx := s.requestCtx(r)
		_ = s.ensureHLTV(reqCtx)
		s.mu.Lock()
		syncer := s.syncer
		s.mu.Unlock()
		if syncer != nil {
			event, err := s.db.GetEvent(id)
			if err != nil {
				event = models.Event{ID: id, Name: fmt.Sprintf("Event %d", id)}
			}
			if event.Syncable() {
				if _, err := syncer.SyncEvent(reqCtx, event); err == nil {
					teams, _ = s.db.GetEventTeams(id)
				}
			}
		}
	}

	writeJSON(w, teams)
}

func (s *Server) handleDBStats(w http.ResponseWriter, r *http.Request) {
	stats, err := s.db.Stats()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, stats)
}

func parseIntPath(r *http.Request, key string) (int, error) {
	return parseInt(r.PathValue(key))
}

func parseInt(v string) (int, error) {
	if v == "" {
		return 0, errors.New("empty value")
	}
	var n int
	_, err := fmt.Sscanf(v, "%d", &n)
	return n, err
}
