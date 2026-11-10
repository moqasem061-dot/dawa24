package http

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/moqasem061-dot/dawa24/internal/modules/erp"
	"github.com/moqasem061-dot/dawa24/internal/platform/observability"
	"github.com/moqasem061-dot/dawa24/internal/shared/i18n"
)

func requestID(w http.ResponseWriter, r *http.Request) string {
	id := observability.RequestIDFrom(r.Context())
	if w.Header().Get("X-Request-ID") == "" {
		w.Header().Set("X-Request-ID", id)
	}
	return id
}
func writeOK(w http.ResponseWriter, r *http.Request, status int, data any) {
	id := requestID(w, r)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{"status": "ok", "requestId": id, "data": data})
}
func writeError(w http.ResponseWriter, r *http.Request, err error) {
	var e *erp.Error
	if !errors.As(err, &e) {
		// The log carries the raw error and the request id so a 500 can be
		// debugged from the server side. No key, hash or pepper can reach
		// this line: every error arriving here is either an *erp.Error —
		// whose Error() is the stable code alone — or a database error whose
		// text is PostgreSQL's catalog message without bound parameters, and
		// the bearer value never enters error strings anywhere in the erp
		// module (GenerateKey returns the secret out-of-band; key_hash is
		// never placed in an error).
		slog.ErrorContext(r.Context(), "ERP request failed", "error", err, "requestId", requestID(w, r))
		e = erp.NewError("ERP_INTERNAL", 500, nil)
	}
	noteError(r.Context(), e)
	id := requestID(w, r)
	key := "erp.err." + strings.ToLower(strings.TrimPrefix(e.Code, "ERP_"))
	args := messageArgs(e.Code, e.Details)
	translate := i18n.T
	message := map[string]string{"ar": translate(i18n.AR, key, args...), "en": translate(i18n.EN, key, args...)}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(e.Status)
	_ = json.NewEncoder(w).Encode(map[string]any{"status": "error", "requestId": id, "error": map[string]any{"code": e.Code, "message": message, "details": e.Details}})
}
func messageArgs(code string, details map[string]any) []any {
	var key string
	switch code {
	case "ERP_RATE_LIMITED":
		key = "retryAfterSeconds"
	case "ERP_BODY_TOO_LARGE":
		key = "limitBytes"
	case "ERP_BATCH_TOO_LARGE":
		key = "limit"
	case "ERP_PO_ITEM_NOT_FOUND", "ERP_PO_NO_ACTIVE_OFFER", "ERP_PO_QUANTITY_INVALID", "ERP_PO_SUPPLIER_NOT_LINKED":
		key = "line"
	case "ERP_ORDER_STATE_CONFLICT":
		key = "currentStatus"
	case "ERP_PO_REFUSED":
		key = "reason"
	default:
		return nil
	}
	value, ok := details[key]
	if !ok {
		return nil
	}
	if key == "currentStatus" || key == "reason" {
		v, ok := value.(string)
		if !ok {
			return nil
		}
		return []any{v}
	}
	var n int64
	switch v := value.(type) {
	case int:
		n = int64(v)
	case int64:
		n = v
	case int32:
		n = int64(v)
	case json.Number:
		var err error
		n, err = v.Int64()
		if err != nil {
			return nil
		}
	default:
		return nil
	}
	if key == "limitBytes" {
		n /= 1048576
	}
	return []any{n}
}
