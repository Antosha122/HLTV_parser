package sync

import (
	"sync"
	"time"

	"psr/internal/logx"
)

// Status is the live sync/predict progress exposed to the UI.
type Status struct {
	Running        bool           `json:"running"`
	Phase          string         `json:"phase"`
	Detail         string         `json:"detail"`
	StartedAt      string         `json:"started_at,omitempty"`
	LastFinishedAt string         `json:"last_finished_at,omitempty"`
	LastResult     *RefreshResult `json:"last_result,omitempty"`
}

// refreshPhases lists phases that represent an active (cancellable) refresh.
var refreshPhases = map[string]bool{
	"start": true, "events": true, "event": true,
	"history": true, "teams": true, "maps": true, "ranking": true,
}

// StatusTracker holds the live progress of sync/predict operations.
// It replaces the old package-level global state, making it possible to
// run/test multiple Service instances in isolation.
type StatusTracker struct {
	mu     sync.RWMutex
	status Status
}

// NewStatusTracker creates a ready-to-use StatusTracker.
func NewStatusTracker() *StatusTracker {
	return &StatusTracker{}
}

func (t *StatusTracker) BeginRefresh() {
	t.mu.Lock()
	t.status = Status{
		Running:   true,
		Phase:     "start",
		Detail:    "Starting refresh...",
		StartedAt: time.Now().UTC().Format(time.RFC3339),
	}
	t.mu.Unlock()
}

func (t *StatusTracker) EndRefresh(res RefreshResult) {
	clearRefreshCancel()
	res.FinishedAt = time.Now().UTC().Format(time.RFC3339)
	t.mu.Lock()
	t.status.Running = false
	t.status.Phase = "done"
	t.status.Detail = "Refresh complete"
	t.status.LastFinishedAt = res.FinishedAt
	copy := res
	t.status.LastResult = &copy
	t.mu.Unlock()
	logx.Info("sync", "done: events=%d teams=%d matches=%d history=%d",
		res.EventsSynced, res.TeamsSaved, res.MatchesSynced, res.HistoryMatches)
}

func (t *StatusTracker) EndRefreshCancelled() {
	clearRefreshCancel()
	t.mu.Lock()
	t.status.Running = false
	t.status.Phase = "cancelled"
	t.status.Detail = "Refresh cancelled"
	t.status.LastFinishedAt = time.Now().UTC().Format(time.RFC3339)
	t.mu.Unlock()
}

func (t *StatusTracker) FailRefresh(err error) {
	clearRefreshCancel()
	t.mu.Lock()
	t.status.Running = false
	t.status.Phase = "error"
	t.status.Detail = err.Error()
	t.mu.Unlock()
	logx.Error("sync", "refresh failed: %v", err)
}

func (t *StatusTracker) SetProgress(phase, detail string) {
	t.mu.Lock()
	t.status.Phase = phase
	t.status.Detail = detail
	t.mu.Unlock()
	logx.Info("sync", "[%s] %s", phase, detail)
}

func (t *StatusTracker) BeginOperation(phase, detail string) {
	t.mu.Lock()
	t.status = Status{
		Running:   true,
		Phase:     phase,
		Detail:    detail,
		StartedAt: time.Now().UTC().Format(time.RFC3339),
	}
	t.mu.Unlock()
	logx.Info("sync", "[%s] %s", phase, detail)
}

func (t *StatusTracker) EndOperation(detail string) {
	t.mu.Lock()
	t.status.Running = false
	t.status.Phase = "done"
	if detail != "" {
		t.status.Detail = detail
	}
	t.status.LastFinishedAt = time.Now().UTC().Format(time.RFC3339)
	t.mu.Unlock()
}

func (t *StatusTracker) SetPredictProgress(detail string) {
	t.SetProgress("predict", detail)
}

func (t *StatusTracker) SetTeamProgress(detail string) {
	t.SetProgress("team", detail)
}

// IsBusy is true while any long-running operation is active.
func (t *StatusTracker) IsBusy() bool {
	return t.CurrentStatus().Running
}

// ClearStaleRunning resets a stuck Running flag left by older builds or interrupted ops.
func (t *StatusTracker) ClearStaleRunning() {
	t.mu.Lock()
	defer t.mu.Unlock()
	if !t.status.Running {
		return
	}
	if refreshPhases[t.status.Phase] {
		return
	}
	t.status.Running = false
	t.status.Phase = ""
	t.status.Detail = ""
}

func (t *StatusTracker) IsRefreshRunning() bool {
	st := t.CurrentStatus()
	if !st.Running {
		return false
	}
	return refreshPhases[st.Phase]
}

func (t *StatusTracker) IsCancellableRefresh() bool {
	return t.IsRefreshRunning()
}

func (t *StatusTracker) CurrentStatus() Status {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.status
}

// ---- Backward-compatible package-level wrappers ----
// These delegate to a default tracker so existing callers (api package,
// cmd/psr) continue to work. New code should prefer the instance-based
// StatusTracker owned by Service.

var defaultTracker = NewStatusTracker()

func BeginRefresh()                       { defaultTracker.BeginRefresh() }
func EndRefresh(res RefreshResult)        { defaultTracker.EndRefresh(res) }
func EndRefreshCancelled()                { defaultTracker.EndRefreshCancelled() }
func FailRefresh(err error)               { defaultTracker.FailRefresh(err) }
func SetProgress(phase, detail string)    { defaultTracker.SetProgress(phase, detail) }
func BeginOperation(phase, detail string) { defaultTracker.BeginOperation(phase, detail) }
func EndOperation(detail string)          { defaultTracker.EndOperation(detail) }
func SetPredictProgress(detail string)    { defaultTracker.SetPredictProgress(detail) }
func ClearPredictProgress()               {}
func SetTeamProgress(detail string)       { defaultTracker.SetTeamProgress(detail) }
func ClearTeamProgress()                  {}
func IsBusy() bool                        { return defaultTracker.IsBusy() }
func ClearStaleRunning()                  { defaultTracker.ClearStaleRunning() }
func IsRefreshRunning() bool              { return defaultTracker.IsRefreshRunning() }
func IsCancellableRefresh() bool          { return defaultTracker.IsCancellableRefresh() }
func CurrentStatus() Status               { return defaultTracker.CurrentStatus() }

// DefaultTracker returns the package-level default tracker, used by the API
// layer for status endpoints. In the future each Service can own its own.
func DefaultTracker() *StatusTracker { return defaultTracker }
