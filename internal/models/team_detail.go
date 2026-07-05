package models

import "time"

type TeamDetail struct {
	Team
	Players       []Player
	RecentMatches []MatchSummary
}

type MatchSummary struct {
	ID         int
	Team1      Team
	Team2      Team
	Date       time.Time
	Format     MatchFormat
	WinnerID   int
	WinnerName string // из .team-won на /results?team=
	Event      string
	Score      string
}

type MatchDetail struct {
	Match
	EventName string
	Maps      []MatchMap
	Vetoes    []Veto
	H2H       []MatchSummary
}
