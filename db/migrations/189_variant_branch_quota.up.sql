-- 189_variant_branch_quota (up)
--
-- حصة الفرع من الصنف — a per-branch purchase quota on a supplier's variant.
--
-- A supplier allocating a scarce line (a controlled drug, an imported batch, a
-- promotional allocation) needs to say "any one pharmacy branch may take at
-- most N packs of this, ever" — not per order, not per company, per *branch*.
-- Two branches of the same منشأة each get their own N, because the allocation
-- is about where the stock physically lands.
--
-- Two objects carry the whole feature:
--
--   * catalog.product_variants.quota_limit — the cap. NULL means no quota at
--     all, which is the state every existing row starts in and the state the
--     supplier returns a variant to when they remove the limit. It is never 0:
--     a zero cap would be indistinguishable from "nobody may buy this", which
--     is what status = 'inactive' already says.
--
--   * commerce.variant_branch_quota_releases — the supplier's reset. One row
--     per (variant, branch) carrying the instant the supplier freed that
--     branch's consumption.
--
-- Consumption itself is NOT stored. It is summed from commerce.order_lines
-- joined to commerce.orders on every read:
--
--     SUM(quantity) WHERE variant = X AND orders.branch_id = Y
--       AND orders.status NOT IN (cancelled, failed, returned, refunded)
--       AND orders.created_at > COALESCE(released_at, -infinity)
--
-- A stored counter would have to be moved by every path that changes an order:
-- checkout, cancellation, a pharmacy editing a pending order's quantities, a
-- return, a refund, a soft delete, an admin edit. Nine call sites, each able to
-- drift, and the drift is invisible until a supplier is asked why a branch that
-- cancelled everything still cannot buy. Summing the orders themselves cannot
-- drift, because the orders *are* the consumption.
--
-- The release row therefore stores no quantity. Releasing is a cut-off in time,
-- not a credit: everything the branch bought before that instant stops
-- counting, and the branch starts again from zero against the same cap.

BEGIN;

ALTER TABLE catalog.product_variants
    ADD COLUMN IF NOT EXISTS quota_limit INTEGER;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conname = 'product_variants_quota_limit_chk'
          AND conrelid = 'catalog.product_variants'::regclass
    ) THEN
        ALTER TABLE catalog.product_variants
            ADD CONSTRAINT product_variants_quota_limit_chk
            CHECK (quota_limit IS NULL OR quota_limit > 0);
    END IF;
END $$;

COMMENT ON COLUMN catalog.product_variants.quota_limit IS
    'حد الحصة لكل فرع مشترٍ — the maximum quantity any single buying branch may purchase of this variant in total; NULL means unlimited';

-- The supplier's quota screen opens on "my variants that have a quota", which
-- is a small subset of a catalogue that can run to nine thousand rows.
CREATE INDEX IF NOT EXISTS product_variants_quota_idx
    ON catalog.product_variants (organization_id, id)
    WHERE quota_limit IS NOT NULL AND deleted_at IS NULL;

CREATE TABLE IF NOT EXISTS commerce.variant_branch_quota_releases (
    id                 BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    public_id          UUID        NOT NULL DEFAULT gen_random_uuid(),
    organization_id    BIGINT      NOT NULL REFERENCES org.organizations(id) ON DELETE CASCADE,
    product_variant_id BIGINT      NOT NULL REFERENCES catalog.product_variants(id) ON DELETE CASCADE,
    branch_id          BIGINT      NOT NULL REFERENCES org.branches(id) ON DELETE CASCADE,
    released_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    released_by        BIGINT      REFERENCES identity.users(id) ON DELETE SET NULL,
    release_count      INTEGER     NOT NULL DEFAULT 1,
    note               TEXT        NOT NULL DEFAULT '',
    created_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at         TIMESTAMPTZ NOT NULL DEFAULT now()
);

COMMENT ON TABLE  commerce.variant_branch_quota_releases IS
    'تحرير حصة فرع من صنف — the supplier resetting one branch''s quota consumption on one variant';
COMMENT ON COLUMN commerce.variant_branch_quota_releases.organization_id IS
    'المورد صاحب الصنف — the supplier that owns the variant, not the buyer';
COMMENT ON COLUMN commerce.variant_branch_quota_releases.branch_id IS
    'فرع المشتري — the buying branch whose consumption was reset';
COMMENT ON COLUMN commerce.variant_branch_quota_releases.released_at IS
    'لحظة التحرير — orders placed at or before this instant no longer count against the quota';
COMMENT ON COLUMN commerce.variant_branch_quota_releases.release_count IS
    'عدد مرات التحرير — how many times this pairing has been reset, kept so the history is not lost when the row is updated in place';

CREATE UNIQUE INDEX IF NOT EXISTS variant_branch_quota_releases_pair_key
    ON commerce.variant_branch_quota_releases (product_variant_id, branch_id);

CREATE UNIQUE INDEX IF NOT EXISTS variant_branch_quota_releases_public_id_key
    ON commerce.variant_branch_quota_releases (public_id);

-- The supplier's screen lists every release they made, newest first.
CREATE INDEX IF NOT EXISTS variant_branch_quota_releases_org_idx
    ON commerce.variant_branch_quota_releases (organization_id, released_at DESC);

ALTER TABLE commerce.variant_branch_quota_releases ENABLE ROW LEVEL SECURITY;
ALTER TABLE commerce.variant_branch_quota_releases FORCE ROW LEVEL SECURITY;

CREATE POLICY variant_branch_quota_releases_tenant_isolation
    ON commerce.variant_branch_quota_releases
    USING (platform.tenant_visible(organization_id))
    WITH CHECK (platform.tenant_visible(organization_id));

-- The quota sum reads one variant's lines and joins each to its order. The
-- existing idx_commerce_order_lines_product_variant_id finds the lines; this
-- one carries the quantity and the order so the join needs no heap fetch for
-- the common case of a variant with a few dozen buyers.
CREATE INDEX IF NOT EXISTS order_lines_variant_quota_idx
    ON commerce.order_lines (product_variant_id, order_id, quantity)
    WHERE product_variant_id IS NOT NULL;

-- And the other direction: "which branches bought this supplier's quota'd
-- variants" walks the supplier's own lines.
CREATE INDEX IF NOT EXISTS order_lines_org_variant_idx
    ON commerce.order_lines (organization_id, product_variant_id)
    WHERE product_variant_id IS NOT NULL;

COMMIT;
