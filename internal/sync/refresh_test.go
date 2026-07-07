package sync

import (
	"context"
	"errors"
	"testing"

	"psr/internal/models"
)

func TestRefreshRankingOnly(t *testing.T) {
	src := &fakeSource{
		events: []models.Event{
			{ID: 1, Name: "Test Cup", Status: models.EventStatusOngoing, Featured: true},
		},
		ranking: []models.Team{
			{ID: 10, Name: "Team A", WorldRank: 1},
			{ID: 20, Name: "Team B", WorldRank: 2},
		},
	}
	store := newFakeStore()
	svc := New(src, store)

	res, err := svc.Refresh(context.Background(), RefreshOptions{MaxEvents: 1, MaxMatches: 5})
	if err != nil {
		t.Fatalf("Refresh failed: %v", err)
	}
	if res.EventsFetched != 1 {
		t.Fatalf("expected 1 fetched event, got %d", res.EventsFetched)
	}
	if res.TeamsSaved < 2 {
		t.Fatalf("expected >=2 ranking teams saved, got %d", res.TeamsSaved)
	}
	st := svc.Tracker().CurrentStatus()
	if st.Running || st.Phase != "done" || st.LastResult == nil {
		t.Fatalf("unexpected status after success: %+v", st)
	}
	foundOK := false
	for _, l := range store.logs {
		if l.status == "ok" {
			foundOK = true
		}
	}
	if !foundOK {
		t.Fatal("expected an 'ok' sync_log entry")
	}
}

func TestRefreshSourceError(t *testing.T) {
	srcErr := errors.New("boom: hltv unavailable")
	src := &fakeSource{eventsErr: srcErr}
	svc := New(src, newFakeStore())
	_, err := svc.Refresh(context.Background(), RefreshOptions{})
	if !errors.Is(err, srcErr) {
		t.Fatalf("expected srcErr, got %v", err)
	}
	st := svc.Tracker().CurrentStatus()
	if st.Running || st.Phase != "error" {
		t.Fatalf("unexpected status after failure: %+v", st)
	}
}

func TestRefreshCancelledByContext(t *testing.T) {
	src := &fakeSource{
		events: []models.Event{
			{ID: 1, Name: "Test Cup", Status: models.EventStatusOngoing, Featured: true},
		},
		ranking: []models.Team{
			{ID: 10, Name: "Team A", WorldRank: 1},
		},
	}
	svc := New(src, newFakeStore())
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := svc.Refresh(ctx, RefreshOptions{MaxEvents: 1, MaxMatches: 1})
	if err == nil {
		t.Fatal("expected error from cancelled refresh")
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
	st := svc.Tracker().CurrentStatus()
	if st.Running || st.Phase != "cancelled" {
		t.Fatalf("unexpected status after cancel: %+v", st)
	}
}

func TestRefreshNoData(t *testing.T) {
	src := &fakeSource{}
	svc := New(src, newFakeStore())
	_, err := svc.Refresh(context.Background(), RefreshOptions{})
	if err == nil {
		t.Fatal("expected error when no events/ranking are synced")
	}
}

func TestCancelManagerLifecycle(t *testing.T) {
	cm := NewCancelManager()
	if cm.Cancel() {
		t.Fatal("expected Cancel()=false when nothing is running")
	}
	ctx := cm.NewContext(context.Background())
	if err := ctx.Err(); err != nil {
		t.Fatalf("fresh ctx should not be cancelled: %v", err)
	}
	if !cm.Cancel() {
		t.Fatal("expected Cancel()=true after NewContext")
	}
	if err := ctx.Err(); !errors.Is(err, context.Canceled) {
		t.Fatalf("expected ctx cancelled, got %v", err)
	}
	cm.Clear()
	if cm.Cancel() {
		t.Fatal("expected Cancel()=false after Clear")
	}
}

func TestServiceTrackerIsolation(t *testing.T) {
	a := New(&fakeSource{}, newFakeStore())
	b := New(&fakeSource{}, newFakeStore())
	a.Tracker().BeginOperation("test", "A running")
	if !a.Tracker().IsBusy() {
		t.Fatal("expected A busy")
	}
	if b.Tracker().IsBusy() {
		t.Fatal("expected B not busy (isolated tracker)")
	}
}
