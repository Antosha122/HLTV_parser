package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"strconv"
	"strings"
	"sync"

	"psr/internal/config"
	"psr/internal/hltv"
	"psr/internal/logx"
	"psr/internal/platform"
	"psr/internal/models"
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

// detachedCtx is for long work that must not stop when the browser closes the HTTP request.
func (s *Server) detachedCtx() context.Context {
	if s.appCtx != nil {
		return s.appCtx
	}
	return context.Background()
}

func (s *Server) Handler() http.Handler {
	return cors(s.mux)
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
	logx.Info("api", "сохранение cookie от UI")
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
	logx.Info("api", "cookie принят, HLTV доступен")
	writeJSON(w, map[string]string{"status": "ok"})
}

func (s *Server) handleOpenHLTV(w http.ResponseWriter, r *http.Request) {
	url := strings.TrimRight(s.cfg.BaseURL, "/")
	logx.Info("api", "открытие HLTV в Chrome PSR: %s (профиль: %s)", url, s.cfg.ChromeProfileDir)
	if err := platform.OpenChromeWithProfile(url, s.cfg.ChromeProfileDir, s.cfg.ChromeDebugPort); err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Errorf("не удалось открыть Chrome: %w", err))
		return
	}
	writeJSON(w, map[string]string{"status": "opened"})
}

func (s *Server) handleConnect(w http.ResponseWriter, r *http.Request) {
	logx.Info("api", "подключение к HLTV через профиль Chrome")
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
	logx.Info("api", "подключение к HLTV успешно")
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

func (s *Server) handleRefresh(w http.ResponseWriter, r *http.Request) {
	logx.Info("api", "запрос обновления данных от UI")

	if psync.IsRefreshRunning() {
		st := psync.CurrentStatus()
		logx.Warn("api", "обновление уже выполняется: %s", st.Detail)
		writeJSON(w, map[string]any{"started": false, "status": st})
		return
	}
	psync.ClearStaleRunning()

	reqCtx := s.requestCtx(r)
	if err := s.ensureHLTV(reqCtx); err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			writeError(w, http.StatusRequestTimeout, errors.New("операция прервана"))
			return
		}
		writeError(w, http.StatusBadRequest, s.hltvError(err))
		return
	}

	s.mu.Lock()
	syncer := s.syncer
	s.mu.Unlock()
	if syncer == nil {
		writeError(w, http.StatusServiceUnavailable, errors.New("сервис синхронизации не готов"))
		return
	}

	// Фоновое обновление не должно использовать r.Context() — он отменяется
	// сразу после ответа HTTP, из‑за чего sync мгновенно прерывался.
	parent := s.appCtx
	if parent == nil {
		parent = context.Background()
	}
	bgCtx := psync.NewRefreshContext(parent)
	go func() {
		logx.Info("api", "фоновое обновление запущено")
		res, err := syncer.Refresh(bgCtx, psync.RefreshOptions{})
		if err != nil {
			if errors.Is(err, context.Canceled) {
				logx.Info("api", "обновление остановлено")
				return
			}
			if errors.Is(err, context.DeadlineExceeded) {
				logx.Info("api", "обновление прервано")
				return
			}
			logx.Error("api", "обновление не удалось: %v", err)
			return
		}
		logx.Info("api", "обновление завершено: турниров=%d матчей=%d история=%d",
			res.EventsSynced, res.MatchesSynced, res.HistoryMatches)
	}()

	writeJSON(w, map[string]any{"started": true, "status": psync.CurrentStatus()})
}

func (s *Server) handleRefreshEvents(w http.ResponseWriter, r *http.Request) {
	logx.Info("api", "запрос обновления только турниров от UI")

	if psync.IsBusy() {
		writeJSON(w, map[string]any{"started": false, "status": psync.CurrentStatus()})
		return
	}

	reqCtx := s.requestCtx(r)
	if err := s.ensureHLTV(reqCtx); err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			writeError(w, http.StatusRequestTimeout, errors.New("операция прервана"))
			return
		}
		writeError(w, http.StatusBadRequest, s.hltvError(err))
		return
	}

	s.mu.Lock()
	syncer := s.syncer
	s.mu.Unlock()
	if syncer == nil {
		writeError(w, http.StatusServiceUnavailable, errors.New("сервис синхронизации не готов"))
		return
	}

	parent := s.appCtx
	if parent == nil {
		parent = context.Background()
	}
	go func() {
		if _, err := syncer.RefreshEventsList(parent); err != nil {
			if !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
				logx.Error("api", "обновление турниров не удалось: %v", err)
			}
		}
	}()

	writeJSON(w, map[string]any{"started": true, "status": psync.CurrentStatus()})
}

func (s *Server) handleSyncStop(w http.ResponseWriter, r *http.Request) {
	if !psync.IsCancellableRefresh() {
		writeJSON(w, map[string]any{"stopped": false, "status": psync.CurrentStatus()})
		return
	}
	if psync.CancelRefresh() {
		logx.Info("api", "запрос остановки обновления от UI")
		writeJSON(w, map[string]any{"stopped": true, "status": psync.CurrentStatus()})
		return
	}
	writeJSON(w, map[string]any{"stopped": false, "status": psync.CurrentStatus()})
}

func (s *Server) handleSyncStatus(w http.ResponseWriter, r *http.Request) {
	st := psync.CurrentStatus()
	out := map[string]any{
		"running":            st.Running,
		"phase":              st.Phase,
		"detail":             st.Detail,
		"started_at":         st.StartedAt,
		"last_finished_at":   st.LastFinishedAt,
		"last_result":        st.LastResult,
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

func (s *Server) handleTeams(w http.ResponseWriter, r *http.Request) {
	search := strings.TrimSpace(r.URL.Query().Get("search"))
	limit := queryInt(r, "limit", 100)

	teams, err := s.db.ListTeamsWithMeta(search, limit)
	if err != nil {
		logx.Warn("api", "ListTeamsWithMeta: %v — fallback ListTeams", err)
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

func (s *Server) handleEvents(w http.ResponseWriter, r *http.Request) {
	search := strings.TrimSpace(r.URL.Query().Get("search"))
	events, err := s.db.ListEvents(search, queryInt(r, "limit", 50))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, events)
}

func (s *Server) handleEventTeams(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(r.PathValue("id"))
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
			syncErr = "другая операция уже выполняется — дождитесь завершения"
		} else {
			psync.BeginOperation("team", fmt.Sprintf("Команда %d: загрузка с HLTV...", id))
			syncErr = s.runTeamSync(s.detachedCtx(), id)
			if syncErr != "" {
				psync.FailRefresh(fmt.Errorf("%s", syncErr))
			} else {
				psync.EndOperation("Загрузка команды завершена")
			}
		}
	}

	profile, err := s.db.GetTeamProfile(id)
	if err != nil {
		writeError(w, http.StatusNotFound, fmt.Errorf("команда не найдена — сначала обновите данные"))
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
		psync.BeginOperation("team", fmt.Sprintf("Команда %d: загрузка с HLTV...", id))
		if syncErr := s.runTeamSync(ctx, id); syncErr != "" {
			psync.FailRefresh(fmt.Errorf("%s", syncErr))
			return
		}
		psync.EndOperation("Загрузка команды завершена")
	}()

	writeJSON(w, map[string]any{"started": true, "status": psync.CurrentStatus()})
}

func (s *Server) handleTeamsSyncAll(w http.ResponseWriter, r *http.Request) {
	if psync.IsBusy() {
		logx.Warn("api", "обновление команд: уже выполняется другая операция")
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
		writeError(w, http.StatusServiceUnavailable, errors.New("сервис синхронизации не готов"))
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
		_ = s.db.LogSync("refresh", 0, "ok", fmt.Sprintf("teams=%d matches=%d", teamsSynced, matchesAdded))
		psync.EndRefresh(res)
		logx.Info("api", "обновление всех команд: %d команд, +%d матчей", teamsSynced, matchesAdded)
	}()

	writeJSON(w, map[string]any{"started": true, "status": psync.CurrentStatus()})
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
		return "сервис синхронизации не готов"
	}
	if err := syncer.SyncTeamFull(ctx, teamID); err != nil {
		if errors.Is(err, context.Canceled) {
			return "загрузка прервана"
		}
		logx.Warn("api", "загрузка команды %d: %v", teamID, err)
		return err.Error()
	}
	if p, e := s.db.GetTeamProfile(teamID); e == nil && p.MatchesTotal == 0 {
		return "профиль сохранён, матчи не найдены — обновите cookie"
	}
	return ""
}

func teamProfileResponse(profile storage.TeamProfile, meta storage.TeamMeta, synced bool, syncErr string) map[string]any {
	out := map[string]any{
		"team":            profile.Team,
		"players":         profile.Players,
		"matches_total":   profile.MatchesTotal,
		"wins":            profile.Wins,
		"losses":          profile.Losses,
		"win_rate":        profile.WinRate,
		"recent_wins":     profile.RecentWins,
		"recent_total":    profile.RecentTotal,
		"recent_win_rate": profile.RecentWinRate,
		"map_stats":       profile.MapStats,
		"events":          profile.Events,
		"recent_matches":  profile.RecentMatches,
		"last_sync_at":    meta.LastSyncAt,
		"last_sync_status": meta.LastSyncStatus,
		"players_count":   meta.PlayersCount,
		"map_stats_count": meta.MapStatsCount,
		"stale":           meta.Stale,
		"synced":          synced && syncErr == "",
	}
	if syncErr != "" {
		out["sync_error"] = syncErr
	}
	return out
}

func (s *Server) handleDBClear(w http.ResponseWriter, r *http.Request) {
	logx.Warn("api", "очистка БД по запросу UI")
	if err := s.db.ClearAll(); err != nil {
		logx.Error("api", "очистка БД не удалась: %v", err)
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	stats, _ := s.db.Stats()
	logx.Info("api", "БД очищена: teams=%d matches=%d events=%d",
		stats["teams"], stats["matches"], stats["events"])
	writeJSON(w, map[string]any{"status": "ok", "db": stats})
}

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

	logx.Info("api", "прогноз: команда %d vs %d (%s)", req.Team1ID, req.Team2ID, format)
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
		psync.BeginOperation("predict", "Подготовка данных команд для прогноза...")
		defer psync.EndOperation("Прогноз готов")
		if err := s.ensureHLTV(workCtx); err != nil {
			writeError(w, http.StatusBadRequest, s.hltvError(err))
			return
		}
		if syncer != nil {
			if err := syncer.EnsureTeamsData(workCtx, req.Team1ID, req.Team2ID); err != nil {
				logx.Warn("api", "подготовка прогноза: %v", err)
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

func (s *Server) handleDBStats(w http.ResponseWriter, r *http.Request) {
	stats, err := s.db.Stats()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, stats)
}

func queryInt(r *http.Request, key string, fallback int) int {
	v := r.URL.Query().Get(key)
	if v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return fallback
	}
	return n
}

func readJSON(r *http.Request, dst any) error {
	defer r.Body.Close()
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		return err
	}
	return json.Unmarshal(body, dst)
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	_ = enc.Encode(v)
}

func writeError(w http.ResponseWriter, code int, err error) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
}

func cors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}
