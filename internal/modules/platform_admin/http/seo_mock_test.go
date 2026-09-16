package http_test

import (
	"context"

	platformadmin "github.com/muhiya/dawa24-store/internal/modules/platform_admin"
)

func (r stubRepo) ListSEOPages(context.Context, platformadmin.SEOPagesFilter) ([]*platformadmin.SEOPage, int, error) {
	r.fail("ListSEOPages")
	return nil, 0, nil
}
func (r stubRepo) GetSEOPageByRoute(context.Context, string) (*platformadmin.SEOPage, error) {
	r.fail("GetSEOPageByRoute")
	return nil, nil
}
func (r stubRepo) UpsertSEOPage(context.Context, *platformadmin.SEOPage) error {
	r.fail("UpsertSEOPage")
	return nil
}
func (r stubRepo) GetSEOSettings(context.Context) (*platformadmin.SEOSettings, error) {
	r.fail("GetSEOSettings")
	return nil, nil
}
func (r stubRepo) UpdateSEORobotsTxt(context.Context, string) error {
	r.fail("UpdateSEORobotsTxt")
	return nil
}

func (happyRepo) ListSEOPages(context.Context, platformadmin.SEOPagesFilter) ([]*platformadmin.SEOPage, int, error) {
	return nil, 0, nil
}
func (happyRepo) GetSEOPageByRoute(context.Context, string) (*platformadmin.SEOPage, error) {
	return nil, nil
}
func (happyRepo) UpsertSEOPage(context.Context, *platformadmin.SEOPage) error {
	return nil
}
func (happyRepo) GetSEOSettings(context.Context) (*platformadmin.SEOSettings, error) {
	return &platformadmin.SEOSettings{RobotsTxt: "User-agent: *\nDisallow: /admin/\n", SitemapEnabled: true}, nil
}
func (happyRepo) UpdateSEORobotsTxt(context.Context, string) error {
	return nil
}
