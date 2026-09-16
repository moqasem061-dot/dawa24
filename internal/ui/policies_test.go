package ui

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
)

func newTestPolicyRouter() http.Handler {
	r := chi.NewRouter()
	h := &UIHandler{}
	h.RegisterPublicRoutes(r)
	return r
}

func TestPolicyPages(t *testing.T) {
	router := newTestPolicyRouter()

	tests := []struct {
		url          string
		expectedCode int
		keywords     []string
	}{
		{"/terms", http.StatusOK, []string{"شروط", "Dawa24"}},
		{"/privacy", http.StatusOK, []string{"الخصوصية", "البيانات"}},
		{"/shipping-returns", http.StatusOK, []string{"الشحن", "الاسترجاع"}},
		{"/refund", http.StatusOK, []string{"الشحن", "الاسترجاع"}},
		{"/cookies", http.StatusOK, []string{"ملفات تعريف الارتباط", "Cookies"}},
		{"/payment-policy", http.StatusOK, []string{"الدفع", "المالية"}},
		{"/payments", http.StatusOK, []string{"الدفع", "المالية"}},
	}

	for _, tc := range tests {
		t.Run(tc.url, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tc.url, nil)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			resp := w.Result()
			if resp.StatusCode != tc.expectedCode {
				t.Fatalf("for %s: expected status %d, got %d", tc.url, tc.expectedCode, resp.StatusCode)
			}

			body, err := io.ReadAll(resp.Body)
			if err != nil {
				t.Fatalf("failed to read body: %v", err)
			}
			bodyStr := string(body)

			for _, kw := range tc.keywords {
				if !strings.Contains(bodyStr, kw) {
					t.Errorf("for %s: expected body to contain %q", tc.url, kw)
				}
			}
		})
	}
}

func TestFooterContainsAllPolicyLinks(t *testing.T) {
	router := newTestPolicyRouter()

	req := httptest.NewRequest(http.MethodGet, "/terms", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %d", resp.StatusCode)
	}

	body, _ := io.ReadAll(resp.Body)
	bodyStr := string(body)

	expectedLinks := []string{
		`href="/privacy"`,
		`href="/terms"`,
		`href="/shipping-returns"`,
		`href="/cookies"`,
		`href="/payment-policy"`,
	}

	for _, link := range expectedLinks {
		if !strings.Contains(bodyStr, link) {
			t.Errorf("footer missing link %s", link)
		}
	}
}

func TestAdminSettingsPolicySubmit_Validation(t *testing.T) {
	h := &UIHandler{}

	// Test missing fields
	req := httptest.NewRequest(http.MethodPost, "/admin/settings/policies", nil)
	w := httptest.NewRecorder()
	h.AdminSettingsPolicySubmit(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusSeeOther {
		t.Errorf("expected 303 See Other redirect, got %d", resp.StatusCode)
	}
	loc := resp.Header.Get("Location")
	if !strings.Contains(loc, "tab=policies") {
		t.Errorf("expected redirect to policies tab, got %s", loc)
	}
}

func TestVendorAgreementRedirect(t *testing.T) {
	router := newTestPolicyRouter()

	req := httptest.NewRequest(http.MethodGet, "/vendor_agreement", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusMovedPermanently {
		t.Fatalf("expected 301 Moved Permanently, got %d", resp.StatusCode)
	}
	if loc := resp.Header.Get("Location"); loc != "/terms" {
		t.Errorf("expected redirect to /terms, got %s", loc)
	}
}

func TestContactAndHowItWorksPages(t *testing.T) {
	router := newTestPolicyRouter()

	t.Run("Contact Page", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/contact", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		resp := w.Result()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d", resp.StatusCode)
		}
		body, _ := io.ReadAll(resp.Body)
		bodyStr := string(body)
		if !strings.Contains(bodyStr, "01065397000") {
			t.Errorf("expected contact page to contain hotline number")
		}
		if !strings.Contains(bodyStr, "support@dawa24.com") {
			t.Errorf("expected contact page to contain support email")
		}
	})

	t.Run("How It Works Page", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/how-it-works", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		resp := w.Result()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d", resp.StatusCode)
		}
		body, _ := io.ReadAll(resp.Body)
		bodyStr := string(body)
		if !strings.Contains(bodyStr, "كيف تعمل منصة دوا 24") {
			t.Errorf("expected how it works page to contain main heading")
		}
		if !strings.Contains(bodyStr, "مسار الصيدليات") {
			t.Errorf("expected how it works page to contain pharmacy workflow")
		}
	})
}
