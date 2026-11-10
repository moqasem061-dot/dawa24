BEGIN;
-- The item code a buyer's own ERP uses for a product it purchases from a
-- supplier. It maps (buyer organization, variant) to the code the buyer's ERP
-- sent with the purchase line, so the Orgasoft order feed can show the code
-- the buyer knows. A mapping, not a log: rows are never purged.
CREATE TABLE erp.item_codes (
  organization_id BIGINT NOT NULL REFERENCES org.organizations(id) ON DELETE CASCADE,
  variant_id      BIGINT NOT NULL REFERENCES catalog.product_variants(id) ON DELETE CASCADE,
  integration_id  BIGINT REFERENCES erp.integrations(id) ON DELETE SET NULL,
  external_code   TEXT   NOT NULL CHECK (length(external_code) BETWEEN 1 AND 64),
  updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (organization_id, variant_id)
);
ALTER TABLE erp.item_codes ENABLE ROW LEVEL SECURITY;
ALTER TABLE erp.item_codes FORCE ROW LEVEL SECURITY;
CREATE POLICY item_codes_tenant_isolation ON erp.item_codes USING (platform.tenant_visible(organization_id)) WITH CHECK (platform.tenant_visible(organization_id));
COMMENT ON TABLE erp.item_codes IS 'رموز الأصناف في أنظمة المنشآت المشترية، لكل منشأة ومتغير';
COMMENT ON COLUMN erp.item_codes.organization_id IS 'المنشأة المشترية صاحبة الرمز';
COMMENT ON COLUMN erp.item_codes.variant_id IS 'الصنف (المتغير) المشترى من المورد';
COMMENT ON COLUMN erp.item_codes.integration_id IS 'الربط الذي سجّل الرمز';
COMMENT ON COLUMN erp.item_codes.external_code IS 'رمز الصنف في نظام تخطيط موارد المنشأة';
COMMENT ON COLUMN erp.item_codes.updated_at IS 'تاريخ آخر تحديث للرمز';
DO $$ BEGIN IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname='dawa24_app') THEN
  GRANT USAGE ON SCHEMA erp TO dawa24_app;
  GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA erp TO dawa24_app;
  GRANT USAGE, SELECT, UPDATE ON ALL SEQUENCES IN SCHEMA erp TO dawa24_app;
  ALTER DEFAULT PRIVILEGES IN SCHEMA erp GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES TO dawa24_app;
  ALTER DEFAULT PRIVILEGES IN SCHEMA erp GRANT USAGE, SELECT, UPDATE ON SEQUENCES TO dawa24_app;
END IF; END $$;
COMMIT;
