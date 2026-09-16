BEGIN;

-- Pagination composite indexes for server-side LIMIT/OFFSET queries with deterministic tiebreakers.

-- 1. Commerce orders
CREATE INDEX IF NOT EXISTS idx_orders_created_at_id
    ON commerce.orders (created_at DESC, id DESC)
    WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_orders_customer_created_id
    ON commerce.orders (customer_id, created_at DESC, id DESC)
    WHERE deleted_at IS NULL;

-- 2. Commerce shipments
CREATE INDEX IF NOT EXISTS idx_order_shipments_org_created_id
    ON commerce.order_shipments (organization_id, created_at DESC, id DESC);

-- 3. Identity users
CREATE INDEX IF NOT EXISTS idx_users_created_at_id
    ON identity.users (created_at DESC, id DESC)
    WHERE deleted_at IS NULL;

-- 4. Promotional offers
CREATE INDEX IF NOT EXISTS idx_offers_created_at_id
    ON promo.offers (created_at DESC, id DESC)
    WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_offers_org_created_id
    ON promo.offers (organization_id, created_at DESC, id DESC)
    WHERE deleted_at IS NULL;

-- 5. Platform audit log
CREATE INDEX IF NOT EXISTS idx_audit_log_created_at_id
    ON platform.audit_log (created_at DESC, id DESC);

COMMIT;
