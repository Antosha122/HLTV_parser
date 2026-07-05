package sync

import (
	"context"
	"fmt"
	"time"

	"psr/internal/logx"
)

// syncTeamMapStatsFromMatches loads /matches/ID pages (same HTTP path as results) and aggregates map W/L.
func (s *Service) syncTeamMapStatsFromMatches(ctx context.Context, teamID int, teamName string, start, end time.Time) {
	ids, err := s.db.ListTeamMatchesNeedingMaps(teamID, start)
	if err != nil {
		logx.Warn("sync", "команда %d: список матчей для карт: %v", teamID, err)
		return
	}

	if len(ids) > 0 {
		logx.Info("sync", "команда %d: загрузка карт из %d матчей (/matches/)", teamID, len(ids))
		SetTeamProgress(fmt.Sprintf("%s: карты из %d матчей...", teamName, len(ids)))

		withMaps := 0
		for i, matchID := range ids {
			if err := ctx.Err(); err != nil {
				return
			}
			SetTeamProgress(fmt.Sprintf("%s: карты %d/%d (матч %d)", teamName, i+1, len(ids), matchID))
			detail, err := s.client.GetMatch(ctx, matchID)
			if err != nil {
				logx.Warn("sync", "матч %d (карты): %v", matchID, err)
				continue
			}
			if len(detail.Maps) == 0 {
				continue
			}
			if err := s.saveMatchFromDetail(ctx, detail); err != nil {
				logx.Warn("sync", "матч %d: сохранение карт: %v", matchID, err)
				continue
			}
			withMaps++
		}
		logx.Info("sync", "команда %d: карты загружены из %d/%d матчей", teamID, withMaps, len(ids))
	}

	stats, err := s.db.AggregateTeamMapStats(teamID, start, end)
	if err != nil {
		logx.Warn("sync", "команда %d: агрегация карт: %v", teamID, err)
		return
	}
	if len(stats) == 0 {
		logx.Warn("sync", "команда %d: нет данных по картам за период", teamID)
		return
	}

	if err := s.db.UpsertTeamMapStats(stats, start, end); err != nil {
		logx.Warn("sync", "команда %d: сохранение карт: %v", teamID, err)
		return
	}
	logx.Info("sync", "команда %d: сохранено %d карт из матчей (%s — %s)",
		teamID, len(stats), start.Format(time.DateOnly), end.Format(time.DateOnly))
}
