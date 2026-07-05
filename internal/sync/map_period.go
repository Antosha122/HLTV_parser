package sync

import "time"

// MapStatsPeriod is the rolling 6-month window for aggregated map stats.
// Dates are normalized so repeated syncs update the same DB rows.
func MapStatsPeriod() (start, end time.Time) {
	now := time.Now().UTC()
	end = time.Date(now.Year(), now.Month(), now.Day(), 23, 59, 59, 0, time.UTC)
	start = end.AddDate(0, -6, 0)
	start = time.Date(start.Year(), start.Month(), start.Day(), 0, 0, 0, 0, time.UTC)
	return start, end
}
