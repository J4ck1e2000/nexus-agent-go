package runtime

import (
	"context"
)

type recentHistoryContextKey struct{}

// WithRecentHistory attaches bounded prior conversation entries to the run
// context. Like RunPrincipal, it travels through the context so the executor
// can forward it to the runtime without changing executor signatures.
// Empty entries keep the context untouched.
func WithRecentHistory(ctx context.Context, entries []RunHistoryEntry) context.Context {
	if len(entries) == 0 {
		return ctx
	}
	return context.WithValue(ctx, recentHistoryContextKey{}, entries)
}

// RecentHistoryFromCtx returns the prior conversation entries attached by
// WithRecentHistory; nil when absent.
func RecentHistoryFromCtx(ctx context.Context) []RunHistoryEntry {
	entries, ok := ctx.Value(recentHistoryContextKey{}).([]RunHistoryEntry)
	if !ok {
		return nil
	}
	return entries
}
