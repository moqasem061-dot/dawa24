BEGIN;

-- Reinstating catalog.saving_products (Plan V5 Phase 0 Task 0.6)
CREATE TABLE IF NOT EXISTS catalog.saving_products (
    id BIGSERIAL PRIMARY KEY,
    public_id TEXT NOT NULL UNIQUE DEFAULT ('sp_' || substr(md5(random()::text), 1, 16)),
    user_id BIGINT REFERENCES identity.users(id) ON DELETE CASCADE,
    organization_id BIGINT NOT NULL REFERENCES org.organizations(id) ON DELETE CASCADE,
    product_id BIGINT REFERENCES catalog.products(id) ON DELETE SET NULL,
    name_product TEXT NOT NULL,
    sku TEXT,
    qty NUMERIC(10,2) NOT NULL DEFAULT 0.00,
    price NUMERIC(12,2) NOT NULL DEFAULT 0.00,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS saving_products_org_idx ON catalog.saving_products (organization_id) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS saving_products_user_idx ON catalog.saving_products (user_id) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS saving_products_product_idx ON catalog.saving_products (product_id) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS saving_products_name_idx ON catalog.saving_products (name_product) WHERE deleted_at IS NULL;

-- Enable Row-Level Security
ALTER TABLE catalog.saving_products ENABLE ROW LEVEL SECURITY;

-- RLS Policy (Rule R8)
DROP POLICY IF EXISTS saving_products_tenant_isolation ON catalog.saving_products;
CREATE POLICY saving_products_tenant_isolation ON catalog.saving_products
    USING (
        organization_id = NULLIF(current_setting('app.current_tenant', true), '')::BIGINT
        OR current_setting('app.is_system', true) = 'true'
    );

COMMENT ON TABLE catalog.saving_products IS 'قائمة منتجات التوفير للصيدليات والمنشآت لتتبع التوفير والأسعار';
COMMENT ON COLUMN catalog.saving_products.organization_id IS 'معرف المنشأة المالكة لقائمة التوفير';
COMMENT ON COLUMN catalog.saving_products.name_product IS 'اسم المنتج كما تم رفعه من الصيدلية';
COMMENT ON COLUMN catalog.saving_products.qty IS 'الكمية المطلوبة للتوفير';
COMMENT ON COLUMN catalog.saving_products.price IS 'السعر المستهدف للتوفير';

COMMIT;
