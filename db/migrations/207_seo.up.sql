BEGIN;

CREATE TABLE IF NOT EXISTS platform_admin.seo_pages (
    id                BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    route_pattern     TEXT NOT NULL UNIQUE,
    title_ar          TEXT NOT NULL DEFAULT '',
    title_en          TEXT NOT NULL DEFAULT '',
    meta_desc_ar      TEXT NOT NULL DEFAULT '',
    meta_desc_en      TEXT NOT NULL DEFAULT '',
    canonical_url     TEXT NOT NULL DEFAULT '',
    robots_directives TEXT NOT NULL DEFAULT 'index, follow',
    og_title          TEXT NOT NULL DEFAULT '',
    og_desc           TEXT NOT NULL DEFAULT '',
    og_image          TEXT NOT NULL DEFAULT '/static/img/og-default.jpg',
    twitter_card      TEXT NOT NULL DEFAULT 'summary_large_image',
    json_ld           JSONB NOT NULL DEFAULT '{}'::jsonb,
    keywords          TEXT[] NOT NULL DEFAULT '{}',
    is_public         BOOLEAN NOT NULL DEFAULT true,
    changefreq        TEXT NOT NULL DEFAULT 'monthly',
    priority          NUMERIC(3,2) NOT NULL DEFAULT 0.70,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_seo_pages_is_public ON platform_admin.seo_pages (is_public);
CREATE INDEX IF NOT EXISTS idx_seo_pages_route ON platform_admin.seo_pages (route_pattern);

CREATE TABLE IF NOT EXISTS platform_admin.seo_settings (
    id              INT PRIMARY KEY DEFAULT 1,
    robots_txt      TEXT NOT NULL,
    sitemap_enabled BOOLEAN NOT NULL DEFAULT true,
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Seed default robots.txt content
INSERT INTO platform_admin.seo_settings (id, robots_txt, sitemap_enabled)
VALUES (
    1,
    'User-agent: *
Content-Signal: ai-train=no, search=yes, ai-input=no
Disallow: /admin/
Disallow: /vendor/
Disallow: /account/
Disallow: /customer/
Disallow: /api/
Allow: /api/v1/openapi.json
Allow: /api/v1/openapi.yaml
Disallow: /catalog?*page=
Disallow: /catalog?*page_size=
Disallow: /catalog?*sort=
Disallow: /catalog?*min_price=
Disallow: /catalog?*max_price=
Disallow: /catalog?*in_stock=
Disallow: /catalog?*has_discount=
Allow: /
Crawl-delay: 5

User-agent: ChatGPT-User
Allow: /
Content-Signal: ai-train=no, search=yes, ai-input=yes

User-agent: OAI-SearchBot
Allow: /
Content-Signal: ai-train=no, search=yes, ai-input=yes

User-agent: Perplexity-User
Allow: /
Content-Signal: ai-train=no, search=yes, ai-input=yes

User-agent: Claude-User
Allow: /
Content-Signal: ai-train=no, search=yes, ai-input=yes

User-agent: Claude-SearchBot
Allow: /
Content-Signal: ai-train=no, search=yes, ai-input=yes

User-agent: DuckAssistBot
Allow: /
Content-Signal: ai-train=no, search=yes, ai-input=yes

User-agent: AhrefsBot
Disallow: /
Content-Signal: ai-train=no, search=no, ai-input=no

User-agent: SemrushBot
Disallow: /
Content-Signal: ai-train=no, search=no, ai-input=no

User-agent: GPTBot
Disallow: /
Content-Signal: ai-train=no, search=no, ai-input=no

User-agent: ClaudeBot
Disallow: /
Content-Signal: ai-train=no, search=no, ai-input=no

User-agent: CCBot
Disallow: /
Content-Signal: ai-train=no, search=no, ai-input=no

User-agent: Google-Extended
Disallow: /
Content-Signal: ai-train=no, search=no, ai-input=no

User-agent: Applebot-Extended
Disallow: /
Content-Signal: ai-train=no, search=no, ai-input=no

User-agent: Meta-ExternalAgent
Disallow: /
Content-Signal: ai-train=no, search=no, ai-input=no

Sitemap: /sitemap.xml',
    true
)
ON CONFLICT (id) DO NOTHING;

-- Seed seo_pages from managed_pages
INSERT INTO platform_admin.seo_pages (
    route_pattern,
    title_ar,
    title_en,
    meta_desc_ar,
    meta_desc_en,
    canonical_url,
    robots_directives,
    og_title,
    og_desc,
    og_image,
    twitter_card,
    is_public,
    changefreq,
    priority
)
SELECT
    path,
    COALESCE(label->>'ar', path),
    COALESCE(label->>'en', path),
    COALESCE(description, ''),
    '',
    path,
    CASE WHEN resource IN ('admin', 'vendor') THEN 'noindex, nofollow' ELSE 'index, follow' END,
    COALESCE(label->>'ar', path),
    COALESCE(description, ''),
    '/static/img/og-default.jpg',
    'summary_large_image',
    CASE WHEN resource IN ('admin', 'vendor') THEN false ELSE true END,
    CASE WHEN path = '/' THEN 'daily' WHEN path IN ('/catalog', '/suppliers') THEN 'daily' ELSE 'monthly' END,
    CASE WHEN path = '/' THEN 1.00 WHEN path IN ('/catalog', '/suppliers') THEN 0.90 WHEN resource IN ('admin', 'vendor') THEN 0.10 ELSE 0.70 END
FROM platform_admin.managed_pages
WHERE deleted_at IS NULL
ON CONFLICT (route_pattern) DO NOTHING;

COMMIT;
