package sync

import (
	"sync"
	"time"

	"psr/internal/logx"
)

// Status is the live sync/predict progress exposed to the UI.
type Status struct {
	Running          bool           `json:"running"`
	Phase            string         `json:"phase"`
	Detail           string         `json:"detail"`
	StartedAt        string         `json:"started_at,omitempty"`
	LastFinishedAt   string         `json:"last_finished_at,omitempty"`
	LastResult       *RefreshResult `json:"last_result,omitempty"`
}

var (
	statusMu sync.RWMutex
	status   Status
)

func BeginRefresh() {
	statusMu.Lock()
	status = Status{
		Running:   true,
		Phase:     "start",
		Detail:    "Старт обновления...",
		StartedAt: time.Now().UTC().Format(time.RFC3339),
	}
	statusMu.Unlock()
}

func EndRefresh(res RefreshResult) {
	clearRefreshCancel()
	res.FinishedAt = time.Now().UTC().Format(time.RFC3339)
	statusMu.Lock()
	status.Running = false
	status.Phase = "done"
	status.Detail = "Обновление завершено"
	status.LastFinishedAt = res.FinishedAt
	copy := res
	status.LastResult = &copy
	statusMu.Unlock()
	logx.Info("sync", "✓ завершено: турниров=%d команд=%d матчей=%d история=%d",
		res.EventsSynced, res.TeamsSaved, res.MatchesSynced, res.HistoryMatches)
}

func FailRefresh(err error) {
	clearRefreshCancel()
	statusMu.Lock()
	status.Running = false
	status.Phase = "error"
	status.Detail = err.Error()
	statusMu.Unlock()
	logx.Error("sync", "обновление не удалось: %v", err)
}

func SetProgress(phase, detail string) {
	statusMu.Lock()
	status.Phase = phase
	status.Detail = detail
	statusMu.Unlock()
	logx.Info("sync", "→ [%s] %s", phase, detail)
}

func BeginOperation(phase, detail string) {
	statusMu.Lock()
	status = Status{
		Running:   true,
		Phase:     phase,
		Detail:    detail,
		StartedAt: time.Now().UTC().Format(time.RFC3339),
	}
	statusMu.Unlock()
	logx.Info("sync", "▶ [%s] %s", phase, detail)
}

func EndOperation(detail string) {
	statusMu.Lock()
	status.Running = false
	status.Phase = "done"
	if detail != "" {
		status.Detail = detail
	}
	status.LastFinishedAt = time.Now().UTC().Format(time.RFC3339)
	statusMu.Unlock()
}

func SetPredictProgress(detail string) {
	SetProgress("predict", detail)
}

func ClearPredictProgress() {}

func SetTeamProgress(detail string) {
	SetProgress("team", detail)
}

func ClearTeamProgress() {}

// IsBusy is true while any long-running operation is active (refresh, predict prep, team sync).
func IsBusy() bool {
	return CurrentStatus().Running
}

// ClearStaleRunning resets a stuck Running flag left by older builds or interrupted ops.
func ClearStaleRunning() {
	statusMu.Lock()
	defer statusMu.Unlock()
	if !status.Running {
		return
	}
	switch status.Phase {
	case "start", "events", "event", "history", "teams", "maps":
		return
	}
	status.Running = false
	status.Phase = ""
	status.Detail = ""
}

func IsRefreshRunning() bool {
	st := CurrentStatus()
	if !st.Running {
		return false
	}
	switch st.Phase {
	case "start", "events", "event", "history", "teams", "maps":
		return true
	default:
		return false
	}
}

func IsCancellableRefresh() bool {
	return IsRefreshRunning()
}

func CurrentStatus() Status {
	statusMu.RLock()
	defer statusMu.RUnlock()
	return status
}
