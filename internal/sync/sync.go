package sync

import (
	"context"
	"fmt"

	"psr/internal/logx"
	"psr/internal/models"
)

// Service orchestrates syncing HLTV data into the storage layer.
//
// Each Service owns its own StatusTracker and CancelManager, so multiple
// instances can run in isolation (tests, multi-tenant deployments). The
// package-level wrappers below delegate to a default tracker/cancel manager
// only for backward compatibility with cmd/psr.
type Service struct {
	client  Source
	db      Store
	tracker *StatusTracker
	cancel  *CancelManager
}

// New creates a Service backed by the given Source and Store.
// A fresh StatusTracker and CancelManager are created for this instance.
func New(client Source, db Store) *Service {
	return &Service{
		client:  client,
		db:      db,
		tracker: NewStatusTracker(),
		cancel:  NewCancelManager(),
	}
}

// Tracker returns the Service's StatusTracker. Callers (api layer) should
// use it instead of the package-level global status functions.
func (s *Service) Tracker() *StatusTracker { return s.tracker }

// Cancel returns the Service's CancelManager.
func (s *Service) Cancel() *CancelManager { return s.cancel }

type TeamOptions struct {
	TeamID     int
	Months     int
	MaxMatches int
}

// logSyncBestEffort logs a sync result but never fails the caller — a logging
// error is reported via logx instead of propagating.
func (s *Service) logSyncBestEffort(entityType string, entityID int, status, message string) {
	if err := s.db.LogSync(entityType, entityID, status, message); err != nil {
		logx.Warn("sync", "sync_log write failed (%s %d %s): %v", entityType, entityID, status, err)
	}
}

func (s *Service) Team(ctx context.Context, opt TeamOptions) error {
	if opt.TeamID <= 0 {
		return fmt.Errorf("team id required")
	}

	teamName := ""
	if td, err := s.db.GetTeam(opt.TeamID); err == nil {
		teamName = td.Name
	}

	logx.Info("sync", "team %d (%s): /results?team=", opt.TeamID, teamName)
	s.tracker.SetTeamProgress(fmt.Sprintf("%s: /results?team=%d ...", teamName, opt.TeamID))

	summaries, err := s.client.GetTeamResultsWithFallback(ctx, opt.TeamID, teamName)
	if err != nil {
		s.logSyncBestEffort("team", opt.TeamID, "partial", err.Error())
		logx.Warn("sync", "team %d matches: %v (profile saved)", opt.TeamID, err)
		return nil
	}
	if len(summaries) == 0 {
		s.logSyncBestEffort("team", opt.TeamID, "partial", "no matches in range")
		logx.Warn("sync", "team %d: no matches found (profile saved)", opt.TeamID)
		return nil
	}
	summaries = FilterMatchSummaries(summaries)
	logx.Info("sync", "found %d matches for team %d", len(summaries), opt.TeamID)

	limit := len(summaries)
	if opt.MaxMatches > 0 && opt.MaxMatches < limit {
		limit = opt.MaxMatches
	}
	s.tracker.SetTeamProgress(fmt.Sprintf("Team %d: saving %d matches from results...", opt.TeamID, limit))
	synced, err := s.syncMatchSummaries(ctx, summaries, limit, func(i, total, id int) {
		s.tracker.SetTeamProgress(fmt.Sprintf("Team %d: match %d/%d (id %d)", opt.TeamID, i, total, id))
	})
	if err != nil {
		return err
	}

	s.logSyncBestEffort("team", opt.TeamID, "ok", fmt.Sprintf("matches=%d synced=%d", len(summaries), synced))
	logx.Info("sync", "team %d: synced %d/%d matches", opt.TeamID, synced, len(summaries))

	// Profile (players) — after matches, doesn't block the main task.
	if page, err := s.client.LoadTeamPage(ctx, opt.TeamID, teamName); err == nil {
		if saveErr := s.saveTeam(page.Detail); saveErr != nil {
			logx.Warn("sync", "save team %d profile: %v", opt.TeamID, saveErr)
		}
		if page.Detail.Name != "" {
			teamName = page.Detail.Name
		}
		logx.Info("sync", "team %s: profile, %d players", page.Detail.Name, len(page.Detail.Players))
	}

	start, end := MapStatsPeriod()
	s.syncTeamMapStatsFromMatches(ctx, opt.TeamID, teamName, start, end)

	return nil
}

func (s *Service) Match(ctx context.Context, matchID int) error {
	if matchID <= 0 {
		return fmt.Errorf("match id required")
	}

	detail, err := s.client.GetMatch(ctx, matchID)
	if err != nil {
		s.logSyncBestEffort("match", matchID, "error", err.Error())
		return fmt.Errorf("get match: %w", err)
	}

	if err := s.saveMatchFromDetail(ctx, detail); err != nil {
		s.logSyncBestEffort("match", matchID, "error", err.Error())
		return fmt.Errorf("save match: %w", err)
	}

	s.logSyncBestEffort("match", matchID, "ok", "")
	return nil
}

func (s *Service) Teams(ctx context.Context, team1ID, team2ID, months int) error {
	if err := s.Team(ctx, TeamOptions{TeamID: team1ID, Months: months}); err != nil {
		return err
	}
	if err := s.Team(ctx, TeamOptions{TeamID: team2ID, Months: months}); err != nil {
		return err
	}

	h2h, err := s.db.GetH2H(team1ID, team2ID, 50)
	if err != nil {
		return err
	}
	logx.Info("sync", "h2h %d vs %d: %d matches in DB", team1ID, team2ID, len(h2h))
	return nil
}

func (s *Service) saveTeam(team models.TeamDetail) error {
	if err := s.db.UpsertTeam(team.Team); err != nil {
		return fmt.Errorf("save team: %w", err)
	}
	for _, p := range team.Players {
		if err := s.db.UpsertPlayer(p); err != nil {
			return fmt.Errorf("save player %d: %w", p.ID, err)
		}
	}
	return nil
}
