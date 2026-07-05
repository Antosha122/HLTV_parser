package sync

import (
	"context"
	"fmt"

	"psr/internal/hltv"
	"psr/internal/logx"
	"psr/internal/models"
)

type EventsListResult struct {
	EventsFetched int    `json:"events_fetched"`
	EventsSaved   int    `json:"events_saved"`
	FinishedAt    string `json:"finished_at,omitempty"`
}

// RefreshEventsList loads /events from HLTV and updates tournament metadata in the DB only.
func (s *Service) RefreshEventsList(ctx context.Context) (EventsListResult, error) {
	var res EventsListResult
	BeginOperation("events", "Загрузка списка турниров с HLTV...")

	logx.Info("sync", "обновление только списка турниров")
	events, err := s.client.GetEvents(ctx)
	if err != nil {
		logx.Warn("sync", "список турниров: %v", err)
		EndOperation("Ошибка загрузки турниров")
		return res, err
	}
	res.EventsFetched = len(events)

	saved := 0
	for _, e := range events {
		if err := ctx.Err(); err != nil {
			return res, err
		}
		if hltv.IsJunkEventName(e.Name) || e.Status == "" {
			continue
		}
		switch e.Status {
		case models.EventStatusOngoing, models.EventStatusUpcoming, models.EventStatusPast:
			if err := s.db.UpsertEvent(e); err != nil {
				logx.Warn("sync", "турнир %d: %v", e.ID, err)
				continue
			}
			saved++
		}
	}
	res.EventsSaved = saved
	_ = s.db.LogSync("events", 0, "ok", fmt.Sprintf("fetched=%d saved=%d", res.EventsFetched, saved))
	logx.Info("sync", "список турниров: получено %d, сохранено %d", res.EventsFetched, saved)
	EndOperation(fmt.Sprintf("Турниры обновлены: %d", saved))
	return res, nil
}
