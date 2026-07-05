package api

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"sync"

	"psr/internal/config"
	"psr/internal/hltv"
	"psr/internal/logx"
	"psr/internal/predict"
	"psr/internal/storage"
	psync "psr/internal/sync"
)

type Server struct {
	db     *storage.DB
	engine *predict.Engine
	mux    *http.ServeMux
	cfg    config.Config
	mu     sync.Mutex
	client *hltv.Client
	syncer *psync.Service
	appCtx context.Context
}

type predictRequest struct {
	Team1ID int    `json:"team1_id"`
	Team2ID int    `json:"team2_id"`
	Format  string `json:"format"`
}

func New(db *storage.DB, engine *predict.Engine, cfg config.Config) (*Server, error) {
	s := &Server{
		db:     db,
		engine: engine,
		cfg:    cfg,
		mux:    http.NewServeMux(),
	}
	_ = s.initHLTV()
	s.routes()
	return s, nil
}

func (s *Server) initHLTV() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.client != nil {
		_ = s.client.Close()
	}

	client, err := hltv.NewClient(s.cfg)
	if err != nil {
		return err
	}
	s.client = client
	s.syncer = psync.New(client, s.db)
	return nil
}

func (s *Server) ensureHLTV(ctx context.Context) error {
	s.mu.Lock()
	client := s.client
	if client == nil {
		s.mu.Unlock()
		if err := s.initHLTV(); err != nil {
			return err
		}
		s.mu.Lock()
		client = s.client
	}
	s.mu.Unlock()
	return client.EnsureSession(ctx)
}

func (s *Server) hltvError(err error) error {
	if err == nil {
		return nil
	}
	return errors.New(hltv.UserError(err))
}

func (s *Server) SetAppContext(ctx context.Context) {
	s.appCtx = ctx
}

func (s *Server) requestCtx(r *http.Request) context.Context {
	if s.appCtx == nil {
		return r.Context()
	}
	return mergeContexts(s.appCtx, r.Context())
}

func (s *Server) detachedCtx() context.Context {
	if s.appCtx != nil {
		return s.appCtx
	}
	return context.Background()
}

func (s *Server) Handler() http.Handler {
	var h http.Handler = s.mux
	h = recoverer(h)
	h = cors(s.cfg.AllowedOrigins)(h)
	return h
}

func (s *Server) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.client != nil {
		return s.client.Close()
	}
	return nil
}

func (s *Server) routes() {
	s.mux.HandleFunc("GET /api/health", s.handleHealth)
	s.mux.HandleFunc("GET /api/teams", s.handleTeams)
	s.mux.HandleFunc("GET /api/teams/{id}/profile", s.handleTeamProfile)
	s.mux.HandleFunc("POST /api/teams/{id}/sync", s.handleTeamSync)
	s.mux.HandleFunc("POST /api/teams/sync-all", s.handleTeamsSyncAll)
	s.mux.HandleFunc("GET /api/events", s.handleEvents)
	s.mux.HandleFunc("GET /api/events/{id}/teams", s.handleEventTeams)
	s.mux.HandleFunc("POST /api/db/clear", s.handleDBClear)
	s.mux.HandleFunc("POST /api/refresh", s.handleRefresh)
	s.mux.HandleFunc("POST /api/refresh/events", s.handleRefreshEvents)
	s.mux.HandleFunc("POST /api/sync/stop", s.handleSyncStop)
	s.mux.HandleFunc("GET /api/sync/status", s.handleSyncStatus)
	s.mux.HandleFunc("GET /api/cookie/status", s.handleCookieStatus)
	s.mux.HandleFunc("POST /api/cookie", s.handleCookie)
	s.mux.HandleFunc("POST /api/hltv/open", s.handleOpenHLTV)
	s.mux.HandleFunc("POST /api/connect", s.handleConnect)
	s.mux.HandleFunc("GET /api/logs", s.handleLogs)
	s.mux.HandleFunc("GET /api/logs/stream", s.handleLogsStream)
	s.mux.HandleFunc("POST /api/predict", s.handlePredict)
	s.mux.HandleFunc("GET /api/backtest", s.handleBacktest)
	s.mux.HandleFunc("GET /api/weights", s.handleWeights)
	s.mux.HandleFunc("GET /api/db/stats", s.handleDBStats)

	static, _ := fs.Sub(staticFS, "static")
	fileServer := http.FileServer(http.FS(static))
	s.mux.Handle("GET /{$}", fileServer)
	s.mux.Handle("GET /index.html", fileServer)
	s.mux.Handle("GET /static/", http.StripPrefix("/static/", fileServer))
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, map[string]string{"status": "ok"})
}

func (s *Server) handleLogs(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, logx.History())
}

func (s *Server) handleLogsStream(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, errors.New("streaming not supported"))
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	ch, cancel := logx.Subscribe()
	defer cancel()

	ctx := r.Context()
	for {
		select {
		case <-ctx.Done():
			return
		case line, open := <-ch:
			if !open {
				return
			}
			fmt.Fprintf(w, "data: %s\n\n", line)
			flusher.Flush()
		}
	}
}

func teamProfileResponse(profile storage.TeamProfile, meta storage.TeamMeta, synced bool, syncErr string) map[string]any {
	out := map[string]any{
		"team":             profile.Team,
		"players":          profile.Players,
		"matches_total":    profile.MatchesTotal,
		"wins":             profile.Wins,
		"losses":           profile.Losses,
		"win_rate":         profile.WinRate,
		"recent_wins":      profile.RecentWins,
		"recent_total":     profile.RecentTotal,
		"recent_win_rate":  profile.RecentWinRate,
		"map_stats":        profile.MapStats,
		"events":           profile.Events,
		"recent_matches":   profile.RecentMatches,
		"last_sync_at":     meta.LastSyncAt,
		"last_sync_status": meta.LastSyncStatus,
		"players_count":    meta.PlayersCount,
		"map_stats_count":  meta.MapStatsCount,
		"stale":            meta.Stale,
		"synced":           synced && syncErr == "",
	}
	if syncErr != "" {
		out["sync_error"] = syncErr
	}
	return out
}

func (s *Server) runTeamSync(ctx context.Context, teamID int) string {
	defer psync.ClearTeamProgress()
	if err := s.ensureHLTV(ctx); err != nil {
		return hltv.UserError(err)
	}
	s.mu.Lock()
	syncer := s.syncer
	s.mu.Unlock()
	if syncer == nil {
		return "sync service not ready"
	}
	if err := syncer.SyncTeamFull(ctx, teamID); err != nil {
		if errors.Is(err, context.Canceled) {
			return "sync interrupted"
		}
		logx.Warn("api", "team sync %d: %v", teamID, err)
		return err.Error()
	}
	if p, e := s.db.GetTeamProfile(teamID); e == nil && p.MatchesTotal == 0 {
		return "profile saved, no matches found - update cookie"
	}
	return ""
}

var ()
