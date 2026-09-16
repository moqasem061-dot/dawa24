package httpx

import "context"

type cspNonceKey struct{}

// WithNonce stores the per-request CSP nonce in the context.
func WithNonce(ctx context.Context, nonce string) context.Context {
	return context.WithValue(ctx, cspNonceKey{}, nonce)
}

// Nonce returns the CSP nonce for the current request, or the empty string if none was set.
func Nonce(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	if n, ok := ctx.Value(cspNonceKey{}).(string); ok {
		return n
	}
	return ""
}
