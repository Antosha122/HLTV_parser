package hltv

import (
	"os"
	"path/filepath"
	"testing"

	"psr/internal/models"
)

func readFixture(t *testing.T, name string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", "testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestParseTeamPage(t *testing.T) {
	team, err := ParseTeamPage(readFixture(t, "team_sample.html"), 4608)
	if err != nil {
		t.Fatal(err)
	}
	if team.Name != "Natus Vincere" {
		t.Fatalf("name: %q", team.Name)
	}
	if team.WorldRank != 3 {
		t.Fatalf("rank: %d", team.WorldRank)
	}
	if team.HLTVRating != 892 {
		t.Fatalf("rating: %f", team.HLTVRating)
	}
	if len(team.Players) != 3 {
		t.Fatalf("players: %d", len(team.Players))
	}
}

func TestParseMatchPage(t *testing.T) {
	match, err := ParseMatchPage(readFixture(t, "match_sample.html"), 2300000)
	if err != nil {
		t.Fatal(err)
	}
	if match.Team1.ID != 4608 || match.Team2.ID != 6667 {
		t.Fatalf("teams: %+v vs %+v", match.Team1, match.Team2)
	}
	if match.Format != models.FormatBO3 {
		t.Fatalf("format: %s", match.Format)
	}
	if len(match.Vetoes) != 5 {
		t.Fatalf("vetoes: %d", len(match.Vetoes))
	}
	if len(match.Maps) != 2 {
		t.Fatalf("maps: %d", len(match.Maps))
	}
	if len(match.H2H) != 2 {
		t.Fatalf("h2h refs: %d", len(match.H2H))
	}
}

func TestParseResultsPage(t *testing.T) {
	matches, err := ParseResultsPage(readFixture(t, "results_sample.html"))
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 2 {
		t.Fatalf("matches: %d", len(matches))
	}
	if matches[0].ID != 2300100 {
		t.Fatalf("first id: %d", matches[0].ID)
	}
	if matches[0].Team1.ID != 4608 || matches[0].Team2.ID != 5995 {
		t.Fatalf("teams: %+v vs %+v", matches[0].Team1, matches[0].Team2)
	}
}

func TestParseResultsTeamTable(t *testing.T) {
	matches, err := ParseResultsPage(readFixture(t, "results_team_table.html"))
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 2 {
		t.Fatalf("matches: %d", len(matches))
	}
	if matches[0].Team1.Name != "Spirit" || matches[0].Team2.Name != "MIBR" {
		t.Fatalf("m1: %+v vs %+v", matches[0].Team1, matches[0].Team2)
	}
	if matches[0].Score != "13 - 1" {
		t.Fatalf("score: %q", matches[0].Score)
	}
	if matches[0].Event != "IEM Cologne Major 2026 Stage 2" {
		t.Fatalf("event: %q", matches[0].Event)
	}
	if matches[1].Format != models.FormatBO5 {
		t.Fatalf("format: %s", matches[1].Format)
	}
	if matches[1].Team2.Name != "Falcons" {
		t.Fatalf("team2: %q", matches[1].Team2.Name)
	}
}

func TestParseResultsFuriaAstana(t *testing.T) {
	matches, err := ParseResultsPage(readFixture(t, "results_furia_astana.html"))
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 2 {
		t.Fatalf("matches: %d", len(matches))
	}
	loss := matches[0]
	if loss.Team1.Name != "FURIA" || loss.Team2.Name != "Falcons" {
		t.Fatalf("loss teams: %s vs %s", loss.Team1.Name, loss.Team2.Name)
	}
	if loss.Score != "1 - 2" {
		t.Fatalf("loss score: %q", loss.Score)
	}
	if loss.WinnerName != "Falcons" {
		t.Fatalf("loss winner: %q", loss.WinnerName)
	}
	win := matches[1]
	if win.Team1.Name != "FURIA" || win.Team2.Name != "Gentle Mates" {
		t.Fatalf("win teams: %s vs %s", win.Team1.Name, win.Team2.Name)
	}
	if win.Score != "2 - 1" {
		t.Fatalf("win score: %q", win.Score)
	}
	if win.WinnerName != "FURIA" {
		t.Fatalf("win winner: %q", win.WinnerName)
	}
}

func TestParseResultsSpiritLoss(t *testing.T) {
	matches, err := ParseResultsPage(readFixture(t, "results_spirit_loss.html"))
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 1 {
		t.Fatalf("matches: %d", len(matches))
	}
	m := matches[0]
	if m.Score != "0 - 3" {
		t.Fatalf("score: %q", m.Score)
	}
	if m.Team1.Name != "Spirit" || m.Team2.Name != "Vitality" {
		t.Fatalf("teams: %s vs %s", m.Team1.Name, m.Team2.Name)
	}
	w := winnerFromScore(m.Score, models.Team{ID: 7020, Name: "Spirit"}, models.Team{ID: 9565, Name: "Vitality"})
	if w != 9565 {
		t.Fatalf("winner: %d", w)
	}
}

func TestParseTeamPageMatches(t *testing.T) {
	matches, err := ParseTeamPageMatches(readFixture(t, "team_matches_sample.html"))
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 1 {
		t.Fatalf("matches: %d", len(matches))
	}
	if matches[0].Team1.Name != "Spirit" {
		t.Fatalf("team1: %q", matches[0].Team1.Name)
	}
}

func TestRefererForHLTV(t *testing.T) {
	base := "https://www.hltv.org"
	r := refererForHLTV(base, base+"/results?team=7020&startDate=2025-01-01")
	if r != base+"/team/7020/_" {
		t.Fatalf("referer: %q", r)
	}
}

func TestParseResultsPageEventLayout(t *testing.T) {
	matches, err := ParseResultsPage(readFixture(t, "results_event_no_team_links.html"))
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 1 {
		t.Fatalf("matches: %d", len(matches))
	}
	if matches[0].ID != 2394879 {
		t.Fatalf("id: %d", matches[0].ID)
	}
	if matches[0].Team1.Name != "Spirit" || matches[0].Team2.Name != "MIBR" {
		t.Fatalf("teams: %q vs %q", matches[0].Team1.Name, matches[0].Team2.Name)
	}
}

func TestTeamNamesFromMatchHref(t *testing.T) {
	t1, t2 := teamNamesFromMatchHref("/matches/2394879/spirit-vs-mibr-iem-cologne-major-2026-stage-2")
	if t1 != "spirit" || t2 != "mibr" {
		t.Fatalf("got %q vs %q", t1, t2)
	}
}

func TestParseTeamMapStats(t *testing.T) {
	stats, err := ParseTeamMapStats(readFixture(t, "team_map_stats_sample.html"), 4608)
	if err != nil {
		t.Fatal(err)
	}
	if len(stats) != 3 {
		t.Fatalf("stats: %d", len(stats))
	}
	if stats[0].MapName != "Mirage" || stats[0].Wins != 12 {
		t.Fatalf("first stat: %+v", stats[0])
	}
}

func TestParseTeamMapsStatsPath(t *testing.T) {
	html := `<a href="/stats/teams/map/32/4608/bench">map</a><a href="/stats/teams/maps/4608/bench">maps</a>`
	path := ParseTeamMapsStatsPath(html, 4608)
	if path != "/stats/teams/maps/4608/bench" {
		t.Fatalf("path: %q", path)
	}
}

func TestParseTeamMapStatsPoolLayout(t *testing.T) {
	stats, err := ParseTeamMapStats(readFixture(t, "team_map_stats_pool.html"), 4863)
	if err != nil {
		t.Fatal(err)
	}
	if len(stats) != 2 {
		t.Fatalf("stats: %d", len(stats))
	}
	m := stats[0]
	if m.MapName != "Mirage" || m.Wins != 20 || m.Losses != 4 {
		t.Fatalf("mirage: %+v", m)
	}
	if m.PickRate < 0.428 || m.PickRate > 0.43 {
		t.Fatalf("pick rate: %v", m.PickRate)
	}
	if m.BanRate < 0.055 || m.BanRate > 0.057 {
		t.Fatalf("ban rate: %v", m.BanRate)
	}
	inf := stats[1]
	if inf.MapName != "Inferno" || inf.Wins != 8 || inf.Losses != 7 {
		t.Fatalf("inferno: %+v", inf)
	}
}
