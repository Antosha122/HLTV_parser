package sync

import (
	"context"
	"errors"
	"testing"
	"time"

	"psr/internal/hltv"
	"psr/internal/models"
	"psr/internal/storage"
)

// fakeSource returns configurable responses. Zero values keep the legacy
// "empty" behaviour used by older tests; new fields let Refresh tests drive
// the service without a real network.
type fakeSource struct {
	events   []models.Event
	ranking  []models.Team
	team     models.TeamDetail
	match    models.MatchDetail
	teamPage hltv.TeamPageData

	// Configurable answers (nil -> empty/zero).
	eventResults  map[int][]models.MatchSummary
	eventMatches  map[int][]models.MatchSummary
	eventTeams    map[int][]models.Team
	teamResults   map[int][]models.MatchSummary
	eventsErr     error
	rankingErr    error
	eventResultsF func(eventID int) ([]models.MatchSummary, []models.Team, error)
}

func (f *fakeSource) GetEvents(ctx context.Context) ([]models.Event, error) {
	if f.eventsErr != nil {
		return nil, f.eventsErr
	}
	return f.events, nil
}
func (f *fakeSource) GetRankingTeams(ctx context.Context) ([]models.Team, error) {
	if f.rankingErr != nil {
		return nil, f.rankingErr
	}
	return f.ranking, nil
}
func (f *fakeSource) GetEventResults(ctx context.Context, eventID int) ([]models.MatchSummary, []models.Team, error) {
	if f.eventResultsF != nil {
		return f.eventResultsF(eventID)
	}
	if f.eventResults != nil {
		if ms, ok := f.eventResults[eventID]; ok {
			var teams []models.Team
			if f.eventTeams != nil {
				teams = f.eventTeams[eventID]
			}
			return ms, teams, nil
		}
	}
	return nil, nil, nil
}
func (f *fakeSource) GetEventMatches(ctx context.Context, event models.Event) ([]models.MatchSummary, error) {
	if f.eventMatches != nil {
		if ms, ok := f.eventMatches[event.ID]; ok {
			return ms, nil
		}
	}
	return nil, nil
}
func (f *fakeSource) GetEventParticipants(ctx context.Context, event models.Event) ([]models.Team, error) {
	if f.eventTeams != nil {
		if t, ok := f.eventTeams[event.ID]; ok {
			return t, nil
		}
	}
	return nil, nil
}
func (f *fakeSource) GetTeam(ctx context.Context, teamID int) (models.TeamDetail, error) {
	return f.team, nil
}
func (f *fakeSource) GetTeamByID(ctx context.Context, teamID int, name string) (models.TeamDetail, error) {
	return f.team, nil
}
func (f *fakeSource) GetTeamResultsWithFallback(ctx context.Context, teamID int, teamName string) ([]models.MatchSummary, error) {
	if f.teamResults != nil {
		if ms, ok := f.teamResults[teamID]; ok {
			return ms, nil
		}
	}
	return nil, nil
}
func (f *fakeSource) LoadTeamPage(ctx context.Context, teamID int, teamName string) (hltv.TeamPageData, error) {
	return f.teamPage, nil
}
func (f *fakeSource) GetMatch(ctx context.Context, matchID int) (models.MatchDetail, error) {
	return f.match, nil
}

type fakeStore struct {
	teams   map[int]models.Team
	matches map[int]models.MatchDetail
	logs    []logEntry
}

type logEntry struct {
	entityType string
	entityID   int
	status     string
	message    string
}

func newFakeStore() *fakeStore {
	return &fakeStore{
		teams:   make(map[int]models.Team),
		matches: make(map[int]models.MatchDetail),
	}
}

func (s *fakeStore) UpsertTeam(t models.Team) error         { s.teams[t.ID] = t; return nil }
func (s *fakeStore) UpsertPlayer(p models.Player) error     { return nil }
func (s *fakeStore) UpsertEvent(e models.Event) error       { return nil }
func (s *fakeStore) UpsertMatch(m models.MatchDetail) error { s.matches[m.ID] = m; return nil }
func (s *fakeStore) UpsertTeamMapStats(stats []models.TeamMapStat, periodStart, periodEnd time.Time) error {
	return nil
}
func (s *fakeStore) UpsertEventTeam(eventID, teamID int) error                  { return nil }
func (s *fakeStore) SetMatchEvent(matchID, eventID int, eventName string) error { return nil }
func (s *fakeStore) LogSync(entityType string, entityID int, status, message string) error {
	s.logs = append(s.logs, logEntry{entityType, entityID, status, message})
	return nil
}
func (s *fakeStore) GetTeam(id int) (models.TeamDetail, error) {
	if t, ok := s.teams[id]; ok {
		return models.TeamDetail{Team: t}, nil
	}
	return models.TeamDetail{}, errFakeNotFound
}
func (s *fakeStore) GetMatch(id int) (models.MatchDetail, error) {
	if m, ok := s.matches[id]; ok {
		return m, nil
	}
	return models.MatchDetail{}, errFakeNotFound
}
func (s *fakeStore) GetH2H(team1ID, team2ID, limit int) ([]models.MatchSummary, error) {
	return nil, nil
}
func (s *fakeStore) CountTeamMatches(teamID int) (int, error) { return 0, nil }
func (s *fakeStore) IsTeamStale(teamID int) (bool, error)     { return false, nil }
func (s *fakeStore) GetTeamMeta(teamID int) (storage.TeamMeta, error) {
	return storage.TeamMeta{}, nil
}
func (s *fakeStore) ListAllTeamIDs() ([]int, error) { return nil, nil }
func (s *fakeStore) ListTeams(search string, limit int) ([]models.Team, error) {
	return nil, nil
}
func (s *fakeStore) ListTeamMatchesNeedingMaps(teamID int, since time.Time) ([]int, error) {
	return nil, nil
}
func (s *fakeStore) AggregateTeamMapStats(teamID int, start, end time.Time) ([]models.TeamMapStat, error) {
	return nil, nil
}

var errFakeNotFound = errors.New("fake: not found")

func TestServiceTeamWithEmptyResults(t *testing.T) {
	src := &fakeSource{}
	store := newFakeStore()
	svc := New(src, store)

	err := svc.Team(context.Background(), TeamOptions{TeamID: 1, Months: 3})
	if err != nil {
		t.Fatalf("expected nil error for empty results, got %v", err)
	}
	if len(store.logs) == 0 {
		t.Fatal("expected at least one sync_log entry")
	}
	if store.logs[0].status != "partial" {
		t.Fatalf("expected partial status, got %q", store.logs[0].status)
	}
}

func TestServiceMatchSavesDetail(t *testing.T) {
	src := &fakeSource{
		match: models.MatchDetail{
			Match: models.Match{
				ID:    100,
				Team1: models.Team{ID: 1, Name: "A"},
				Team2: models.Team{ID: 2, Name: "B"},
			},
		},
	}
	store := newFakeStore()
	svc := New(src, store)

	if err := svc.Match(context.Background(), 100); err != nil {
		t.Fatalf("Match failed: %v", err)
	}
	if _, ok := store.matches[100]; !ok {
		t.Fatal("expected match 100 to be saved")
	}
}
