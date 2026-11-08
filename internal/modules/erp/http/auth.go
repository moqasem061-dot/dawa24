package http

import (
	"context"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/moqasem061-dot/dawa24/internal/modules/erp"
	"github.com/moqasem061-dot/dawa24/internal/platform/database"
	"github.com/moqasem061-dot/dawa24/internal/platform/httpx"
	"github.com/moqasem061-dot/dawa24/internal/platform/observability"
)

type principalContextKey struct{}

func principalFromContext(ctx context.Context) (erp.Principal, bool) {
	p, ok := ctx.Value(principalContextKey{}).(erp.Principal)
	return p, ok
}

// callState carries per-call flags from the handler to the call recorder. It
// holds flags only, never request data (trap 24).
type callState struct{ sensitiveRead bool }

type callStateKey struct{}

func withCallState(ctx context.Context, st *callState) context.Context {
	return context.WithValue(ctx, callStateKey{}, st)
}

// markSensitiveRead flags the call as a read of sensitive data (customer names,
// contacts, prices). The middleware copies the flag into erp.api_calls.
func markSensitiveRead(ctx context.Context) {
	if st, ok := ctx.Value(callStateKey{}).(*callState); ok {
		st.sensitiveRead = true
	}
}

type authenticateFunc func(context.Context, string, string) (erp.Principal, error)
type authLayer struct {
	authenticate authenticateFunc
	lim          *httpx.Limiter
	log          *slog.Logger
	record       func(erp.CallRecord)
}
type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(status int) {
	if w.status == 0 {
		w.status = status
		w.ResponseWriter.WriteHeader(status)
	}
}
func (w *statusWriter) Write(p []byte) (int, error) {
	if w.status == 0 {
		w.WriteHeader(http.StatusOK)
	}
	return w.ResponseWriter.Write(p)
}
func (a *authLayer) logRedis(ctx context.Context, err error) {
	if err != nil && a.log != nil {
		a.log.ErrorContext(ctx, "ERP rate limit Redis error", "error", err)
	}
}
func (a *authLayer) middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		ip := httpx.ClientIP(r, 1)
		sw := &statusWriter{ResponseWriter: w}
		state := &callState{}
		var principal *erp.Principal
		var code string
		defer func() {
			if a.record == nil {
				return
			}
			route := ""
			if rc := chi.RouteContext(r.Context()); rc != nil {
				route = rc.RoutePattern()
			}
			record := erp.CallRecord{RequestID: observability.RequestIDFrom(r.Context()), Method: r.Method, Route: route, Status: sw.status, ErrorCode: code, DurationMS: int(time.Since(start).Milliseconds()), IP: &ip, SensitiveRead: state.sensitiveRead, CreatedAt: time.Now()}
			if r.ContentLength > 0 {
				record.BytesIn = int(r.ContentLength)
			}
			if principal != nil {
				record.OrganizationID = &principal.OrganizationID
				record.APIKeyID = &principal.KeyID
				record.KeyPrefix = principal.KeyPrefix
			}
			a.record(record)
		}()
		failure, err := checkAuthFailures(r.Context(), a.lim, ip)
		a.logRedis(r.Context(), err)
		if err == nil && failure.count >= int64(erp.AuthFailuresPerIPPerMinute) {
			code = "ERP_RATE_LIMITED"
			setRetryAfter(sw, failure.retry)
			writeError(sw, r, erp.NewError(code, 429, map[string]any{"retryAfterSeconds": int64(failure.retry.Seconds()), "limitPerMinute": failure.limit}))
			return
		}
		fields := strings.Fields(r.Header.Get("Authorization"))
		if len(fields) != 2 || !strings.EqualFold(fields[0], "Bearer") || fields[1] == "" {
			code = "ERP_AUTH_MISSING"
			a.logRedis(r.Context(), a.lim.IncrementAuthFailure(r.Context(), failureKey(ip), time.Minute))
			writeError(sw, r, erp.NewError(code, 401, nil))
			return
		}
		p, authErr := a.authenticate(r.Context(), fields[1], ip)
		if authErr != nil {
			a.logRedis(r.Context(), a.lim.IncrementAuthFailure(r.Context(), failureKey(ip), time.Minute))
			code = "ERP_AUTH_INVALID"
			writeError(sw, r, authErr)
			return
		}
		principal = &p
		headers, limitErr, redisErr := checkKeyLimit(r.Context(), a.lim, p)
		a.logRedis(r.Context(), redisErr)
		setLimitHeaders(sw, headers)
		if limitErr != nil {
			code = limitErr.Code
			setRetryAfter(sw, headers.retry)
			writeError(sw, r, limitErr)
			return
		}
		ctx := database.WithTenant(withCallState(context.WithValue(r.Context(), principalContextKey{}, p), state), p.OrganizationID)
		next.ServeHTTP(sw, r.WithContext(ctx))
	})
}
func (h *Handler) auth(next http.Handler) http.Handler {
	layer := authLayer{authenticate: h.svc.Authenticate, lim: h.lim, log: h.log}
	if h.calls != nil {
		layer.record = h.calls.Record
	}
	return layer.middleware(next)
}
