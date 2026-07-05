package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"psr/internal/api"
	"psr/internal/config"
	"psr/internal/hltv"
	"psr/internal/logx"
	"psr/internal/models"
	"psr/internal/platform"
	"psr/internal/predict"
	"psr/internal/storage"
	"psr/internal/sync"
)

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if len(args) == 0 {
		return cmdServe(ctx, nil, true)
	}

	switch args[0] {
	case "events":
		return cmdEvents(ctx, args[1:])
	case "teams":
		return cmdTeams(ctx, args[1:])
	case "sync":
		return cmdSync(ctx, args[1:])
	case "show":
		return cmdShow(ctx, args[1:])
	case "db":
		return cmdDB(ctx, args[1:])
	case "predict":
		return cmdPredict(ctx, args[1:])
	case "serve":
		return cmdServe(ctx, args[1:], false)
	case "backtest":
		return cmdBacktest(ctx, args[1:])
	case "calibrate":
		return cmdCalibrate(ctx, args[1:])
	case "help", "-h", "--help":
		printUsage()
		return 0
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n\n", args[0])
		printUsage()
		return 1
	}
}

func openDB(path string) (*storage.DB, error) {
	if path == "" {
		path = config.Load().DBPath
	}
	return storage.Open(path)
}

func cmdEvents(ctx context.Context, args []string) int {
	fs := flag.NewFlagSet("events", flag.ExitOnError)
	status := fs.String("status", "", "filter: ongoing, upcoming, past")
	fs.Parse(args)

	cfg := config.Load()
	client, err := hltv.NewClient(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "client error: %v\n", err)
		return 1
	}
	defer client.Close()

	fetchCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()

	events, err := client.GetEvents(fetchCtx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "fetch events: %v\n", err)
		fmt.Fprintln(os.Stderr, "\nTip: open hltv.org in browser, copy Cookie header into HLTV_COOKIE env var.")
		return 1
	}

	filter := strings.ToLower(strings.TrimSpace(*status))
	printed := 0
	for _, e := range events {
		if filter != "" && string(e.Status) != filter {
			continue
		}
		fmt.Printf("[%s] %d — %s\n", e.Status, e.ID, e.Name)
		printed++
	}

	if printed == 0 {
		fmt.Println("no events matched filter")
	} else {
		fmt.Printf("\nTotal: %d events\n", printed)
	}
	return 0
}

func cmdTeams(ctx context.Context, args []string) int {
	fs := flag.NewFlagSet("teams", flag.ExitOnError)
	search := fs.String("search", "", "team name to search")
	fs.Parse(args)

	if strings.TrimSpace(*search) == "" {
		fmt.Fprintln(os.Stderr, "teams: -search is required")
		return 1
	}

	cfg := config.Load()
	client, err := hltv.NewClient(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "client error: %v\n", err)
		return 1
	}
	defer client.Close()

	fetchCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()

	teams, err := client.SearchTeams(fetchCtx, *search)
	if err != nil {
		fmt.Fprintf(os.Stderr, "search teams: %v\n", err)
		return 1
	}

	if len(teams) == 0 {
		fmt.Println("no teams found")
		return 0
	}

	for _, t := range teams {
		fmt.Printf("%d — %s\n", t.ID, t.Name)
	}
	return 0
}

func cmdSync(ctx context.Context, args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "sync: specify team, match, or teams")
		return 1
	}

	fs := flag.NewFlagSet("sync", flag.ExitOnError)
	dbPath := fs.String("db", "", "sqlite database path")
	teamID := fs.Int("team", 0, "team id to sync")
	matchID := fs.Int("match", 0, "single match id to sync")
	team1 := fs.Int("team1", 0, "first team id (with -teams)")
	team2 := fs.Int("team2", 0, "second team id (with -teams)")
	months := fs.Int("months", 3, "months of history to load")
	fs.Parse(args[1:])

	cfg := config.Load()
	if *dbPath != "" {
		cfg.DBPath = *dbPath
	}

	db, err := storage.Open(cfg.DBPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "db error: %v\n", err)
		return 1
	}
	defer db.Close()

	client, err := hltv.NewClient(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "client error: %v\n", err)
		return 1
	}
	defer client.Close()

	svc := sync.New(client, db)
	fetchCtx, cancel := context.WithTimeout(ctx, 30*time.Minute)
	defer cancel()

	switch args[0] {
	case "team":
		if *teamID <= 0 {
			fmt.Fprintln(os.Stderr, "sync team: -team is required")
			return 1
		}
		if err := svc.Team(fetchCtx, sync.TeamOptions{TeamID: *teamID, Months: *months}); err != nil {
			fmt.Fprintf(os.Stderr, "sync team: %v\n", err)
			return 1
		}
	case "match":
		if *matchID <= 0 {
			fmt.Fprintln(os.Stderr, "sync match: -match is required")
			return 1
		}
		if err := svc.Match(fetchCtx, *matchID); err != nil {
			fmt.Fprintf(os.Stderr, "sync match: %v\n", err)
			return 1
		}
	case "teams":
		if *team1 <= 0 || *team2 <= 0 {
			fmt.Fprintln(os.Stderr, "sync teams: -team1 and -team2 are required")
			return 1
		}
		if err := svc.Teams(fetchCtx, *team1, *team2, *months); err != nil {
			fmt.Fprintf(os.Stderr, "sync teams: %v\n", err)
			return 1
		}
	default:
		// Legacy: psr sync -team 4608
		if *teamID > 0 {
			if err := svc.Team(fetchCtx, sync.TeamOptions{TeamID: *teamID, Months: *months}); err != nil {
				fmt.Fprintf(os.Stderr, "sync team: %v\n", err)
				return 1
			}
			return 0
		}
		if *matchID > 0 {
			if err := svc.Match(fetchCtx, *matchID); err != nil {
				fmt.Fprintf(os.Stderr, "sync match: %v\n", err)
				return 1
			}
			return 0
		}
		fmt.Fprintln(os.Stderr, "sync: use team, match, or teams subcommand")
		return 1
	}

	fmt.Println("sync completed")
	return 0
}

func cmdShow(ctx context.Context, args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "show: specify team, match, or h2h")
		return 1
	}

	switch args[0] {
	case "team", "match":
		fs := flag.NewFlagSet("show "+args[0], flag.ExitOnError)
		dbPath := fs.String("db", "", "sqlite database path")
		id := fs.Int("id", 0, "entity id")
		fs.Parse(args[1:])

		if *id <= 0 {
			fmt.Fprintf(os.Stderr, "show %s: -id is required\n", args[0])
			return 1
		}

		db, err := openDB(*dbPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "db error: %v\n", err)
			return 1
		}
		defer db.Close()

		if args[0] == "team" {
			team, err := db.GetTeam(*id)
			if err != nil {
				fmt.Fprintf(os.Stderr, "show team: %v\n", err)
				return 1
			}
			printTeam(team)
			return 0
		}

		match, err := db.GetMatch(*id)
		if err != nil {
			fmt.Fprintf(os.Stderr, "show match: %v\n", err)
			return 1
		}
		printMatch(match)
	case "h2h":
		fs := flag.NewFlagSet("show h2h", flag.ExitOnError)
		dbPath := fs.String("db", "", "sqlite database path")
		team1 := fs.Int("team1", 0, "first team id")
		team2 := fs.Int("team2", 0, "second team id")
		fs.Parse(args[1:])

		if *team1 <= 0 || *team2 <= 0 {
			fmt.Fprintln(os.Stderr, "show h2h: -team1 and -team2 required")
			return 1
		}

		db, err := openDB(*dbPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "db error: %v\n", err)
			return 1
		}
		defer db.Close()

		matches, err := db.GetH2H(*team1, *team2, 20)
		if err != nil {
			fmt.Fprintf(os.Stderr, "show h2h: %v\n", err)
			return 1
		}
		for _, m := range matches {
			fmt.Printf("%d: %s vs %s — %s (%s)\n", m.ID, m.Team1.Name, m.Team2.Name, m.Date.Format("2006-01-02"), m.Event)
		}
	default:
		fmt.Fprintln(os.Stderr, "show: unknown target")
		return 1
	}
	_ = ctx
	return 0
}

func cmdPredict(ctx context.Context, args []string) int {
	_ = ctx
	fs := flag.NewFlagSet("predict", flag.ExitOnError)
	dbPath := fs.String("db", "", "sqlite database path")
	team1 := fs.Int("team1", 0, "first team id")
	team2 := fs.Int("team2", 0, "second team id")
	format := fs.String("format", "bo3", "match format: bo1, bo3, bo5")
	fs.Parse(args)

	if *team1 <= 0 || *team2 <= 0 {
		fmt.Fprintln(os.Stderr, "predict: -team1 and -team2 are required")
		return 1
	}

	db, err := openDB(*dbPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "db error: %v\n", err)
		return 1
	}
	defer db.Close()

	cfg := config.Load()
	if *dbPath != "" {
		cfg.DBPath = *dbPath
	}
	pred, err := predict.NewWithWeightsPath(db, cfg.WeightsPath).Predict(predict.Options{
		Team1ID: *team1,
		Team2ID: *team2,
		Format:  models.MatchFormat(strings.ToLower(*format)),
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "predict: %v\n", err)
		fmt.Fprintln(os.Stderr, "\nTip: run sync first — psr sync teams -team1 X -team2 Y -months 3")
		return 1
	}

	printPrediction(pred)
	return 0
}

func printPrediction(p models.Prediction) {
	fmt.Printf("=== %s vs %s (%s) ===\n", p.Team1.Name, p.Team2.Name, p.Format)
	fmt.Printf("Confidence: %s\n\n", p.Confidence)

	fmt.Println("WIN PROBABILITY")
	fmt.Printf("  %s: %.1f%%\n", p.Team1.Name, p.WinProb.Team1)
	fmt.Printf("  %s: %.1f%%\n\n", p.Team2.Name, p.WinProb.Team2)

	if len(p.SeriesScores) > 0 {
		fmt.Println("SERIES SCORE")
		order := []string{"2-0", "2-1", "1-2", "0-2", "1-0", "0-1"}
		for _, k := range order {
			if v, ok := p.SeriesScores[k]; ok {
				fmt.Printf("  %s: %.1f%%\n", k, v)
			}
		}
		fmt.Println()
	}

	fmt.Println("MODEL BREAKDOWN (team1 win %)")
	fmt.Printf("  Elo:   %.1f%%\n", p.Breakdown.Elo)
	fmt.Printf("  Form:  %.1f%%\n", p.Breakdown.Form)
	fmt.Printf("  H2H:   %.1f%%\n", p.Breakdown.H2H)
	fmt.Printf("  Maps:  %.1f%%\n", p.Breakdown.Maps)
	fmt.Printf("  Final: %.1f%%\n\n", p.Breakdown.Final)

	fmt.Printf("FORM (last matches): %s %.1f%% (%d) | %s %.1f%% (%d)\n",
		p.Team1.Name, p.Form.Team1WinPct, p.Form.Team1Sample,
		p.Team2.Name, p.Form.Team2WinPct, p.Form.Team2Sample)
	fmt.Printf("H2H: %d games — %s %d / %s %d (%.1f%% for %s)\n\n",
		p.H2H.Total, p.Team1.Name, p.H2H.Team1Wins, p.Team2.Name, p.H2H.Team2Wins,
		p.H2H.Team1WinPct, p.Team1.Name)

	if len(p.Veto.Steps) > 0 {
		fmt.Println("PREDICTED VETO")
		for _, s := range p.Veto.Steps {
			team := s.TeamName
			if team == "" {
				team = "—"
			}
			fmt.Printf("  %d. [%s] %s → %s (%.0f%%)\n", s.Order, s.Action, team, s.MapName, s.Confidence*100)
		}
		fmt.Println()
	}

	fmt.Println("MAP POOL")
	for _, m := range p.Maps {
		marker := ""
		if m.InSeries {
			marker = " *"
		}
		role := ""
		if m.Role != "" {
			role = " [" + m.Role + "]"
		}
		fmt.Printf("  %s%s: %.1f%% for %s%s\n", m.MapName, role, m.Team1WinPct, p.Team1.Name, marker)
	}
}

func cmdServe(ctx context.Context, args []string, openUI bool) int {
	fs := flag.NewFlagSet("serve", flag.ExitOnError)
	addr := fs.String("addr", "", "listen address")
	dbPath := fs.String("db", "", "sqlite database path")
	if args != nil {
		fs.Parse(args)
	}

	cfg := config.Load()
	if *addr != "" {
		cfg.APIAddr = *addr
	}
	if *dbPath != "" {
		cfg.DBPath = *dbPath
	}

	db, err := storage.Open(cfg.DBPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "db error: %v\n", err)
		return 1
	}
	defer db.Close()

	engine := predict.NewWithWeightsPath(db, cfg.WeightsPath)

	srv, _ := api.New(db, engine, cfg)
	srv.SetAppContext(ctx)
	defer srv.Close()

	httpSrv := &http.Server{
		Addr:              cfg.APIAddr,
		Handler:           srv.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
		WriteTimeout:      30 * time.Minute,
	}

	errCh := make(chan error, 1)
	go func() {
		errCh <- httpSrv.ListenAndServe()
	}()

	go func() {
		<-ctx.Done()
		logx.Info("api", "остановка по Ctrl+C — прерываем загрузку HLTV")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Close()
		_ = httpSrv.Shutdown(shutdownCtx)
	}()

	appURL := appURL(cfg.APIAddr)
	if waitForServer(appURL, 5*time.Second) {
		if openUI {
			if err := platform.OpenChrome(appURL); err != nil {
				fmt.Fprintf(os.Stderr, "не удалось открыть браузер: %v\n", err)
				fmt.Printf("Откройте вручную: %s\n", appURL)
			}
		}
	}

	fmt.Printf("PSR запущен: %s\n", appURL)
	fmt.Println("Логи работы программы выводятся ниже в этом окне.")
	fmt.Println("Остановка: Ctrl+C (затем Y), или запустите stop.bat в другом окне")

	select {
	case <-ctx.Done():
		return 0
	case err := <-errCh:
		if err != nil && err != http.ErrServerClosed {
			fmt.Fprintf(os.Stderr, "serve: %v\n", err)
			return 1
		}
		return 0
	}
}

func appURL(addr string) string {
	host := addr
	if strings.HasPrefix(host, ":") {
		host = "127.0.0.1" + host
	}
	if !strings.HasPrefix(host, "http") {
		host = "http://" + host
	}
	return host
}

func waitForServer(url string, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	client := &http.Client{Timeout: time.Second}
	for time.Now().Before(deadline) {
		resp, err := client.Get(url + "/api/health")
		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return true
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	return false
}

func cmdBacktest(ctx context.Context, args []string) int {
	_ = ctx
	fs := flag.NewFlagSet("backtest", flag.ExitOnError)
	dbPath := fs.String("db", "", "sqlite database path")
	limit := fs.Int("samples", 15, "sample predictions to include")
	warmup := fs.Int("warmup", 30, "matches to skip at start")
	fs.Parse(args)

	cfg := config.Load()
	if *dbPath != "" {
		cfg.DBPath = *dbPath
	}

	db, err := storage.Open(cfg.DBPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "db error: %v\n", err)
		return 1
	}
	defer db.Close()

	res, err := predict.NewWithWeightsPath(db, cfg.WeightsPath).Backtest(predict.BacktestOptions{
		Limit:  *limit,
		Warmup: *warmup,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "backtest: %v\n", err)
		return 1
	}

	fmt.Printf("Backtest: %d matches\n", res.Total)
	fmt.Printf("Accuracy: %.1f%% (%d correct)\n", res.Accuracy, res.Correct)
	fmt.Printf("Log-loss: %.3f | Brier: %.3f\n", res.LogLoss, res.Brier)
	fmt.Printf("Weights: elo=%.2f form=%.2f h2h=%.2f maps=%.2f\n",
		res.Weights.Elo, res.Weights.Form, res.Weights.H2H, res.Weights.Maps)
	for format, st := range res.ByFormat {
		fmt.Printf("  %s: %.1f%% (%d/%d)\n", format, st.Accuracy, st.Correct, st.Total)
	}
	return 0
}

func cmdCalibrate(ctx context.Context, args []string) int {
	_ = ctx
	fs := flag.NewFlagSet("calibrate", flag.ExitOnError)
	dbPath := fs.String("db", "", "sqlite database path")
	weightsPath := fs.String("weights", "", "output weights json path")
	warmup := fs.Int("warmup", 30, "warmup matches")
	fs.Parse(args)

	cfg := config.Load()
	if *dbPath != "" {
		cfg.DBPath = *dbPath
	}
	if *weightsPath != "" {
		cfg.WeightsPath = *weightsPath
	}

	db, err := storage.Open(cfg.DBPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "db error: %v\n", err)
		return 1
	}
	defer db.Close()

	fmt.Println("Calibrating weights (grid search)...")
	res, err := predict.CalibrateAndSave(db, cfg.WeightsPath, predict.BacktestOptions{Warmup: *warmup})
	if err != nil {
		fmt.Fprintf(os.Stderr, "calibrate: %v\n", err)
		return 1
	}

	fmt.Printf("Iterations: %d\n", res.Iterations)
	fmt.Printf("Before: accuracy %.1f%% log-loss %.3f\n", res.Before.Accuracy, res.Before.LogLoss)
	fmt.Printf("After:  accuracy %.1f%% log-loss %.3f\n", res.After.Accuracy, res.After.LogLoss)
	fmt.Printf("Best weights saved to %s\n", cfg.WeightsPath)
	fmt.Printf("  elo=%.2f form=%.2f h2h=%.2f maps=%.2f\n",
		res.BestWeights.Elo, res.BestWeights.Form, res.BestWeights.H2H, res.BestWeights.Maps)
	return 0
}

func cmdDB(ctx context.Context, args []string) int {
	_ = ctx
	if len(args) == 0 || args[0] != "stats" {
		fmt.Fprintln(os.Stderr, "db: use db stats")
		return 1
	}

	fs := flag.NewFlagSet("db stats", flag.ExitOnError)
	dbPath := fs.String("db", "", "sqlite database path")
	fs.Parse(args[1:])

	db, err := openDB(*dbPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "db error: %v\n", err)
		return 1
	}
	defer db.Close()

	stats, err := db.Stats()
	if err != nil {
		fmt.Fprintf(os.Stderr, "db stats: %v\n", err)
		return 1
	}
	for k, v := range stats {
		fmt.Printf("%-16s %d\n", k+":", v)
	}
	return 0
}

func printTeam(t models.TeamDetail) {
	fmt.Printf("Team %d — %s\n", t.ID, t.Name)
	if t.WorldRank > 0 {
		fmt.Printf("World rank: #%d\n", t.WorldRank)
	}
	if t.HLTVRating > 0 {
		fmt.Printf("HLTV rating: %.0f\n", t.HLTVRating)
	}
	fmt.Printf("Players (%d):\n", len(t.Players))
	for _, p := range t.Players {
		fmt.Printf("  %d — %s\n", p.ID, p.Name)
	}
	fmt.Printf("Recent matches (%d):\n", len(t.RecentMatches))
	for _, m := range t.RecentMatches {
		winner := ""
		if m.WinnerID == m.Team1.ID {
			winner = m.Team1.Name
		} else if m.WinnerID == m.Team2.ID {
			winner = m.Team2.Name
		}
		fmt.Printf("  %d: %s vs %s", m.ID, m.Team1.Name, m.Team2.Name)
		if !m.Date.IsZero() {
			fmt.Printf(" (%s)", m.Date.Format("2006-01-02"))
		}
		if winner != "" {
			fmt.Printf(" — winner: %s", winner)
		}
		fmt.Println()
	}
}

func printMatch(m models.MatchDetail) {
	fmt.Printf("Match %d — %s vs %s\n", m.ID, m.Team1.Name, m.Team2.Name)
	if m.EventName != "" {
		fmt.Printf("Event: %s\n", m.EventName)
	}
	if !m.Date.IsZero() {
		fmt.Printf("Date: %s\n", m.Date.Format("2006-01-02 15:04"))
	}
	if m.Format != "" {
		fmt.Printf("Format: %s\n", m.Format)
	}
	if m.WinnerID > 0 {
		winner := m.Team1.Name
		if m.WinnerID == m.Team2.ID {
			winner = m.Team2.Name
		}
		fmt.Printf("Winner: %s\n", winner)
	}
	if len(m.Vetoes) > 0 {
		fmt.Println("Veto:")
		for _, v := range m.Vetoes {
			fmt.Printf("  %d. [%s] %s — %s\n", v.Order, v.Action, v.TeamName, v.MapName)
		}
	}
	if len(m.Maps) > 0 {
		fmt.Println("Maps:")
		for _, mp := range m.Maps {
			fmt.Printf("  %s: %d-%d\n", mp.MapName, mp.Team1Score, mp.Team2Score)
		}
	}
	if len(m.H2H) > 0 {
		fmt.Printf("H2H in DB (%d):\n", len(m.H2H))
		for _, h := range m.H2H {
			fmt.Printf("  %d: %s vs %s", h.ID, h.Team1.Name, h.Team2.Name)
			if !h.Date.IsZero() {
				fmt.Printf(" (%s)", h.Date.Format("2006-01-02"))
			}
			fmt.Println()
		}
	}
}

func printUsage() {
	fmt.Println(`PSR — CS2 match predictor (HLTV data)

Запуск UI (одна команда):
  go run ./cmd/psr
  или:  run.bat

Прочие команды:
  psr events [-status ongoing|upcoming|past]
  psr teams -search <name>

  psr sync team  -team <id> [-months 3] [-db path]
  psr sync match -match <id> [-db path]
  psr sync teams -team1 <id> -team2 <id> [-months 3]

  psr show team  -id <id> [-db path]
  psr show match -id <id> [-db path]
  psr show h2h   -team1 <id> -team2 <id>

  psr db stats [-db path]

  psr predict -team1 <id> -team2 <id> [-format bo3] [-db path]

  psr serve [-addr :8080] [-db path]
  psr backtest [-samples 15] [-warmup 30]
  psr calibrate [-weights data/weights.json]

Environment:
  PSR_DB_PATH=data/psr.db
  PSR_API_ADDR=:8080
  PSR_WEIGHTS_PATH=data/weights.json
  HLTV_USE_BROWSER=true
  HLTV_COOKIE=...
  HLTV_REQUEST_DELAY=2`)
	_ = strconv.Itoa(0)
}
