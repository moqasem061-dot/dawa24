package platformadmin

import (
	"context"
	"io"
	"log/slog"
	"testing"
)

func (m *mockPlatformAdminRepo) ListSEOPages(_ context.Context, _ SEOPagesFilter) ([]*SEOPage, int, error) {
	return []*SEOPage{
		{
			ID:           1,
			RoutePattern: "/",
			TitleAR:      "الرئيسية",
			TitleEN:      "Home",
			IsPublic:     true,
		},
	}, 1, nil
}

func (m *mockPlatformAdminRepo) GetSEOPageByRoute(_ context.Context, route string) (*SEOPage, error) {
	if route == "/" {
		return &SEOPage{
			ID:           1,
			RoutePattern: "/",
			TitleAR:      "الرئيسية",
			TitleEN:      "Home",
			CanonicalURL: "https://dawa24.com/",
			IsPublic:     true,
		}, nil
	}
	return nil, nil
}

func (m *mockPlatformAdminRepo) UpsertSEOPage(_ context.Context, page *SEOPage) error {
	page.ID = 1
	return nil
}

func (m *mockPlatformAdminRepo) GetSEOSettings(_ context.Context) (*SEOSettings, error) {
	return &SEOSettings{
		RobotsTxt:      "User-agent: *\nAllow: /",
		SitemapEnabled: true,
	}, nil
}

func (m *mockPlatformAdminRepo) UpdateSEORobotsTxt(_ context.Context, robotsTxt string) error {
	return nil
}

func TestSEOService(t *testing.T) {
	ctx := context.Background()
	repo := newMockPlatformAdminRepo()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	svc := NewService(repo, logger)

	// Test GetSEOPageByRoute
	page, err := svc.GetSEOPageByRoute(ctx, "/")
	if err != nil {
		t.Fatalf("GetSEOPageByRoute failed: %v", err)
	}
	if page == nil || page.TitleAR != "الرئيسية" {
		t.Fatalf("expected title 'الرئيسية', got: %v", page)
	}

	// Test ListSEOPages
	pages, total, err := svc.ListSEOPages(ctx, SEOPagesFilter{})
	if err != nil {
		t.Fatalf("ListSEOPages failed: %v", err)
	}
	if total != 1 || len(pages) != 1 {
		t.Fatalf("expected 1 page, got: %d", total)
	}

	// Test UpsertSEOPage
	err = svc.UpsertSEOPage(ctx, &SEOPage{RoutePattern: "/test"})
	if err != nil {
		t.Fatalf("UpsertSEOPage failed: %v", err)
	}

	// Test Robots.txt
	settings, err := svc.GetSEOSettings(ctx)
	if err != nil {
		t.Fatalf("GetSEOSettings failed: %v", err)
	}
	if settings.RobotsTxt == "" {
		t.Fatalf("expected non-empty robots.txt")
	}
}
