-- 085_product_index.up.sql
-- Denormalized read model for high-throughput product discovery and faceted search (Laravel product_infos / product_search_index parity).
-- Distinct from catalog.product_infos (which is the legacy 5-column key-value attribute bag).

CREATE TABLE IF NOT EXISTS catalog.product_index (
    unique_row_id           TEXT PRIMARY KEY,
    product_id              BIGINT NOT NULL REFERENCES catalog.products(id) ON DELETE CASCADE,
    variant_id              BIGINT REFERENCES catalog.product_variants(id) ON DELETE CASCADE,
    sku                     TEXT,
    name_ar                 TEXT,
    name_en                 TEXT,
    search_text             TEXT,
    search_ar               TEXT,
    search_en               TEXT,
    search_simple           TEXT,
    search_vector           tsvector GENERATED ALWAYS AS (
                                to_tsvector('simple', COALESCE(search_text, '') || ' ' || COALESCE(search_simple, ''))
                            ) STORED,
    organization_name       TEXT,
    branch_city             TEXT,
    scientific_name         TEXT,
    price                   NUMERIC(12,2) NOT NULL DEFAULT 0,
    discount                NUMERIC(5,2) NOT NULL DEFAULT 0,
    stock_quantity          INTEGER NOT NULL DEFAULT 0,
    category_id             BIGINT REFERENCES catalog.categories(id) ON DELETE SET NULL,
    brand_id                BIGINT REFERENCES catalog.brands(id) ON DELETE SET NULL,
    has_discount            BOOLEAN NOT NULL DEFAULT false,
    discount_percentage     NUMERIC(5,2) NOT NULL DEFAULT 0,
    price_after_discount    NUMERIC(12,2) NOT NULL DEFAULT 0,
    organization_id         BIGINT NOT NULL REFERENCES org.organizations(id) ON DELETE CASCADE,
    branch_id               BIGINT REFERENCES org.branches(id) ON DELETE SET NULL,
    status                  TEXT NOT NULL DEFAULT 'active',
    product_type            TEXT NOT NULL DEFAULT 'parent',
    institutional_work_ids  BIGINT[] NOT NULL DEFAULT '{}'::BIGINT[],
    created_at              TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at              TIMESTAMPTZ NOT NULL DEFAULT now()
);

COMMENT ON TABLE catalog.product_index IS 'جدول القراءة المفهرس والمجرد للبحث السريع وتصفية المنتجات مع التوافق التام مع منظومة دواء 24';
COMMENT ON COLUMN catalog.product_index.unique_row_id IS 'المعرف الفريد لسجل الفهرس المركب (p_{id} أو p_{id}_v_{id}_b_{id})';
COMMENT ON COLUMN catalog.product_index.institutional_work_ids IS 'معرفات الهيكل والأنشطة المؤسسية المسموح لها برؤية المنتج';

CREATE INDEX IF NOT EXISTS idx_product_index_org_status ON catalog.product_index (organization_id, status);
CREATE INDEX IF NOT EXISTS idx_product_index_branch ON catalog.product_index (branch_id);
CREATE INDEX IF NOT EXISTS idx_product_index_category ON catalog.product_index (category_id);
CREATE INDEX IF NOT EXISTS idx_product_index_brand ON catalog.product_index (brand_id);
CREATE INDEX IF NOT EXISTS idx_product_index_price_discount ON catalog.product_index (price_after_discount ASC);
CREATE INDEX IF NOT EXISTS idx_product_index_inst_works ON catalog.product_index USING GIN (institutional_work_ids);
CREATE INDEX IF NOT EXISTS idx_product_index_search_vector ON catalog.product_index USING GIN (search_vector);
CREATE INDEX IF NOT EXISTS idx_product_index_search_simple_trgm ON catalog.product_index USING GIN (search_simple gin_trgm_ops);

-- RLS Enforcement: Follows tenant isolation. Cross-tenant marketplace search utilizes database.AsSystem with explicit intent documentation.
ALTER TABLE catalog.product_index ENABLE ROW LEVEL SECURITY;
ALTER TABLE catalog.product_index FORCE ROW LEVEL SECURITY;

DO $$ BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_policies 
        WHERE schemaname = 'catalog' 
          AND tablename = 'product_index' 
          AND policyname = 'tenant_isolation_product_index'
    ) THEN
        CREATE POLICY tenant_isolation_product_index ON catalog.product_index
            FOR ALL
            USING (platform.tenant_visible(organization_id))
            WITH CHECK (platform.tenant_visible(organization_id));
    END IF;
END $$;
