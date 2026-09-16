// Package observability configures structured logging.
//
// Logs go to stdout as JSON and are collected by the platform. The legacy system
// wrote errors into a 69-column full_error_logs table and activity into
// full_activity_logs, which put log volume on the same disk and connection pool
// as order writes. Operational logs belong outside the OLTP database.
//
// The business audit trail is a separate concern and does stay in PostgreSQL —
// it is a compliance record, not a log.
package observability

import (
	"context"
	"log/slog"
	"os"
	"strings"

	"github.com/muhiya/dawa24-store/internal/platform/config"
)

type ctxKey int

const (
	ctxKeyRequestID ctxKey = iota
	ctxKeyUserID
	ctxKeyOrgID
)

// NewLogger builds the root logger.
func NewLogger(cfg config.Observability, env config.Env) *slog.Logger {
	opts := &slog.HandlerOptions{
		Level:     parseLevel(cfg.LogLevel),
		AddSource: env != config.EnvProd,
		ReplaceAttr: func(_ []string, a slog.Attr) slog.Attr {
			// Defence in depth against a secret reaching the log stream. The
			// value is dropped entirely rather than partially masked, because a
			// prefix of an API key is still a useful hint to an attacker.
			if isSensitiveKey(a.Key) {
				return slog.String(a.Key, "[REDACTED]")
			}
			return a
		},
	}

	var handler slog.Handler
	if strings.EqualFold(cfg.LogFormat, "text") {
		handler = slog.NewTextHandler(os.Stdout, opts)
	} else {
		handler = slog.NewJSONHandler(os.Stdout, opts)
	}

	return slog.New(&contextHandler{Handler: handler})
}

// contextHandler copies request-scoped identifiers onto every record, so a log
// line can always be tied back to a request, a user, and a tenant without each
// call site remembering to pass them.
type contextHandler struct{ slog.Handler }

func (h *contextHandler) Handle(ctx context.Context, r slog.Record) error {
	if v, ok := ctx.Value(ctxKeyRequestID).(string); ok && v != "" {
		r.AddAttrs(slog.String("request_id", v))
	}
	if v, ok := ctx.Value(ctxKeyUserID).(int64); ok && v > 0 {
		r.AddAttrs(slog.Int64("user_id", v))
	}
	if v, ok := ctx.Value(ctxKeyOrgID).(int64); ok && v > 0 {
		r.AddAttrs(slog.Int64("org_id", v))
	}
	return h.Handler.Handle(ctx, r)
}

func (h *contextHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &contextHandler{Handler: h.Handler.WithAttrs(attrs)}
}

func (h *contextHandler) WithGroup(name string) slog.Handler {
	return &contextHandler{Handler: h.Handler.WithGroup(name)}
}

// WithRequestID binds a request id to the context for logging.
func WithRequestID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, ctxKeyRequestID, id)
}

// RequestIDFrom returns the bound request id.
func RequestIDFrom(ctx context.Context) string {
	v, _ := ctx.Value(ctxKeyRequestID).(string)
	return v
}

// WithActor binds the authenticated user and organisation for logging.
func WithActor(ctx context.Context, userID, orgID int64) context.Context {
	ctx = context.WithValue(ctx, ctxKeyUserID, userID)
	return context.WithValue(ctx, ctxKeyOrgID, orgID)
}

func parseLevel(s string) slog.Level {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

var sensitiveKeys = []string{
	"password", "passwd", "secret", "token", "api_key", "apikey",
	"authorization", "cookie", "session", "virtual_key", "totp",
	"recovery_code", "national_id", "passport", "iban", "bank_account",
}

func isSensitiveKey(key string) bool {
	k := strings.ToLower(key)
	for _, s := range sensitiveKeys {
		if strings.Contains(k, s) {
			return true
		}
	}
	return false
}
