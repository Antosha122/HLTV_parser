package sync

import (
	"context"
	"sync"

	"psr/internal/logx"
)

var (
	refreshCancelMu sync.Mutex
	refreshCancel   context.CancelFunc
)

// NewRefreshContext returns a cancellable context for a background refresh.
func NewRefreshContext(parent context.Context) context.Context {
	refreshCancelMu.Lock()
	defer refreshCancelMu.Unlock()
	if refreshCancel != nil {
		refreshCancel()
	}
	ctx, cancel := context.WithCancel(parent)
	refreshCancel = cancel
	return ctx
}

// CancelRefresh stops the active refresh, if any.
func CancelRefresh() bool {
	refreshCancelMu.Lock()
	cancel := refreshCancel
	refreshCancelMu.Unlock()
	if cancel == nil {
		return false
	}
	cancel()
	logx.Info("sync", "refresh cancellation requested")
	return true
}

func clearRefreshCancel() {
	refreshCancelMu.Lock()
	refreshCancel = nil
	refreshCancelMu.Unlock()
}
