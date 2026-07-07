package sync

import (
	"context"
	"sync"

	"psr/internal/logx"
)

// CancelManager owns the cancellable context for background refreshes.
// It replaces the former package-level globals, so each Service instance
// can track its own active refresh independently.
type CancelManager struct {
	mu     sync.Mutex
	cancel context.CancelFunc
}

// NewCancelManager creates a ready-to-use CancelManager.
func NewCancelManager() *CancelManager {
	return &CancelManager{}
}

// NewContext returns a cancellable context for a background refresh.
// Any previously issued cancel function is replaced.
func (c *CancelManager) NewContext(parent context.Context) context.Context {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.cancel != nil {
		c.cancel()
	}
	ctx, cancel := context.WithCancel(parent)
	c.cancel = cancel
	return ctx
}

// Cancel stops the active refresh, if any. Returns true if a refresh
// was running and a cancellation was requested.
func (c *CancelManager) Cancel() bool {
	c.mu.Lock()
	cancel := c.cancel
	c.mu.Unlock()
	if cancel == nil {
		return false
	}
	cancel()
	logx.Info("sync", "refresh cancellation requested")
	return true
}

// Clear resets the stored cancel function (called when a refresh ends).
func (c *CancelManager) Clear() {
	c.mu.Lock()
	c.cancel = nil
	c.mu.Unlock()
}

// ---- Backward-compatible package-level wrappers ----
// These delegate to a default CancelManager so existing callers (api package,
// cmd/psr) keep working. New code should use Service.Cancel().

var defaultCancel = NewCancelManager()

// NewRefreshContext returns a cancellable context for a background refresh.
//
// Deprecated: prefer Service.Cancel().NewContext.
func NewRefreshContext(parent context.Context) context.Context {
	return defaultCancel.NewContext(parent)
}

// CancelRefresh stops the active refresh, if any.
//
// Deprecated: prefer Service.Cancel().Cancel.
func CancelRefresh() bool { return defaultCancel.Cancel() }

func clearRefreshCancel() { defaultCancel.Clear() }
