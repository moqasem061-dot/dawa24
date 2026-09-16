package ui

import (
	"net/http"
	"strings"

	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/httpx"
	"github.com/muhiya/dawa24-store/internal/platform/pagecontrol"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

// PageMaintenanceHandler renders a beautifully designed 503 maintenance page
// whenever an anonymous or authenticated user accesses a route that has been disabled
// by administrators in the page control console.
func (h *UIHandler) PageMaintenanceHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	path := r.URL.Path

	// 1. If API or JSON request, return structured 503 JSON
	isAPI := strings.HasPrefix(path, "/api/") ||
		strings.Contains(r.Header.Get("Accept"), "application/json") ||
		(r.Header.Get("HX-Request") == "true" && r.Header.Get("HX-Boosted") != "true" && r.Method != http.MethodGet)

	if isAPI {
		w.Header().Set("Retry-After", "300")
		httpx.Error(w, r, h.log, apperr.New(apperr.KindUnavailable, "page.under_maintenance",
			"هذه الصفحة أو الخدمة تحت الصيانة حالياً. يرجى المحاولة لاحقاً."))
		return
	}

	// 2. Resolve page control metadata
	info, ok := pagecontrol.BlockedInfoFrom(ctx)
	if !ok {
		_, info = pagecontrol.CheckBlocked(path)
	}

	lang, dir := h.localeAndDir(r)

	pageName := info.Label(lang)
	if pageName == info.Path || pageName == path {
		pageName = ""
	}

	// 3. Resolve appropriate Home / Dashboard URL based on authentication
	homeURL := "/"
	if actor, ok := authctx.From(ctx); ok && actor.UserID > 0 {
		if actor.IsPlatformAdmin() {
			homeURL = "/admin/dashboard"
		} else if actor.IsVendor() {
			homeURL = "/vendor/dashboard"
		} else if actor.IsCustomer() {
			homeURL = "/customer/dashboard"
		}
	}

	data := pages.MaintenancePageView{
		PageName:    pageName,
		Path:        path,
		Description: info.Description,
		HomeURL:     homeURL,
		SupportURL:  "/help",
	}

	w.Header().Set("Retry-After", "300")
	w.WriteHeader(http.StatusServiceUnavailable)

	h.renderPage(ctx, w, "render maintenance page", pages.MaintenancePage(data, lang, dir))
}

// AdminSystemPagesMaintenancePreview renders a live preview of the maintenance page
// so platform administrators can review the design and messaging directly.
func (h *UIHandler) AdminSystemPagesMaintenancePreview(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	testPath := r.URL.Query().Get("path")
	if testPath == "" {
		testPath = "/market"
	}
	testName := r.URL.Query().Get("name")
	if testName == "" {
		testName = "سوق الأدوية والمستلزمات"
	}

	data := pages.MaintenancePageView{
		PageName:    testName,
		Path:        testPath,
		Description: "نعمل حالياً على إجراء أعمال صيانة دورية وتحديثات هامة لتحسين الأداء وتطوير تجربة الاستخدام. نعتذر عن أي إزعاج مؤقت، وسيعود هذا القسم للعمل قريباً.",
		HomeURL:     "/admin/system-pages",
		SupportURL:  "/help",
	}

	h.renderPage(ctx, w, "preview maintenance page", pages.MaintenancePage(data, lang, dir))
}
