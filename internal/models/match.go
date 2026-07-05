package models

import "time"

type MatchFormat string

const (
	FormatBO1 MatchFormat = "bo1"
	FormatBO3 MatchFormat = "bo3"
	FormatBO5 MatchFormat = "bo5"
)

type Match struct {
	ID        int
	EventID   int
	Team1     Team
	Team2     Team
	Format    MatchFormat
	Date      time.Time
	WinnerID  int
	Stars     int
}
