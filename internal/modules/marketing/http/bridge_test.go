package http

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/modules/promo"
)

const testToken = "0123456789abcdef0123456789abcdef"

type fakeOffers struct{ since time.Time }

func (f *fakeOffers) ListPublishedOffers(_ context.Context, since time.Time, _ int) ([]*promo.PublishedOffer, error) {
	f.since = since
	return []*promo.PublishedOffer{{ID: 7, TitleAr: "عرض"}}, nil
}

func serve(t *testing.T, token, path, auth string) (*httptest.ResponseRecorder, *fakeOffers) {
	t.Helper()
	src := &fakeOffers{}
	r := chi.NewRouter()
	NewBridge(src, "https://example.test/", token, slog.Default()).RegisterRoutes(r)
	req := httptest.NewRequest(http.MethodGet, Prefix+path, nil)
	if auth != "" {
		req.Header.Set("Authorization", auth)
	}
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec, src
}

func TestDigestRefusesWithoutTheToken(t *testing.T) {
	for name, c := range map[string]struct{ token, auth string }{
		"no header":           {testToken, ""},
		"wrong token":         {testToken, "Bearer nope"},
		"wrong scheme":        {testToken, "Basic " + testToken},
		"bridge unconfigured": {"", "Bearer "},
	} {
		if rec, _ := serve(t, c.token, "/digest", c.auth); rec.Code != http.StatusUnauthorized {
			t.Errorf("%s: status %d, want 401", name, rec.Code)
		}
	}
}

func TestDigestWindow(t *testing.T) {
	if rec, _ := serve(t, testToken, "/digest?hours=500", "Bearer "+testToken); rec.Code != http.StatusBadRequest {
		t.Fatalf("hours=500: status %d, want 400", rec.Code)
	}
	rec, src := serve(t, testToken, "/digest", "Bearer "+testToken)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body)
	}
	if age := time.Since(src.since); age < 23*time.Hour || age > 25*time.Hour {
		t.Errorf("default window starts %v ago, want 24h", age)
	}
	var body digest
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.NewOffers) != 1 || body.ImageURL != "https://example.test/static/img/doctor-capsule-social.jpg" {
		t.Errorf("unexpected digest %+v", body)
	}
}
