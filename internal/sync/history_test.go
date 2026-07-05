package sync

import (
	"testing"
	"time"

	"psr/internal/models"
)

func TestMatchIsAnalyzable(t *testing.T) {
	today := time.Now().UTC()
	yesterday := today.AddDate(0, 0, -1)
	tomorrow := today.AddDate(0, 0, 1)

	cases := []struct {
		name string
		m    models.MatchSummary
		want bool
	}{
		{
			name: "played with winner",
			m:    models.MatchSummary{ID: 1, Date: yesterday, WinnerID: 10, Score: "2 - 1"},
			want: true,
		},
		{
			name: "played with score only",
			m:    models.MatchSummary{ID: 2, Date: yesterday, Score: "13 - 9"},
			want: true,
		},
		{
			name: "scheduled future",
			m:    models.MatchSummary{ID: 3, Date: tomorrow},
			want: false,
		},
		{
			name: "scheduled no result",
			m:    models.MatchSummary{ID: 4, Date: yesterday, Score: "vs"},
			want: false,
		},
		{
			name: "old match with winner",
			m:    models.MatchSummary{ID: 5, Date: time.Date(2024, 12, 1, 0, 0, 0, 0, time.UTC), WinnerID: 1},
			want: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := matchIsAnalyzable(tc.m); got != tc.want {
				t.Fatalf("matchIsAnalyzable() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestEventAnalyzable(t *testing.T) {
	upcoming := models.Event{Status: models.EventStatusUpcoming}
	if upcoming.Analyzable() {
		t.Fatal("upcoming must not be analyzable")
	}
	ongoing := models.Event{Status: models.EventStatusOngoing}
	if !ongoing.Analyzable() {
		t.Fatal("ongoing must be analyzable")
	}
	past := models.Event{Status: models.EventStatusPast}
	if !past.Analyzable() {
		t.Fatal("past must be analyzable")
	}
}

func TestEventSyncable(t *testing.T) {
	upcoming := models.Event{Status: models.EventStatusUpcoming}
	if !upcoming.Syncable() {
		t.Fatal("upcoming must be syncable")
	}
	ongoing := models.Event{Status: models.EventStatusOngoing}
	if !ongoing.Syncable() {
		t.Fatal("ongoing must be syncable")
	}
	unknown := models.Event{Status: "live"}
	if unknown.Syncable() {
		t.Fatal("unknown status must not be syncable")
	}
}
