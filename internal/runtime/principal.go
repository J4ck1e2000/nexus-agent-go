package runtime

import (
	"context"
)

// RunPrincipal identifies the authenticated gateway user that initiated a run.
// It travels through the request context so the executor can sign run
// credentials without importing gateway packages.
type RunPrincipal struct {
	UserID   int64
	Username string
	Role     string
}

type principalContextKey struct{}

// WithPrincipal attaches the run principal to the context.
func WithPrincipal(ctx context.Context, principal RunPrincipal) context.Context {
	return context.WithValue(ctx, principalContextKey{}, principal)
}

// PrincipalFromCtx returns the run principal; ok is false when absent.
func PrincipalFromCtx(ctx context.Context) (RunPrincipal, bool) {
	principal, ok := ctx.Value(principalContextKey{}).(RunPrincipal)
	return principal, ok
}

// AnonymousPrincipal is the fallback when no authenticated user is present.
func AnonymousPrincipal() RunPrincipal {
	return RunPrincipal{UserID: 0, Username: "anonymous", Role: "user"}
}
