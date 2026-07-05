package sync

import (
	"errors"
	"sync"
	"testing"
)

func TestStatusTrackerLifecycle(t *testing.T) {
	tr := NewStatusTracker()

	tr.BeginRefresh()
	if !tr.IsBusy() {
		t.Fatal("expected busy after BeginRefresh")
	}
	if !tr.IsRefreshRunning() {
		t.Fatal("expected refresh running")
	}

	tr.EndRefresh(RefreshResult{EventsSynced: 3, TeamsSaved: 10, MatchesSynced: 50})
	if tr.IsBusy() {
		t.Fatal("expected not busy after EndRefresh")
	}
	st := tr.CurrentStatus()
	if st.LastResult == nil || st.LastResult.EventsSynced != 3 {
		t.Fatalf("unexpected last result: %+v", st.LastResult)
	}
}

func TestStatusTrackerFailRefresh(t *testing.T) {
	tr := NewStatusTracker()
	tr.BeginRefresh()
	tr.FailRefresh(errors.New("boom"))

	st := tr.CurrentStatus()
	if st.Running {
		t.Fatal("expected not running after failure")
	}
	if st.Phase != "error" {
		t.Fatalf("expected phase=error, got %q", st.Phase)
	}
}

func TestStatusTrackerConcurrent(t *testing.T) {
	tr := NewStatusTracker()
	var wg sync.WaitGroup

	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			tr.SetProgress("event", "working")
			_ = tr.CurrentStatus()
			_ = tr.IsBusy()
		}()
	}
	wg.Wait()
}
