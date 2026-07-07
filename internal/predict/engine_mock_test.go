package predict

import (
	"testing"
	"time"

	"psr/internal/models"
	"psr/internal/storage"
)

// fakeDataProvider is an in-memory DataProvider for testing Engine without SQLite.
type fakeDataProvider struct {
	teams    map[int]models.TeamDetail
	matches  []storage.MatchRecord
	h2h      map[string][]models.MatchSummary
	form     map[int]formResult
	mapStats map[int][]models.TeamMapStat
	vetoes   map[int][]storage.VetoRecord
	eloCache map[int]float64
	eloFresh bool
}

type formResult struct {
	wins  int
	total int
}

func newFakeDataProvider() *fakeDataProvider {
	return &fakeDataProvider{
		teams:    make(map[int]models.TeamDetail),
		h2h:      make(map[string][]models.MatchSummary),
		form:     make(map[int]formResult),
		mapStats: make(map[int][]models.TeamMapStat),
		vetoes:   make(map[int][]storage.VetoRecord),
	}
}

func (f *fakeDataProvider) GetTeam(id int) (models.TeamDetail, error) {
	if t, ok := f.teams[id]; ok {
		return t, nil
	}
	return models.TeamDetail{}, errFakeTeamNotFound
}

func (f *fakeDataProvider) GetMatchesChronological(limit int) ([]storage.MatchRecord, error) {
	if limit > 0 && limit < len(f.matches) {
		return f.matches[:limit], nil
	}
	return f.matches, nil
}

func (f *fakeDataProvider) GetMatchesBefore(asOf time.Time, limit int) ([]storage.MatchRecord, error) {
	var out []storage.MatchRecord
	for _, m := range f.matches {
		if m.Date.Before(asOf) {
			out = append(out, m)
		}
	}
	if limit > 0 && limit < len(out) {
		return out[:limit], nil
	}
	return out, nil
}

func (f *fakeDataProvider) GetTeamForm(teamID, limit int) (wins, total int, err error) {
	r := f.form[teamID]
	return r.wins, r.total, nil
}

func (f *fakeDataProvider) GetTeamFormBefore(teamID, limit int, asOf time.Time) (wins, total int, err error) {
	r := f.form[teamID]
	return r.wins, r.total, nil
}

func (f *fakeDataProvider) GetH2H(team1ID, team2ID, limit int) ([]models.MatchSummary, error) {
	return f.h2h[h2hKey(team1ID, team2ID)], nil
}

func (f *fakeDataProvider) GetH2HBefore(team1ID, team2ID, limit int, asOf time.Time) ([]models.MatchSummary, error) {
	return f.h2h[h2hKey(team1ID, team2ID)], nil
}

func (f *fakeDataProvider) GetTeamMapStatsForTeam(teamID int) ([]models.TeamMapStat, error) {
	return f.mapStats[teamID], nil
}

func (f *fakeDataProvider) GetTeamVetoes(teamID, limit int) ([]storage.VetoRecord, error) {
	return f.vetoes[teamID], nil
}

func (f *fakeDataProvider) GetEloRatings() (map[int]float64, error) {
	return f.eloCache, nil
}

func (f *fakeDataProvider) EloCacheFresh() (bool, error) {
	return f.eloFresh, nil
}

func (f *fakeDataProvider) SaveEloRatings(ratings map[int]float64) error {
	if f.eloCache == nil {
		f.eloCache = make(map[int]float64)
	}
	for k, v := range ratings {
		f.eloCache[k] = v
	}
	return nil
}

func h2hKey(a, b int) string {
	if a > b {
		a, b = b, a
	}
	return itoa(a) + "-" + itoa(b)
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}

var errFakeTeamNotFound = fakeErr("fake: team not found")

type fakeErr string

func (e fakeErr) Error() string { return string(e) }

func TestEnginePredictWithFakeData(t *testing.T) {
	dp := newFakeDataProvider()
	dp.teams[1] = models.TeamDetail{Team: models.Team{ID: 1, Name: "Team A"}}
	dp.teams[2] = models.TeamDetail{Team: models.Team{ID: 2, Name: "Team B"}}
	dp.form[1] = formResult{wins: 8, total: 10}
	dp.form[2] = formResult{wins: 4, total: 10}

	engine := NewEngine(dp, DefaultWeights)
	pred, err := engine.Predict(Options{
		Team1ID: 1,
		Team2ID: 2,
		Format:  models.FormatBO3,
	})
	if err != nil {
		t.Fatalf("Predict failed: %v", err)
	}
	if pred.Team1.ID != 1 || pred.Team2.ID != 2 {
		t.Fatalf("unexpected teams: %d vs %d", pred.Team1.ID, pred.Team2.ID)
	}
	if pred.WinProb.Team1+pred.WinProb.Team2 == 0 {
		t.Fatal("expected non-zero probabilities")
	}
}

func TestEnginePredictMissingTeam(t *testing.T) {
	dp := newFakeDataProvider()
	engine := NewEngine(dp, DefaultWeights)

	_, err := engine.Predict(Options{Team1ID: 999, Team2ID: 2})
	if err == nil {
		t.Fatal("expected error for missing team")
	}
}
