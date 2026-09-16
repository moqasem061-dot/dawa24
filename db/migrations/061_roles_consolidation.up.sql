-- 061_roles_consolidation (up)
--
-- Seven role surfaces collapse to two (Rebuild V2 §2.1):
--   * identity.roles + identity.role_permissions  — the permission vocabulary
--     and the platform-role map the resolver reads.
--   * org.roles + org.role_permissions            — per-organization roles.
--
-- Dropped here: org.custom_roles (merged into org.roles) and
-- identity.user_roles (platform role assignment lives in identity.users.role,
-- which 060 restricted; membership carries the org role).
--
-- org.members.role_key stays one release, marked deprecated, because the
-- permission resolver still reads it; org_role_id becomes the joining column.

BEGIN;

-- ---------------------------------------------------------------------------
-- 1. The two role templates Part 2.1 lists that the vocabulary lacks
-- ---------------------------------------------------------------------------
INSERT INTO identity.roles (key, name, scope, is_system, description) VALUES
    ('org_pharmacist', '{"ar":"صيدلي مسؤول","en":"Responsible Pharmacist"}', 'organization', true, 'Browse, order and receive supplies'),
    ('org_sales_rep',  '{"ar":"مندوب مبيعات","en":"Sales Representative"}', 'organization', true, 'Offers and customer orders (vendors)')
ON CONFLICT (key) DO UPDATE SET name = EXCLUDED.name, description = EXCLUDED.description;

INSERT INTO identity.role_permissions (role_key, permission_key) VALUES
    -- Customer: browse, order, receive
    ('org_pharmacist','catalog.product.view'),
    ('org_pharmacist','commerce.order.view'),
    ('org_pharmacist','commerce.order.fulfil'),
    -- Vendor: offers, customer orders
    ('org_sales_rep','catalog.product.view'),
    ('org_sales_rep','commerce.order.view'),
    ('org_sales_rep','promo.offer.manage')
ON CONFLICT DO NOTHING;

-- ---------------------------------------------------------------------------
-- 2. Merge org.custom_roles into org.roles (is_system = false)
-- ---------------------------------------------------------------------------
INSERT INTO org.roles (organization_id, key, name, description, is_system)
SELECT cr.organization_id, 'custom_' || cr.id, cr.name, cr.description, false
FROM org.custom_roles cr
ON CONFLICT (organization_id, key) DO NOTHING;

-- Their permissions array becomes org.role_permissions rows.
INSERT INTO org.role_permissions (role_id, permission_key)
SELECT r.id, perm
FROM org.custom_roles cr
CROSS JOIN LATERAL unnest(cr.permissions) AS perm
JOIN org.roles r ON r.organization_id = cr.organization_id AND r.key = 'custom_' || cr.id
ON CONFLICT DO NOTHING;

-- ---------------------------------------------------------------------------
-- 3. Backfill org.members.org_role_id from role_key
-- ---------------------------------------------------------------------------
UPDATE org.members m
SET org_role_id = r.id
FROM org.roles r
WHERE r.organization_id = m.organization_id
  AND r.key = m.role_key
  AND m.org_role_id IS NULL
  AND m.role_key <> '';

COMMENT ON COLUMN org.members.role_key IS
  'Deprecated — kept one release for the permission resolver; use org_role_id. Org role now lives in org.roles.';

-- ---------------------------------------------------------------------------
-- 4. Drop the duplicated sources
-- ---------------------------------------------------------------------------
DROP TABLE IF EXISTS org.custom_roles;
DROP TABLE IF EXISTS identity.user_roles;

COMMIT;