package api

import "context"

//nolint:lostcancel // cancel is invoked by goroutines watching parent contexts
func mergeContexts(parents ...context.Context) context.Context {
	ctx, cancel := context.WithCancel(context.Background())
	for _, parent := range parents {
		if parent == nil {
			continue
		}
		p := parent
		go func() {
			select {
			case <-p.Done():
				cancel()
			case <-ctx.Done():
			}
		}()
	}
	return ctx
}
