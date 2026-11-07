package http

import (
	"encoding/json"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/moqasem061-dot/dawa24/internal/modules/erp"
	"github.com/moqasem061-dot/dawa24/internal/platform/observability"
)

type responseEnvelope struct {
	Status    string `json:"status"`
	RequestID string `json:"requestId"`
	Data      any    `json:"data"`
	Error     struct {
		Code    string            `json:"code"`
		Message map[string]string `json:"message"`
		Details map[string]any    `json:"details"`
	} `json:"error"`
}

func responseRequest() *httptest.ResponseRecorder { return httptest.NewRecorder() }
func TestRespondErrors(t *testing.T) {
	for _, code := range erp.AllCodes {
		t.Run(code, func(t *testing.T) {
			w := responseRequest()
			r := httptest.NewRequest("GET", "/", nil)
			r = r.WithContext(observability.WithRequestID(r.Context(), "req-123"))
			writeError(w, r, erp.NewError(code, 400, nil))
			var env responseEnvelope
			if err := json.Unmarshal(w.Body.Bytes(), &env); err != nil {
				t.Fatal(err)
			}
			if w.Code != 400 || env.Status != "error" || env.RequestID != "req-123" || w.Header().Get("X-Request-ID") != "req-123" || env.Error.Code != code || env.Error.Message["ar"] == "" || env.Error.Message["en"] == "" {
				t.Errorf("unexpected response: %+v", env)
			}
		})
	}
	w := responseRequest()
	r := httptest.NewRequest("GET", "/", nil)
	writeError(w, r, errors.New("internal"))
	if w.Code != 500 {
		t.Errorf("internal status=%d", w.Code)
	}
}
func TestRespondOK(t *testing.T) {
	w := responseRequest()
	r := httptest.NewRequest("GET", "/", nil)
	r = r.WithContext(observability.WithRequestID(r.Context(), "req-123"))
	writeOK(w, r, 201, map[string]string{"value": "yes"})
	var env responseEnvelope
	if err := json.Unmarshal(w.Body.Bytes(), &env); err != nil {
		t.Fatal(err)
	}
	if w.Code != 201 || env.Status != "ok" || env.RequestID != "req-123" || w.Header().Get("X-Request-ID") != "req-123" || env.Data.(map[string]any)["value"] != "yes" {
		t.Errorf("response %+v", env)
	}
}
func TestRespondPlaceholders(t *testing.T) {
	for _, tc := range []struct {
		code, key string
		value     any
		want      string
	}{
		{"ERP_RATE_LIMITED", "retryAfterSeconds", 7, "7"}, {"ERP_BODY_TOO_LARGE", "limitBytes", 2097152, "2"}, {"ERP_BATCH_TOO_LARGE", "limit", 42, "42"}, {"ERP_PO_ITEM_NOT_FOUND", "line", 17, "17"}, {"ERP_PO_NO_ACTIVE_OFFER", "line", 17, "17"}, {"ERP_PO_QUANTITY_INVALID", "line", 17, "17"}, {"ERP_PO_SUPPLIER_NOT_LINKED", "line", 17, "17"}, {"ERP_ORDER_STATE_CONFLICT", "currentStatus", "pending", "pending"}, {"ERP_PO_REFUSED", "reason", "quota.exhausted", "quota.exhausted"},
	} {
		t.Run(tc.code, func(t *testing.T) {
			for _, withValue := range []bool{true, false} {
				w := responseRequest()
				r := httptest.NewRequest("GET", "/", nil)
				details := map[string]any{}
				if withValue {
					details[tc.key] = tc.value
				}
				writeError(w, r, erp.NewError(tc.code, 400, details))
				var env responseEnvelope
				if err := json.Unmarshal(w.Body.Bytes(), &env); err != nil {
					t.Fatal(err)
				}
				for _, lang := range []string{"ar", "en"} {
					msg := env.Error.Message[lang]
					if withValue && (!strings.Contains(msg, tc.want) || strings.Contains(msg, "%d") || strings.Contains(msg, "%s")) {
						t.Errorf("%s: %q", lang, msg)
					}
					if !withValue && !strings.Contains(msg, "%d") && !strings.Contains(msg, "%s") {
						t.Errorf("missing placeholder in %q", msg)
					}
				}
			}
		})
	}
}
