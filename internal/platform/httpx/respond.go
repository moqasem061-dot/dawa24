package httpx

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/muhiya/dawa24-store/internal/platform/errtrack"
	"github.com/muhiya/dawa24-store/internal/platform/observability"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
)

type langKey struct{}

// WithLang stores the resolved language in the context.
func WithLang(ctx context.Context, l i18n.Lang) context.Context {
	return context.WithValue(ctx, langKey{}, l)
}

// LangFrom returns the request language, defaulting to Arabic.
func LangFrom(ctx context.Context) i18n.Lang {
	if l, ok := ctx.Value(langKey{}).(i18n.Lang); ok {
		return l
	}
	return i18n.Default
}

// ErrorBody is the single JSON error envelope for the whole API surface.
//
// One shape everywhere means clients write one error handler. RequestID is
// included so a user can quote it in a support ticket and an operator can find
// the exact log line.
type ErrorBody struct {
	Error struct {
		Code      string            `json:"code"`
		Message   string            `json:"message"`
		Fields    map[string]string `json:"fields,omitempty"`
		RequestID string            `json:"request_id,omitempty"`
	} `json:"error"`
}

// JSON writes a successful JSON response.
func JSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if v != nil {
		_ = json.NewEncoder(w).Encode(v)
	}
}

// Error maps a domain error onto an HTTP response.
//
// Internal errors are logged in full and reported to the caller generically. The
// alternative — rendering the underlying message — is how database schema and
// query fragments end up in a browser, which is precisely what APP_DEBUG=true
// was doing in the legacy deployment.
func Error(w http.ResponseWriter, r *http.Request, log *slog.Logger, err error) {
	if log == nil {
		log = slog.Default()
	}

	// Client cancellations are normal HTTP lifecycle events; do not report to errtrack or log as error.
	if errors.Is(err, context.Canceled) || (r != nil && errors.Is(r.Context().Err(), context.Canceled)) || (err != nil && strings.Contains(strings.ToLower(err.Error()), "context canceled")) {
		log.DebugContext(r.Context(), "client canceled request", "path", r.URL.Path, "method", r.Method)
		return
	}

	status, appError := classify(err)

	if status >= 500 {
		log.ErrorContext(r.Context(), "request failed",
			"error", err, "path", r.URL.Path, "method", r.Method)
	} else {
		log.WarnContext(r.Context(), "request rejected",
			"error", err, "code", appError.Code, "path", r.URL.Path)
	}

	// The admin error screen is meant to show what users actually hit, so this
	// records the same failures the log line above describes.
	//
	// Below 500 the request was refused on purpose -- a validation failure, a
	// missing record, a permission the caller does not hold -- and those are
	// the normal traffic of a working system, not defects. Recording them would
	// bury the real faults under thousands of rows. Only 5xx is captured.
	if status >= 500 {
		errtrack.ReportRequest(r.Context(), r, err, errtrack.LevelError, status)
	}

	var body ErrorBody
	body.Error.Code = appError.Code
	body.Error.Message = appError.LocalizedMsg(string(LangFrom(r.Context())))
	body.Error.Fields = appError.Fields
	body.Error.RequestID = observability.RequestIDFrom(r.Context())

	JSON(w, status, body)
}

func classify(err error) (int, *apperr.Error) {
	var e *apperr.Error
	if !errors.As(err, &e) {
		e = apperr.Internal(err)
	}

	switch e.Kind {
	case apperr.KindValidation:
		return http.StatusUnprocessableEntity, e
	case apperr.KindNotFound:
		return http.StatusNotFound, e
	case apperr.KindConflict:
		return http.StatusConflict, e
	case apperr.KindUnauthorized:
		return http.StatusUnauthorized, e
	case apperr.KindForbidden:
		return http.StatusForbidden, e
	case apperr.KindRateLimited:
		return http.StatusTooManyRequests, e
	case apperr.KindUnavailable:
		return http.StatusServiceUnavailable, e
	default:
		return http.StatusInternalServerError, e
	}
}

// DecodeJSON reads a JSON body with a size limit and strict field checking.
//
// DisallowUnknownFields turns a typo in a client payload into an immediate 422
// rather than a silently ignored field — the failure mode where a vendor sets
// "discount_percent" instead of "discount_percentage" and wonders why nothing
// changed.
func DecodeJSON(w http.ResponseWriter, r *http.Request, dst any) error {
	const maxBody = 1 << 20 // 1 MiB; file uploads use a different path

	r.Body = http.MaxBytesReader(w, r.Body, maxBody)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()

	if err := dec.Decode(dst); err != nil {
		return apperr.Validation("request.malformed", "The request body could not be parsed.", nil).Wrap(err)
	}
	if dec.More() {
		return apperr.Validation("request.trailing_data", "The request body contained unexpected trailing data.", nil)
	}
	return nil
}
