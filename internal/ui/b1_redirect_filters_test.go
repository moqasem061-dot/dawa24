package ui

import (
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

func TestB1_RedirectWithNotice_PreservesFilters(t *testing.T) {
	h := &UIHandler{}

	t.Run("preserves referer query parameters on same path and host", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/admin/users/123/suspend", nil)
		req.Host = "store.dawa24.com"
		req.Header.Set("Referer", "https://store.dawa24.com/admin/users?q=ahmed&role=vendor&page=3&limit=50")

		rec := httptest.NewRecorder()
		h.redirectWithNotice(rec, req, "/admin/users", "success", "تم إيقاف المستخدم")

		loc := rec.Header().Get("Location")
		if loc == "" {
			t.Fatalf("expected Location header on redirect, got empty")
		}

		u, err := url.Parse(loc)
		if err != nil {
			t.Fatalf("parse location url %q: %v", loc, err)
		}

		q := u.Query()
		if q.Get("q") != "ahmed" {
			t.Errorf("expected q=ahmed, got %q", q.Get("q"))
		}
		if q.Get("role") != "vendor" {
			t.Errorf("expected role=vendor, got %q", q.Get("role"))
		}
		if q.Get("page") != "3" {
			t.Errorf("expected page=3, got %q", q.Get("page"))
		}
		if q.Get("limit") != "50" {
			t.Errorf("expected limit=50, got %q", q.Get("limit"))
		}
		if q.Get("notice") != "success" {
			t.Errorf("expected notice=success, got %q", q.Get("notice"))
		}
		if q.Get("msg") != "تم إيقاف المستخدم" {
			t.Errorf("expected msg='تم إيقاف المستخدم', got %q", q.Get("msg"))
		}
	})

	t.Run("ignores referer from different host", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/admin/users/123/suspend", nil)
		req.Host = "store.dawa24.com"
		req.Header.Set("Referer", "https://evil.com/admin/users?q=hacked&role=admin")

		rec := httptest.NewRecorder()
		h.redirectWithNotice(rec, req, "/admin/users", "success", "تم بنجاح")

		loc := rec.Header().Get("Location")
		u, _ := url.Parse(loc)
		q := u.Query()
		if q.Get("q") != "" || q.Get("role") != "" {
			t.Errorf("expected evil referer params to be dropped, got %v", q)
		}
	})

	t.Run("ignores referer from different path", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/admin/users/123/suspend", nil)
		req.Host = "store.dawa24.com"
		req.Header.Set("Referer", "https://store.dawa24.com/admin/dashboard?q=ahmed")

		rec := httptest.NewRecorder()
		h.redirectWithNotice(rec, req, "/admin/users", "success", "تم بنجاح")

		loc := rec.Header().Get("Location")
		u, _ := url.Parse(loc)
		q := u.Query()
		if q.Get("q") != "" {
			t.Errorf("expected different path params to be dropped, got %v", q)
		}
	})

	t.Run("drops flash message params from referer to avoid loops", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/admin/users/123/suspend", nil)
		req.Host = "store.dawa24.com"
		req.Header.Set("Referer", "https://store.dawa24.com/admin/users?q=ahmed&notice=old&msg=oldmsg&notice_type=oldtype&notice_msg=oldnotice")

		rec := httptest.NewRecorder()
		h.redirectWithNotice(rec, req, "/admin/users", "success", "جديد")

		loc := rec.Header().Get("Location")
		u, _ := url.Parse(loc)
		q := u.Query()
		if q.Get("notice") != "success" {
			t.Errorf("expected notice=success, got %q", q.Get("notice"))
		}
		if q.Get("msg") != "جديد" {
			t.Errorf("expected msg='جديد', got %q", q.Get("msg"))
		}
		if q.Has("notice_type") || q.Has("notice_msg") {
			t.Errorf("expected old notice_type/notice_msg to be stripped, got %v", q)
		}
	})

	t.Run("explicit target params override referer", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/admin/users/create", nil)
		req.Host = "store.dawa24.com"
		req.Header.Set("Referer", "https://store.dawa24.com/admin/users?tab=all")

		rec := httptest.NewRecorder()
		h.redirectWithNotice(rec, req, "/admin/users?tab=staff", "success", "تمت الإضافة")

		loc := rec.Header().Get("Location")
		u, _ := url.Parse(loc)
		if u.Query().Get("tab") != "staff" {
			t.Errorf("expected explicit tab=staff to win, got %q", u.Query().Get("tab"))
		}
	})

	t.Run("supports HTMX HX-Redirect", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/admin/users/123/suspend", nil)
		req.Host = "store.dawa24.com"
		req.Header.Set("HX-Request", "true")
		req.Header.Set("Referer", "https://store.dawa24.com/admin/users?q=ahmed")

		rec := httptest.NewRecorder()
		h.redirectWithNotice(rec, req, "/admin/users", "success", "تم")

		hxLoc := rec.Header().Get("HX-Redirect")
		if hxLoc == "" {
			t.Fatalf("expected HX-Redirect header, got empty")
		}
		u, _ := url.Parse(hxLoc)
		if u.Query().Get("q") != "ahmed" {
			t.Errorf("expected q=ahmed in HX-Redirect, got %q", u.Query().Get("q"))
		}
	})

	t.Run("boosted request gets a real 303, not HX-Redirect", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/admin/users/123/suspend", nil)
		req.Header.Set("HX-Request", "true")
		req.Header.Set("HX-Boosted", "true")
		rec := httptest.NewRecorder()
		h.redirectWithNotice(rec, req, "/admin/users", "success", "ok")
		if rec.Code != 303 {
			t.Fatalf("expected 303 for boosted request, got %d", rec.Code)
		}
		if rec.Header().Get("HX-Redirect") != "" {
			t.Fatalf("boosted request must not receive HX-Redirect")
		}
		if rec.Header().Get("Location") == "" {
			t.Fatalf("expected Location header")
		}
	})
}

func TestB1_PaginationQueryValues(t *testing.T) {
	t.Run("product children pager carries every active filter", func(t *testing.T) {
		// The page grew a branch, warehouse, stock and expiry filter. A pager
		// that carried only the search term and the status would silently widen
		// the question on page two, which is the defect B1 exists to prevent.
		q := pages.AdminProductChildrenData{
			SearchQuery:    "aspirin",
			StatusFilter:   "active",
			OrganizationID: 192,
			BranchID:       76,
			WarehouseID:    51,
			StockFilter:    "low",
			ExpiringSoon:   true,
		}.QueryValues()

		for key, want := range map[string]string{
			"q":            "aspirin",
			"status":       "active",
			"org_id":       "192",
			"branch_id":    "76",
			"warehouse_id": "51",
			"stock":        "low",
			"expiring":     "1",
		} {
			if got := q.Get(key); got != want {
				t.Errorf("%s: got %q, want %q", key, got, want)
			}
		}

		qEmpty := pages.AdminProductChildrenData{StatusFilter: "all"}.QueryValues()
		if len(qEmpty) != 0 {
			t.Errorf("expected no query values when nothing is filtered, got %v", qEmpty)
		}
	})

	t.Run("adminOrganizationsQuery omits empty and all values", func(t *testing.T) {
		data := pages.AdminOrganizationsPageData{
			SearchQuery:  "dawa",
			TypeFilter:   "vendor",
			StatusFilter: "approved",
		}
		q := pages.AdminOrganizationsQuery(data)
		if q.Get("q") != "dawa" || q.Get("type") != "vendor" || q.Get("status") != "approved" {
			t.Errorf("unexpected query values: %v", q)
		}

		emptyData := pages.AdminOrganizationsPageData{
			TypeFilter:   "all",
			StatusFilter: "all",
		}
		qEmpty := pages.AdminOrganizationsQuery(emptyData)
		if len(qEmpty) != 0 {
			t.Errorf("expected empty query values for all/blank, got %v", qEmpty)
		}
	})

	t.Run("adminOrdersQueryValues omits zero org IDs", func(t *testing.T) {
		data := pages.AdminOrdersData{
			ActiveTab:     "direct",
			Query:         "ORD-100",
			CustomerOrgID: 0,
			VendorOrgID:   505,
		}
		q := pages.AdminOrdersQueryValues(data)
		if q.Get("tab") != "direct" {
			t.Errorf("expected tab=direct, got %q", q.Get("tab"))
		}
		if q.Get("q") != "ORD-100" {
			t.Errorf("expected q=ORD-100, got %q", q.Get("q"))
		}
		if q.Has("customer_org_id") {
			t.Errorf("expected customer_org_id 0 to be omitted, got %q", q.Get("customer_org_id"))
		}
		if q.Get("vendor_org_id") != "505" {
			t.Errorf("expected vendor_org_id=505, got %q", q.Get("vendor_org_id"))
		}
	})

	t.Run("adminCitiesQuery omits zero governorate ID and blank query", func(t *testing.T) {
		data := pages.AdminCitiesData{
			SelectedGovernorateID: 0,
			Query:                 "",
		}
		q := pages.AdminCitiesQuery(data)
		if len(q) != 0 {
			t.Errorf("expected empty query values, got %v", q)
		}

		dataWithGov := pages.AdminCitiesData{
			SelectedGovernorateID: 12,
			Query:                 "nasr",
		}
		qWithGov := pages.AdminCitiesQuery(dataWithGov)
		if qWithGov.Get("gov_id") != "12" || qWithGov.Get("q") != "nasr" {
			t.Errorf("unexpected query values: %v", qWithGov)
		}
	})
}
