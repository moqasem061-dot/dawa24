-- 066_merge_highlight_sections (up)
--
-- Rebuild V2 §2.3 — org.highlight_sections/_items duplicate the promo family.
-- One promo.highlight_sections now serves both audiences: owner_type
-- ('platform' | 'organization') with organization_id for supplier rows. Ids
-- are preserved so item links keep their meaning; the org tables are dropped.

BEGIN;

-- 1. Enrich the promo table.
ALTER TABLE promo.highlight_sections
    ADD COLUMN IF NOT EXISTS owner_type      TEXT NOT NULL DEFAULT 'platform'
        CHECK (owner_type IN ('platform','organization')),
    ADD COLUMN IF NOT EXISTS organization_id BIGINT REFERENCES org.organizations(id) ON DELETE CASCADE,
    -- org.highlight_sections tracks updated_at and the promo table never did.
    -- The merged table must carry it, or the organization rows lose their
    -- modification history on the way across.
    ADD COLUMN IF NOT EXISTS updated_at      TIMESTAMPTZ NOT NULL DEFAULT now();

-- The old UNIQUE(slug) cannot survive the merge: an organization's 'best' and
-- the platform's 'best' are different rows. Platform slugs stay unique.
ALTER TABLE promo.highlight_sections DROP CONSTRAINT IF EXISTS highlight_sections_slug_key;
CREATE UNIQUE INDEX IF NOT EXISTS highlight_sections_platform_slug_key
    ON promo.highlight_sections (slug) WHERE owner_type = 'platform';

-- 2. Migrate the organization rows, preserving ids.
INSERT INTO promo.highlight_sections (
    id, public_id, title, slug, display_order, is_active,
    owner_type, organization_id, created_at, updated_at
)
SELECT id, gen_random_uuid(), title, slug, display_order, is_active,
       'organization', organization_id, created_at, updated_at
FROM org.highlight_sections
ON CONFLICT (id) DO NOTHING;

SELECT setval(pg_get_serial_sequence('promo.highlight_sections', 'id'),
              (SELECT COALESCE(MAX(id), 0) + 1 FROM promo.highlight_sections), false);

-- 3. Migrate the items, preserving ids.
INSERT INTO promo.highlight_section_items (id, section_id, product_id, offer_id, display_order)
SELECT i.id, i.section_id, i.product_id, i.offer_id, i.display_order
FROM org.highlight_section_items i
ON CONFLICT (id) DO NOTHING;

SELECT setval(pg_get_serial_sequence('promo.highlight_section_items', 'id'),
              (SELECT COALESCE(MAX(id), 0) + 1 FROM promo.highlight_section_items), false);

-- 4. Items must reference a real section: the org table enforced it.
ALTER TABLE promo.highlight_section_items
    DROP CONSTRAINT IF EXISTS highlight_section_items_non_empty;
ALTER TABLE promo.highlight_section_items
    ADD CONSTRAINT highlight_section_items_non_empty
        CHECK (product_id IS NOT NULL OR offer_id IS NOT NULL);


-- 5. Tenant isolation: organization rows are tenant-scoped, platform rows are
--    visible to everyone (the policy short-circuits on owner_type).
ALTER TABLE promo.highlight_sections ENABLE ROW LEVEL SECURITY;
ALTER TABLE promo.highlight_sections FORCE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS highlight_sections_tenant_isolation ON promo.highlight_sections;
CREATE POLICY highlight_sections_tenant_isolation ON promo.highlight_sections
    USING (owner_type = 'platform' OR platform.tenant_visible(organization_id))
    WITH CHECK (owner_type = 'platform' OR platform.tenant_visible(organization_id));

ALTER TABLE promo.highlight_section_items ENABLE ROW LEVEL SECURITY;
ALTER TABLE promo.highlight_section_items FORCE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS highlight_items_via_section ON promo.highlight_section_items;
CREATE POLICY highlight_items_via_section ON promo.highlight_section_items
    USING (EXISTS (
        SELECT 1 FROM promo.highlight_sections s
        WHERE s.id = section_id
          AND (s.owner_type = 'platform' OR platform.tenant_visible(s.organization_id))
    ))
    WITH CHECK (EXISTS (
        SELECT 1 FROM promo.highlight_sections s
        WHERE s.id = section_id
          AND (s.owner_type = 'platform' OR platform.tenant_visible(s.organization_id))
    ));

-- 6. Drop the org family.
DROP TABLE org.highlight_section_items;
DROP TABLE org.highlight_sections;

COMMENT ON COLUMN promo.highlight_sections.owner_type IS 'صاحب القسم — platform: أقسام المنصة، organization: أقسام المؤسسة (066)';
COMMENT ON COLUMN promo.highlight_sections.organization_id IS 'المؤسسة المالكة عندما يكون owner_type = organization (066)';

COMMIT;