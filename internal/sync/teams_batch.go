package sync

import (
	"context"
	"fmt"
	"sync"
	"time"

	"psr/internal/logx"
	"psr/internal/models"
)

// HLTV rate-limits /results?team= when many teams sync in parallel — keep sequential.
const teamSyncWorkers = 1
const teamSyncPause = 5 * time.Second

// SyncTeamFull loads all data from HLTV into DB (players, maps, all results?team= matches).
func (s *Service) SyncTeamFull(ctx context.Context, teamID int) error {
	if teamID <= 0 {
		return fmt.Errorf("team id required")
	}
	s.tracker.SetTeamProgress(fmt.Sprintf("Команда %d: профиль и игроки...", teamID))
	logx.Info("sync", "полная загрузка команды %d с HLTV", teamID)

	return s.Team(ctx, TeamOptions{
		TeamID:     teamID,
		MaxMatches: 0,
	})
}

// SyncTeamsParallel syncs multiple teams concurrently.
func (s *Service) SyncTeamsParallel(ctx context.Context, teams []models.Team, workers int) int {
	if workers <= 0 {
		workers = teamSyncWorkers
	}
	if len(teams) == 0 {
		return 0
	}

	sem := make(chan struct{}, workers)
	var wg sync.WaitGroup
	var mu sync.Mutex
	var totalAdded int

	for i, team := range teams {
		if err := ctx.Err(); err != nil {
			break
		}
		if team.ID <= 0 {
			continue
		}
		wg.Add(1)
		go func(idx int, t models.Team) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			name := t.Name
			if name == "" {
				name = fmt.Sprintf("team-%d", t.ID)
			}
			s.tracker.SetProgress("history", fmt.Sprintf("[%d/%d] %s — загрузка с HLTV", idx+1, len(teams), name))

			before, _ := s.db.CountTeamMatches(t.ID)
			if err := s.SyncTeamFull(ctx, t.ID); err != nil {
				if ctx.Err() != nil {
					return
				}
				logx.Warn("sync", "команда %s: %v", name, err)
				return
			}
			after, _ := s.db.CountTeamMatches(t.ID)
			added := after - before
			if added < 0 {
				added = 0
			}
			mu.Lock()
			totalAdded += added
			mu.Unlock()
			logx.Info("sync", "команда %s: +%d матчей (всего %d)", name, added, after)

			if teamSyncPause > 0 && idx+1 < len(teams) {
				timer := time.NewTimer(teamSyncPause)
				select {
				case <-ctx.Done():
					timer.Stop()
				case <-timer.C:
				}
			}
		}(i, team)
	}
	wg.Wait()
	return totalAdded
}

// SyncAllTeams reloads every team in DB from HLTV (players, maps, all results?team= matches).
func (s *Service) SyncAllTeams(ctx context.Context) (teamsSynced int, matchesAdded int, err error) {
	ids, err := s.db.ListAllTeamIDs()
	if err != nil {
		return 0, 0, err
	}
	if len(ids) == 0 {
		return 0, 0, nil
	}

	teams := make([]models.Team, 0, len(ids))
	for _, id := range ids {
		if td, e := s.db.GetTeam(id); e == nil {
			teams = append(teams, td.Team)
		} else {
			teams = append(teams, models.Team{ID: id})
		}
	}

	s.tracker.SetProgress("teams", fmt.Sprintf("Обновление %d команд с HLTV (последовательно)...", len(teams)))
	matchesAdded = s.SyncTeamsParallel(ctx, teams, teamSyncWorkers)
	logx.Info("sync", "обновлено команд: %d, новых матчей: %d", len(teams), matchesAdded)
	return len(teams), matchesAdded, nil
}
