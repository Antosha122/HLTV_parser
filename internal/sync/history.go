package sync

import (
	"context"
	"strings"
	"time"

	"psr/internal/logx"
	"psr/internal/models"
	"psr/internal/storage"
)

const defaultHistoryMonths = 6
const defaultHistoryMatches = 80

func MatchHistoryStart() time.Time {
	return storage.MinMatchDate
}

func MatchHistoryEnd() time.Time {
	return time.Now().UTC()
}

func FilterMatchSummaries(summaries []models.MatchSummary) []models.MatchSummary {
	var out []models.MatchSummary
	for _, m := range summaries {
		if matchIsAnalyzable(m) {
			out = append(out, m)
		}
	}
	return out
}

func matchInHistoryRange(summary models.MatchSummary) bool {
	return matchIsAnalyzable(summary)
}

func matchIsAnalyzable(summary models.MatchSummary) bool {
	return isPlayedMatch(summary)
}

func isPlayedMatch(summary models.MatchSummary) bool {
	if summary.WinnerID > 0 {
		return true
	}
	score := strings.TrimSpace(summary.Score)
	if score == "" || score == "vs" || score == "-" || strings.EqualFold(score, "tba") {
		return false
	}
	return true
}

func (s *Service) syncTeamHistory(ctx context.Context, team models.Team) int {
	if team.ID <= 0 {
		var err error
		team, err = s.resolveTeam(ctx, team)
		if err != nil {
			logx.Warn("sync", "  история: %v", err)
			return 0
		}
	}
	stale, err := s.db.IsTeamStale(team.ID)
	if err == nil && !stale {
		logx.Info("sync", "  история %s: данные актуальны, пропуск", team.Name)
		return 0
	}

	before, _ := s.db.CountTeamMatches(team.ID)
	if err := s.SyncTeamFull(ctx, team.ID); err != nil {
		if ctx.Err() != nil {
			return 0
		}
		logx.Warn("sync", "  история %s: %v", team.Name, err)
		return 0
	}
	after, _ := s.db.CountTeamMatches(team.ID)
	added := after - before
	if added < 0 {
		added = 0
	}
	logx.Info("sync", "  история %s: +%d матчей (всего %d)", team.Name, added, after)
	return added
}

func (s *Service) filterStaleTeams(teams []models.Team) []models.Team {
	var out []models.Team
	for _, t := range teams {
		if t.ID <= 0 {
			continue
		}
		stale, err := s.db.IsTeamStale(t.ID)
		if err != nil || stale {
			out = append(out, t)
		}
	}
	return out
}

// filterTeamsNeedingHistory — команды без матчей или с неполной синхронизацией с /results?team=.
func (s *Service) filterTeamsNeedingHistory(teams []models.Team, limit int) []models.Team {
	var out []models.Team
	for _, t := range teams {
		if t.ID <= 0 {
			continue
		}
		n, err := s.db.CountTeamMatches(t.ID)
		if err != nil {
			out = append(out, t)
		} else if n == 0 {
			out = append(out, t)
		} else if meta, e := s.db.GetTeamMeta(t.ID); e == nil && meta.LastSyncStatus != "ok" {
			out = append(out, t)
		}
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	return out
}

func (s *Service) collectTeamsFromSummaries(ctx context.Context, summaries []models.MatchSummary) map[int]models.Team {
	out := make(map[int]models.Team)
	for _, summary := range summaries {
		for _, raw := range []models.Team{summary.Team1, summary.Team2} {
			if raw.ID <= 0 && raw.Name == "" {
				continue
			}
			t, err := s.resolveTeam(ctx, raw)
			if err != nil {
				continue
			}
			out[t.ID] = t
		}
	}
	return out
}
