package ui

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	platformadmin "github.com/muhiya/dawa24-store/internal/modules/platform_admin"
	"github.com/muhiya/dawa24-store/internal/ui/layouts"
)

type mockSEORepo struct {
	platformadmin.Repository
	pages     map[string]*platformadmin.SEOPage
	robotsTxt string
}

func newMockSEORepo() *mockSEORepo {
	return &mockSEORepo{
		pages: map[string]*platformadmin.SEOPage{
			"/": {
				ID:               1,
				RoutePattern:     "/",
				TitleAR:          "دوا 24 | منصة توريد صيدلانية",
				TitleEN:          "Dawa24 | B2B Pharmacy Marketplace",
				MetaDescAR:       "المنصة الأولى لربط الصيدليات بالموردين والمستودعات في مصر",
				CanonicalURL:     "https://dawa24.com/",
				RobotsDirectives: "index, follow",
				Keywords:         []string{"أدوية", "صيدليات", "توريد"},
				IsPublic:         true,
				ChangeFreq:       "daily",
				Priority:         1.0,
			},
			"/about": {
				ID:               2,
				RoutePattern:     "/about",
				TitleAR:          "عن منصة دوا 24",
				TitleEN:          "About Dawa24",
				MetaDescAR:       "تعرف على رؤية منصة دوا 24 وخدمات التوريد الرقمي",
				CanonicalURL:     "https://dawa24.com/about",
				RobotsDirectives: "index, follow",
				Keywords:         []string{"من نحن", "دوا 24"},
				IsPublic:         true,
				ChangeFreq:       "monthly",
				Priority:         0.8,
			},
		},
		robotsTxt: "User-agent: *\nContent-Signal: ai-train=no, search=yes, ai-input=no\nDisallow: /admin/\n",
	}
}

func (m *mockSEORepo) ListSEOPages(_ context.Context, filter platformadmin.SEOPagesFilter) ([]*platformadmin.SEOPage, int, error) {
	var list []*platformadmin.SEOPage
	for _, p := range m.pages {
		if filter.Search != "" && !strings.Contains(p.RoutePattern, filter.Search) && !strings.Contains(p.TitleAR, filter.Search) {
			continue
		}
		if filter.IsPublic != nil && p.IsPublic != *filter.IsPublic {
			continue
		}
		list = append(list, p)
	}
	return list, len(list), nil
}

func (m *mockSEORepo) GetSEOPageByRoute(_ context.Context, route string) (*platformadmin.SEOPage, error) {
	if p, ok := m.pages[route]; ok {
		return p, nil
	}
	return nil, nil
}

func (m *mockSEORepo) UpsertSEOPage(_ context.Context, page *platformadmin.SEOPage) error {
	page.UpdatedAt = time.Now()
	m.pages[page.RoutePattern] = page
	return nil
}

func (m *mockSEORepo) GetSEOSettings(_ context.Context) (*platformadmin.SEOSettings, error) {
	return &platformadmin.SEOSettings{
		RobotsTxt:      m.robotsTxt,
		SitemapEnabled: true,
		UpdatedAt:      time.Now(),
	}, nil
}

func (m *mockSEORepo) UpdateSEORobotsTxt(_ context.Context, robotsTxt string) error {
	m.robotsTxt = robotsTxt
	return nil
}

func (m *mockSEORepo) GetSetting(_ context.Context, key string) (*platformadmin.SystemSetting, error) {
	return &platformadmin.SystemSetting{
		Key: key,
		Value: map[string]any{
			"endpoint_url": "https://api.muhiya.com",
			"is_active":    true,
		},
	}, nil
}

func (m *mockSEORepo) SetSetting(_ context.Context, _ *platformadmin.SystemSetting) error {
	return nil
}

func (m *mockSEORepo) GetGatewaySettings(_ context.Context) (*platformadmin.GatewaySettings, error) {
	return &platformadmin.GatewaySettings{EndpointURL: "https://api.muhiya.com", IsActive: true}, nil
}

func (m *mockSEORepo) GetAISettings(_ context.Context) (*platformadmin.AISettings, error) {
	return &platformadmin.AISettings{EndpointURL: "https://api.muhiya.com", IsActive: true}, nil
}

func (m *mockSEORepo) ListSQLLogs(_ context.Context, _, _ int) ([]*platformadmin.SQLLog, error) {
	return nil, nil
}

func (m *mockSEORepo) ListAIRoleModels(_ context.Context) ([]*platformadmin.AIRoleModel, error) {
	return nil, nil
}

func (m *mockSEORepo) ListErrorLogs(_ context.Context, _ platformadmin.ErrorLogFilter) ([]*platformadmin.ErrorLog, int, error) {
	return nil, 0, nil
}

func (m *mockSEORepo) GetErrorDiagnosticsMetrics(_ context.Context) (int, int, int, int, error) {
	return 0, 0, 0, 0, nil
}

func (m *mockSEORepo) ListAuditLogWithFilter(_ context.Context, _ platformadmin.AuditLogFilter) ([]*platformadmin.AuditEntry, int, error) {
	return nil, 0, nil
}

func setupSEOTestHandler() (*UIHandler, *mockSEORepo) {
	repo := newMockSEORepo()
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	adminSvc := platformadmin.NewService(repo, logger)
	handler := &UIHandler{log: logger, adminSvc: adminSvc}
	return handler, repo
}

func TestAdminDevelopersPage_SEOTab(t *testing.T) {
	h, _ := setupSEOTestHandler()

	req := httptest.NewRequest(http.MethodGet, "/admin/developers?tab=seo", nil)
	rec := httptest.NewRecorder()

	h.AdminDevelopersPage(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", rec.Code)
	}

	body := rec.Body.String()
	if !strings.Contains(body, "تهيئة محركات البحث والذكاء الاصطناعي") {
		t.Errorf("expected SEO tab header in response")
	}
	if !strings.Contains(body, "دوا 24 | منصة توريد صيدلانية") {
		t.Errorf("expected seeded SEO page title in response")
	}
}

func TestAdminDevelopersSEOSaveSubmit(t *testing.T) {
	h, repo := setupSEOTestHandler()

	form := url.Values{
		"route_pattern":     []string{"/products"},
		"title_ar":          []string{"كتالوج المنتجات والأدوية | دوا 24"},
		"title_en":          []string{"Pharmaceutical Catalog | Dawa24"},
		"meta_desc_ar":      []string{"تصفح جميع الأدوية والمستلزمات الطبية بأفضل الأسعار"},
		"canonical_url":     []string{"https://dawa24.com/products"},
		"robots_directives": []string{"index, follow"},
		"keywords":          []string{"أدوية, صيدليات, كتالوج"},
	}

	req := httptest.NewRequest(http.MethodPost, "/admin/developers/seo/save", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()

	h.AdminDevelopersSEOSaveSubmit(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("expected 303 See Other redirect, got %d", rec.Code)
	}

	loc := rec.Header().Get("Location")
	if !strings.Contains(loc, "tab=seo") {
		t.Errorf("expected redirect to seo tab, got %s", loc)
	}

	saved, ok := repo.pages["/products"]
	if !ok {
		t.Fatalf("expected /products page to be saved in repository")
	}
	if saved.TitleAR != "كتالوج المنتجات والأدوية | دوا 24" {
		t.Errorf("unexpected TitleAR: %s", saved.TitleAR)
	}
}

func TestAdminDevelopersSEORobotsSubmit(t *testing.T) {
	h, repo := setupSEOTestHandler()

	updatedRobots := "User-agent: *\nDisallow: /admin/\nDisallow: /private/\n"
	form := url.Values{
		"robots_txt": []string{updatedRobots},
	}

	req := httptest.NewRequest(http.MethodPost, "/admin/developers/seo/robots", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()

	h.AdminDevelopersSEORobotsSubmit(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("expected 303 redirect, got %d", rec.Code)
	}

	if repo.robotsTxt != updatedRobots {
		t.Errorf("expected robots.txt to be updated in repo, got %q", repo.robotsTxt)
	}
}

func TestSitemapXML_DynamicSEOPages(t *testing.T) {
	h, _ := setupSEOTestHandler()

	req := httptest.NewRequest(http.MethodGet, "/sitemap.xml", nil)
	rec := httptest.NewRecorder()

	h.SitemapXML(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", rec.Code)
	}

	ct := rec.Header().Get("Content-Type")
	if !strings.Contains(ct, "application/xml") {
		t.Errorf("expected application/xml content type, got %s", ct)
	}

	body := rec.Body.String()
	if !strings.Contains(body, "<urlset") || !strings.Contains(body, "</urlset>") {
		t.Errorf("expected valid urlset root in sitemap, got:\n%s", body)
	}
	if !strings.Contains(body, "<loc>https://example.com/</loc>") && !strings.Contains(body, "<loc>http://example.com/</loc>") {
		t.Errorf("expected root loc entry in sitemap, got:\n%s", body)
	}
	if !strings.Contains(body, "<priority>1.0</priority>") {
		t.Errorf("expected priority entry in sitemap, got:\n%s", body)
	}
}

func TestDynamicRobotsTxt_Integration(t *testing.T) {
	h, repo := setupSEOTestHandler()

	r := chi.NewRouter()
	h.RegisterPublicRoutes(r)

	req := httptest.NewRequest(http.MethodGet, "/robots.txt", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", rec.Code)
	}

	headerSig := rec.Header().Get("Content-Signal")
	if !strings.Contains(headerSig, "ai-train=no") {
		t.Errorf("expected Content-Signal header, got %s", headerSig)
	}

	// The operator's file is served, less the lines that would name private
	// areas; those carry X-Robots-Tag instead.
	body := rec.Body.String()
	if !strings.Contains(body, "Content-Signal: ai-train=no, search=yes, ai-input=no") {
		t.Errorf("expected dynamic robots.txt from repo, got:\n%s", body)
	}
	if strings.Contains(body, "/admin/") {
		t.Errorf("robots.txt must not name the admin area, got:\n%s", body)
	}
	_ = repo
}

func TestPrivateAreasAreNotPublished(t *testing.T) {
	for _, p := range []string{"/admin/offers", "/api/v1/auth/me", "/vendor/dashboard", "/customer/orders/{id}", "/cart"} {
		if sitemapPath(p) {
			t.Errorf("sitemap would publish %s", p)
		}
	}
	for _, p := range []string{"/", "/about", "/catalog", "/offers", "/suppliers"} {
		if !sitemapPath(p) {
			t.Errorf("sitemap would omit public page %s", p)
		}
	}
	robots := publicRobots("User-agent: *" + "\n" + "Disallow: /admin/" + "\n" + "disallow: /api/" + "\n" + "Disallow: /catalog?*page=" + "\n" + "Allow: /")
	if strings.Contains(robots, "/admin/") || strings.Contains(robots, "/api/") || !strings.Contains(robots, "/catalog?*page=") {
		t.Errorf("publicRobots kept the wrong lines:\n%s", robots)
	}
}

func TestSEO_ResolveSEOHelper(t *testing.T) {
	ctx := context.Background()
	defaultTitle := "دوا 24"

	// Fallback case: no SEOPage in context
	resolved := layouts.ResolveSEO(ctx, defaultTitle, "ar")
	if resolved.Title != defaultTitle {
		t.Errorf("expected fallback title %q, got %q", defaultTitle, resolved.Title)
	}

	// Active case: with SEOPage in context
	page := &platformadmin.SEOPage{
		RoutePattern:     "/special-deal",
		TitleAR:          "عرض خاص صيدلاني",
		MetaDescAR:       "أقوى العروض الحصرية",
		CanonicalURL:     "https://dawa24.com/special-deal",
		RobotsDirectives: "index, follow",
		OGTitle:          "عرض خاص",
		OGImage:          "https://dawa24.com/deal.jpg",
	}
	ctx = layouts.WithSEOPage(ctx, page)
	resolved = layouts.ResolveSEO(ctx, defaultTitle, "ar")

	if resolved.Title != "عرض خاص صيدلاني" {
		t.Errorf("expected resolved title %q, got %q", "عرض خاص صيدلاني", resolved.Title)
	}
	if resolved.MetaDescription != "أقوى العروض الحصرية" {
		t.Errorf("expected meta description %q, got %q", "أقوى العروض الحصرية", resolved.MetaDescription)
	}
	if resolved.CanonicalURL != "https://dawa24.com/special-deal" {
		t.Errorf("expected canonical URL %q, got %q", "https://dawa24.com/special-deal", resolved.CanonicalURL)
	}
	if resolved.OGImage != "https://dawa24.com/deal.jpg" {
		t.Errorf("expected OGImage %q, got %q", "https://dawa24.com/deal.jpg", resolved.OGImage)
	}
}
