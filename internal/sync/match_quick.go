package sync

import (
	"context"
	"fmt"
	"strings"

	"psr/internal/logx"
	"psr/internal/models"
)

// saveMatchFromSummary stores match data from /results without fetching the match page.
func (s *Service) saveMatchFromSummary(ctx context.Context, summary models.MatchSummary) error {
	if summary.ID <= 0 {
		return nil
	}
	if !matchInHistoryRange(summary) {
		return nil
	}
	if err := s.ensureTeams(ctx, summary.Team1, summary.Team2); err != nil {
		return err
	}
	t1, err := s.resolveTeam(ctx, summary.Team1)
	if err != nil {
		return err
	}
	t2, err := s.resolveTeam(ctx, summary.Team2)
	if err != nil {
		return err
	}
	winnerID := summary.WinnerID
	if winnerID == 0 && summary.WinnerName != "" {
		if strings.EqualFold(t1.Name, summary.WinnerName) {
			winnerID = t1.ID
		} else if strings.EqualFold(t2.Name, summary.WinnerName) {
			winnerID = t2.ID
		}
	}
	if winnerID == 0 && summary.Score != "" {
		winnerID = inferWinnerFromScore(summary.Score, t1.ID, t2.ID)
	}
	detail := models.MatchDetail{
		Match: models.Match{
			ID:       summary.ID,
			Team1:    t1,
			Team2:    t2,
			Format:   summary.Format,
			Date:     summary.Date,
			WinnerID: winnerID,
		},
		EventName: summary.Event,
	}
	return s.db.UpsertMatch(detail)
}

// saveScheduledMatch stores an upcoming match (no winner/score) and links it to an event.
func (s *Service) saveScheduledMatch(ctx context.Context, summary models.MatchSummary, event models.Event) error {
	if summary.ID <= 0 || summary.Team1.ID <= 0 || summary.Team2.ID <= 0 {
		return nil
	}
	if isPlayedMatch(summary) {
		return s.saveMatchFromSummary(ctx, summary)
	}
	if existing, err := s.db.GetMatch(summary.ID); err == nil && existing.WinnerID > 0 {
		return nil
	}
	if err := s.ensureTeams(ctx, summary.Team1, summary.Team2); err != nil {
		return err
	}
	t1, err := s.resolveTeam(ctx, summary.Team1)
	if err != nil {
		return err
	}
	t2, err := s.resolveTeam(ctx, summary.Team2)
	if err != nil {
		return err
	}
	eventName := event.Name
	if eventName == "" {
		eventName = summary.Event
	}
	detail := models.MatchDetail{
		Match: models.Match{
			ID:       summary.ID,
			EventID:  event.ID,
			Team1:    t1,
			Team2:    t2,
			Format:   summary.Format,
			Date:     summary.Date,
			WinnerID: 0,
		},
		EventName: eventName,
	}
	if err := s.db.UpsertMatch(detail); err != nil {
		return err
	}
	if event.ID > 0 {
		_ = s.db.UpsertEvent(event)
		return s.db.SetMatchEvent(summary.ID, event.ID, eventName)
	}
	return nil
}

// saveMatchFromDetail stores a full match page (maps, vetoes) ensuring FK targets exist.
func (s *Service) saveMatchFromDetail(ctx context.Context, detail models.MatchDetail) error {
	if detail.ID <= 0 {
		return fmt.Errorf("match id required")
	}

	if existing, err := s.db.GetMatch(detail.ID); err == nil {
		if detail.Team1.ID <= 0 {
			detail.Team1 = existing.Team1
		}
		if detail.Team2.ID <= 0 {
			detail.Team2 = existing.Team2
		}
		if detail.WinnerID == 0 {
			detail.WinnerID = existing.WinnerID
		}
		if detail.Date.IsZero() {
			detail.Date = existing.Date
		}
		if detail.Format == "" {
			detail.Format = existing.Format
		}
		if detail.EventName == "" {
			detail.EventName = existing.EventName
		}
		if detail.EventID == 0 {
			detail.EventID = existing.EventID
		}
	}
	if detail.Team1.ID <= 0 || detail.Team2.ID <= 0 {
		return fmt.Errorf("match %d: teams not found", detail.ID)
	}

	if err := s.ensureTeams(ctx, detail.Team1, detail.Team2); err != nil {
		return err
	}
	t1, err := s.resolveTeam(ctx, detail.Team1)
	if err != nil {
		return err
	}
	t2, err := s.resolveTeam(ctx, detail.Team2)
	if err != nil {
		return err
	}
	detail.Team1 = t1
	detail.Team2 = t2

	if detail.EventID > 0 {
		name := detail.EventName
		if name == "" {
			name = fmt.Sprintf("Event %d", detail.EventID)
		}
		_ = s.db.UpsertEvent(models.Event{ID: detail.EventID, Name: name})
	}

	for i := range detail.Vetoes {
		if detail.Vetoes[i].TeamID == 0 {
			detail.Vetoes[i].TeamID = vetoTeamID(detail.Vetoes[i].TeamName, t1, t2)
		}
	}

	return s.db.UpsertMatch(detail)
}

func vetoTeamID(name string, team1, team2 models.Team) int {
	n := strings.ToLower(strings.TrimSpace(name))
	if n == "" {
		return 0
	}
	if strings.Contains(strings.ToLower(team1.Name), n) || strings.Contains(n, strings.ToLower(team1.Name)) {
		return team1.ID
	}
	if strings.Contains(strings.ToLower(team2.Name), n) || strings.Contains(n, strings.ToLower(team2.Name)) {
		return team2.ID
	}
	return 0
}

func inferWinnerFromScore(score string, team1ID, team2ID int) int {
	var s1, s2 int
	for i, part := range splitScore(score) {
		if i == 0 {
			s1 = parseIntLoose(part)
		}
		if i == 1 {
			s2 = parseIntLoose(part)
		}
	}
	if s1 == s2 || (s1 == 0 && s2 == 0) {
		return 0
	}
	if s1 > s2 {
		return team1ID
	}
	return team2ID
}

func splitScore(score string) []string {
	var out []string
	cur := ""
	for _, r := range score {
		if r == '-' || r == ':' || r == ' ' {
			if cur != "" {
				out = append(out, cur)
				cur = ""
			}
			continue
		}
		if r >= '0' && r <= '9' {
			cur += string(r)
		}
	}
	if cur != "" {
		out = append(out, cur)
	}
	return out
}

func parseIntLoose(s string) int {
	n := 0
	for _, r := range s {
		if r >= '0' && r <= '9' {
			n = n*10 + int(r-'0')
		}
	}
	return n
}

func (s *Service) syncMatchSummaries(ctx context.Context, summaries []models.MatchSummary, limit int, progress func(i, total int, id int)) (int, error) {
	if limit <= 0 || limit > len(summaries) {
		limit = len(summaries)
	}
	synced := 0
	summaries = FilterMatchSummaries(summaries)
	if limit > len(summaries) {
		limit = len(summaries)
	}
	for i, summary := range summaries {
		if err := ctx.Err(); err != nil {
			return synced, err
		}
		if i >= limit {
			break
		}
		if progress != nil {
			progress(i+1, limit, summary.ID)
		}
		if err := s.saveMatchFromSummary(ctx, summary); err != nil {
			logx.Warn("sync", "матч %d (results): %v", summary.ID, err)
			continue
		}
		synced++
	}
	return synced, nil
}
