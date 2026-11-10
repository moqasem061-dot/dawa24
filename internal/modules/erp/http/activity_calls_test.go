package http

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"unicode/utf8"

	"github.com/go-chi/chi/v5"
	"github.com/moqasem061-dot/dawa24/internal/modules/erp"
)

// callRouter mounts one handler behind the auth layer and returns the recorded
// call with the response.
func callRouter(t *testing.T, layer *authLayer, handler http.HandlerFunc, target string) (*httptest.ResponseRecorder, erp.CallRecord) {
	t.Helper()
	var records []erp.CallRecord
	layer.record = func(c erp.CallRecord) { records = append(records, c) }
	r := chi.NewRouter()
	r.With(layer.middleware).Get("/products/sync/{id}", handler)
	req := httptest.NewRequest("GET", target, nil)
	req.RemoteAddr = "203.0.113.7:1234"
	req.Header.Set("Authorization", "Bearer valid")
	req.Header.Set("User-Agent", "orgasoft-test/1.0")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if len(records) != 1 {
		t.Fatalf("records = %d, want 1", len(records))
	}
	return w, records[0]
}

func TestCallRecordsHandlerErrorCodeAndDetails(t *testing.T) {
	layer, _, _, _ := newAuthHarness(t)
	w, rec := callRouter(t, layer, func(w http.ResponseWriter, r *http.Request) {
		writeError(w, r, erp.NewError("ERP_VALIDATION", 422, map[string]any{"fields": []string{"name"}}))
	}, "/products/sync/12?state=new")
	if w.Code != 422 {
		t.Fatalf("status = %d, want 422", w.Code)
	}
	if rec.ErrorCode != "ERP_VALIDATION" {
		t.Fatalf("error code = %q, want ERP_VALIDATION (a handler error was logged empty)", rec.ErrorCode)
	}
	var detail map[string][]string
	if err := json.Unmarshal(rec.ErrorDetail, &detail); err != nil || detail["fields"][0] != "name" {
		t.Fatalf("error detail = %s (%v)", rec.ErrorDetail, err)
	}
}

func TestCallRecordsOversizedDetailAsTruncated(t *testing.T) {
	layer, _, _, _ := newAuthHarness(t)
	big := make([]string, 0, 400)
	for i := 0; i < 400; i++ {
		big = append(big, "a-long-value-that-fills-the-detail-column")
	}
	_, rec := callRouter(t, layer, func(w http.ResponseWriter, r *http.Request) {
		writeError(w, r, erp.NewError("ERP_VALIDATION", 422, map[string]any{"fields": big}))
	}, "/products/sync/1")
	if string(rec.ErrorDetail) != `{"truncated":true}` {
		t.Fatalf("detail = %s, want the truncation marker", rec.ErrorDetail)
	}
}

func TestCallRecordsRefQueryAgentAndBytes(t *testing.T) {
	layer, _, _, _ := newAuthHarness(t)
	w, rec := callRouter(t, layer, func(w http.ResponseWriter, r *http.Request) {
		setCallRef(r.Context(), chi.URLParam(r, "id"))
		_, _ = w.Write([]byte("hello"))
	}, "/products/sync/4821?state=new")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d", w.Code)
	}
	if rec.Ref != "4821" {
		t.Fatalf("ref = %q, want 4821", rec.Ref)
	}
	if rec.BytesOut != 5 {
		t.Fatalf("bytes out = %d, want 5", rec.BytesOut)
	}
	if rec.Query != "state=new" || rec.UserAgent != "orgasoft-test/1.0" {
		t.Fatalf("query %q user agent %q", rec.Query, rec.UserAgent)
	}
	if rec.ErrorDetail != nil {
		t.Fatalf("a successful call carries error detail %s", rec.ErrorDetail)
	}
}

func TestCallRecordsClipsQueryOnRuneBoundary(t *testing.T) {
	long := ""
	for i := 0; i < 300; i++ {
		long += "ع"
	}
	got := clip(long, maxQueryChars)
	if len(got) > maxQueryChars || !utf8.ValidString(got) {
		t.Fatalf("clip returned %d bytes, valid %v", len(got), utf8.ValidString(got))
	}
}

// A refused key with a known prefix gets the same answer as an unknown prefix,
// but the call log names the organization, key and reason.
func TestRefusedKeyWithKnownPrefixIsLoggedWithSameAnswer(t *testing.T) {
	layer, _, _, _ := newAuthHarness(t)
	unknown := func() *httptest.ResponseRecorder {
		layer.authenticate = func(context.Context, string, string) (erp.Principal, error) {
			return erp.Principal{}, erp.NewError("ERP_AUTH_INVALID", 401, nil)
		}
		w, _ := callRouter(t, layer, func(http.ResponseWriter, *http.Request) {}, "/products/sync/1")
		return w
	}
	known := func() (*httptest.ResponseRecorder, erp.CallRecord) {
		layer.authenticate = func(context.Context, string, string) (erp.Principal, error) {
			return erp.Principal{}, &erp.AuthRefusal{OrganizationID: 55, KeyID: 9, KeyPrefix: "abcdefgh", Reason: "secret_mismatch", Err: erp.NewError("ERP_AUTH_INVALID", 401, nil)}
		}
		return callRouter(t, layer, func(http.ResponseWriter, *http.Request) {}, "/products/sync/1")
	}
	wantBody := bodyWithoutRequestID(t, unknown().Body.Bytes())
	w, rec := known()
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", w.Code)
	}
	if got := bodyWithoutRequestID(t, w.Body.Bytes()); got != wantBody {
		t.Fatalf("refused-known body %s differs from unknown-prefix body %s", got, wantBody)
	}
	if rec.OrganizationID == nil || *rec.OrganizationID != 55 || rec.APIKeyID == nil || *rec.APIKeyID != 9 {
		t.Fatalf("organization and key not recorded: %+v", rec)
	}
	if rec.KeyPrefix != "abcdefgh" || rec.AuthReason != "secret_mismatch" || rec.ErrorCode != "ERP_AUTH_INVALID" {
		t.Fatalf("prefix %q reason %q code %q", rec.KeyPrefix, rec.AuthReason, rec.ErrorCode)
	}
}

// An unknown prefix stays anonymous: no organization, no reason.
func TestUnknownPrefixStaysAnonymous(t *testing.T) {
	layer, _, _, _ := newAuthHarness(t)
	layer.authenticate = func(context.Context, string, string) (erp.Principal, error) {
		return erp.Principal{}, erp.NewError("ERP_AUTH_INVALID", 401, nil)
	}
	_, rec := callRouter(t, layer, func(http.ResponseWriter, *http.Request) {}, "/products/sync/1")
	if rec.OrganizationID != nil || rec.AuthReason != "" {
		t.Fatalf("unknown prefix recorded organization %v reason %q", rec.OrganizationID, rec.AuthReason)
	}
}

// bodyWithoutRequestID drops the per-request id so two answers can be compared.
func bodyWithoutRequestID(t *testing.T, raw []byte) string {
	t.Helper()
	var v map[string]any
	if err := json.Unmarshal(raw, &v); err != nil {
		t.Fatalf("body is not JSON: %v", err)
	}
	delete(v, "requestId")
	out, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(out)
}
