package api

import "context"

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
