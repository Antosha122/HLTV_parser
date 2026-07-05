package sync

import (
	"context"
	"fmt"

	"psr/internal/hltv"
	"psr/internal/logx"
	"psr/internal/models"
	"psr/internal/storage"
)

type Service struct {
	client *hltv.Client
	db     *storage.DB
}

func New(client *hltv.Client, db *storage.DB) *Service {
	return &Service{client: client, db: db}
}

type TeamOptions struct {
	TeamID     int
	Months     int
	MaxMatches int
}

func (s *Service) Team(ctx context.Context, opt TeamOptions) error {
	if opt.TeamID <= 0 {
		return fmt.Errorf("team id required")
	}

	teamName := ""
	if td, err := s.db.GetTeam(opt.TeamID); err == nil {
		teamName = td.Name
	}

	logx.Info("sync", "команда %d (%s): /results?team=", opt.TeamID, teamName)
	SetTeamProgress(fmt.Sprintf("%s: /results?team=%d ...", teamName, opt.TeamID))

	summaries, err := s.client.GetTeamResultsWithFallback(ctx, opt.TeamID, teamName)
	if err != nil {
		_ = s.db.LogSync("team", opt.TeamID, "partial", err.Error())
		logx.Warn("sync", "матчи команды %d: %v (профиль сохранён)", opt.TeamID, err)
		return nil
	}
	if len(summaries) == 0 {
		_ = s.db.LogSync("team", opt.TeamID, "partial", "no matches in range")
		logx.Warn("sync", "команда %d: матчи не найдены (профиль сохранён)", opt.TeamID)
		return nil
	}
	summaries = FilterMatchSummaries(summaries)
	logx.Info("sync", "найдено %d матчей для команды %d", len(summaries), opt.TeamID)

	limit := len(summaries)
	if opt.MaxMatches > 0 && opt.MaxMatches < limit {
		limit = opt.MaxMatches
	}
	SetTeamProgress(fmt.Sprintf("Команда %d: сохранение %d матчей из results...", opt.TeamID, limit))
	synced, err := s.syncMatchSummaries(ctx, summaries, limit, func(i, total, id int) {
		SetTeamProgress(fmt.Sprintf("Команда %d: матч %d/%d (id %d)", opt.TeamID, i, total, id))
	})
	if err != nil {
		return err
	}

	_ = s.db.LogSync("team", opt.TeamID, "ok", fmt.Sprintf("matches=%d synced=%d", len(summaries), synced))
	logx.Info("sync", "команда %d: синхронизировано %d/%d матчей", opt.TeamID, synced, len(summaries))

	// Профиль (игроки) — после матчей, не блокирует основную задачу.
	if page, err := s.client.LoadTeamPage(ctx, opt.TeamID, teamName); err == nil {
		_ = s.saveTeam(page.Detail)
		if page.Detail.Name != "" {
			teamName = page.Detail.Name
		}
		logx.Info("sync", "команда %s: профиль, игроков %d", page.Detail.Name, len(page.Detail.Players))
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
		_ = s.db.LogSync("match", matchID, "error", err.Error())
		return fmt.Errorf("get match: %w", err)
	}

	if err := s.saveMatchFromDetail(ctx, detail); err != nil {
		_ = s.db.LogSync("match", matchID, "error", err.Error())
		return fmt.Errorf("save match: %w", err)
	}

	_ = s.db.LogSync("match", matchID, "ok", "")
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
	logx.Info("sync", "h2h %d vs %d: %d матчей в БД", team1ID, team2ID, len(h2h))
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

