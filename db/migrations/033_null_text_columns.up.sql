-- Every column below is selected by a repository into a non-pointer Go string.
-- pgx cannot scan NULL into a string, so a single row with the field empty
-- fails the whole query and takes the endpoint down with it. This already
-- happened twice on live paths: org.organizations.tax_number and
-- catalog.products.sku. Rather than fix them one at a time as each is
-- discovered by an outage, this closes the class: absent and empty mean the
-- same thing for all of these, so '' is the honest representation.
--
-- Excluded deliberately:
--   commerce.order_status_history.from_status - NULL means 'no prior status'
--     on the first transition, which is not the same as an empty one. It is
--     already scanned into a *string.
--   org.branches.code - carries a unique index. NULLs do not collide but
--     empty strings do, so back-filling would fail on the second branch
--     without a code. That one is fixed in Go instead.

UPDATE billing.invoices SET notes = '' WHERE notes IS NULL;
ALTER TABLE billing.invoices ALTER COLUMN notes SET DEFAULT '';
ALTER TABLE billing.invoices ALTER COLUMN notes SET NOT NULL;
UPDATE billing.wallet_transactions SET reference_type = '' WHERE reference_type IS NULL;
ALTER TABLE billing.wallet_transactions ALTER COLUMN reference_type SET DEFAULT '';
ALTER TABLE billing.wallet_transactions ALTER COLUMN reference_type SET NOT NULL;
UPDATE billing.wallet_transactions SET description = '' WHERE description IS NULL;
ALTER TABLE billing.wallet_transactions ALTER COLUMN description SET DEFAULT '';
ALTER TABLE billing.wallet_transactions ALTER COLUMN description SET NOT NULL;
UPDATE billing.payments SET transaction_id = '' WHERE transaction_id IS NULL;
ALTER TABLE billing.payments ALTER COLUMN transaction_id SET DEFAULT '';
ALTER TABLE billing.payments ALTER COLUMN transaction_id SET NOT NULL;
UPDATE billing.payments SET reference_number = '' WHERE reference_number IS NULL;
ALTER TABLE billing.payments ALTER COLUMN reference_number SET DEFAULT '';
ALTER TABLE billing.payments ALTER COLUMN reference_number SET NOT NULL;
UPDATE catalog.products SET sku = '' WHERE sku IS NULL;
ALTER TABLE catalog.products ALTER COLUMN sku SET DEFAULT '';
ALTER TABLE catalog.products ALTER COLUMN sku SET NOT NULL;
UPDATE catalog.products SET barcode = '' WHERE barcode IS NULL;
ALTER TABLE catalog.products ALTER COLUMN barcode SET DEFAULT '';
ALTER TABLE catalog.products ALTER COLUMN barcode SET NOT NULL;
UPDATE catalog.products SET image = '' WHERE image IS NULL;
ALTER TABLE catalog.products ALTER COLUMN image SET DEFAULT '';
ALTER TABLE catalog.products ALTER COLUMN image SET NOT NULL;
UPDATE catalog.products SET image_link = '' WHERE image_link IS NULL;
ALTER TABLE catalog.products ALTER COLUMN image_link SET DEFAULT '';
ALTER TABLE catalog.products ALTER COLUMN image_link SET NOT NULL;
UPDATE catalog.products SET dosage_form = '' WHERE dosage_form IS NULL;
ALTER TABLE catalog.products ALTER COLUMN dosage_form SET DEFAULT '';
ALTER TABLE catalog.products ALTER COLUMN dosage_form SET NOT NULL;
UPDATE catalog.products SET scientific_name = '' WHERE scientific_name IS NULL;
ALTER TABLE catalog.products ALTER COLUMN scientific_name SET DEFAULT '';
ALTER TABLE catalog.products ALTER COLUMN scientific_name SET NOT NULL;
UPDATE catalog.products SET pharmacology = '' WHERE pharmacology IS NULL;
ALTER TABLE catalog.products ALTER COLUMN pharmacology SET DEFAULT '';
ALTER TABLE catalog.products ALTER COLUMN pharmacology SET NOT NULL;
UPDATE catalog.products SET active = '' WHERE active IS NULL;
ALTER TABLE catalog.products ALTER COLUMN active SET DEFAULT '';
ALTER TABLE catalog.products ALTER COLUMN active SET NOT NULL;
UPDATE catalog.products SET concentration = '' WHERE concentration IS NULL;
ALTER TABLE catalog.products ALTER COLUMN concentration SET DEFAULT '';
ALTER TABLE catalog.products ALTER COLUMN concentration SET NOT NULL;
UPDATE catalog.products SET unit = '' WHERE unit IS NULL;
ALTER TABLE catalog.products ALTER COLUMN unit SET DEFAULT '';
ALTER TABLE catalog.products ALTER COLUMN unit SET NOT NULL;
UPDATE catalog.products SET manufacturing_companies = '' WHERE manufacturing_companies IS NULL;
ALTER TABLE catalog.products ALTER COLUMN manufacturing_companies SET DEFAULT '';
ALTER TABLE catalog.products ALTER COLUMN manufacturing_companies SET NOT NULL;
UPDATE catalog.categories SET icon = '' WHERE icon IS NULL;
ALTER TABLE catalog.categories ALTER COLUMN icon SET DEFAULT '';
ALTER TABLE catalog.categories ALTER COLUMN icon SET NOT NULL;
UPDATE catalog.categories SET image = '' WHERE image IS NULL;
ALTER TABLE catalog.categories ALTER COLUMN image SET DEFAULT '';
ALTER TABLE catalog.categories ALTER COLUMN image SET NOT NULL;
UPDATE catalog.brands SET image = '' WHERE image IS NULL;
ALTER TABLE catalog.brands ALTER COLUMN image SET DEFAULT '';
ALTER TABLE catalog.brands ALTER COLUMN image SET NOT NULL;
UPDATE catalog.product_variants SET sku = '' WHERE sku IS NULL;
ALTER TABLE catalog.product_variants ALTER COLUMN sku SET DEFAULT '';
ALTER TABLE catalog.product_variants ALTER COLUMN sku SET NOT NULL;
UPDATE catalog.product_variants SET barcode = '' WHERE barcode IS NULL;
ALTER TABLE catalog.product_variants ALTER COLUMN barcode SET DEFAULT '';
ALTER TABLE catalog.product_variants ALTER COLUMN barcode SET NOT NULL;
UPDATE catalog.product_variants SET unit = '' WHERE unit IS NULL;
ALTER TABLE catalog.product_variants ALTER COLUMN unit SET DEFAULT '';
ALTER TABLE catalog.product_variants ALTER COLUMN unit SET NOT NULL;
UPDATE catalog.product_variants SET image = '' WHERE image IS NULL;
ALTER TABLE catalog.product_variants ALTER COLUMN image SET DEFAULT '';
ALTER TABLE catalog.product_variants ALTER COLUMN image SET NOT NULL;
UPDATE commerce.order_shipments SET tracking_number = '' WHERE tracking_number IS NULL;
ALTER TABLE commerce.order_shipments ALTER COLUMN tracking_number SET DEFAULT '';
ALTER TABLE commerce.order_shipments ALTER COLUMN tracking_number SET NOT NULL;
UPDATE commerce.order_shipments SET carrier_name = '' WHERE carrier_name IS NULL;
ALTER TABLE commerce.order_shipments ALTER COLUMN carrier_name SET DEFAULT '';
ALTER TABLE commerce.order_shipments ALTER COLUMN carrier_name SET NOT NULL;
UPDATE commerce.order_status_history SET notes = '' WHERE notes IS NULL;
ALTER TABLE commerce.order_status_history ALTER COLUMN notes SET DEFAULT '';
ALTER TABLE commerce.order_status_history ALTER COLUMN notes SET NOT NULL;
UPDATE commerce.orders SET notes = '' WHERE notes IS NULL;
ALTER TABLE commerce.orders ALTER COLUMN notes SET DEFAULT '';
ALTER TABLE commerce.orders ALTER COLUMN notes SET NOT NULL;
UPDATE commerce.quote_requests SET buyer_notes = '' WHERE buyer_notes IS NULL;
ALTER TABLE commerce.quote_requests ALTER COLUMN buyer_notes SET DEFAULT '';
ALTER TABLE commerce.quote_requests ALTER COLUMN buyer_notes SET NOT NULL;
UPDATE commerce.quote_requests SET supplier_notes = '' WHERE supplier_notes IS NULL;
ALTER TABLE commerce.quote_requests ALTER COLUMN supplier_notes SET DEFAULT '';
ALTER TABLE commerce.quote_requests ALTER COLUMN supplier_notes SET NOT NULL;
UPDATE identity.user_addresses SET phone = '' WHERE phone IS NULL;
ALTER TABLE identity.user_addresses ALTER COLUMN phone SET DEFAULT '';
ALTER TABLE identity.user_addresses ALTER COLUMN phone SET NOT NULL;
UPDATE identity.user_addresses SET building = '' WHERE building IS NULL;
ALTER TABLE identity.user_addresses ALTER COLUMN building SET DEFAULT '';
ALTER TABLE identity.user_addresses ALTER COLUMN building SET NOT NULL;
UPDATE identity.user_addresses SET floor = '' WHERE floor IS NULL;
ALTER TABLE identity.user_addresses ALTER COLUMN floor SET DEFAULT '';
ALTER TABLE identity.user_addresses ALTER COLUMN floor SET NOT NULL;
UPDATE identity.user_addresses SET apartment = '' WHERE apartment IS NULL;
ALTER TABLE identity.user_addresses ALTER COLUMN apartment SET DEFAULT '';
ALTER TABLE identity.user_addresses ALTER COLUMN apartment SET NOT NULL;
UPDATE identity.users SET phone = '' WHERE phone IS NULL;
ALTER TABLE identity.users ALTER COLUMN phone SET DEFAULT '';
ALTER TABLE identity.users ALTER COLUMN phone SET NOT NULL;
UPDATE identity.user_security SET last_user_agent = '' WHERE last_user_agent IS NULL;
ALTER TABLE identity.user_security ALTER COLUMN last_user_agent SET DEFAULT '';
ALTER TABLE identity.user_security ALTER COLUMN last_user_agent SET NOT NULL;
UPDATE ingest.import_sessions SET error_message = '' WHERE error_message IS NULL;
ALTER TABLE ingest.import_sessions ALTER COLUMN error_message SET DEFAULT '';
ALTER TABLE ingest.import_sessions ALTER COLUMN error_message SET NOT NULL;
UPDATE ingest.import_rows SET normalized_name = '' WHERE normalized_name IS NULL;
ALTER TABLE ingest.import_rows ALTER COLUMN normalized_name SET DEFAULT '';
ALTER TABLE ingest.import_rows ALTER COLUMN normalized_name SET NOT NULL;
UPDATE inventory.stock_movements SET details = '' WHERE details IS NULL;
ALTER TABLE inventory.stock_movements ALTER COLUMN details SET DEFAULT '';
ALTER TABLE inventory.stock_movements ALTER COLUMN details SET NOT NULL;
UPDATE inventory.stock_movements SET reference_type = '' WHERE reference_type IS NULL;
ALTER TABLE inventory.stock_movements ALTER COLUMN reference_type SET DEFAULT '';
ALTER TABLE inventory.stock_movements ALTER COLUMN reference_type SET NOT NULL;
UPDATE inventory.warehouse_transfers SET notes = '' WHERE notes IS NULL;
ALTER TABLE inventory.warehouse_transfers ALTER COLUMN notes SET DEFAULT '';
ALTER TABLE inventory.warehouse_transfers ALTER COLUMN notes SET NOT NULL;
UPDATE inventory.warehouses SET code = '' WHERE code IS NULL;
ALTER TABLE inventory.warehouses ALTER COLUMN code SET DEFAULT '';
ALTER TABLE inventory.warehouses ALTER COLUMN code SET NOT NULL;
UPDATE inventory.warehouses SET address = '' WHERE address IS NULL;
ALTER TABLE inventory.warehouses ALTER COLUMN address SET DEFAULT '';
ALTER TABLE inventory.warehouses ALTER COLUMN address SET NOT NULL;
UPDATE inventory.warehouses SET phone = '' WHERE phone IS NULL;
ALTER TABLE inventory.warehouses ALTER COLUMN phone SET DEFAULT '';
ALTER TABLE inventory.warehouses ALTER COLUMN phone SET NOT NULL;
UPDATE notifications.logs SET error_message = '' WHERE error_message IS NULL;
ALTER TABLE notifications.logs ALTER COLUMN error_message SET DEFAULT '';
ALTER TABLE notifications.logs ALTER COLUMN error_message SET NOT NULL;
UPDATE org.organization_reviews SET review_text = '' WHERE review_text IS NULL;
ALTER TABLE org.organization_reviews ALTER COLUMN review_text SET DEFAULT '';
ALTER TABLE org.organization_reviews ALTER COLUMN review_text SET NOT NULL;
UPDATE platform_admin.system_settings SET description = '' WHERE description IS NULL;
ALTER TABLE platform_admin.system_settings ALTER COLUMN description SET DEFAULT '';
ALTER TABLE platform_admin.system_settings ALTER COLUMN description SET NOT NULL;
UPDATE platform_admin.contact_messages SET phone = '' WHERE phone IS NULL;
ALTER TABLE platform_admin.contact_messages ALTER COLUMN phone SET DEFAULT '';
ALTER TABLE platform_admin.contact_messages ALTER COLUMN phone SET NOT NULL;
UPDATE workflow.weekly_coverages SET address = '' WHERE address IS NULL;
ALTER TABLE workflow.weekly_coverages ALTER COLUMN address SET DEFAULT '';
ALTER TABLE workflow.weekly_coverages ALTER COLUMN address SET NOT NULL;
UPDATE workflow.report_issues SET response_notes = '' WHERE response_notes IS NULL;
ALTER TABLE workflow.report_issues ALTER COLUMN response_notes SET DEFAULT '';
ALTER TABLE workflow.report_issues ALTER COLUMN response_notes SET NOT NULL;
