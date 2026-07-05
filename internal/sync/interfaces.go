package sync

import (
	"context"
	"time"

	"psr/internal/hltv"
	"psr/internal/models"
	"psr/internal/storage"
)

// Source is the subset of the HLTV client that sync.Service consumes.
// Defining it here (consumer side) lets us test sync with fixtures/mocks
// and swap implementations (HTTP, CDP, fixtures) without touching the service.
type Source interface {
	GetEvents(ctx context.Context) ([]models.Event, error)
	GetRankingTeams(ctx context.Context) ([]models.Team, error)
	GetEventResults(ctx context.Context, eventID int) ([]models.MatchSummary, []models.Team, error)
	GetEventMatches(ctx context.Context, event models.Event) ([]models.MatchSummary, error)
	GetEventParticipants(ctx context.Context, event models.Event) ([]models.Team, error)
	GetTeam(ctx context.Context, teamID int) (models.TeamDetail, error)
	GetTeamByID(ctx context.Context, teamID int, name string) (models.TeamDetail, error)
	GetTeamResultsWithFallback(ctx context.Context, teamID int, teamName string) ([]models.MatchSummary, error)
	LoadTeamPage(ctx context.Context, teamID int, teamName string) (hltv.TeamPageData, error)
	GetMatch(ctx context.Context, matchID int) (models.MatchDetail, error)
}

// Store is the subset of storage.DB that sync.Service consumes.
type Store interface {
	UpsertTeam(t models.Team) error
	UpsertPlayer(p models.Player) error
	UpsertEvent(e models.Event) error
	UpsertMatch(m models.MatchDetail) error
	UpsertTeamMapStats(stats []models.TeamMapStat, periodStart, periodEnd time.Time) error
	UpsertEventTeam(eventID, teamID int) error
	SetMatchEvent(matchID, eventID int, eventName string) error
	LogSync(entityType string, entityID int, status, message string) error

	GetTeam(id int) (models.TeamDetail, error)
	GetMatch(id int) (models.MatchDetail, error)
	GetH2H(team1ID, team2ID, limit int) ([]models.MatchSummary, error)

	CountTeamMatches(teamID int) (int, error)
	IsTeamStale(teamID int) (bool, error)
	GetTeamMeta(teamID int) (storage.TeamMeta, error)
	ListAllTeamIDs() ([]int, error)
	ListTeams(search string, limit int) ([]models.Team, error)
	ListTeamMatchesNeedingMaps(teamID int, since time.Time) ([]int, error)
	AggregateTeamMapStats(teamID int, start, end time.Time) ([]models.TeamMapStat, error)
}
