package ui

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/muhiya/dawa24-store/internal/platform/pagecontrol"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

func TestMaintenancePage_Render(t *testing.T) {
	data := pages.MaintenancePageView{
		PageName:    "سوق الأدوية والمستلزمات",
		Path:        "/customer/market",
		Description: "نعمل حالياً على صيانة دورية للمتجر.",
		HomeURL:     "/customer/dashboard",
		SupportURL:  "/help",
	}

	var buf bytes.Buffer
	err := pages.MaintenancePage(data, "ar", "rtl").Render(context.Background(), &buf)
	if err != nil {
		t.Fatalf("failed to render MaintenancePage: %v", err)
	}

	html := buf.String()

	if !strings.Contains(html, "تحت الصيانة حالياً") {
		t.Errorf("expected maintenance text in HTML, got: %s", html)
	}
	if !strings.Contains(html, "سوق الأدوية والمستلزمات") {
		t.Errorf("expected page name in HTML")
	}
	if !strings.Contains(html, "/customer/market") {
		t.Errorf("expected path in HTML")
	}
	if !strings.Contains(html, "وضع الصيانة والتطوير") {
		t.Errorf("expected maintenance badge in HTML")
	}
	if !strings.Contains(html, "العودة للصفحة الرئيسية") {
		t.Errorf("expected home button in HTML")
	}
	if !strings.Contains(html, "إعادة المحاولة") {
		t.Errorf("expected retry button in HTML")
	}
}

func TestPageMaintenanceHandler_HTMLResponse(t *testing.T) {
	h := &UIHandler{log: slog.Default()}

	req := httptest.NewRequest(http.MethodGet, "/vendor/offers", nil)
	rec := httptest.NewRecorder()

	h.PageMaintenanceHandler(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("expected status 503, got %d", rec.Code)
	}
	if retryAfter := rec.Header().Get("Retry-After"); retryAfter != "300" {
		t.Errorf("expected Retry-After: 300, got %s", retryAfter)
	}

	body := rec.Body.String()
	if !strings.Contains(body, "تحت الصيانة حالياً") {
		t.Errorf("expected maintenance message in response body")
	}
}

func TestPageMaintenanceHandler_APIResponse(t *testing.T) {
	h := &UIHandler{log: slog.Default()}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/orders", nil)
	req.Header.Set("Accept", "application/json")
	rec := httptest.NewRecorder()

	h.PageMaintenanceHandler(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("expected status 503, got %d", rec.Code)
	}

	var res map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil {
		t.Fatalf("failed to decode json response: %v", err)
	}

	errMap, ok := res["error"].(map[string]any)
	if !ok {
		t.Fatalf("expected error object in response: %v", res)
	}
	if errMap["code"] != "page.under_maintenance" {
		t.Errorf("expected error code page.under_maintenance, got %v", errMap["code"])
	}
}

func TestPageMaintenanceHandler_WithBlockedInfo(t *testing.T) {
	h := &UIHandler{log: slog.Default()}

	req := httptest.NewRequest(http.MethodGet, "/customer/orders", nil)
	info := pagecontrol.BlockedInfo{
		RuleID:      10,
		Path:        "/customer/orders",
		LabelAr:     "إدارة الطلبات",
		LabelEn:     "Order Management",
		Description: "تحديث قاعدة بيانات الطلبات",
	}

	ctx := pagecontrol.WithBlockedInfo(req.Context(), info)
	req = req.WithContext(ctx)

	rec := httptest.NewRecorder()
	h.PageMaintenanceHandler(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("expected status 503, got %d", rec.Code)
	}

	body := rec.Body.String()
	if !strings.Contains(body, "إدارة الطلبات") {
		t.Errorf("expected Arabic page name 'إدارة الطلبات' in response")
	}
	if !strings.Contains(body, "تحديث قاعدة بيانات الطلبات") {
		t.Errorf("expected custom description in response")
	}
}
