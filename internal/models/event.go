package models

import "time"

type EventStatus string

const (
	EventStatusOngoing  EventStatus = "ongoing"
	EventStatusUpcoming EventStatus = "upcoming"
	EventStatusPast     EventStatus = "past"
)

type Event struct {
	ID        int         `json:"id"`
	Name      string      `json:"name"`
	StartDate time.Time   `json:"start_date,omitempty"`
	EndDate   time.Time   `json:"end_date,omitempty"`
	Status    EventStatus `json:"status,omitempty"`
	PrizePool string      `json:"prize_pool,omitempty"`
	Location  string      `json:"location,omitempty"`
	Featured  bool        `json:"featured,omitempty"`
}

// Analyzable — турнир уже идёт или завершён (есть сыгранные матчи для формы/Elo).
func (e Event) Analyzable() bool {
	return e.Status == EventStatusOngoing || e.Status == EventStatusPast
}

// Syncable — турнир можно загрузить с HLTV (участники, матчи, в т.ч. предстоящие).
func (e Event) Syncable() bool {
	switch e.Status {
	case EventStatusOngoing, EventStatusPast, EventStatusUpcoming:
		return true
	default:
		return false
	}
}
