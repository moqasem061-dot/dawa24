-- 029_rls_coverage
--
-- ADR 0003 requires every tenant-owned table to have row-level security. A
-- check against the live database found 38 of 68 did. Ten carried
-- organization_id with no policy at all — every row of every other supplier's
-- wallets, payments, subscriptions, carts, ads and offer analytics was readable
-- by any query that forgot a WHERE clause.
--
-- Child tables are worse, not better: they hold the detail. cart_items without
-- a policy exposes what every competitor's customers are buying even while
-- commerce.carts is protected.
--
-- Deliberately left unprotected, with reasons:
--
--   catalog.brands, catalog.categories   platform taxonomy, shared by design
--   billing.plans, billing.plan_features platform pricing, public
--   promo.ad_plans, hr.job_categories    platform reference data
--   promo.offer_packages                 sponsorship pricing catalogue; holds
--                                        no tenant column and no tenant data
--   billing.payment_integrations         payment provider configuration,
--                                        platform-wide
--   org.organizations                    the marketplace directory; a buyer
--                                        must see suppliers before belonging
--                                        to one. Its *contents* are protected.
--
-- Every policy uses platform.tenant_visible(), which grants access when the
-- row's organization matches the transaction's app.current_org_id, or when the
-- caller opened the transaction with database.AsSystem().

BEGIN;

-- ---------------------------------------------------------------------------
-- Directly scoped: the table carries organization_id
-- ---------------------------------------------------------------------------
DO $$
DECLARE
    t TEXT;
    direct TEXT[] := ARRAY[
        'billing.payments',
        'billing.subscriptions',
        'billing.wallets',
        'catalog.saving_products',
        'commerce.carts',
        'promo.ads',
        'promo.offer_clicks',
        'promo.offer_views',
        'workflow.purchase_priority_engines',
        'workflow.report_issues'
    ];
BEGIN
    FOREACH t IN ARRAY direct LOOP
        EXECUTE format('ALTER TABLE %s ENABLE ROW LEVEL SECURITY', t);
        -- FORCE is the half that matters here: the application connects as the
        -- owner of these tables, and an owner bypasses every policy without it.
        EXECUTE format('ALTER TABLE %s FORCE ROW LEVEL SECURITY', t);
        EXECUTE format(
            'CREATE POLICY tenant_isolation ON %s
               USING (platform.tenant_visible(organization_id))
               WITH CHECK (platform.tenant_visible(organization_id))', t);
    END LOOP;
END $$;

-- ---------------------------------------------------------------------------
-- Indirectly scoped: reached through a parent that carries organization_id
-- ---------------------------------------------------------------------------
-- An EXISTS against the parent costs an index lookup per row. That is the price
-- of not duplicating organization_id onto every child and then having to keep
-- the copy honest.

ALTER TABLE commerce.cart_items ENABLE ROW LEVEL SECURITY;
ALTER TABLE commerce.cart_items FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON commerce.cart_items
    USING (EXISTS (SELECT 1 FROM commerce.carts c
                   WHERE c.id = cart_id AND platform.tenant_visible(c.organization_id)))
    WITH CHECK (EXISTS (SELECT 1 FROM commerce.carts c
                        WHERE c.id = cart_id AND platform.tenant_visible(c.organization_id)));

ALTER TABLE commerce.order_status_history ENABLE ROW LEVEL SECURITY;
ALTER TABLE commerce.order_status_history FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON commerce.order_status_history
    USING (EXISTS (SELECT 1 FROM commerce.orders o
                   WHERE o.id = order_id AND platform.tenant_visible(o.organization_id)))
    WITH CHECK (EXISTS (SELECT 1 FROM commerce.orders o
                        WHERE o.id = order_id AND platform.tenant_visible(o.organization_id)));

ALTER TABLE billing.wallet_transactions ENABLE ROW LEVEL SECURITY;
ALTER TABLE billing.wallet_transactions FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON billing.wallet_transactions
    USING (EXISTS (SELECT 1 FROM billing.wallets w
                   WHERE w.id = wallet_id AND platform.tenant_visible(w.organization_id)))
    WITH CHECK (EXISTS (SELECT 1 FROM billing.wallets w
                        WHERE w.id = wallet_id AND platform.tenant_visible(w.organization_id)));

ALTER TABLE billing.invoice_lines ENABLE ROW LEVEL SECURITY;
ALTER TABLE billing.invoice_lines FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON billing.invoice_lines
    USING (EXISTS (SELECT 1 FROM billing.invoices i
                   WHERE i.id = invoice_id AND platform.tenant_visible(i.organization_id)))
    WITH CHECK (EXISTS (SELECT 1 FROM billing.invoices i
                        WHERE i.id = invoice_id AND platform.tenant_visible(i.organization_id)));

ALTER TABLE promo.offer_products ENABLE ROW LEVEL SECURITY;
ALTER TABLE promo.offer_products FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON promo.offer_products
    USING (EXISTS (SELECT 1 FROM promo.offers o
                   WHERE o.id = offer_id AND platform.tenant_visible(o.organization_id)))
    WITH CHECK (EXISTS (SELECT 1 FROM promo.offers o
                        WHERE o.id = offer_id AND platform.tenant_visible(o.organization_id)));

-- ---------------------------------------------------------------------------
-- User-scoped: these belong to a person, not an organization
-- ---------------------------------------------------------------------------
-- Row-level security cannot express "the calling user" — there is no per-user
-- GUC, and adding one would duplicate the session state authctx already holds.
-- These stay enforced by a user_id predicate in every query, which is why
-- notifications and wishlists carry that predicate explicitly. Recorded here so
-- the gap is deliberate and visible rather than an oversight:
--
--   billing.user_payment_methods, catalog.product_alerts,
--   commerce.wishlists, promo.ad_clicks

COMMIT;
