package ui

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

func TestRedirectWithNotice(t *testing.T) {
	h := &UIHandler{}

	tests := []struct {
		name           string
		path           string
		referer        string
		reqHost        string
		fwdHost        string
		isHX           bool
		kind           string
		message        string
		wantLocation   string
		wantHXRedirect string
	}{
		{
			name:         "clean target without referer",
			path:         "/admin/users",
			kind:         "success",
			message:      "User updated",
			wantLocation: "/admin/users?msg=User+updated&notice=success",
		},
		{
			name:         "same path referer carries filters and pagination",
			path:         "/admin/users",
			referer:      "http://example.com/admin/users?q=ahmed&role=vendor&page=3&limit=50",
			reqHost:      "example.com",
			kind:         "success",
			message:      "Done",
			wantLocation: "/admin/users?limit=50&msg=Done&notice=success&page=3&q=ahmed&role=vendor",
		},
		{
			name:         "does not copy old notice or msg from referer",
			path:         "/admin/users",
			referer:      "http://example.com/admin/users?q=ahmed&notice=error&msg=old&notice_type=error&notice_msg=old",
			reqHost:      "example.com",
			kind:         "success",
			message:      "new message",
			wantLocation: "/admin/users?msg=new+message&notice=success&q=ahmed",
		},
		{
			name:         "explicit query parameter on target path overrides referer",
			path:         "/admin/users?tab=deletion_requests",
			referer:      "http://example.com/admin/users?tab=active&page=2",
			reqHost:      "example.com",
			kind:         "info",
			message:      "switched",
			wantLocation: "/admin/users?msg=switched&notice=info&page=2&tab=deletion_requests",
		},
		{
			name:         "different path referer is not carried over",
			path:         "/admin/users",
			referer:      "http://example.com/admin/orders?status=pending&page=2",
			reqHost:      "example.com",
			kind:         "success",
			message:      "Done",
			wantLocation: "/admin/users?msg=Done&notice=success",
		},
		{
			name:         "different host referer is not carried over",
			path:         "/admin/users",
			referer:      "http://attacker.com/admin/users?q=evil",
			reqHost:      "example.com",
			kind:         "success",
			message:      "Done",
			wantLocation: "/admin/users?msg=Done&notice=success",
		},
		{
			name:         "trusted X-Forwarded-Host matches referer",
			path:         "/admin/users",
			referer:      "http://dawa24.store/admin/users?q=med",
			reqHost:      "localhost:8080",
			fwdHost:      "dawa24.store",
			kind:         "success",
			message:      "Done",
			wantLocation: "/admin/users?msg=Done&notice=success&q=med",
		},
		{
			name:           "HX-Request returns 200 with HX-Redirect header",
			path:           "/admin/users",
			referer:        "http://example.com/admin/users?page=4",
			reqHost:        "example.com",
			isHX:           true,
			kind:           "success",
			message:        "Done",
			wantHXRedirect: "/admin/users?msg=Done&notice=success&page=4",
		},
		{
			name:         "preserves fragment from target path",
			path:         "/admin/settings?tab=site#section-logo",
			referer:      "http://example.com/admin/settings?mode=edit",
			reqHost:      "example.com",
			kind:         "success",
			message:      "Saved",
			wantLocation: "/admin/settings?mode=edit&msg=Saved&notice=success&tab=site#section-logo",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "http://example.com/submit", nil)
			if tt.reqHost != "" {
				req.Host = tt.reqHost
			}
			if tt.fwdHost != "" {
				req.Header.Set("X-Forwarded-Host", tt.fwdHost)
			}
			if tt.referer != "" {
				req.Header.Set("Referer", tt.referer)
			}
			if tt.isHX {
				req.Header.Set("HX-Request", "true")
			}

			w := httptest.NewRecorder()
			h.redirectWithNotice(w, req, tt.path, tt.kind, tt.message)

			if tt.isHX {
				if w.Code != http.StatusOK {
					t.Fatalf("expected status 200 for HX-Request, got %d", w.Code)
				}
				hxRedir := w.Header().Get("HX-Redirect")
				assertURLsEqual(t, tt.wantHXRedirect, hxRedir)
			} else {
				if w.Code != http.StatusSeeOther {
					t.Fatalf("expected status 303 SeeOther, got %d", w.Code)
				}
				loc := w.Header().Get("Location")
				assertURLsEqual(t, tt.wantLocation, loc)
			}
		})
	}
}

func assertURLsEqual(t *testing.T, expected, actual string) {
	t.Helper()
	uExpected, err := url.Parse(expected)
	if err != nil {
		t.Fatalf("bad expected url %q: %v", expected, err)
	}
	uActual, err := url.Parse(actual)
	if err != nil {
		t.Fatalf("bad actual url %q: %v", actual, err)
	}

	if uExpected.Path != uActual.Path {
		t.Errorf("Path mismatch: expected %q, got %q", uExpected.Path, uActual.Path)
	}
	if uExpected.Fragment != uActual.Fragment {
		t.Errorf("Fragment mismatch: expected %q, got %q", uExpected.Fragment, uActual.Fragment)
	}

	qExpected := uExpected.Query()
	qActual := uActual.Query()

	if len(qExpected) != len(qActual) {
		t.Errorf("Query param count mismatch: expected %v, got %v", qExpected, qActual)
	}
	for k, vExpected := range qExpected {
		vActual, ok := qActual[k]
		if !ok {
			t.Errorf("Missing query param %q in actual URL: %v", k, qActual)
			continue
		}
		if len(vExpected) != len(vActual) || vExpected[0] != vActual[0] {
			t.Errorf("Query param %q value mismatch: expected %v, got %v", k, vExpected, vActual)
		}
	}
}

func TestResolveLocaleAndLangOf(t *testing.T) {
	h := &UIHandler{}

	t.Run("default to arabic and rtl", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/dashboard", nil)
		lang, dir := h.localeAndDir(req)
		if lang != "ar" || dir != "rtl" {
			t.Fatalf("expected (ar, rtl), got (%s, %s)", lang, dir)
		}
		if got := langOf(req); got != "ar" {
			t.Fatalf("expected langOf=ar, got %s", got)
		}
	})

	t.Run("cookie dawa24_lang=en", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/dashboard", nil)
		req.AddCookie(&http.Cookie{Name: "dawa24_lang", Value: "en"})
		lang, dir := h.localeAndDir(req)
		if lang != "en" || dir != "ltr" {
			t.Fatalf("expected (en, ltr), got (%s, %s)", lang, dir)
		}
		if got := langOf(req); got != "en" {
			t.Fatalf("expected langOf=en, got %s", got)
		}
	})

	t.Run("query overrides cookie", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/dashboard?lang=ar", nil)
		req.AddCookie(&http.Cookie{Name: "dawa24_lang", Value: "en"})
		lang, dir := h.localeAndDir(req)
		if lang != "ar" || dir != "rtl" {
			t.Fatalf("expected (ar, rtl), got (%s, %s)", lang, dir)
		}
		if got := langOf(req); got != "ar" {
			t.Fatalf("expected langOf=ar, got %s", got)
		}
	})

	t.Run("accept-language header en when no cookie or query", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/dashboard", nil)
		req.Header.Set("Accept-Language", "en-US,en;q=0.9")
		lang, dir := h.localeAndDir(req)
		if lang != "en" || dir != "ltr" {
			t.Fatalf("expected (en, ltr), got (%s, %s)", lang, dir)
		}
		if got := langOf(req); got != "en" {
			t.Fatalf("expected langOf=en, got %s", got)
		}
	})
}

