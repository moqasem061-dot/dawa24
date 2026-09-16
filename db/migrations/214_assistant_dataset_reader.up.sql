-- 214_assistant_dataset_reader.up.sql
--
-- The database role Capsule's dataset engine reads as.
--
-- The application connects as a superuser, so nothing below the application
-- stops a query from reading any column of any table. For the assistant that is
-- not good enough: its statements are compiled from a model's request, and a
-- mistake in a declaration must not be able to reach a password hash, a payout
-- account or a delivery PIN, or write anything at all.
--
-- The executor therefore runs every dataset statement inside a read-only
-- transaction after SET LOCAL ROLE dawa24_assistant_ro. This role:
--
--   * cannot log in, and owns nothing;
--   * holds SELECT only — no INSERT, UPDATE, DELETE, TRUNCATE, or DDL;
--   * holds it only on the tables the dataset catalogue reads, and on the
--     sensitive ones only column by column, leaving secrets ungranted;
--   * has BYPASSRLS, because the platform's RLS policies do not match its
--     two-sided data model (see docs/AUDIT_FULL_2026-09-12.md). Tenant
--     isolation for datasets is the compiler's mandatory predicate, which
--     internal/modules/assistant/datasets tests; this role is the second line
--     for columns and writes, not a replacement for that predicate.
--
-- internal/modules/assistant/datasets/explain_test.go plans every dataset
-- statement under this role, so a declaration that reads an ungranted column
-- fails there rather than in production.
BEGIN;

DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'dawa24_assistant_ro') THEN
        CREATE ROLE dawa24_assistant_ro NOLOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOINHERIT BYPASSRLS;
    END IF;
END $$;

-- The migrating user, and the least-privilege application role where it
-- exists, must be able to SET ROLE to it.
DO $$
BEGIN
    EXECUTE format('GRANT dawa24_assistant_ro TO %I', current_user);
    IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'dawa24_app') THEN
        GRANT dawa24_assistant_ro TO dawa24_app;
    END IF;
END $$;

GRANT USAGE ON SCHEMA commerce, billing, catalog, org, identity, inventory, promo,
    workflow, smartorder, platform_admin, platform, ai TO dawa24_assistant_ro;

-- Whole tables with nothing secret in them.
GRANT SELECT ON
    commerce.order_lines, commerce.order_status_history, commerce.purchase_requests,
    commerce.purchase_request_lines, commerce.quote_requests, commerce.carts, commerce.cart_items,
    billing.invoices, billing.payments, billing.wallets, billing.wallet_transactions,
    billing.subscriptions, billing.plans,
    catalog.products, catalog.product_variants, catalog.categories, catalog.brands,
    inventory.stocks, inventory.stock_movements, inventory.warehouses, inventory.warehouse_transfers,
    org.branches, org.roles, org.organization_reviews,
    promo.offers, promo.offer_products,
    workflow.report_issues, workflow.weekly_coverages,
    smartorder.runs,
    ai.usage_events
TO dawa24_assistant_ro;

-- Column by column where a table also holds something no dataset may read.

-- shipping_address is a delivery address; notes are free text between parties.
GRANT SELECT (id, order_number, organization_id, customer_id, status, payment_status, payment_method,
    subtotal, discount_amount, shipping_fee, tax_amount, total_amount, is_negotiation,
    negotiation_status, branch_id, vendor_branch_id, created_at, delivered_at, deleted_at)
    ON commerce.orders TO dawa24_assistant_ro;

-- delivery_code is the PIN a courier must be given at the door.
GRANT SELECT (id, order_id, organization_id, branch_id, shipment_number, status, subtotal,
    shipping_fee, total_amount, tracking_number, carrier_name, shipped_at, delivered_at,
    created_at, updated_at)
    ON commerce.order_shipments TO dawa24_assistant_ro;

-- password_hash, referral codes and moderator links stay out.
GRANT SELECT (id, first_name, last_name, name, email, phone, role, status, email_verified_at,
    created_at, deleted_at)
    ON identity.users TO dawa24_assistant_ro;

-- ai_virtual_key and ai_user_id are gateway credentials; tax and register
-- numbers are not needed by any dataset.
GRANT SELECT (id, organization_number, name, trade_name, legal_name, type, status, email, phone,
    branch_count, is_sponsored, created_at, approved_at, deleted_at)
    ON org.organizations TO dawa24_assistant_ro;

-- Salaries are not part of the team screen's view permission.
GRANT SELECT (id, organization_id, user_id, branch_id, role_key, org_role_id, status, is_active,
    job_title, joined_at, created_at)
    ON org.members TO dawa24_assistant_ro;

-- destination_details is the payout account; receipts are private files.
GRANT SELECT (id, organization_id, amount, status, payout_method_type, rejection_reason,
    created_at, reviewed_at)
    ON billing.wallet_withdrawals TO dawa24_assistant_ro;

-- sender_account and the transfer attachment identify a bank account.
GRANT SELECT (id, organization_id, amount, status, payment_method, created_at, reviewed_at)
    ON billing.wallet_deposits TO dawa24_assistant_ro;

-- Stack traces, request payloads, IPs and user agents can carry credentials.
GRANT SELECT (id, error_level, error_message, exception_class, url_path, http_method, status,
    user_email, organization_name, created_at)
    ON platform_admin.error_logs TO dawa24_assistant_ro;

-- Before/after images can hold any column of any table; IPs are personal data.
GRANT SELECT (id, organization_id, actor_user_id, action, entity_type, entity_id, created_at)
    ON platform.audit_log TO dawa24_assistant_ro;

COMMIT;
