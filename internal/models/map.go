package models

type VetoAction string

const (
	VetoBan     VetoAction = "ban"
	VetoPick    VetoAction = "pick"
	VetoDecider VetoAction = "decider"
)

type MatchMap struct {
	MapName    string
	Team1Score int
	Team2Score int
	PickedBy   int
}

type Veto struct {
	Order    int
	Action   VetoAction
	TeamID   int
	TeamName string
	MapName  string
}

type TeamMapStat struct {
	TeamID   int
	MapName  string
	Wins     int
	Losses   int
	PickRate float64 // 0..1 from HLTV Pick %
	BanRate  float64 // 0..1 from HLTV Ban %
}
