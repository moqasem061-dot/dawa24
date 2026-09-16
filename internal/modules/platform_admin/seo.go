package platformadmin

import (
	"context"
	"encoding/json"
	"sync"
	"time"
)

// SEOPage models metadata and social crawler configurations for a route pattern.
type SEOPage struct {
	ID               int64           `json:"id"`
	RoutePattern     string          `json:"route_pattern"`
	TitleAR          string          `json:"title_ar"`
	TitleEN          string          `json:"title_en"`
	MetaDescAR       string          `json:"meta_desc_ar"`
	MetaDescEN       string          `json:"meta_desc_en"`
	CanonicalURL     string          `json:"canonical_url"`
	RobotsDirectives string          `json:"robots_directives"`
	OGTitle          string          `json:"og_title"`
	OGDesc           string          `json:"og_desc"`
	OGImage          string          `json:"og_image"`
	TwitterCard      string          `json:"twitter_card"`
	JSONLD           json.RawMessage `json:"json_ld"`
	Keywords         []string        `json:"keywords"`
	IsPublic         bool            `json:"is_public"`
	ChangeFreq       string          `json:"changefreq"`
	Priority         float64         `json:"priority"`
	CreatedAt        time.Time       `json:"created_at"`
	UpdatedAt        time.Time       `json:"updated_at"`
}

// SEOSettings holds platform-wide crawler configurations (robots.txt, sitemap).
type SEOSettings struct {
	RobotsTxt      string    `json:"robots_txt"`
	SitemapEnabled bool      `json:"sitemap_enabled"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// SEOPagesFilter carries pagination and search criteria for SEO route records.
type SEOPagesFilter struct {
	Search   string
	IsPublic *bool
	Limit    int
	Offset   int
}

// ListSEOPages returns paginated SEO route entries.
func (s *Service) ListSEOPages(ctx context.Context, filter SEOPagesFilter) ([]*SEOPage, int, error) {
	return s.repo.ListSEOPages(ctx, filter)
}

// GetSEOPageByRoute fetches the SEO configuration for a specific route pattern or exact path.
//
// Every page request resolves its SEO row, and the table changes only when an
// operator edits it, so resolutions are kept for a minute per path. An edit
// through this process clears them at once; other processes see it within the
// minute.
func (s *Service) GetSEOPageByRoute(ctx context.Context, route string) (*SEOPage, error) {
	if page, ok := seoRoutes.get(route); ok {
		return page, nil
	}
	page, err := s.repo.GetSEOPageByRoute(ctx, route)
	if err != nil {
		return nil, err
	}
	seoRoutes.put(route, page)
	return page, nil
}

// UpsertSEOPage updates or creates an SEO configuration row.
func (s *Service) UpsertSEOPage(ctx context.Context, page *SEOPage) error {
	defer seoRoutes.clear()
	return s.repo.UpsertSEOPage(ctx, page)
}

const (
	seoRouteTTL     = time.Minute
	seoRouteEntries = 4096
)

// seoRouteCache holds resolved SEO rows by request path, including "no row".
type seoRouteCache struct {
	mu      sync.RWMutex
	entries map[string]seoRouteEntry
}

type seoRouteEntry struct {
	page    *SEOPage
	expires time.Time
}

var seoRoutes = &seoRouteCache{entries: map[string]seoRouteEntry{}}

func (c *seoRouteCache) get(route string) (*SEOPage, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	e, ok := c.entries[route]
	if !ok || time.Now().After(e.expires) {
		return nil, false
	}
	return e.page, true
}

func (c *seoRouteCache) put(route string, page *SEOPage) {
	c.mu.Lock()
	defer c.mu.Unlock()
	// Paths carry ids, so the key space is unbounded; start over rather than
	// grow without limit.
	if len(c.entries) >= seoRouteEntries {
		c.entries = map[string]seoRouteEntry{}
	}
	c.entries[route] = seoRouteEntry{page: page, expires: time.Now().Add(seoRouteTTL)}
}

func (c *seoRouteCache) clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries = map[string]seoRouteEntry{}
}

// GetSEOSettings fetches global crawler configurations.
func (s *Service) GetSEOSettings(ctx context.Context) (*SEOSettings, error) {
	return s.repo.GetSEOSettings(ctx)
}

// UpdateSEORobotsTxt updates the live robots.txt directive text in the database.
func (s *Service) UpdateSEORobotsTxt(ctx context.Context, robotsTxt string) error {
	return s.repo.UpdateSEORobotsTxt(ctx, robotsTxt)
}
