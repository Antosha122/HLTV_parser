package predict

import (
	"time"

	"psr/internal/models"
	"psr/internal/storage"
)

// TeamProvider gives access to team-related reads the engine needs.
// Defined here (consumer side) following the Go idiom "accept interfaces,
// return structs". The storage.DB satisfies it implicitly.
type TeamProvider interface {
	GetTeam(id int) (models.TeamDetail, error)
}

// MatchProvider exposes historical match reads used by Elo/form/H2H.
type MatchProvider interface {
	GetMatchesChronological(limit int) ([]storage.MatchRecord, error)
	GetMatchesBefore(asOf time.Time, limit int) ([]storage.MatchRecord, error)
	GetTeamForm(teamID, limit int) (wins, total int, err error)
	GetTeamFormBefore(teamID, limit int, asOf time.Time) (wins, total int, err error)
	GetH2H(team1ID, team2ID, limit int) ([]models.MatchSummary, error)
	GetH2HBefore(team1ID, team2ID, limit int, asOf time.Time) ([]models.MatchSummary, error)
}

// MapProfileProvider exposes map stats and vetoes used to build map profiles.
type MapProfileProvider interface {
	GetTeamMapStatsForTeam(teamID int) ([]models.TeamMapStat, error)
	GetTeamVetoes(teamID int, limit int) ([]storage.VetoRecord, error)
}

// EloCacheProvider exposes the persisted Elo cache so ComputeElo doesn't have
// to run over the full match history on every prediction. When the cache is
// stale (e.g. after new matches were synced) the engine recomputes and stores
// the updated ratings.
type EloCacheProvider interface {
	GetEloRatings() (map[int]float64, error)
	EloCacheFresh() (bool, error)
	SaveEloRatings(ratings map[int]float64) error
}

// DataProvider bundles everything predict.Engine needs from storage.
// storage.DB implements this interface; tests can supply a fake.
type DataProvider interface {
	TeamProvider
	MatchProvider
	MapProfileProvider
	EloCacheProvider
}
