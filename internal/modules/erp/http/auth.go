package http

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

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

// callState carries per-call facts from the handler to the call recorder. It
// holds flags, the error code and details the handler returned, and the ref
// the call touched. Never request data, never the key or the Authorization
// header (trap 24).
type callState struct {
	sensitiveRead bool
	errorCode     string
	errorDetail   []byte
	ref           string
}

// The call log keeps at most this much of the error details and of the refs
// and request-line text.
const (
	maxErrorDetailBytes = 2048
	maxQueryChars       = 500
	maxUserAgentChars   = 200
	maxRefChars         = 100
)

type callStateKey struct{}

func withCallState(ctx context.Context, st *callState) context.Context {
	return context.WithValue(ctx, callStateKey{}, st)
}

// noteError copies a handler's error code and details onto the call state, so
// the call log records ERP_VALIDATION and the like instead of an empty code.
// Details over 2 KB are replaced by a truncation marker.
func noteError(ctx context.Context, e *erp.Error) {
	st, ok := ctx.Value(callStateKey{}).(*callState)
	if !ok {
		return
	}
	st.errorCode = e.Code
	st.errorDetail = nil
	if len(e.Details) == 0 {
		return
	}
	raw, err := json.Marshal(e.Details)
	if err != nil {
		return
	}
	if len(raw) > maxErrorDetailBytes {
		raw = []byte(`{"truncated":true}`)
	}
	st.errorDetail = raw
}

// setCallRef records the identifier the call touched (a sync id, an order
// number, an external reference). Only the identifier goes in, never a body.
func setCallRef(ctx context.Context, ref string) {
	if st, ok := ctx.Value(callStateKey{}).(*callState); ok {
		st.ref = clip(ref, maxRefChars)
	}
}

// clip shortens s to at most max bytes without splitting a UTF-8 sequence, so
// the text stays valid for PostgreSQL.
func clip(s string, max int) string {
	if len(s) <= max {
		return s
	}
	cut := 0
	for i := 0; i < len(s); {
		_, width := utf8.DecodeRuneInString(s[i:])
		if i+width > max {
			break
		}
		i += width
		cut = i
	}
	return s[:cut]
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
	bytes  int
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
	n, err := w.ResponseWriter.Write(p)
	w.bytes += n
	return n, err
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
		var refusal *erp.AuthRefusal
		var code string
		defer func() {
			if a.record == nil {
				return
			}
			route := ""
			if rc := chi.RouteContext(r.Context()); rc != nil {
				route = rc.RoutePattern()
			}
			if code == "" {
				// A handler's own error (ERP_VALIDATION and the like) reaches
				// the log through the call state, not the middleware's code.
				code = state.errorCode
			}
			record := erp.CallRecord{RequestID: observability.RequestIDFrom(r.Context()), Method: r.Method, Route: route, Status: sw.status, ErrorCode: code, DurationMS: int(time.Since(start).Milliseconds()), IP: &ip, SensitiveRead: state.sensitiveRead, CreatedAt: time.Now()}
			record.BytesOut = sw.bytes
			record.Query = clip(r.URL.RawQuery, maxQueryChars)
			record.UserAgent = clip(r.UserAgent(), maxUserAgentChars)
			record.Ref = state.ref
			record.ErrorDetail = state.errorDetail
			if r.ContentLength > 0 {
				record.BytesIn = int(r.ContentLength)
			}
			if principal != nil {
				record.OrganizationID = &principal.OrganizationID
				record.APIKeyID = &principal.KeyID
				record.KeyPrefix = principal.KeyPrefix
			}
			if refusal != nil {
				// A refused key with a known prefix is attributed to its own
				// organization, so the owner sees it in the activity log.
				record.OrganizationID = &refusal.OrganizationID
				record.APIKeyID = &refusal.KeyID
				record.KeyPrefix = refusal.KeyPrefix
				record.AuthReason = refusal.Reason
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
			if errors.As(authErr, &refusal) {
				// The answer is the same 401 as any refusal; only the log differs.
				code = refusal.Err.Code
			}
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
