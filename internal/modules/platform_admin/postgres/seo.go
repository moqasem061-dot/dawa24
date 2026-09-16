package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	platformadmin "github.com/muhiya/dawa24-store/internal/modules/platform_admin"
)

const seoPageFields = `
	id, route_pattern, title_ar, title_en, meta_desc_ar, meta_desc_en,
	canonical_url, robots_directives, og_title, og_desc, og_image,
	twitter_card, json_ld, keywords, is_public, changefreq, priority,
	created_at, updated_at
`

func scanSEOPage(row pgx.Row) (*platformadmin.SEOPage, error) {
	var p platformadmin.SEOPage
	var rawJSON []byte
	var keywords []string

	err := row.Scan(
		&p.ID,
		&p.RoutePattern,
		&p.TitleAR,
		&p.TitleEN,
		&p.MetaDescAR,
		&p.MetaDescEN,
		&p.CanonicalURL,
		&p.RobotsDirectives,
		&p.OGTitle,
		&p.OGDesc,
		&p.OGImage,
		&p.TwitterCard,
		&rawJSON,
		&keywords,
		&p.IsPublic,
		&p.ChangeFreq,
		&p.Priority,
		&p.CreatedAt,
		&p.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	p.JSONLD = json.RawMessage(rawJSON)
	p.Keywords = keywords
	return &p, nil
}

// ListSEOPages returns paginated SEO route entries matching optional filters.
func (r *Repository) ListSEOPages(ctx context.Context, filter platformadmin.SEOPagesFilter) ([]*platformadmin.SEOPage, int, error) {
	var pages []*platformadmin.SEOPage
	var total int

	err := r.db.InReadTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		var whereClauses []string
		var args []any
		argIdx := 1

		if filter.Search != "" {
			term := "%" + strings.TrimSpace(filter.Search) + "%"
			whereClauses = append(whereClauses, fmt.Sprintf("(route_pattern ILIKE $%d OR title_ar ILIKE $%d OR title_en ILIKE $%d)", argIdx, argIdx, argIdx))
			args = append(args, term)
			argIdx++
		}

		if filter.IsPublic != nil {
			whereClauses = append(whereClauses, fmt.Sprintf("is_public = $%d", argIdx))
			args = append(args, *filter.IsPublic)
			argIdx++
		}

		whereSQL := ""
		if len(whereClauses) > 0 {
			whereSQL = " WHERE " + strings.Join(whereClauses, " AND ")
		}

		countQuery := "SELECT count(*) FROM platform_admin.seo_pages" + whereSQL
		if err := tx.QueryRow(txCtx, countQuery, args...).Scan(&total); err != nil {
			return err
		}

		limit := filter.Limit
		if limit <= 0 {
			limit = 50
		}
		offset := filter.Offset
		if offset < 0 {
			offset = 0
		}

		listQuery := "SELECT " + seoPageFields + " FROM platform_admin.seo_pages" + whereSQL +
			fmt.Sprintf(" ORDER BY is_public DESC, priority DESC, route_pattern ASC LIMIT $%d OFFSET $%d", argIdx, argIdx+1)
		listArgs := append(args, limit, offset)

		rows, err := tx.Query(txCtx, listQuery, listArgs...)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			p, sErr := scanSEOPage(rows)
			if sErr != nil {
				return sErr
			}
			pages = append(pages, p)
		}
		return rows.Err()
	})

	if err != nil {
		return nil, 0, err
	}
	return pages, total, nil
}

// GetSEOPageByRoute fetches the SEO configuration for a path with fallback prefix matching.
func (r *Repository) GetSEOPageByRoute(ctx context.Context, route string) (*platformadmin.SEOPage, error) {
	var page *platformadmin.SEOPage
	err := r.db.InReadTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		// An exact match (the root also matches an empty pattern) wins over the
		// longest prefix, in one statement rather than two.
		query := "SELECT " + seoPageFields + ` FROM platform_admin.seo_pages
			WHERE route_pattern = $1 OR ($1 = '/' AND route_pattern = '') OR $1 LIKE route_pattern || '%'
			ORDER BY (route_pattern = $1 OR ($1 = '/' AND route_pattern = '')) DESC, length(route_pattern) DESC
			LIMIT 1`
		p, err := scanSEOPage(tx.QueryRow(txCtx, query, route))
		if err == pgx.ErrNoRows {
			return nil
		}
		if err != nil {
			return err
		}
		page = p
		return nil
	})

	if err != nil {
		return nil, err
	}
	return page, nil
}

// UpsertSEOPage inserts or updates an SEO configuration row.
func (r *Repository) UpsertSEOPage(ctx context.Context, page *platformadmin.SEOPage) error {
	return r.db.InTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		jsonLD := page.JSONLD
		if len(jsonLD) == 0 {
			jsonLD = json.RawMessage("{}")
		}
		keywords := page.Keywords
		if keywords == nil {
			keywords = []string{}
		}

		query := `
			INSERT INTO platform_admin.seo_pages (
				route_pattern, title_ar, title_en, meta_desc_ar, meta_desc_en,
				canonical_url, robots_directives, og_title, og_desc, og_image,
				twitter_card, json_ld, keywords, is_public, changefreq, priority,
				updated_at
			) VALUES (
				$1, $2, $3, $4, $5,
				$6, $7, $8, $9, $10,
				$11, $12, $13, $14, $15, $16,
				now()
			)
			ON CONFLICT (route_pattern) DO UPDATE SET
				title_ar = EXCLUDED.title_ar,
				title_en = EXCLUDED.title_en,
				meta_desc_ar = EXCLUDED.meta_desc_ar,
				meta_desc_en = EXCLUDED.meta_desc_en,
				canonical_url = EXCLUDED.canonical_url,
				robots_directives = EXCLUDED.robots_directives,
				og_title = EXCLUDED.og_title,
				og_desc = EXCLUDED.og_desc,
				og_image = EXCLUDED.og_image,
				twitter_card = EXCLUDED.twitter_card,
				json_ld = EXCLUDED.json_ld,
				keywords = EXCLUDED.keywords,
				is_public = EXCLUDED.is_public,
				changefreq = EXCLUDED.changefreq,
				priority = EXCLUDED.priority,
				updated_at = now()
			RETURNING id;
		`
		return tx.QueryRow(txCtx, query,
			page.RoutePattern, page.TitleAR, page.TitleEN, page.MetaDescAR, page.MetaDescEN,
			page.CanonicalURL, page.RobotsDirectives, page.OGTitle, page.OGDesc, page.OGImage,
			page.TwitterCard, jsonLD, keywords, page.IsPublic, page.ChangeFreq, page.Priority,
		).Scan(&page.ID)
	})
}

// GetSEOSettings retrieves crawler settings or returns complete default.
func (r *Repository) GetSEOSettings(ctx context.Context) (*platformadmin.SEOSettings, error) {
	var s platformadmin.SEOSettings
	err := r.db.InReadTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		query := "SELECT robots_txt, sitemap_enabled, updated_at FROM platform_admin.seo_settings WHERE id = 1"
		row := tx.QueryRow(txCtx, query)
		return row.Scan(&s.RobotsTxt, &s.SitemapEnabled, &s.UpdatedAt)
	})
	if err != nil {
		if err == pgx.ErrNoRows {
			return &platformadmin.SEOSettings{
				RobotsTxt:      "User-agent: *\nContent-Signal: ai-train=no, search=yes, ai-input=no\nAllow: /\nSitemap: /sitemap.xml",
				SitemapEnabled: true,
			}, nil
		}
		return nil, err
	}
	return &s, nil
}

// UpdateSEORobotsTxt updates robots.txt content in the database.
func (r *Repository) UpdateSEORobotsTxt(ctx context.Context, robotsTxt string) error {
	return r.db.InTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			INSERT INTO platform_admin.seo_settings (id, robots_txt, sitemap_enabled, updated_at)
			VALUES (1, $1, true, now())
			ON CONFLICT (id) DO UPDATE SET
				robots_txt = EXCLUDED.robots_txt,
				updated_at = now();
		`
		_, err := tx.Exec(txCtx, query, robotsTxt)
		return err
	})
}
