package httpx

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"log/slog"
	"net/http"
	"net/url"
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

	if r != nil && !strings.HasPrefix(r.URL.Path, "/api/") && !strings.HasPrefix(r.URL.Path, "/webhook") {
		accept := r.Header.Get("Accept")
		isBrowser := strings.Contains(accept, "text/html") || !strings.Contains(accept, "application/json") || strings.Contains(accept, "*/*")
		if isBrowser {
			if status == http.StatusUnauthorized {
				target := "/auth/login"
				if r.URL.RequestURI() != "" && r.URL.Path != "/auth/login" {
					target += "?redirect=" + url.QueryEscape(r.URL.RequestURI())
				}
				http.Redirect(w, r, target, http.StatusSeeOther)
				return
			}
			renderHTMLError(w, r, status, body.Error.Code, body.Error.Message, body.Error.RequestID)
			return
		}
	}

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

// renderHTMLError renders a polished, responsive HTML error page for human browser visitors
// instead of raw JSON envelopes.
func renderHTMLError(w http.ResponseWriter, r *http.Request, status int, _, msg, reqID string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)

	safeTitle := "تنبيه"
	btnLabel := "العودة للرئيسية"
	btnLink := "/"
	if status == http.StatusUnauthorized || status == http.StatusForbidden {
		safeTitle = "تسجيل الدخول مطلوب"
		btnLabel = "تسجيل الدخول"
		btnLink = "/auth/login"
		if r != nil && r.URL.RequestURI() != "" && r.URL.Path != "/auth/login" {
			btnLink += "?redirect=" + url.QueryEscape(r.URL.RequestURI())
		}
	} else if status == http.StatusNotFound {
		safeTitle = "الصفحة غير موجودة"
	} else if status >= 500 {
		safeTitle = "حدث خطأ غير متوقع"
	}

	safeMsg := html.EscapeString(msg)
	if safeMsg == "" {
		safeMsg = "تعذر إكمال العملية في الوقت الحالي. يرجى إعادة المحاولة."
	}
	safeReqID := html.EscapeString(reqID)

	reqIDBlock := ""
	if safeReqID != "" {
		reqIDBlock = fmt.Sprintf(`<div class="req-id">Request ID: %s</div>`, safeReqID)
	}

	fmt.Fprintf(w, `<!DOCTYPE html>
<html lang="ar" dir="rtl">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>%s - دواء 24</title>
  <style>
    body { font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif; background: #0f172a; color: #f8fafc; display: flex; align-items: center; justify-content: center; min-height: 100vh; margin: 0; padding: 20px; box-sizing: border-box; }
    .card { background: #1e293b; border-radius: 20px; padding: 36px 28px; max-width: 440px; width: 100%%; text-align: center; border: 1px solid #334155; box-shadow: 0 25px 50px -12px rgba(0,0,0,0.5); }
    .icon { width: 64px; height: 64px; border-radius: 50%%; background: rgba(239, 68, 68, 0.15); color: #ef4444; display: inline-flex; align-items: center; justify-content: center; font-size: 28px; margin: 0 auto 20px auto; }
    h1 { font-size: 22px; font-weight: 700; margin: 0 0 10px 0; color: #f8fafc; }
    p { font-size: 14px; color: #94a3b8; margin: 0 0 24px 0; line-height: 1.6; }
    .btn { display: inline-block; background: #0284c7; color: white; text-decoration: none; padding: 12px 28px; border-radius: 12px; font-weight: 600; font-size: 15px; transition: all 0.2s ease; box-shadow: 0 4px 12px rgba(2, 132, 199, 0.3); }
    .btn:hover { background: #0369a1; transform: translateY(-1px); }
    .req-id { font-size: 11px; color: #64748b; margin-top: 24px; font-family: monospace; direction: ltr; }
  </style>
</head>
<body>
  <div class="card">
    <div class="icon">⚠️</div>
    <h1>%s</h1>
    <p>%s</p>
    <a href="%s" class="btn">%s</a>
    %s
  </div>
</body>
</html>`, html.EscapeString(safeTitle), html.EscapeString(safeTitle), safeMsg, html.EscapeString(btnLink), html.EscapeString(btnLabel), reqIDBlock)
}
