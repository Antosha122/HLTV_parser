package sync

import (
	"context"

	"psr/internal/logx"
	"psr/internal/models"
)

func (s *Service) SyncRankingTeams(ctx context.Context) ([]models.Team, error) {
	teams, err := s.client.GetRankingTeams(ctx)
	if err != nil {
		return nil, err
	}
	saved := 0
	for _, t := range teams {
		if err := ctx.Err(); err != nil {
			return teams, err
		}
		if t.ID <= 0 || t.Name == "" {
			continue
		}
		if err := s.db.UpsertTeam(t); err != nil {
			logx.Warn("sync", "рейтинг: команда %d: %v", t.ID, err)
			continue
		}
		saved++
	}
	logx.Info("sync", "рейтинг HLTV: %d команд сохранено", saved)
	return teams, nil
}
