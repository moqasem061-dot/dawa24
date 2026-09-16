BEGIN;

DROP INDEX IF EXISTS platform.idx_audit_log_created_at_id;
DROP INDEX IF EXISTS promo.idx_offers_org_created_id;
DROP INDEX IF EXISTS promo.idx_offers_created_at_id;
DROP INDEX IF EXISTS identity.idx_users_created_at_id;
DROP INDEX IF EXISTS commerce.idx_order_shipments_org_created_id;
DROP INDEX IF EXISTS commerce.idx_orders_customer_created_id;
DROP INDEX IF EXISTS commerce.idx_orders_created_at_id;

COMMIT;
