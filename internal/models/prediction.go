package models

type Prediction struct {
	Team1        Team                `json:"team1"`
	Team2        Team                `json:"team2"`
	Format       MatchFormat         `json:"format"`
	WinProb      WinProbability      `json:"win_prob"`
	SeriesScores map[string]float64  `json:"series_scores"`
	Veto         VetoPrediction      `json:"veto"`
	Maps         []MapPrediction     `json:"maps"`
	Breakdown    ModelBreakdown      `json:"breakdown"`
	H2H          H2HSummary          `json:"h2h"`
	Form          FormSummary      `json:"form"`
	Confidence    string           `json:"confidence"`
	Team1MapStats []MapStatSummary `json:"team1_map_stats,omitempty"`
	Team2MapStats []MapStatSummary `json:"team2_map_stats,omitempty"`
}

type MapStatSummary struct {
	MapName  string  `json:"map_name"`
	Wins     int     `json:"wins"`
	Losses   int     `json:"losses"`
	WinRate  float64 `json:"win_rate"`
	PickRate float64 `json:"pick_rate,omitempty"`
	BanRate  float64 `json:"ban_rate,omitempty"`
}

type WinProbability struct {
	Team1 float64 `json:"team1"`
	Team2 float64 `json:"team2"`
}

type MapPrediction struct {
	MapName     string  `json:"map_name"`
	Team1WinPct float64 `json:"team1_win_pct"`
	InSeries    bool    `json:"in_series"`
	Role        string  `json:"role,omitempty"`
}

type VetoPrediction struct {
	Steps []VetoStep `json:"steps"`
}

type VetoStep struct {
	Order      int        `json:"order"`
	Action     VetoAction `json:"action"`
	TeamID     int        `json:"team_id,omitempty"`
	TeamName   string     `json:"team_name,omitempty"`
	MapName    string     `json:"map_name"`
	Confidence float64    `json:"confidence"`
}

type ModelBreakdown struct {
	Elo   float64 `json:"elo"`
	Form  float64 `json:"form"`
	H2H   float64 `json:"h2h"`
	Maps  float64 `json:"maps"`
	Final float64 `json:"final"`
}

type H2HSummary struct {
	Total       int          `json:"total"`
	Team1Wins   int          `json:"team1_wins"`
	Team2Wins   int          `json:"team2_wins"`
	Team1WinPct float64      `json:"team1_win_pct"`
	Matches     []H2HMatch   `json:"matches,omitempty"`
}

type H2HMatch struct {
	ID         int         `json:"id"`
	Date       string      `json:"date,omitempty"`
	Event      string      `json:"event,omitempty"`
	Format     MatchFormat `json:"format,omitempty"`
	Team1Name  string      `json:"team1_name"`
	Team2Name  string      `json:"team2_name"`
	WinnerName string      `json:"winner_name,omitempty"`
	Team1Won   bool        `json:"team1_won"`
	Played     bool        `json:"played"`
}

type FormSummary struct {
	Team1WinPct float64 `json:"team1_win_pct"`
	Team2WinPct float64 `json:"team2_win_pct"`
	Team1Sample int     `json:"team1_sample"`
	Team2Sample int     `json:"team2_sample"`
}

type EloRating struct {
	TeamID int     `json:"team_id"`
	Rating float64 `json:"rating"`
}

type MapProfile struct {
	MapName  string  `json:"map_name"`
	WinRate  float64 `json:"win_rate"`
	Wins     int     `json:"wins"`
	Losses   int     `json:"losses"`
	PickRate float64 `json:"pick_rate"`
	BanRate  float64 `json:"ban_rate"`
}
