-- 233_curated_identity.up.sql
--
-- Curated identity facts and pack relationships for catalog products (data-model §3.2, §3.3).
-- Platform-owned administrative tables; statement triggers bump catalog_revisions.

BEGIN;

CREATE TABLE IF NOT EXISTS catalog.curated_identity_facts (
    id          BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    product_id  BIGINT NOT NULL REFERENCES catalog.products(id) ON DELETE CASCADE,
    attribute   TEXT NOT NULL,
    value       JSONB NOT NULL,
    reason      TEXT NOT NULL DEFAULT '',
    created_by  TEXT NOT NULL DEFAULT 'system',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    revoked_at  TIMESTAMPTZ NULL,
    revoked_by  TEXT NULL
);

COMMENT ON TABLE catalog.curated_identity_facts IS 'خصائص هوية المنتج المعتمدة والمعدلة يدوياً من مسؤولي المنصة';
COMMENT ON COLUMN catalog.curated_identity_facts.id IS 'المعرف الفريد لسجل الخاصية المعدلة';
COMMENT ON COLUMN catalog.curated_identity_facts.product_id IS 'معرف المنتج في الكتالوج الأساسي';
COMMENT ON COLUMN catalog.curated_identity_facts.attribute IS 'اسم الخاصية (مثال: strength, form, brand)';
COMMENT ON COLUMN catalog.curated_identity_facts.value IS 'القيمة المعتمدة بصيغة JSONB';
COMMENT ON COLUMN catalog.curated_identity_facts.reason IS 'سبب التعديل أو الاعتماد';
COMMENT ON COLUMN catalog.curated_identity_facts.created_by IS 'المستخدم أو النظام الذي أنشأ السجل';
COMMENT ON COLUMN catalog.curated_identity_facts.created_at IS 'تاريخ ووقت إنشاء السجل';
COMMENT ON COLUMN catalog.curated_identity_facts.revoked_at IS 'تاريخ ووقت إلغاء الخاصية إن وجدت';
COMMENT ON COLUMN catalog.curated_identity_facts.revoked_by IS 'المستخدم الذي قام بإلغاء الخاصية';

CREATE UNIQUE INDEX IF NOT EXISTS uq_curated_identity_facts_active
    ON catalog.curated_identity_facts (product_id, attribute)
    WHERE revoked_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_curated_identity_facts_lookup
    ON catalog.curated_identity_facts (product_id)
    WHERE revoked_at IS NULL;

CREATE TABLE IF NOT EXISTS catalog.pack_relationships (
    id          BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    product_id  BIGINT NOT NULL REFERENCES catalog.products(id) ON DELETE CASCADE,
    from_unit   TEXT NOT NULL,
    to_unit     TEXT NOT NULL,
    factor      INT NOT NULL CHECK (factor > 0),
    created_by  TEXT NOT NULL DEFAULT 'system',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    revoked_at  TIMESTAMPTZ NULL,
    revoked_by  TEXT NULL
);

COMMENT ON TABLE catalog.pack_relationships IS 'علاقات ومعاملات التحويل بين وحدات العبوات المعتمدة للمنتج';
COMMENT ON COLUMN catalog.pack_relationships.id IS 'المعرف الفريد لعلاقة العبوة';
COMMENT ON COLUMN catalog.pack_relationships.product_id IS 'معرف المنتج في الكتالوج الأساسي';
COMMENT ON COLUMN catalog.pack_relationships.from_unit IS 'الوحدة المصدر أو الأكبر (مثال: علبة)';
COMMENT ON COLUMN catalog.pack_relationships.to_unit IS 'الوحدة الهدف أو الأصغر (مثال: شريط)';
COMMENT ON COLUMN catalog.pack_relationships.factor IS 'معامل التحويل الصحيح (أكبر من صفر قطعا)';
COMMENT ON COLUMN catalog.pack_relationships.created_by IS 'المستخدم أو النظام الذي أنشأ السجل';
COMMENT ON COLUMN catalog.pack_relationships.created_at IS 'تاريخ ووقت إنشاء السجل';
COMMENT ON COLUMN catalog.pack_relationships.revoked_at IS 'تاريخ ووقت إلغاء العلاقة إن وجدت';
COMMENT ON COLUMN catalog.pack_relationships.revoked_by IS 'المستخدم الذي قام بإلغاء العلاقة';

CREATE UNIQUE INDEX IF NOT EXISTS uq_pack_relationships_active
    ON catalog.pack_relationships (product_id, from_unit, to_unit)
    WHERE revoked_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_pack_relationships_lookup
    ON catalog.pack_relationships (product_id)
    WHERE revoked_at IS NULL;

-- Trigger to bump catalog revision on curated identity changes
DROP TRIGGER IF EXISTS trg_curated_identity_facts_revision_bump ON catalog.curated_identity_facts;
CREATE TRIGGER trg_curated_identity_facts_revision_bump
    AFTER INSERT OR DELETE OR UPDATE
    ON catalog.curated_identity_facts
    FOR EACH STATEMENT
    EXECUTE FUNCTION catalog.bump_master_catalog_revision();

-- Trigger to bump catalog revision on pack relationship changes
DROP TRIGGER IF EXISTS trg_pack_relationships_revision_bump ON catalog.pack_relationships;
CREATE TRIGGER trg_pack_relationships_revision_bump
    AFTER INSERT OR DELETE OR UPDATE
    ON catalog.pack_relationships
    FOR EACH STATEMENT
    EXECUTE FUNCTION catalog.bump_master_catalog_revision();

COMMIT;
