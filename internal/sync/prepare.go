package sync

import (
	"context"
	"fmt"

	"psr/internal/logx"
)

// EnsureTeamsData loads team data from HLTV only when stale or incomplete in DB.
func (s *Service) EnsureTeamsData(ctx context.Context, team1ID, team2ID int) error {
	defer s.tracker.SetProgress("predict", "")

	for i, teamID := range []int{team1ID, team2ID} {
		stale, err := s.db.IsTeamStale(teamID)
		if err == nil && !stale {
			logx.Info("predict", "команда %d: данные в БД актуальны", teamID)
			continue
		}
		n, _ := s.db.CountTeamMatches(teamID)
		s.tracker.SetPredictProgress(fmt.Sprintf("[%d/2] Загрузка команды %d в БД (%d матчей)...", i+1, teamID, n))
		logx.Info("predict", "команда %d: устаревшие данные — полная загрузка с HLTV", teamID)

		if err := s.SyncTeamFull(ctx, teamID); err != nil {
			return fmt.Errorf("team %d: %w", teamID, err)
		}
	}
	return nil
}
