-- 191_notification_permissions.up.sql
--
-- Security fix: Gate in-app notifications by RBAC permission so that employees
-- with restricted roles (such as couriers / مندوبي التوصيل) cannot view company
-- orders, wallet transactions, purchase requests, sponsorships, ads, or pricing.

BEGIN;

-- 1. Add required_permission column to notifications.logs
ALTER TABLE notifications.logs
    ADD COLUMN IF NOT EXISTS required_permission TEXT NOT NULL DEFAULT '';

-- 2. Index for efficient filtering by user and permission
CREATE INDEX IF NOT EXISTS notification_logs_perm_idx
    ON notifications.logs (user_id, required_permission);

-- 3. Backfill permissions on historical notifications based on content/intent

-- Supply orders for vendors
UPDATE notifications.logs
SET required_permission = 'vendor.order.view'
WHERE (title ILIKE '%طلب توريد%' OR title ILIKE '%new supply order%')
  AND (organization_id IS NULL OR organization_id IN (SELECT id FROM org.organizations WHERE type = 'vendor'));

-- Orders for pharmacies/customers
UPDATE notifications.logs
SET required_permission = 'pharmacy.order.view'
WHERE (title ILIKE '%استلام طلبك%' OR title ILIKE '%طلب التوريد%' OR title ILIKE '%تأكيد طلبك%' OR title ILIKE '%شحن طلبك%' OR title ILIKE '%تسليم طلبك%' OR title ILIKE '%إلغاء طلبك%')
  AND (organization_id IS NULL OR organization_id IN (SELECT id FROM org.organizations WHERE type = 'customer'));

-- Wallet transactions for vendors
UPDATE notifications.logs
SET required_permission = 'vendor.wallet.view'
WHERE (title ILIKE '%محفظة%' OR title ILIKE '%شحن رصيد%' OR title ILIKE '%إيداع%' OR title ILIKE '%سحب%' OR title ILIKE '%wallet%')
  AND organization_id IN (SELECT id FROM org.organizations WHERE type = 'vendor');

-- Wallet transactions for pharmacies
UPDATE notifications.logs
SET required_permission = 'pharmacy.wallet.view'
WHERE (title ILIKE '%محفظة%' OR title ILIKE '%شحن رصيد%' OR title ILIKE '%إيداع%' OR title ILIKE '%سحب%' OR title ILIKE '%wallet%')
  AND organization_id IN (SELECT id FROM org.organizations WHERE type = 'customer');

-- Purchase requests (RFQs)
UPDATE notifications.logs
SET required_permission = 'vendor.purchase_request.view'
WHERE (title ILIKE '%طلب تسعير%' OR title ILIKE '%تسعير جديد%');

-- Sponsorships
UPDATE notifications.logs
SET required_permission = 'vendor.offer_package.view'
WHERE (title ILIKE '%رعاية%' OR title ILIKE '%sponsorship%');

-- Ads
UPDATE notifications.logs
SET required_permission = 'vendor.ad.view'
WHERE (title ILIKE '%إعلان%' OR title ILIKE '%advertisement%');

-- Special offers
UPDATE notifications.logs
SET required_permission = 'vendor.offer.view'
WHERE (title ILIKE '%عرض خاص%' OR title ILIKE '%special offer%');

-- Courier parcel assignments
UPDATE notifications.logs
SET required_permission = 'vendor.delivery.view'
WHERE (title ILIKE '%مسند إليك%' OR title ILIKE '%إسناد%');

-- Document verification
UPDATE notifications.logs
SET required_permission = 'vendor.organization.view'
WHERE title ILIKE '%المستند%'
  AND organization_id IN (SELECT id FROM org.organizations WHERE type = 'vendor');

-- 4. Purge leaked historical notifications from courier (مندوب) accounts:
-- A courier must not hold order, wallet, ad, sponsorship, or finance notification rows.
DELETE FROM notifications.logs l
USING org.members m
WHERE l.user_id = m.user_id
  AND m.role_key = 'org_courier'
  AND (
    l.required_permission NOT IN ('', 'vendor.delivery.view', 'vendor.delivery.update')
    OR l.title ILIKE '%طلب%'
    OR l.title ILIKE '%شحن رصيد%'
    OR l.title ILIKE '%إيداع%'
    OR l.title ILIKE '%سحب%'
    OR l.title ILIKE '%محفظة%'
    OR l.title ILIKE '%رعاية%'
    OR l.title ILIKE '%إعلان%'
    OR l.title ILIKE '%تسعير%'
    OR l.body ILIKE '%صيدليتك%'
  );

COMMIT;
