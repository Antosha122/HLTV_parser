package sync

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	"psr/internal/hltv"
	"psr/internal/logx"
	"psr/internal/models"
)

type RefreshResult struct {
	EventsFetched  int      `json:"events_fetched"`
	EventsSynced   int      `json:"events_synced"`
	TeamsSaved     int      `json:"teams_saved"`
	MatchesSynced  int      `json:"matches_synced"`
	HistoryMatches int      `json:"history_matches"`
	Messages       []string `json:"messages"`
	FinishedAt     string   `json:"finished_at,omitempty"`
}

type RefreshOptions struct {
	SyncOngoing bool
	MaxEvents   int
	MaxMatches  int
}

func (s *Service) Refresh(ctx context.Context, opt RefreshOptions) (RefreshResult, error) {
	if opt.MaxEvents <= 0 {
		opt.MaxEvents = 5
	}
	if opt.MaxMatches <= 0 {
		opt.MaxMatches = 120
	}

	s.tracker.BeginRefresh()
	var res RefreshResult
	var refreshErr error
	defer func() {
		if refreshErr != nil {
			if errors.Is(refreshErr, context.Canceled) {
				s.tracker.EndRefreshCancelled()
			} else {
				s.tracker.FailRefresh(refreshErr)
			}
			s.cancel.Clear()
		}
	}()

	logx.Info("sync", "начало обновления (макс. турниров: %d, матчей: %d)", opt.MaxEvents, opt.MaxMatches)
	s.tracker.SetProgress("events", "Загрузка списка турниров с HLTV...")

	events, err := s.client.GetEvents(ctx)
	if err != nil {
		refreshErr = err
		return res, err
	}
	res.EventsFetched = len(events)
	logx.Info("sync", "получено турниров с HLTV: %d", len(events))

	s.tracker.SetProgress("ranking", "Загрузка рейтинга команд HLTV...")
	rankingTeams, rankErr := s.SyncRankingTeams(ctx)
	if rankErr != nil {
		logx.Warn("sync", "рейтинг HLTV: %v", rankErr)
		res.Messages = append(res.Messages, "ranking: "+rankErr.Error())
	} else {
		res.TeamsSaved += len(rankingTeams)
	}

	s.tracker.SetProgress("events", "Сохранение турниров в БД...")
	for _, e := range events {
		if e.Status != models.EventStatusOngoing && e.Status != models.EventStatusUpcoming {
			continue
		}
		if err := s.db.UpsertEvent(e); err == nil {
			res.EventsSynced++
		}
	}
	logx.Info("sync", "турниров сохранено в БД: %d (ongoing + upcoming)", res.EventsSynced)

	var ongoing, upcomingFeatured, pastMajors []models.Event
	for _, e := range events {
		if hltv.IsJunkEventName(e.Name) {
			continue
		}
		switch e.Status {
		case models.EventStatusOngoing:
			ongoing = append(ongoing, e)
		case models.EventStatusUpcoming:
			if e.Featured || isRecentMajorEvent(e.Name) {
				upcomingFeatured = append(upcomingFeatured, e)
			}
		case models.EventStatusPast:
			if isRecentMajorEvent(e.Name) {
				pastMajors = append(pastMajors, e)
			}
		}
	}
	sort.Slice(ongoing, func(i, j int) bool {
		pi, pj := eventSyncPriority(ongoing[i]), eventSyncPriority(ongoing[j])
		if pi != pj {
			return pi > pj
		}
		return ongoing[i].Name < ongoing[j].Name
	})
	sort.Slice(upcomingFeatured, func(i, j int) bool {
		pi, pj := eventSyncPriority(upcomingFeatured[i]), eventSyncPriority(upcomingFeatured[j])
		if pi != pj {
			return pi > pj
		}
		return upcomingFeatured[i].Name < upcomingFeatured[j].Name
	})
	sort.Slice(pastMajors, func(i, j int) bool {
		return pastMajors[i].Name > pastMajors[j].Name
	})

	slotsForOngoing := opt.MaxEvents
	if len(pastMajors) > 0 && slotsForOngoing > 0 {
		slotsForOngoing--
	}
	if len(upcomingFeatured) > 0 && slotsForOngoing > 0 {
		slotsForOngoing--
	}
	target := append([]models.Event{}, ongoing...)
	if len(target) > slotsForOngoing {
		target = target[:slotsForOngoing]
	}
	if len(upcomingFeatured) > 0 {
		target = append(target, upcomingFeatured[0])
	}
	if len(pastMajors) > 0 {
		target = append(target, pastMajors[0])
	}
	logx.Info("sync", "турниров к синхронизации: %d (ongoing≤%d, upcoming=%d, past=%d)",
		len(target), slotsForOngoing, len(upcomingFeatured), len(pastMajors))

	historyTeams := make(map[int]models.Team)
	for _, t := range rankingTeams {
		if t.ID > 0 {
			historyTeams[t.ID] = t
		}
	}

	matchesLeft := opt.MaxMatches
	for i, e := range target {
		if err := ctx.Err(); err != nil {
			refreshErr = err
			s.logSyncBestEffort("refresh", 0, "cancelled", "user stop")
			return res, err
		}
		s.tracker.SetProgress("event", fmt.Sprintf("[%d/%d] %s", i+1, len(target), e.Name))
		logx.Info("sync", "загрузка данных [%s] id=%d status=%s", e.Name, e.ID, e.Status)

		n, msg, teams := s.syncEvent(ctx, e, matchesLeft)
		res.TeamsSaved += n.teams
		res.MatchesSynced += n.matches
		matchesLeft -= n.matches
		for _, t := range teams {
			if t.ID > 0 {
				historyTeams[t.ID] = t
			}
		}
		logx.Info("sync", "  → команд: %d, матчей: %d", n.teams, n.matches)
		if msg != "" {
			res.Messages = append(res.Messages, msg)
		}
		if matchesLeft <= 0 {
			logx.Warn("sync", "достигнут лимит матчей, остановка")
			res.Messages = append(res.Messages, "match limit reached, run refresh again for more")
			break
		}
	}

	if res.EventsSynced == 0 && len(rankingTeams) == 0 {
		refreshErr = fmt.Errorf("no events or ranking synced from HLTV")
		return res, refreshErr
	}

	if len(historyTeams) > 0 {
		teamList := make([]models.Team, 0, len(historyTeams))
		for _, t := range historyTeams {
			teamList = append(teamList, t)
		}
		sort.Slice(teamList, func(i, j int) bool {
			if teamList[i].WorldRank > 0 && teamList[j].WorldRank > 0 && teamList[i].WorldRank != teamList[j].WorldRank {
				return teamList[i].WorldRank < teamList[j].WorldRank
			}
			return teamList[i].Name < teamList[j].Name
		})

		s.tracker.SetProgress("history", fmt.Sprintf("Матчи %d команд: /results?team= ...", len(teamList)))
		logx.Info("sync", "история: загрузка матчей для %d команд с /results?team=", len(teamList))
		res.HistoryMatches = s.SyncTeamsParallel(ctx, teamList, teamSyncWorkers)
	}

	logx.Info("sync", "готово: турниров=%d команд=%d матчей=%d история=%d",
		res.EventsSynced, res.TeamsSaved, res.MatchesSynced, res.HistoryMatches)
	s.logSyncBestEffort("refresh", 0, "ok", fmt.Sprintf("events=%d matches=%d history=%d", res.EventsSynced, res.MatchesSynced, res.HistoryMatches))
	s.tracker.EndRefresh(res)
	s.cancel.Clear()
	return res, nil
}

type eventSyncCount struct {
	teams   int
	matches int
}

func (s *Service) SyncEvent(ctx context.Context, event models.Event) (RefreshResult, error) {
	var res RefreshResult
	if !event.Syncable() {
		return res, fmt.Errorf("турнир %q нельзя синхронизировать (статус: %s)", event.Name, event.Status)
	}
	logx.Info("sync", "синхронизация турнира: %s (%d)", event.Name, event.ID)
	if err := s.db.UpsertEvent(event); err != nil {
		return res, err
	}
	res.EventsSynced = 1
	n, msg, _ := s.syncEvent(ctx, event, 200)
	res.TeamsSaved = n.teams
	res.MatchesSynced = n.matches
	if msg != "" {
		res.Messages = append(res.Messages, msg)
	}
	return res, nil
}

func (s *Service) syncEvent(ctx context.Context, event models.Event, maxMatches int) (eventSyncCount, string, []models.Team) {
	var count eventSyncCount
	if !event.Syncable() {
		return count, fmt.Sprintf("%s: турнир не синхронизируется (статус %s)", event.Name, event.Status), nil
	}

	var summaries []models.MatchSummary
	var teams []models.Team

	if event.Analyzable() {
		logx.Info("sync", "  загрузка /results?event=%d", event.ID)
		resSummaries, resTeams, err := s.client.GetEventResults(ctx, event.ID)
		if err != nil {
			logx.Warn("sync", "  /results?event=%d: %v", event.ID, err)
		} else {
			summaries = FilterMatchSummaries(resSummaries)
			teams = resTeams
			logx.Info("sync", "  /results?event=%d: %d матчей, %d команд", event.ID, len(summaries), len(teams))
		}
	}

	var eventPageMatches []models.MatchSummary
	logx.Info("sync", "  загрузка /events/%d/matches", event.ID)
	if pageMatches, err := s.client.GetEventMatches(ctx, event); err != nil {
		logx.Warn("sync", "  матчи турнира: %v", err)
	} else if len(pageMatches) > 0 {
		eventPageMatches = pageMatches
		logx.Info("sync", "  /events/%d/matches: %d матчей", event.ID, len(pageMatches))
		for _, m := range pageMatches {
			if m.Event == "" {
				m.Event = event.Name
			}
			if isPlayedMatch(m) {
				summaries = appendUniqueSummary(summaries, m)
			}
		}
	}

	if pageTeams, err := s.client.GetEventParticipants(ctx, event); err != nil {
		logx.Warn("sync", "  участники турнира: %v", err)
	} else if len(pageTeams) > 0 {
		teams = mergeEventTeams(teams, pageTeams)
		logx.Info("sync", "  участников на странице турнира: %d", len(pageTeams))
	}

	logx.Info("sync", "  матчей (сыгранных): %d, команд: %d", len(summaries), len(teams))

	resolvedTeams := make(map[int]models.Team)
	count.teams = s.saveEventTeams(ctx, event, teams, resolvedTeams)
	if count.teams > 0 {
		logx.Info("sync", "  сохранено команд участников: %d", count.teams)
	}

	synced := 0
	for i, summary := range summaries {
		if err := ctx.Err(); err != nil {
			partial := make([]models.Team, 0, len(resolvedTeams))
			for _, t := range resolvedTeams {
				partial = append(partial, t)
			}
			return count, "", partial
		}
		if i >= maxMatches {
			break
		}
		logx.Info("sync", "  матч %d/%d: id=%d %s vs %s",
			i+1, min(len(summaries), maxMatches), summary.ID, summary.Team1.Name, summary.Team2.Name)
		if err := s.ensureTeams(ctx, summary.Team1, summary.Team2); err != nil {
			logx.Warn("sync", "  команды матча %d: %v", summary.ID, err)
		} else {
			for _, raw := range []models.Team{summary.Team1, summary.Team2} {
				t, err := s.resolveTeam(ctx, raw)
				if err == nil && t.ID > 0 {
					if _, known := resolvedTeams[t.ID]; !known {
						resolvedTeams[t.ID] = t
						if err := s.db.UpsertTeam(t); err == nil {
							count.teams++
						}
						s.logEventTeamBestEffort(event.ID, t.ID)
					}
				}
			}
		}
		if err := s.saveMatchFromSummary(ctx, summary); err != nil {
			logx.Warn("sync", "  матч %d (results): %v — пробуем полную страницу", summary.ID, err)
			if err := s.MatchWithEvent(ctx, summary.ID, event); err != nil {
				logx.Warn("sync", "  матч %d: %v", summary.ID, err)
				continue
			}
		} else {
			s.logEventBestEffort(event)
			if err := s.db.SetMatchEvent(summary.ID, event.ID, event.Name); err != nil {
				logx.Warn("sync", "  матч %d: связь с турниром: %v", summary.ID, err)
			}
		}
		synced++
	}
	count.matches = synced

	if len(eventPageMatches) > 0 {
		scheduled := 0
		for _, m := range eventPageMatches {
			if isPlayedMatch(m) {
				continue
			}
			if err := s.saveScheduledMatch(ctx, m, event); err != nil {
				logx.Warn("sync", "  предстоящий матч %d: %v", m.ID, err)
				continue
			}
			for _, raw := range []models.Team{m.Team1, m.Team2} {
				if raw.ID > 0 {
					if _, known := resolvedTeams[raw.ID]; !known {
						resolvedTeams[raw.ID] = raw
						s.logEventTeamBestEffort(event.ID, raw.ID)
					}
				}
			}
			scheduled++
		}
		if scheduled > 0 {
			logx.Info("sync", "  предстоящих матчей сохранено: %d", scheduled)
		}
	}

	allTeams := make([]models.Team, 0, len(resolvedTeams))
	for _, t := range resolvedTeams {
		allTeams = append(allTeams, t)
	}

	return count, "", allTeams
}

func appendUniqueSummary(summaries []models.MatchSummary, m models.MatchSummary) []models.MatchSummary {
	for _, existing := range summaries {
		if existing.ID == m.ID {
			return summaries
		}
	}
	return append(summaries, m)
}

func isRecentMajorEvent(name string) bool {
	low := strings.ToLower(name)
	return strings.Contains(low, "major") ||
		strings.Contains(low, "iem cologne") ||
		strings.Contains(low, "iem katowice") ||
		strings.Contains(low, "blast") && strings.Contains(low, "final")
}

func mergeEventTeams(base, extra []models.Team) []models.Team {
	seen := make(map[int]struct{})
	seenName := make(map[string]struct{})
	var out []models.Team
	add := func(t models.Team) {
		if t.ID > 0 {
			if _, ok := seen[t.ID]; ok {
				return
			}
			seen[t.ID] = struct{}{}
			out = append(out, t)
			return
		}
		if t.Name == "" {
			return
		}
		key := strings.ToLower(t.Name)
		if _, ok := seenName[key]; ok {
			return
		}
		seenName[key] = struct{}{}
		out = append(out, t)
	}
	for _, t := range base {
		add(t)
	}
	for _, t := range extra {
		add(t)
	}
	return out
}

func (s *Service) saveEventTeams(ctx context.Context, event models.Event, teams []models.Team, resolved map[int]models.Team) int {
	saved := 0
	for _, raw := range teams {
		if err := ctx.Err(); err != nil {
			break
		}
		if raw.ID <= 0 {
			continue
		}
		t := raw
		if t.Name == "" {
			if detail, err := s.client.GetTeamByID(ctx, t.ID, ""); err == nil {
				t = detail.Team
			}
		}
		if _, known := resolved[t.ID]; known {
			continue
		}
		resolved[t.ID] = t
		if err := s.db.UpsertTeam(t); err != nil {
			logx.Warn("sync", "  сохранение команды %d: %v", t.ID, err)
			continue
		}
		s.logEventTeamBestEffort(event.ID, t.ID)
		saved++
	}
	return saved
}

func eventSyncPriority(e models.Event) int {
	p := 0
	switch e.Status {
	case models.EventStatusOngoing:
		p += 10000
	case models.EventStatusUpcoming:
		p += 8000
	}
	if e.Featured {
		p += 3000
	}
	low := strings.ToLower(e.Name)
	if strings.Contains(low, "major") {
		p += 2000
	}
	if strings.Contains(low, "iem") {
		p += 1000
	}
	if strings.Contains(low, "2027") || strings.Contains(low, "2028") {
		p -= 1500
	}
	return p
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func (s *Service) MatchWithEvent(ctx context.Context, matchID int, event models.Event) error {
	if err := s.Match(ctx, matchID); err != nil {
		return err
	}
	s.logEventBestEffort(event)
	return s.db.SetMatchEvent(matchID, event.ID, event.Name)
}

// logEventTeamBestEffort links a team to an event but never fails the caller —
// a linkage error is reported via logx instead of propagating.
func (s *Service) logEventTeamBestEffort(eventID, teamID int) {
	if err := s.db.UpsertEventTeam(eventID, teamID); err != nil {
		logx.Warn("sync", "event_team link (%d, %d): %v", eventID, teamID, err)
	}
}

// logEventBestEffort upserts an event but never fails the caller.
func (s *Service) logEventBestEffort(event models.Event) {
	if err := s.db.UpsertEvent(event); err != nil {
		logx.Warn("sync", "event upsert (%d): %v", event.ID, err)
	}
}
