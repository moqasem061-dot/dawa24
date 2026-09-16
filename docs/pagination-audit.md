# Server-Side Pagination Audit

Generated on: 2026-09-02 05:51:49 EEST

## A1. Live Database Table Row Counts

| Schema | Table | Estimated Rows |
|---|---|---:|
| `compare` | `file_rows` | 48627 |
| `ingest` | `catalog_import_rows` | 22582 |
| `catalog` | `product_index` | 19996 |
| `catalog` | `products` | 19996 |
| `catalog` | `brands` | 4287 |
| `org` | `role_permissions` | 1736 |
| `catalog` | `product_variants` | 1357 |
| `catalog` | `import_staging_rows` | 1133 |
| `catalog` | `match_decisions` | 762 |
| `catalog` | `product_aliases` | 658 |
| `platform` | `translations` | 476 |
| `identity` | `role_permissions` | 317 |
| `identity` | `permissions` | 267 |
| `compare` | `files` | 156 |
| `org` | `roles` | 98 |
| `org` | `branch_institutional_works` | 48 |
| `ai` | `usage_events` | 38 |
| `platform` | `audit_log` | 32 |
| `identity` | `rbac_version` | 21 |
| `identity` | `user_security` | 18 |
| `catalog` | `categories` | 16 |
| `org` | `institutional_works` | 15 |
| `commerce` | `order_status_history` | 15 |
| `org` | `review_criteria` | 15 |
| `billing` | `wallets` | 14 |
| `identity` | `roles` | 14 |
| `org` | `branches` | 13 |
| `identity` | `users` | 12 |
| `org` | `institutional_work_connections` | 12 |
| `billing` | `plan_features` | 10 |
| `compare` | `plan_features` | 9 |
| `org` | `organizations` | 8 |
| `org` | `members` | 7 |
| `billing` | `wallet_transactions` | 6 |
| `commerce` | `order_shipments` | 4 |
| `commerce` | `orders` | 4 |
| `inventory` | `warehouses` | 4 |
| `billing` | `platform_payment_methods` | 3 |
| `billing` | `wallet_deposits` | 3 |
| `identity` | `user_address_histories` | 3 |
| `billing` | `plans` | 3 |
| `billing` | `invoices` | 3 |
| `org` | `organization_policies` | 3 |
| `compare` | `plans` | 3 |
| `hr` | `job_offers` | 2 |
| `billing` | `user_payment_methods` | 2 |
| `org` | `user_organizations` | 2 |
| `compare` | `subscriptions` | 1 |
| `promo` | `ads` | 1 |
| `hr` | `job_applications` | 1 |
| `commerce` | `carts` | 1 |
| `billing` | `subscriptions` | 1 |
| `org` | `organization_followers` | 1 |
| `org` | `organization_social_media` | 1 |
| `promo` | `highlight_sections` | 1 |
| `promo` | `offers` | 1 |
| `workflow` | `purchase_priority_engines` | 1 |
| `compare` | `subscription_users` | 1 |
| `promo` | `offer_packages` | 1 |
| `promo` | `ad_clicks` | 1 |
| `workflow` | `requests` | 0 |
| `compare` | `plan_requests` | 0 |
| `org` | `delivery_bands` | 0 |
| `billing` | `payments` | 0 |
| `hr` | `work_times` | 0 |
| `billing` | `payment_integrations` | 0 |
| `hr` | `job_seeker_profiles` | 0 |
| `commerce` | `purchase_requests` | 0 |
| `billing` | `subscription_histories` | 0 |
| `hr` | `job_categories` | 0 |
| `identity` | `account_deletion_requests` | 0 |
| `inventory` | `father_user_temparte_warehouses` | 0 |
| `identity` | `session_plans` | 0 |
| `billing` | `subscription_users` | 0 |
| `identity` | `user_addresses` | 0 |
| `identity` | `user_mfa` | 0 |
| `identity` | `user_sessions` | 0 |
| `promo` | `offer_sponsorships` | 0 |
| `promo` | `ad_impressions` | 0 |
| `promo` | `sponsorship_requests` | 0 |
| `promo` | `sponsorship_purchases` | 0 |
| `workflow` | `weekly_coverages` | 0 |
| `promo` | `ad_plans` | 0 |
| `promo` | `offer_clicks` | 0 |
| `workflow` | `report_issues` | 0 |
| `org` | `employee_institutional_works` | 0 |
| `promo` | `offer_location_covers` | 0 |
| `compare` | `user_sessions` | 0 |
| `org` | `organization_reviews` | -1 |
| `promo` | `highlight_section_items` | -1 |
| `org` | `review_ratings` | -1 |
| `inventory` | `stocks` | -1 |
| `inventory` | `temp_warehouses` | -1 |
| `commerce` | `cart_items` | -1 |
| `catalog` | `brand_categories` | -1 |
| `catalog` | `product_alerts` | -1 |
| `identity` | `user_favorites` | -1 |
| `promo` | `offer_products` | -1 |
| `inventory` | `warehouse_transfers` | -1 |
| `ingest` | `import_rows` | -1 |
| `ingest` | `import_progress` | -1 |
| `catalog` | `saving_products` | -1 |
| `billing` | `invoice_lines` | -1 |
| `ingest` | `import_sessions` | -1 |
| `commerce` | `wishlists` | -1 |
| `commerce` | `order_lines` | -1 |
| `catalog` | `import_sessions` | -1 |
| `commerce` | `quote_requests` | -1 |
| `commerce` | `purchase_request_lines` | -1 |
| `catalog` | `customer_product_mappings` | -1 |
| `ingest` | `catalog_imports` | -1 |
| `inventory` | `stock_movements` | -1 |

## A2. Existing Database Indexes

| Schema | Table | Index Name | Definition |
|---|---|---|---|
| `ai` | `usage_events` | `ai_usage_org_feature_time_idx` | `CREATE INDEX ai_usage_org_feature_time_idx ON ai.usage_events USING btree (organization_id, feature, created_at DESC)` |
| `ai` | `usage_events` | `ai_usage_org_time_idx` | `CREATE INDEX ai_usage_org_time_idx ON ai.usage_events USING btree (organization_id, created_at DESC)` |
| `ai` | `usage_events` | `usage_events_pkey` | `CREATE UNIQUE INDEX usage_events_pkey ON ai.usage_events USING btree (id)` |
| `billing` | `invoice_lines` | `idx_billing_invoice_lines_product_id` | `CREATE INDEX idx_billing_invoice_lines_product_id ON billing.invoice_lines USING btree (product_id)` |
| `billing` | `invoice_lines` | `invoice_lines_invoice_idx` | `CREATE INDEX invoice_lines_invoice_idx ON billing.invoice_lines USING btree (invoice_id)` |
| `billing` | `invoice_lines` | `invoice_lines_pkey` | `CREATE UNIQUE INDEX invoice_lines_pkey ON billing.invoice_lines USING btree (id)` |
| `billing` | `invoices` | `idx_billing_invoices_order_id` | `CREATE INDEX idx_billing_invoices_order_id ON billing.invoices USING btree (order_id)` |
| `billing` | `invoices` | `idx_billing_invoices_org_created` | `CREATE INDEX idx_billing_invoices_org_created ON billing.invoices USING btree (organization_id, created_at DESC)` |
| `billing` | `invoices` | `invoices_customer_idx` | `CREATE INDEX invoices_customer_idx ON billing.invoices USING btree (customer_org_id)` |
| `billing` | `invoices` | `invoices_invoice_number_key` | `CREATE UNIQUE INDEX invoices_invoice_number_key ON billing.invoices USING btree (invoice_number)` |
| `billing` | `invoices` | `invoices_org_idx` | `CREATE INDEX invoices_org_idx ON billing.invoices USING btree (organization_id, issue_date DESC)` |
| `billing` | `invoices` | `invoices_pkey` | `CREATE UNIQUE INDEX invoices_pkey ON billing.invoices USING btree (id)` |
| `billing` | `payment_integrations` | `payment_integrations_pkey` | `CREATE UNIQUE INDEX payment_integrations_pkey ON billing.payment_integrations USING btree (id)` |
| `billing` | `payment_integrations` | `payment_integrations_slug_key` | `CREATE UNIQUE INDEX payment_integrations_slug_key ON billing.payment_integrations USING btree (slug)` |
| `billing` | `payments` | `idx_billing_payments_org_created` | `CREATE INDEX idx_billing_payments_org_created ON billing.payments USING btree (organization_id, created_at DESC)` |
| `billing` | `payments` | `idx_billing_payments_organization_id` | `CREATE INDEX idx_billing_payments_organization_id ON billing.payments USING btree (organization_id)` |
| `billing` | `payments` | `idx_billing_payments_payment_integration_id` | `CREATE INDEX idx_billing_payments_payment_integration_id ON billing.payments USING btree (payment_integration_id)` |
| `billing` | `payments` | `payments_order_idx` | `CREATE INDEX payments_order_idx ON billing.payments USING btree (order_id)` |
| `billing` | `payments` | `payments_pkey` | `CREATE UNIQUE INDEX payments_pkey ON billing.payments USING btree (id)` |
| `billing` | `payments` | `payments_public_id_key` | `CREATE UNIQUE INDEX payments_public_id_key ON billing.payments USING btree (public_id)` |
| `billing` | `payments` | `payments_user_idx` | `CREATE INDEX payments_user_idx ON billing.payments USING btree (user_id)` |
| `billing` | `plan_features` | `idx_billing_plan_features_plan_id` | `CREATE INDEX idx_billing_plan_features_plan_id ON billing.plan_features USING btree (plan_id)` |
| `billing` | `plan_features` | `plan_features_pkey` | `CREATE UNIQUE INDEX plan_features_pkey ON billing.plan_features USING btree (id)` |
| `billing` | `plan_features` | `plan_features_unique` | `CREATE UNIQUE INDEX plan_features_unique ON billing.plan_features USING btree (plan_id, feature_key)` |
| `billing` | `plans` | `plans_pkey` | `CREATE UNIQUE INDEX plans_pkey ON billing.plans USING btree (id)` |
| `billing` | `plans` | `plans_slug_key` | `CREATE UNIQUE INDEX plans_slug_key ON billing.plans USING btree (slug)` |
| `billing` | `platform_payment_methods` | `platform_payment_methods_pkey` | `CREATE UNIQUE INDEX platform_payment_methods_pkey ON billing.platform_payment_methods USING btree (id)` |
| `billing` | `subscription_histories` | `idx_billing_subscription_histories_org_created` | `CREATE INDEX idx_billing_subscription_histories_org_created ON billing.subscription_histories USING btree (organization_id, created_at DESC)` |
| `billing` | `subscription_histories` | `idx_billing_subscription_histories_user_id` | `CREATE INDEX idx_billing_subscription_histories_user_id ON billing.subscription_histories USING btree (user_id)` |
| `billing` | `subscription_histories` | `idx_subscription_histories_org` | `CREATE INDEX idx_subscription_histories_org ON billing.subscription_histories USING btree (organization_id)` |
| `billing` | `subscription_histories` | `idx_subscription_histories_plan` | `CREATE INDEX idx_subscription_histories_plan ON billing.subscription_histories USING btree (plan_id)` |
| `billing` | `subscription_histories` | `subscription_histories_pkey` | `CREATE UNIQUE INDEX subscription_histories_pkey ON billing.subscription_histories USING btree (id)` |
| `billing` | `subscription_users` | `idx_billing_subscription_users_org_created` | `CREATE INDEX idx_billing_subscription_users_org_created ON billing.subscription_users USING btree (organization_id, created_at DESC)` |
| `billing` | `subscription_users` | `idx_subscription_users_org` | `CREATE INDEX idx_subscription_users_org ON billing.subscription_users USING btree (organization_id)` |
| `billing` | `subscription_users` | `idx_subscription_users_user` | `CREATE INDEX idx_subscription_users_user ON billing.subscription_users USING btree (user_id)` |
| `billing` | `subscription_users` | `subscription_users_pkey` | `CREATE UNIQUE INDEX subscription_users_pkey ON billing.subscription_users USING btree (id)` |
| `billing` | `subscriptions` | `idx_billing_subscriptions_org_created` | `CREATE INDEX idx_billing_subscriptions_org_created ON billing.subscriptions USING btree (organization_id, created_at DESC)` |
| `billing` | `subscriptions` | `idx_billing_subscriptions_plan_id` | `CREATE INDEX idx_billing_subscriptions_plan_id ON billing.subscriptions USING btree (plan_id)` |
| `billing` | `subscriptions` | `idx_subscriptions_auto_renew` | `CREATE INDEX idx_subscriptions_auto_renew ON billing.subscriptions USING btree (auto_renew, status, expires_at) WHERE ((auto_renew = true) AND (status = 'active'::text))` |
| `billing` | `subscriptions` | `subscriptions_org_idx` | `CREATE INDEX subscriptions_org_idx ON billing.subscriptions USING btree (organization_id) WHERE (organization_id IS NOT NULL)` |
| `billing` | `subscriptions` | `subscriptions_pkey` | `CREATE UNIQUE INDEX subscriptions_pkey ON billing.subscriptions USING btree (id)` |
| `billing` | `subscriptions` | `subscriptions_public_id_key` | `CREATE UNIQUE INDEX subscriptions_public_id_key ON billing.subscriptions USING btree (public_id)` |
| `billing` | `subscriptions` | `subscriptions_user_idx` | `CREATE INDEX subscriptions_user_idx ON billing.subscriptions USING btree (user_id)` |
| `billing` | `user_payment_methods` | `user_payment_methods_pkey` | `CREATE UNIQUE INDEX user_payment_methods_pkey ON billing.user_payment_methods USING btree (id)` |
| `billing` | `user_payment_methods` | `user_payment_methods_user_idx` | `CREATE INDEX user_payment_methods_user_idx ON billing.user_payment_methods USING btree (user_id)` |
| `billing` | `wallet_deposits` | `idx_wallet_deposits_created_at` | `CREATE INDEX idx_wallet_deposits_created_at ON billing.wallet_deposits USING btree (created_at DESC)` |
| `billing` | `wallet_deposits` | `idx_wallet_deposits_org` | `CREATE INDEX idx_wallet_deposits_org ON billing.wallet_deposits USING btree (organization_id)` |
| `billing` | `wallet_deposits` | `idx_wallet_deposits_status` | `CREATE INDEX idx_wallet_deposits_status ON billing.wallet_deposits USING btree (status)` |
| `billing` | `wallet_deposits` | `idx_wallet_deposits_user_id` | `CREATE INDEX idx_wallet_deposits_user_id ON billing.wallet_deposits USING btree (user_id)` |
| `billing` | `wallet_deposits` | `idx_wallet_deposits_user_status` | `CREATE INDEX idx_wallet_deposits_user_status ON billing.wallet_deposits USING btree (user_id, status)` |
| `billing` | `wallet_deposits` | `idx_wallet_deposits_wallet` | `CREATE INDEX idx_wallet_deposits_wallet ON billing.wallet_deposits USING btree (wallet_id, status)` |
| `billing` | `wallet_deposits` | `idx_wallet_deposits_wallet_id` | `CREATE INDEX idx_wallet_deposits_wallet_id ON billing.wallet_deposits USING btree (wallet_id)` |
| `billing` | `wallet_deposits` | `wallet_deposits_pkey` | `CREATE UNIQUE INDEX wallet_deposits_pkey ON billing.wallet_deposits USING btree (id)` |
| `billing` | `wallet_deposits` | `wallet_deposits_reviewed_by_idx` | `CREATE INDEX wallet_deposits_reviewed_by_idx ON billing.wallet_deposits USING btree (reviewed_by)` |
| `billing` | `wallet_deposits` | `wallet_deposits_transaction_id_idx` | `CREATE INDEX wallet_deposits_transaction_id_idx ON billing.wallet_deposits USING btree (transaction_id)` |
| `billing` | `wallet_transactions` | `wallet_transactions_pkey` | `CREATE UNIQUE INDEX wallet_transactions_pkey ON billing.wallet_transactions USING btree (id)` |
| `billing` | `wallet_transactions` | `wallet_tx_wallet_idx` | `CREATE INDEX wallet_tx_wallet_idx ON billing.wallet_transactions USING btree (wallet_id, created_at DESC)` |
| `billing` | `wallets` | `idx_billing_wallets_org_created` | `CREATE INDEX idx_billing_wallets_org_created ON billing.wallets USING btree (organization_id, created_at DESC)` |
| `billing` | `wallets` | `wallets_org_idx` | `CREATE INDEX wallets_org_idx ON billing.wallets USING btree (organization_id) WHERE (organization_id IS NOT NULL)` |
| `billing` | `wallets` | `wallets_pkey` | `CREATE UNIQUE INDEX wallets_pkey ON billing.wallets USING btree (id)` |
| `billing` | `wallets` | `wallets_public_id_key` | `CREATE UNIQUE INDEX wallets_public_id_key ON billing.wallets USING btree (public_id)` |
| `billing` | `wallets` | `wallets_user_currency_unique` | `CREATE UNIQUE INDEX wallets_user_currency_unique ON billing.wallets USING btree (user_id, currency)` |
| `billing` | `wallets` | `wallets_user_idx` | `CREATE INDEX wallets_user_idx ON billing.wallets USING btree (user_id)` |
| `catalog` | `brand_categories` | `brand_categories_category_idx` | `CREATE INDEX brand_categories_category_idx ON catalog.brand_categories USING btree (category_id)` |
| `catalog` | `brand_categories` | `brand_categories_pkey` | `CREATE UNIQUE INDEX brand_categories_pkey ON catalog.brand_categories USING btree (brand_id, category_id)` |
| `catalog` | `brands` | `brands_name_trgm_idx` | `CREATE INDEX brands_name_trgm_idx ON catalog.brands USING gin (platform.normalize_arabic((name ->> 'ar'::text)) gin_trgm_ops)` |
| `catalog` | `brands` | `brands_pkey` | `CREATE UNIQUE INDEX brands_pkey ON catalog.brands USING btree (id)` |
| `catalog` | `brands` | `brands_public_id_key` | `CREATE UNIQUE INDEX brands_public_id_key ON catalog.brands USING btree (public_id)` |
| `catalog` | `brands` | `brands_status_idx` | `CREATE INDEX brands_status_idx ON catalog.brands USING btree (status) WHERE (deleted_at IS NULL)` |
| `catalog` | `brands` | `idx_brands_name_en_trgm` | `CREATE INDEX idx_brands_name_en_trgm ON catalog.brands USING gin (((name ->> 'en'::text)) gin_trgm_ops)` |
| `catalog` | `categories` | `categories_name_trgm_idx` | `CREATE INDEX categories_name_trgm_idx ON catalog.categories USING gin (platform.normalize_arabic((name ->> 'ar'::text)) gin_trgm_ops)` |
| `catalog` | `categories` | `categories_parent_idx` | `CREATE INDEX categories_parent_idx ON catalog.categories USING btree (parent_id) WHERE (deleted_at IS NULL)` |
| `catalog` | `categories` | `categories_pkey` | `CREATE UNIQUE INDEX categories_pkey ON catalog.categories USING btree (id)` |
| `catalog` | `categories` | `categories_public_id_key` | `CREATE UNIQUE INDEX categories_public_id_key ON catalog.categories USING btree (public_id)` |
| `catalog` | `categories` | `categories_status_idx` | `CREATE INDEX categories_status_idx ON catalog.categories USING btree (status) WHERE (deleted_at IS NULL)` |
| `catalog` | `categories` | `idx_categories_name_en_trgm` | `CREATE INDEX idx_categories_name_en_trgm ON catalog.categories USING gin (((name ->> 'en'::text)) gin_trgm_ops)` |
| `catalog` | `customer_product_mappings` | `customer_product_mapping_unique` | `CREATE UNIQUE INDEX customer_product_mapping_unique ON catalog.customer_product_mappings USING btree (organization_id, customer_org_id, product_id, product_variant_id)` |
| `catalog` | `customer_product_mappings` | `customer_product_mappings_lookup_idx` | `CREATE INDEX customer_product_mappings_lookup_idx ON catalog.customer_product_mappings USING btree (customer_org_id, product_id)` |
| `catalog` | `customer_product_mappings` | `customer_product_mappings_pkey` | `CREATE UNIQUE INDEX customer_product_mappings_pkey ON catalog.customer_product_mappings USING btree (id)` |
| `catalog` | `customer_product_mappings` | `customer_product_mappings_raw_name_idx` | `CREATE INDEX customer_product_mappings_raw_name_idx ON catalog.customer_product_mappings USING btree (organization_id, raw_name) WHERE (raw_name <> ''::text)` |
| `catalog` | `customer_product_mappings` | `idx_catalog_customer_product_mappings_branch_id` | `CREATE INDEX idx_catalog_customer_product_mappings_branch_id ON catalog.customer_product_mappings USING btree (branch_id)` |
| `catalog` | `customer_product_mappings` | `idx_catalog_customer_product_mappings_org_created` | `CREATE INDEX idx_catalog_customer_product_mappings_org_created ON catalog.customer_product_mappings USING btree (organization_id, created_at DESC)` |
| `catalog` | `customer_product_mappings` | `idx_catalog_customer_product_mappings_product_id` | `CREATE INDEX idx_catalog_customer_product_mappings_product_id ON catalog.customer_product_mappings USING btree (product_id)` |
| `catalog` | `customer_product_mappings` | `idx_catalog_customer_product_mappings_product_variant_id` | `CREATE INDEX idx_catalog_customer_product_mappings_product_variant_id ON catalog.customer_product_mappings USING btree (product_variant_id)` |
| `catalog` | `customer_product_mappings` | `idx_customer_mappings_norm_name` | `CREATE INDEX idx_customer_mappings_norm_name ON catalog.customer_product_mappings USING btree (platform.normalize_arabic(lower(TRIM(BOTH FROM raw_name)))) WHERE ((product_id IS NOT NULL) AND is_active)` |
| `catalog` | `import_sessions` | `catalog_import_sessions_expiry` | `CREATE INDEX catalog_import_sessions_expiry ON catalog.import_sessions USING btree (expires_at) WHERE (status = ANY (ARRAY['draft'::text, 'ready'::text]))` |
| `catalog` | `import_sessions` | `catalog_import_sessions_org_created` | `CREATE INDEX catalog_import_sessions_org_created ON catalog.import_sessions USING btree (organization_id, created_at DESC)` |
| `catalog` | `import_sessions` | `catalog_import_sessions_public_id_key` | `CREATE UNIQUE INDEX catalog_import_sessions_public_id_key ON catalog.import_sessions USING btree (public_id)` |
| `catalog` | `import_sessions` | `import_sessions_created_by_idx` | `CREATE INDEX import_sessions_created_by_idx ON catalog.import_sessions USING btree (created_by)` |
| `catalog` | `import_sessions` | `import_sessions_pkey` | `CREATE UNIQUE INDEX import_sessions_pkey ON catalog.import_sessions USING btree (id)` |
| `catalog` | `import_staging_rows` | `catalog_import_rows_action` | `CREATE INDEX catalog_import_rows_action ON catalog.import_staging_rows USING btree (session_id, action) WHERE included` |
| `catalog` | `import_staging_rows` | `catalog_import_rows_flags` | `CREATE INDEX catalog_import_rows_flags ON catalog.import_staging_rows USING btree (session_id, has_error, has_warning, has_ai)` |
| `catalog` | `import_staging_rows` | `catalog_import_rows_search` | `CREATE INDEX catalog_import_rows_search ON catalog.import_staging_rows USING gin (search_name gin_trgm_ops)` |
| `catalog` | `import_staging_rows` | `catalog_import_rows_session` | `CREATE INDEX catalog_import_rows_session ON catalog.import_staging_rows USING btree (session_id, source_row)` |
| `catalog` | `import_staging_rows` | `import_staging_rows_pkey` | `CREATE UNIQUE INDEX import_staging_rows_pkey ON catalog.import_staging_rows USING btree (id)` |
| `catalog` | `match_decisions` | `match_decisions_chosen_product_idx` | `CREATE INDEX match_decisions_chosen_product_idx ON catalog.match_decisions USING btree (chosen_product_id)` |
| `catalog` | `match_decisions` | `match_decisions_last_used_idx` | `CREATE INDEX match_decisions_last_used_idx ON catalog.match_decisions USING btree (last_used_at)` |
| `catalog` | `match_decisions` | `match_decisions_norm_idx` | `CREATE INDEX match_decisions_norm_idx ON catalog.match_decisions USING btree (norm_name)` |
| `catalog` | `match_decisions` | `match_decisions_org_idx` | `CREATE INDEX match_decisions_org_idx ON catalog.match_decisions USING btree (organization_id)` |
| `catalog` | `match_decisions` | `match_decisions_org_key_uk` | `CREATE UNIQUE INDEX match_decisions_org_key_uk ON catalog.match_decisions USING btree (COALESCE(organization_id, (0)::bigint), decision_key)` |
| `catalog` | `match_decisions` | `match_decisions_pkey` | `CREATE UNIQUE INDEX match_decisions_pkey ON catalog.match_decisions USING btree (id)` |
| `catalog` | `match_decisions` | `match_decisions_user_idx` | `CREATE INDEX match_decisions_user_idx ON catalog.match_decisions USING btree (user_id)` |
| `catalog` | `product_alerts` | `product_alerts_pkey` | `CREATE UNIQUE INDEX product_alerts_pkey ON catalog.product_alerts USING btree (id)` |
| `catalog` | `product_alerts` | `product_alerts_product_idx` | `CREATE INDEX product_alerts_product_idx ON catalog.product_alerts USING btree (product_id) WHERE (is_triggered = false)` |
| `catalog` | `product_alerts` | `product_alerts_user_idx` | `CREATE INDEX product_alerts_user_idx ON catalog.product_alerts USING btree (user_id)` |
| `catalog` | `product_aliases` | `product_aliases_alias_product_uk` | `CREATE UNIQUE INDEX product_aliases_alias_product_uk ON catalog.product_aliases USING btree (alias, product_id)` |
| `catalog` | `product_aliases` | `product_aliases_alias_trgm_idx` | `CREATE INDEX product_aliases_alias_trgm_idx ON catalog.product_aliases USING gin (alias gin_trgm_ops)` |
| `catalog` | `product_aliases` | `product_aliases_pkey` | `CREATE UNIQUE INDEX product_aliases_pkey ON catalog.product_aliases USING btree (id)` |
| `catalog` | `product_aliases` | `product_aliases_product_idx` | `CREATE INDEX product_aliases_product_idx ON catalog.product_aliases USING btree (product_id)` |
| `catalog` | `product_index` | `idx_catalog_product_index_org_created` | `CREATE INDEX idx_catalog_product_index_org_created ON catalog.product_index USING btree (organization_id, created_at DESC)` |
| `catalog` | `product_index` | `idx_catalog_product_index_product_id` | `CREATE INDEX idx_catalog_product_index_product_id ON catalog.product_index USING btree (product_id)` |
| `catalog` | `product_index` | `idx_catalog_product_index_variant_id` | `CREATE INDEX idx_catalog_product_index_variant_id ON catalog.product_index USING btree (variant_id)` |
| `catalog` | `product_index` | `idx_product_index_branch` | `CREATE INDEX idx_product_index_branch ON catalog.product_index USING btree (branch_id)` |
| `catalog` | `product_index` | `idx_product_index_brand` | `CREATE INDEX idx_product_index_brand ON catalog.product_index USING btree (brand_id)` |
| `catalog` | `product_index` | `idx_product_index_category` | `CREATE INDEX idx_product_index_category ON catalog.product_index USING btree (category_id)` |
| `catalog` | `product_index` | `idx_product_index_inst_works` | `CREATE INDEX idx_product_index_inst_works ON catalog.product_index USING gin (institutional_work_ids)` |
| `catalog` | `product_index` | `idx_product_index_org_status` | `CREATE INDEX idx_product_index_org_status ON catalog.product_index USING btree (organization_id, status)` |
| `catalog` | `product_index` | `idx_product_index_price_discount` | `CREATE INDEX idx_product_index_price_discount ON catalog.product_index USING btree (price_after_discount)` |
| `catalog` | `product_index` | `idx_product_index_search_simple_trgm` | `CREATE INDEX idx_product_index_search_simple_trgm ON catalog.product_index USING gin (search_simple gin_trgm_ops)` |
| `catalog` | `product_index` | `idx_product_index_search_vector` | `CREATE INDEX idx_product_index_search_vector ON catalog.product_index USING gin (search_vector)` |
| `catalog` | `product_index` | `product_index_in_stock_idx` | `CREATE INDEX product_index_in_stock_idx ON catalog.product_index USING btree (product_id) WHERE (stock_quantity > 0)` |
| `catalog` | `product_index` | `product_index_pkey` | `CREATE UNIQUE INDEX product_index_pkey ON catalog.product_index USING btree (unique_row_id)` |
| `catalog` | `product_index` | `product_index_product_type_idx` | `CREATE INDEX product_index_product_type_idx ON catalog.product_index USING btree (product_id, product_type)` |
| `catalog` | `product_index` | `product_index_variant_idx` | `CREATE INDEX product_index_variant_idx ON catalog.product_index USING btree (variant_id) WHERE (variant_id IS NOT NULL)` |
| `catalog` | `product_variants` | `idx_catalog_product_variants_branch_id` | `CREATE INDEX idx_catalog_product_variants_branch_id ON catalog.product_variants USING btree (branch_id)` |
| `catalog` | `product_variants` | `idx_catalog_product_variants_org_created` | `CREATE INDEX idx_catalog_product_variants_org_created ON catalog.product_variants USING btree (organization_id, created_at DESC)` |
| `catalog` | `product_variants` | `idx_product_variants_barcode` | `CREATE INDEX idx_product_variants_barcode ON catalog.product_variants USING btree (barcode)` |
| `catalog` | `product_variants` | `idx_product_variants_name_en_trgm` | `CREATE INDEX idx_product_variants_name_en_trgm ON catalog.product_variants USING gin (((name ->> 'en'::text)) gin_trgm_ops)` |
| `catalog` | `product_variants` | `product_variants_offer_id_idx` | `CREATE INDEX product_variants_offer_id_idx ON catalog.product_variants USING btree (offer_id) WHERE (offer_id IS NOT NULL)` |
| `catalog` | `product_variants` | `product_variants_org_idx` | `CREATE INDEX product_variants_org_idx ON catalog.product_variants USING btree (organization_id) WHERE (deleted_at IS NULL)` |
| `catalog` | `product_variants` | `product_variants_org_sku_key` | `CREATE UNIQUE INDEX product_variants_org_sku_key ON catalog.product_variants USING btree (organization_id, sku) WHERE ((deleted_at IS NULL) AND (sku <> ''::text))` |
| `catalog` | `product_variants` | `product_variants_pkey` | `CREATE UNIQUE INDEX product_variants_pkey ON catalog.product_variants USING btree (id)` |
| `catalog` | `product_variants` | `product_variants_product_idx` | `CREATE INDEX product_variants_product_idx ON catalog.product_variants USING btree (product_id)` |
| `catalog` | `product_variants` | `product_variants_public_id_key` | `CREATE UNIQUE INDEX product_variants_public_id_key ON catalog.product_variants USING btree (public_id)` |
| `catalog` | `product_variants` | `product_variants_sku_idx` | `CREATE INDEX product_variants_sku_idx ON catalog.product_variants USING btree (sku) WHERE (sku IS NOT NULL)` |
| `catalog` | `product_variants` | `product_variants_variant_type_idx` | `CREATE INDEX product_variants_variant_type_idx ON catalog.product_variants USING btree (variant_type)` |
| `catalog` | `products` | `idx_catalog_products_branch_id` | `CREATE INDEX idx_catalog_products_branch_id ON catalog.products USING btree (branch_id)` |
| `catalog` | `products` | `idx_catalog_products_org_created` | `CREATE INDEX idx_catalog_products_org_created ON catalog.products USING btree (organization_id, created_at DESC)` |
| `catalog` | `products` | `idx_products_active_trgm` | `CREATE INDEX idx_products_active_trgm ON catalog.products USING gin (active gin_trgm_ops) WHERE ((deleted_at IS NULL) AND (active <> ''::text))` |
| `catalog` | `products` | `idx_products_category_brand` | `CREATE INDEX idx_products_category_brand ON catalog.products USING btree (category_id, brand_id) WHERE (deleted_at IS NULL)` |
| `catalog` | `products` | `idx_products_dosage` | `CREATE INDEX idx_products_dosage ON catalog.products USING btree (dosage_form) WHERE ((deleted_at IS NULL) AND (dosage_form <> ''::text))` |
| `catalog` | `products` | `idx_products_name_ar_norm_btree` | `CREATE INDEX idx_products_name_ar_norm_btree ON catalog.products USING btree (platform.normalize_arabic(lower(TRIM(BOTH FROM (name ->> 'ar'::text))))) WHERE (deleted_at IS NULL)` |
| `catalog` | `products` | `idx_products_name_en_lower_btree` | `CREATE INDEX idx_products_name_en_lower_btree ON catalog.products USING btree (lower(TRIM(BOTH FROM (name ->> 'en'::text)))) WHERE (deleted_at IS NULL)` |
| `catalog` | `products` | `idx_products_name_en_trgm` | `CREATE INDEX idx_products_name_en_trgm ON catalog.products USING gin (((name ->> 'en'::text)) gin_trgm_ops)` |
| `catalog` | `products` | `idx_products_org_status` | `CREATE INDEX idx_products_org_status ON catalog.products USING btree (organization_id, status) WHERE (deleted_at IS NULL)` |
| `catalog` | `products` | `idx_products_price_sort` | `CREATE INDEX idx_products_price_sort ON catalog.products USING btree (price) WHERE (deleted_at IS NULL)` |
| `catalog` | `products` | `idx_products_scientific_trgm` | `CREATE INDEX idx_products_scientific_trgm ON catalog.products USING gin (scientific_name gin_trgm_ops) WHERE ((deleted_at IS NULL) AND (scientific_name <> ''::text))` |
| `catalog` | `products` | `products_barcode_idx` | `CREATE INDEX products_barcode_idx ON catalog.products USING btree (barcode) WHERE (barcode IS NOT NULL)` |
| `catalog` | `products` | `products_brand_idx` | `CREATE INDEX products_brand_idx ON catalog.products USING btree (brand_id)` |
| `catalog` | `products` | `products_category_idx` | `CREATE INDEX products_category_idx ON catalog.products USING btree (category_id)` |
| `catalog` | `products` | `products_inst_work_ids_gin` | `CREATE INDEX products_inst_work_ids_gin ON catalog.products USING gin (institutional_work_ids)` |
| `catalog` | `products` | `products_name_ar_trgm_idx` | `CREATE INDEX products_name_ar_trgm_idx ON catalog.products USING gin (platform.normalize_arabic((name ->> 'ar'::text)) gin_trgm_ops)` |
| `catalog` | `products` | `products_org_barcode_lookup` | `CREATE INDEX products_org_barcode_lookup ON catalog.products USING btree (organization_id, lower(btrim(barcode))) WHERE ((deleted_at IS NULL) AND (btrim(barcode) <> ''::text))` |
| `catalog` | `products` | `products_org_idx` | `CREATE INDEX products_org_idx ON catalog.products USING btree (organization_id) WHERE (deleted_at IS NULL)` |
| `catalog` | `products` | `products_org_sku_uniq` | `CREATE UNIQUE INDEX products_org_sku_uniq ON catalog.products USING btree (organization_id, lower(btrim(sku))) WHERE ((deleted_at IS NULL) AND (btrim(sku) <> ''::text))` |
| `catalog` | `products` | `products_pkey` | `CREATE UNIQUE INDEX products_pkey ON catalog.products USING btree (id)` |
| `catalog` | `products` | `products_public_id_key` | `CREATE UNIQUE INDEX products_public_id_key ON catalog.products USING btree (public_id)` |
| `catalog` | `products` | `products_sku_idx` | `CREATE INDEX products_sku_idx ON catalog.products USING btree (sku) WHERE (sku IS NOT NULL)` |
| `catalog` | `products` | `products_status_idx` | `CREATE INDEX products_status_idx ON catalog.products USING btree (status) WHERE (deleted_at IS NULL)` |
| `catalog` | `saving_products` | `idx_catalog_saving_products_org_created` | `CREATE INDEX idx_catalog_saving_products_org_created ON catalog.saving_products USING btree (organization_id, created_at DESC)` |
| `catalog` | `saving_products` | `idx_saving_products_norm_name` | `CREATE INDEX idx_saving_products_norm_name ON catalog.saving_products USING btree (platform.normalize_arabic(lower(TRIM(BOTH FROM name_product)))) WHERE ((deleted_at IS NULL) AND (product_id IS NOT NULL))` |
| `catalog` | `saving_products` | `idx_saving_products_org_prod` | `CREATE INDEX idx_saving_products_org_prod ON catalog.saving_products USING btree (organization_id, product_id) WHERE (deleted_at IS NULL)` |
| `catalog` | `saving_products` | `idx_saving_products_user` | `CREATE INDEX idx_saving_products_user ON catalog.saving_products USING btree (user_id) WHERE ((deleted_at IS NULL) AND (user_id IS NOT NULL))` |
| `catalog` | `saving_products` | `saving_products_name_idx` | `CREATE INDEX saving_products_name_idx ON catalog.saving_products USING btree (name_product) WHERE (deleted_at IS NULL)` |
| `catalog` | `saving_products` | `saving_products_org_idx` | `CREATE INDEX saving_products_org_idx ON catalog.saving_products USING btree (organization_id) WHERE (deleted_at IS NULL)` |
| `catalog` | `saving_products` | `saving_products_pkey` | `CREATE UNIQUE INDEX saving_products_pkey ON catalog.saving_products USING btree (id)` |
| `catalog` | `saving_products` | `saving_products_product_idx` | `CREATE INDEX saving_products_product_idx ON catalog.saving_products USING btree (product_id) WHERE (deleted_at IS NULL)` |
| `catalog` | `saving_products` | `saving_products_public_id_key` | `CREATE UNIQUE INDEX saving_products_public_id_key ON catalog.saving_products USING btree (public_id)` |
| `catalog` | `saving_products` | `saving_products_user_idx` | `CREATE INDEX saving_products_user_idx ON catalog.saving_products USING btree (user_id) WHERE (deleted_at IS NULL)` |
| `commerce` | `cart_items` | `cart_items_cart_idx` | `CREATE INDEX cart_items_cart_idx ON commerce.cart_items USING btree (cart_id)` |
| `commerce` | `cart_items` | `cart_items_cart_offer_uniq` | `CREATE UNIQUE INDEX cart_items_cart_offer_uniq ON commerce.cart_items USING btree (cart_id, offer_id) WHERE ((offer_id IS NOT NULL) AND (product_variant_id IS NULL))` |
| `commerce` | `cart_items` | `cart_items_offer_idx` | `CREATE INDEX cart_items_offer_idx ON commerce.cart_items USING btree (offer_id)` |
| `commerce` | `cart_items` | `cart_items_pkey` | `CREATE UNIQUE INDEX cart_items_pkey ON commerce.cart_items USING btree (id)` |
| `commerce` | `cart_items` | `cart_items_variant_unique` | `CREATE UNIQUE INDEX cart_items_variant_unique ON commerce.cart_items USING btree (cart_id, product_variant_id)` |
| `commerce` | `cart_items` | `idx_commerce_cart_items_product_id` | `CREATE INDEX idx_commerce_cart_items_product_id ON commerce.cart_items USING btree (product_id)` |
| `commerce` | `cart_items` | `idx_commerce_cart_items_product_variant_id` | `CREATE INDEX idx_commerce_cart_items_product_variant_id ON commerce.cart_items USING btree (product_variant_id)` |
| `commerce` | `carts` | `carts_pkey` | `CREATE UNIQUE INDEX carts_pkey ON commerce.carts USING btree (id)` |
| `commerce` | `carts` | `carts_public_id_key` | `CREATE UNIQUE INDEX carts_public_id_key ON commerce.carts USING btree (public_id)` |
| `commerce` | `carts` | `carts_user_unique` | `CREATE UNIQUE INDEX carts_user_unique ON commerce.carts USING btree (user_id)` |
| `commerce` | `carts` | `idx_carts_org_user` | `CREATE INDEX idx_carts_org_user ON commerce.carts USING btree (organization_id, user_id)` |
| `commerce` | `carts` | `idx_commerce_carts_org_created` | `CREATE INDEX idx_commerce_carts_org_created ON commerce.carts USING btree (organization_id, created_at DESC)` |
| `commerce` | `carts` | `idx_commerce_carts_organization_id` | `CREATE INDEX idx_commerce_carts_organization_id ON commerce.carts USING btree (organization_id)` |
| `commerce` | `order_lines` | `idx_commerce_order_lines_org_created` | `CREATE INDEX idx_commerce_order_lines_org_created ON commerce.order_lines USING btree (organization_id, created_at DESC)` |
| `commerce` | `order_lines` | `idx_commerce_order_lines_product_id` | `CREATE INDEX idx_commerce_order_lines_product_id ON commerce.order_lines USING btree (product_id)` |
| `commerce` | `order_lines` | `idx_commerce_order_lines_product_variant_id` | `CREATE INDEX idx_commerce_order_lines_product_variant_id ON commerce.order_lines USING btree (product_variant_id)` |
| `commerce` | `order_lines` | `order_lines_offer_product_idx` | `CREATE INDEX order_lines_offer_product_idx ON commerce.order_lines USING btree (offer_product_id)` |
| `commerce` | `order_lines` | `order_lines_order_idx` | `CREATE INDEX order_lines_order_idx ON commerce.order_lines USING btree (order_id)` |
| `commerce` | `order_lines` | `order_lines_org_idx` | `CREATE INDEX order_lines_org_idx ON commerce.order_lines USING btree (organization_id)` |
| `commerce` | `order_lines` | `order_lines_pkey` | `CREATE UNIQUE INDEX order_lines_pkey ON commerce.order_lines USING btree (id)` |
| `commerce` | `order_lines` | `order_lines_shipment_idx` | `CREATE INDEX order_lines_shipment_idx ON commerce.order_lines USING btree (shipment_id)` |
| `commerce` | `order_shipments` | `idx_commerce_order_shipments_branch_id` | `CREATE INDEX idx_commerce_order_shipments_branch_id ON commerce.order_shipments USING btree (branch_id)` |
| `commerce` | `order_shipments` | `idx_commerce_order_shipments_org_created` | `CREATE INDEX idx_commerce_order_shipments_org_created ON commerce.order_shipments USING btree (organization_id, created_at DESC)` |
| `commerce` | `order_shipments` | `order_shipments_number_key` | `CREATE UNIQUE INDEX order_shipments_number_key ON commerce.order_shipments USING btree (shipment_number)` |
| `commerce` | `order_shipments` | `order_shipments_order_idx` | `CREATE INDEX order_shipments_order_idx ON commerce.order_shipments USING btree (order_id)` |
| `commerce` | `order_shipments` | `order_shipments_org_idx` | `CREATE INDEX order_shipments_org_idx ON commerce.order_shipments USING btree (organization_id)` |
| `commerce` | `order_shipments` | `order_shipments_pkey` | `CREATE UNIQUE INDEX order_shipments_pkey ON commerce.order_shipments USING btree (id)` |
| `commerce` | `order_shipments` | `order_shipments_public_id_key` | `CREATE UNIQUE INDEX order_shipments_public_id_key ON commerce.order_shipments USING btree (public_id)` |
| `commerce` | `order_shipments` | `order_shipments_tracking_number_idx` | `CREATE INDEX order_shipments_tracking_number_idx ON commerce.order_shipments USING btree (tracking_number) WHERE (tracking_number <> ''::text)` |
| `commerce` | `order_status_history` | `idx_commerce_order_status_history_changed_by_user_id` | `CREATE INDEX idx_commerce_order_status_history_changed_by_user_id ON commerce.order_status_history USING btree (changed_by_user_id)` |
| `commerce` | `order_status_history` | `order_history_order_idx` | `CREATE INDEX order_history_order_idx ON commerce.order_status_history USING btree (order_id, created_at DESC)` |
| `commerce` | `order_status_history` | `order_history_shipment_idx` | `CREATE INDEX order_history_shipment_idx ON commerce.order_status_history USING btree (shipment_id, created_at DESC)` |
| `commerce` | `order_status_history` | `order_status_history_pkey` | `CREATE UNIQUE INDEX order_status_history_pkey ON commerce.order_status_history USING btree (id)` |
| `commerce` | `orders` | `idx_commerce_orders_org_created` | `CREATE INDEX idx_commerce_orders_org_created ON commerce.orders USING btree (organization_id, created_at DESC)` |
| `commerce` | `orders` | `idx_commerce_orders_user_address_id` | `CREATE INDEX idx_commerce_orders_user_address_id ON commerce.orders USING btree (user_address_id)` |
| `commerce` | `orders` | `idx_orders_customer_status` | `CREATE INDEX idx_orders_customer_status ON commerce.orders USING btree (customer_id, status, created_at DESC) WHERE (deleted_at IS NULL)` |
| `commerce` | `orders` | `idx_orders_org_status` | `CREATE INDEX idx_orders_org_status ON commerce.orders USING btree (organization_id, status, created_at DESC) WHERE (deleted_at IS NULL)` |
| `commerce` | `orders` | `orders_branch_idx` | `CREATE INDEX orders_branch_idx ON commerce.orders USING btree (branch_id) WHERE (deleted_at IS NULL)` |
| `commerce` | `orders` | `orders_customer_idx` | `CREATE INDEX orders_customer_idx ON commerce.orders USING btree (customer_id)` |
| `commerce` | `orders` | `orders_offer_idx` | `CREATE INDEX orders_offer_idx ON commerce.orders USING btree (offer_id) WHERE (deleted_at IS NULL)` |
| `commerce` | `orders` | `orders_order_number_key` | `CREATE UNIQUE INDEX orders_order_number_key ON commerce.orders USING btree (order_number)` |
| `commerce` | `orders` | `orders_org_idx` | `CREATE INDEX orders_org_idx ON commerce.orders USING btree (organization_id) WHERE (organization_id IS NOT NULL)` |
| `commerce` | `orders` | `orders_pkey` | `CREATE UNIQUE INDEX orders_pkey ON commerce.orders USING btree (id)` |
| `commerce` | `orders` | `orders_public_id_key` | `CREATE UNIQUE INDEX orders_public_id_key ON commerce.orders USING btree (public_id)` |
| `commerce` | `orders` | `orders_rating_idx` | `CREATE INDEX orders_rating_idx ON commerce.orders USING btree (organization_id, rating) WHERE (rating IS NOT NULL)` |
| `commerce` | `orders` | `orders_status_idx` | `CREATE INDEX orders_status_idx ON commerce.orders USING btree (status)` |
| `commerce` | `orders` | `orders_vendor_branch_idx` | `CREATE INDEX orders_vendor_branch_idx ON commerce.orders USING btree (vendor_branch_id) WHERE (deleted_at IS NULL)` |
| `commerce` | `purchase_request_lines` | `purchase_request_lines_pkey` | `CREATE UNIQUE INDEX purchase_request_lines_pkey ON commerce.purchase_request_lines USING btree (id)` |
| `commerce` | `purchase_request_lines` | `purchase_request_lines_product_idx` | `CREATE INDEX purchase_request_lines_product_idx ON commerce.purchase_request_lines USING btree (product_id)` |
| `commerce` | `purchase_request_lines` | `purchase_request_lines_request_idx` | `CREATE INDEX purchase_request_lines_request_idx ON commerce.purchase_request_lines USING btree (request_id)` |
| `commerce` | `purchase_requests` | `idx_commerce_purchase_requests_branch_id` | `CREATE INDEX idx_commerce_purchase_requests_branch_id ON commerce.purchase_requests USING btree (branch_id)` |
| `commerce` | `purchase_requests` | `idx_commerce_purchase_requests_org_created` | `CREATE INDEX idx_commerce_purchase_requests_org_created ON commerce.purchase_requests USING btree (organization_id, created_at DESC)` |
| `commerce` | `purchase_requests` | `idx_commerce_purchase_requests_responded_by` | `CREATE INDEX idx_commerce_purchase_requests_responded_by ON commerce.purchase_requests USING btree (responded_by)` |
| `commerce` | `purchase_requests` | `idx_commerce_purchase_requests_vendor_branch_id` | `CREATE INDEX idx_commerce_purchase_requests_vendor_branch_id ON commerce.purchase_requests USING btree (vendor_branch_id)` |
| `commerce` | `purchase_requests` | `purchase_requests_customer_idx` | `CREATE INDEX purchase_requests_customer_idx ON commerce.purchase_requests USING btree (customer_id, status)` |
| `commerce` | `purchase_requests` | `purchase_requests_org_idx` | `CREATE INDEX purchase_requests_org_idx ON commerce.purchase_requests USING btree (organization_id, status)` |
| `commerce` | `purchase_requests` | `purchase_requests_pkey` | `CREATE UNIQUE INDEX purchase_requests_pkey ON commerce.purchase_requests USING btree (id)` |
| `commerce` | `purchase_requests` | `purchase_requests_request_number_key` | `CREATE UNIQUE INDEX purchase_requests_request_number_key ON commerce.purchase_requests USING btree (request_number)` |
| `commerce` | `purchase_requests` | `purchase_requests_vendor_idx` | `CREATE INDEX purchase_requests_vendor_idx ON commerce.purchase_requests USING btree (vendor_org_id, status)` |
| `commerce` | `quote_requests` | `idx_commerce_quote_requests_delivery_branch_id` | `CREATE INDEX idx_commerce_quote_requests_delivery_branch_id ON commerce.quote_requests USING btree (delivery_branch_id)` |
| `commerce` | `quote_requests` | `idx_commerce_quote_requests_org_created` | `CREATE INDEX idx_commerce_quote_requests_org_created ON commerce.quote_requests USING btree (organization_id, created_at DESC)` |
| `commerce` | `quote_requests` | `idx_commerce_quote_requests_product_id` | `CREATE INDEX idx_commerce_quote_requests_product_id ON commerce.quote_requests USING btree (product_id)` |
| `commerce` | `quote_requests` | `quote_requests_customer_idx` | `CREATE INDEX quote_requests_customer_idx ON commerce.quote_requests USING btree (customer_org_id, status)` |
| `commerce` | `quote_requests` | `quote_requests_delivery_city_idx` | `CREATE INDEX quote_requests_delivery_city_idx ON commerce.quote_requests USING btree (delivery_city_id)` |
| `commerce` | `quote_requests` | `quote_requests_org_idx` | `CREATE INDEX quote_requests_org_idx ON commerce.quote_requests USING btree (organization_id, status)` |
| `commerce` | `quote_requests` | `quote_requests_pkey` | `CREATE UNIQUE INDEX quote_requests_pkey ON commerce.quote_requests USING btree (id)` |
| `commerce` | `wishlists` | `idx_commerce_wishlists_product_id` | `CREATE INDEX idx_commerce_wishlists_product_id ON commerce.wishlists USING btree (product_id)` |
| `commerce` | `wishlists` | `wishlists_pkey` | `CREATE UNIQUE INDEX wishlists_pkey ON commerce.wishlists USING btree (id)` |
| `commerce` | `wishlists` | `wishlists_public_id_key` | `CREATE UNIQUE INDEX wishlists_public_id_key ON commerce.wishlists USING btree (public_id)` |
| `commerce` | `wishlists` | `wishlists_user_idx` | `CREATE INDEX wishlists_user_idx ON commerce.wishlists USING btree (user_id)` |
| `commerce` | `wishlists` | `wishlists_user_product_unique` | `CREATE UNIQUE INDEX wishlists_user_product_unique ON commerce.wishlists USING btree (user_id, product_id)` |
| `compare` | `file_rows` | `compare_file_rows_file_idx` | `CREATE INDEX compare_file_rows_file_idx ON compare.file_rows USING btree (file_id, row_number)` |
| `compare` | `file_rows` | `compare_file_rows_matched_prod_idx` | `CREATE INDEX compare_file_rows_matched_prod_idx ON compare.file_rows USING btree (matched_product_id)` |
| `compare` | `file_rows` | `compare_file_rows_norm_trgm_idx` | `CREATE INDEX compare_file_rows_norm_trgm_idx ON compare.file_rows USING gin (normalized_name gin_trgm_ops)` |
| `compare` | `file_rows` | `compare_file_rows_org_norm_idx` | `CREATE INDEX compare_file_rows_org_norm_idx ON compare.file_rows USING btree (organization_id, normalized_name)` |
| `compare` | `file_rows` | `file_rows_pkey` | `CREATE UNIQUE INDEX file_rows_pkey ON compare.file_rows USING btree (id)` |
| `compare` | `file_rows` | `idx_compare_file_rows_created_at` | `CREATE INDEX idx_compare_file_rows_created_at ON compare.file_rows USING btree (created_at DESC)` |
| `compare` | `file_rows` | `idx_compare_file_rows_file_id` | `CREATE INDEX idx_compare_file_rows_file_id ON compare.file_rows USING btree (file_id)` |
| `compare` | `file_rows` | `idx_compare_file_rows_norm_name` | `CREATE INDEX idx_compare_file_rows_norm_name ON compare.file_rows USING gin (normalized_name gin_trgm_ops) WHERE (normalized_name <> ''::text)` |
| `compare` | `file_rows` | `idx_compare_file_rows_org_created` | `CREATE INDEX idx_compare_file_rows_org_created ON compare.file_rows USING btree (organization_id, created_at DESC)` |
| `compare` | `file_rows` | `idx_compare_file_rows_price_disc` | `CREATE INDEX idx_compare_file_rows_price_disc ON compare.file_rows USING btree (price_after_discount, discount DESC)` |
| `compare` | `file_rows` | `idx_compare_file_rows_sku` | `CREATE INDEX idx_compare_file_rows_sku ON compare.file_rows USING btree (sku) WHERE (sku IS NOT NULL)` |
| `compare` | `files` | `compare_files_org_status_idx` | `CREATE INDEX compare_files_org_status_idx ON compare.files USING btree (organization_id, status) WHERE (deleted_at IS NULL)` |
| `compare` | `files` | `compare_files_public_id_idx` | `CREATE UNIQUE INDEX compare_files_public_id_idx ON compare.files USING btree (public_id)` |
| `compare` | `files` | `compare_files_visibility_idx` | `CREATE INDEX compare_files_visibility_idx ON compare.files USING btree (visibility) WHERE (deleted_at IS NULL)` |
| `compare` | `files` | `files_pkey` | `CREATE UNIQUE INDEX files_pkey ON compare.files USING btree (id)` |
| `compare` | `files` | `idx_compare_files_org_created` | `CREATE INDEX idx_compare_files_org_created ON compare.files USING btree (organization_id, created_at DESC)` |
| `compare` | `files` | `idx_compare_files_temp_wh` | `CREATE INDEX idx_compare_files_temp_wh ON compare.files USING btree (is_temp_warehouse, status) WHERE (deleted_at IS NULL)` |
| `compare` | `files` | `idx_compare_files_user_status` | `CREATE INDEX idx_compare_files_user_status ON compare.files USING btree (user_id, status) WHERE (deleted_at IS NULL)` |
| `compare` | `plan_features` | `compare_plan_features_plan_idx` | `CREATE INDEX compare_plan_features_plan_idx ON compare.plan_features USING btree (plan_id)` |
| `compare` | `plan_features` | `compare_plan_features_plan_key_unique` | `CREATE UNIQUE INDEX compare_plan_features_plan_key_unique ON compare.plan_features USING btree (plan_id, key)` |
| `compare` | `plan_features` | `idx_compare_plan_features_created_by` | `CREATE INDEX idx_compare_plan_features_created_by ON compare.plan_features USING btree (created_by)` |
| `compare` | `plan_features` | `idx_compare_plan_features_updated_by` | `CREATE INDEX idx_compare_plan_features_updated_by ON compare.plan_features USING btree (updated_by)` |
| `compare` | `plan_features` | `plan_features_pkey` | `CREATE UNIQUE INDEX plan_features_pkey ON compare.plan_features USING btree (id)` |
| `compare` | `plan_requests` | `compare_plan_requests_org_idx` | `CREATE INDEX compare_plan_requests_org_idx ON compare.plan_requests USING btree (organization_id)` |
| `compare` | `plan_requests` | `compare_plan_requests_status_idx` | `CREATE INDEX compare_plan_requests_status_idx ON compare.plan_requests USING btree (status)` |
| `compare` | `plan_requests` | `idx_compare_plan_requests_org_created` | `CREATE INDEX idx_compare_plan_requests_org_created ON compare.plan_requests USING btree (organization_id, created_at DESC)` |
| `compare` | `plan_requests` | `idx_compare_plan_requests_plan_id` | `CREATE INDEX idx_compare_plan_requests_plan_id ON compare.plan_requests USING btree (plan_id)` |
| `compare` | `plan_requests` | `idx_compare_plan_requests_reviewed_by` | `CREATE INDEX idx_compare_plan_requests_reviewed_by ON compare.plan_requests USING btree (reviewed_by)` |
| `compare` | `plan_requests` | `idx_compare_plan_requests_user_id` | `CREATE INDEX idx_compare_plan_requests_user_id ON compare.plan_requests USING btree (user_id)` |
| `compare` | `plan_requests` | `plan_requests_pkey` | `CREATE UNIQUE INDEX plan_requests_pkey ON compare.plan_requests USING btree (id)` |
| `compare` | `plans` | `compare_plans_active_idx` | `CREATE INDEX compare_plans_active_idx ON compare.plans USING btree (is_active, sort_order) WHERE (deleted_at IS NULL)` |
| `compare` | `plans` | `compare_plans_public_id_idx` | `CREATE UNIQUE INDEX compare_plans_public_id_idx ON compare.plans USING btree (public_id)` |
| `compare` | `plans` | `idx_compare_plans_created_by` | `CREATE INDEX idx_compare_plans_created_by ON compare.plans USING btree (created_by)` |
| `compare` | `plans` | `idx_compare_plans_updated_by` | `CREATE INDEX idx_compare_plans_updated_by ON compare.plans USING btree (updated_by)` |
| `compare` | `plans` | `plans_pkey` | `CREATE UNIQUE INDEX plans_pkey ON compare.plans USING btree (id)` |
| `compare` | `plans` | `plans_slug_key` | `CREATE UNIQUE INDEX plans_slug_key ON compare.plans USING btree (slug)` |
| `compare` | `subscription_users` | `compare_sub_users_unique` | `CREATE UNIQUE INDEX compare_sub_users_unique ON compare.subscription_users USING btree (subscription_id, user_id)` |
| `compare` | `subscription_users` | `compare_sub_users_user_idx` | `CREATE INDEX compare_sub_users_user_idx ON compare.subscription_users USING btree (user_id)` |
| `compare` | `subscription_users` | `idx_compare_subscription_users_subscription_id` | `CREATE INDEX idx_compare_subscription_users_subscription_id ON compare.subscription_users USING btree (subscription_id)` |
| `compare` | `subscription_users` | `subscription_users_pkey` | `CREATE UNIQUE INDEX subscription_users_pkey ON compare.subscription_users USING btree (id)` |
| `compare` | `subscriptions` | `compare_subscriptions_org_idx` | `CREATE INDEX compare_subscriptions_org_idx ON compare.subscriptions USING btree (organization_id)` |
| `compare` | `subscriptions` | `compare_subscriptions_status_idx` | `CREATE INDEX compare_subscriptions_status_idx ON compare.subscriptions USING btree (status, ends_at)` |
| `compare` | `subscriptions` | `compare_subscriptions_user_idx` | `CREATE INDEX compare_subscriptions_user_idx ON compare.subscriptions USING btree (user_id)` |
| `compare` | `subscriptions` | `idx_compare_subscriptions_org_created` | `CREATE INDEX idx_compare_subscriptions_org_created ON compare.subscriptions USING btree (organization_id, created_at DESC)` |
| `compare` | `subscriptions` | `idx_compare_subscriptions_plan_id` | `CREATE INDEX idx_compare_subscriptions_plan_id ON compare.subscriptions USING btree (plan_id)` |
| `compare` | `subscriptions` | `subscriptions_pkey` | `CREATE UNIQUE INDEX subscriptions_pkey ON compare.subscriptions USING btree (id)` |
| `compare` | `user_sessions` | `compare_user_sessions_sub_user_idx` | `CREATE INDEX compare_user_sessions_sub_user_idx ON compare.user_sessions USING btree (subscription_user_id)` |
| `compare` | `user_sessions` | `compare_user_sessions_user_active_idx` | `CREATE INDEX compare_user_sessions_user_active_idx ON compare.user_sessions USING btree (user_id, is_active, last_activity_at)` |
| `compare` | `user_sessions` | `idx_compare_sessions_user_active` | `CREATE INDEX idx_compare_sessions_user_active ON compare.user_sessions USING btree (user_id, is_active, last_activity_at DESC) WHERE (deleted_at IS NULL)` |
| `compare` | `user_sessions` | `user_sessions_pkey` | `CREATE UNIQUE INDEX user_sessions_pkey ON compare.user_sessions USING btree (id)` |
| `hr` | `job_applications` | `idx_hr_job_applications_applicant_user_id` | `CREATE INDEX idx_hr_job_applications_applicant_user_id ON hr.job_applications USING btree (applicant_user_id)` |
| `hr` | `job_applications` | `idx_hr_job_applications_org_created` | `CREATE INDEX idx_hr_job_applications_org_created ON hr.job_applications USING btree (organization_id, created_at DESC)` |
| `hr` | `job_applications` | `idx_hr_job_applications_organization_id` | `CREATE INDEX idx_hr_job_applications_organization_id ON hr.job_applications USING btree (organization_id)` |
| `hr` | `job_applications` | `idx_job_applications_offer` | `CREATE INDEX idx_job_applications_offer ON hr.job_applications USING btree (job_offer_id)` |
| `hr` | `job_applications` | `job_applications_assigned_role_idx` | `CREATE INDEX job_applications_assigned_role_idx ON hr.job_applications USING btree (assigned_role_key)` |
| `hr` | `job_applications` | `job_applications_branch_id_idx` | `CREATE INDEX job_applications_branch_id_idx ON hr.job_applications USING btree (branch_id)` |
| `hr` | `job_applications` | `job_applications_pkey` | `CREATE UNIQUE INDEX job_applications_pkey ON hr.job_applications USING btree (id)` |
| `hr` | `job_categories` | `job_categories_pkey` | `CREATE UNIQUE INDEX job_categories_pkey ON hr.job_categories USING btree (id)` |
| `hr` | `job_categories` | `uq_job_categories_slug` | `CREATE UNIQUE INDEX uq_job_categories_slug ON hr.job_categories USING btree (slug)` |
| `hr` | `job_offers` | `idx_hr_job_offers_org_created` | `CREATE INDEX idx_hr_job_offers_org_created ON hr.job_offers USING btree (organization_id, created_at DESC)` |
| `hr` | `job_offers` | `idx_job_offers_category` | `CREATE INDEX idx_job_offers_category ON hr.job_offers USING btree (category_id) WHERE (deleted_at IS NULL)` |
| `hr` | `job_offers` | `idx_job_offers_org` | `CREATE INDEX idx_job_offers_org ON hr.job_offers USING btree (organization_id) WHERE (deleted_at IS NULL)` |
| `hr` | `job_offers` | `job_offers_pkey` | `CREATE UNIQUE INDEX job_offers_pkey ON hr.job_offers USING btree (id)` |
| `hr` | `job_seeker_profiles` | `idx_hr_job_seeker_profiles_cv_document_id` | `CREATE INDEX idx_hr_job_seeker_profiles_cv_document_id ON hr.job_seeker_profiles USING btree (cv_document_id)` |
| `hr` | `job_seeker_profiles` | `idx_job_seeker_city` | `CREATE INDEX idx_job_seeker_city ON hr.job_seeker_profiles USING btree (preferred_city_id)` |
| `hr` | `job_seeker_profiles` | `idx_job_seeker_profiles_city` | `CREATE INDEX idx_job_seeker_profiles_city ON hr.job_seeker_profiles USING btree (preferred_city_id) WHERE is_open_to_work` |
| `hr` | `job_seeker_profiles` | `idx_job_seeker_profiles_spec` | `CREATE INDEX idx_job_seeker_profiles_spec ON hr.job_seeker_profiles USING btree (specialisation) WHERE is_open_to_work` |
| `hr` | `job_seeker_profiles` | `idx_job_seeker_specialisation` | `CREATE INDEX idx_job_seeker_specialisation ON hr.job_seeker_profiles USING btree (specialisation) WHERE (is_open_to_work = true)` |
| `hr` | `job_seeker_profiles` | `job_seeker_profiles_pkey` | `CREATE UNIQUE INDEX job_seeker_profiles_pkey ON hr.job_seeker_profiles USING btree (id)` |
| `hr` | `job_seeker_profiles` | `uq_job_seeker_user` | `CREATE UNIQUE INDEX uq_job_seeker_user ON hr.job_seeker_profiles USING btree (user_id)` |
| `hr` | `work_times` | `idx_hr_work_times_org_created` | `CREATE INDEX idx_hr_work_times_org_created ON hr.work_times USING btree (organization_id, created_at DESC)` |
| `hr` | `work_times` | `idx_hr_work_times_organization_id` | `CREATE INDEX idx_hr_work_times_organization_id ON hr.work_times USING btree (organization_id)` |
| `hr` | `work_times` | `work_times_pkey` | `CREATE UNIQUE INDEX work_times_pkey ON hr.work_times USING btree (id)` |
| `identity` | `account_deletion_requests` | `account_deletion_requests_pkey` | `CREATE UNIQUE INDEX account_deletion_requests_pkey ON identity.account_deletion_requests USING btree (id)` |
| `identity` | `account_deletion_requests` | `deletion_requests_status_idx` | `CREATE INDEX deletion_requests_status_idx ON identity.account_deletion_requests USING btree (status, requested_at)` |
| `identity` | `account_deletion_requests` | `idx_deletion_requests_status` | `CREATE INDEX idx_deletion_requests_status ON identity.account_deletion_requests USING btree (status)` |
| `identity` | `account_deletion_requests` | `idx_deletion_requests_user` | `CREATE INDEX idx_deletion_requests_user ON identity.account_deletion_requests USING btree (user_id)` |
| `identity` | `account_deletion_requests` | `idx_identity_account_deletion_requests_reviewed_by` | `CREATE INDEX idx_identity_account_deletion_requests_reviewed_by ON identity.account_deletion_requests USING btree (reviewed_by)` |
| `identity` | `permissions` | `idx_permissions_group` | `CREATE INDEX idx_permissions_group ON identity.permissions USING btree (group_key, sort_order)` |
| `identity` | `permissions` | `idx_permissions_scopes` | `CREATE INDEX idx_permissions_scopes ON identity.permissions USING gin (scopes)` |
| `identity` | `permissions` | `permissions_module_idx` | `CREATE INDEX permissions_module_idx ON identity.permissions USING btree (module)` |
| `identity` | `permissions` | `permissions_pkey` | `CREATE UNIQUE INDEX permissions_pkey ON identity.permissions USING btree (key)` |
| `identity` | `rbac_version` | `rbac_version_pkey` | `CREATE UNIQUE INDEX rbac_version_pkey ON identity.rbac_version USING btree (scope_key)` |
| `identity` | `role_permissions` | `idx_identity_role_permissions_permission_key` | `CREATE INDEX idx_identity_role_permissions_permission_key ON identity.role_permissions USING btree (permission_key)` |
| `identity` | `role_permissions` | `role_permissions_pkey` | `CREATE UNIQUE INDEX role_permissions_pkey ON identity.role_permissions USING btree (role_key, permission_key)` |
| `identity` | `roles` | `identity_roles_created_by_idx` | `CREATE INDEX identity_roles_created_by_idx ON identity.roles USING btree (created_by)` |
| `identity` | `roles` | `idx_identity_roles_staff` | `CREATE INDEX idx_identity_roles_staff ON identity.roles USING btree (is_staff) WHERE (deleted_at IS NULL)` |
| `identity` | `roles` | `roles_pkey` | `CREATE UNIQUE INDEX roles_pkey ON identity.roles USING btree (key)` |
| `identity` | `session_plans` | `session_plans_pkey` | `CREATE UNIQUE INDEX session_plans_pkey ON identity.session_plans USING btree (id)` |
| `identity` | `user_address_histories` | `user_address_histories_pkey` | `CREATE UNIQUE INDEX user_address_histories_pkey ON identity.user_address_histories USING btree (id)` |
| `identity` | `user_address_histories` | `user_address_histories_user_idx` | `CREATE INDEX user_address_histories_user_idx ON identity.user_address_histories USING btree (user_id, changed_at DESC)` |
| `identity` | `user_addresses` | `idx_identity_user_addresses_city_id` | `CREATE INDEX idx_identity_user_addresses_city_id ON identity.user_addresses USING btree (city_id)` |
| `identity` | `user_addresses` | `idx_identity_user_addresses_country_id` | `CREATE INDEX idx_identity_user_addresses_country_id ON identity.user_addresses USING btree (country_id)` |
| `identity` | `user_addresses` | `user_addresses_pkey` | `CREATE UNIQUE INDEX user_addresses_pkey ON identity.user_addresses USING btree (id)` |
| `identity` | `user_addresses` | `user_addresses_public_id_key` | `CREATE UNIQUE INDEX user_addresses_public_id_key ON identity.user_addresses USING btree (public_id)` |
| `identity` | `user_addresses` | `user_addresses_user_idx` | `CREATE INDEX user_addresses_user_idx ON identity.user_addresses USING btree (user_id)` |
| `identity` | `user_favorites` | `idx_identity_user_favorites_product_id` | `CREATE INDEX idx_identity_user_favorites_product_id ON identity.user_favorites USING btree (product_id)` |
| `identity` | `user_favorites` | `idx_user_favorites_user` | `CREATE INDEX idx_user_favorites_user ON identity.user_favorites USING btree (user_id)` |
| `identity` | `user_favorites` | `uq_user_favorite_product` | `CREATE UNIQUE INDEX uq_user_favorite_product ON identity.user_favorites USING btree (user_id, product_id)` |
| `identity` | `user_favorites` | `user_favorites_pkey` | `CREATE UNIQUE INDEX user_favorites_pkey ON identity.user_favorites USING btree (id)` |
| `identity` | `user_mfa` | `user_mfa_pkey` | `CREATE UNIQUE INDEX user_mfa_pkey ON identity.user_mfa USING btree (user_id)` |
| `identity` | `user_security` | `user_security_pkey` | `CREATE UNIQUE INDEX user_security_pkey ON identity.user_security USING btree (user_id)` |
| `identity` | `user_sessions` | `idx_user_sessions_user_active` | `CREATE INDEX idx_user_sessions_user_active ON identity.user_sessions USING btree (user_id, is_active)` |
| `identity` | `user_sessions` | `user_sessions_pkey` | `CREATE UNIQUE INDEX user_sessions_pkey ON identity.user_sessions USING btree (id)` |
| `identity` | `users` | `idx_identity_users_email_trgm` | `CREATE INDEX idx_identity_users_email_trgm ON identity.users USING gin (email gin_trgm_ops)` |
| `identity` | `users` | `idx_identity_users_phone` | `CREATE INDEX idx_identity_users_phone ON identity.users USING btree (phone)` |
| `identity` | `users` | `idx_identity_users_referred_by` | `CREATE INDEX idx_identity_users_referred_by ON identity.users USING btree (referred_by)` |
| `identity` | `users` | `idx_users_referral_code` | `CREATE INDEX idx_users_referral_code ON identity.users USING btree (referral_code)` |
| `identity` | `users` | `idx_users_role` | `CREATE INDEX idx_users_role ON identity.users USING btree (role) WHERE (deleted_at IS NULL)` |
| `identity` | `users` | `users_created_at_idx` | `CREATE INDEX users_created_at_idx ON identity.users USING btree (created_at DESC)` |
| `identity` | `users` | `users_email_key` | `CREATE UNIQUE INDEX users_email_key ON identity.users USING btree (email) WHERE (deleted_at IS NULL)` |
| `identity` | `users` | `users_pkey` | `CREATE UNIQUE INDEX users_pkey ON identity.users USING btree (id)` |
| `identity` | `users` | `users_public_id_key` | `CREATE UNIQUE INDEX users_public_id_key ON identity.users USING btree (public_id)` |
| `identity` | `users` | `users_role_status_idx` | `CREATE INDEX users_role_status_idx ON identity.users USING btree (role, status) WHERE (deleted_at IS NULL)` |
| `ingest` | `catalog_import_rows` | `catalog_import_rows_excluded` | `CREATE INDEX catalog_import_rows_excluded ON ingest.catalog_import_rows USING btree (import_id, is_excluded)` |
| `ingest` | `catalog_import_rows` | `catalog_import_rows_import` | `CREATE INDEX catalog_import_rows_import ON ingest.catalog_import_rows USING btree (import_id, source_row)` |
| `ingest` | `catalog_import_rows` | `catalog_import_rows_match` | `CREATE INDEX catalog_import_rows_match ON ingest.catalog_import_rows USING btree (import_id, match_level)` |
| `ingest` | `catalog_import_rows` | `catalog_import_rows_org_idx` | `CREATE INDEX catalog_import_rows_org_idx ON ingest.catalog_import_rows USING btree (organization_id)` |
| `ingest` | `catalog_import_rows` | `catalog_import_rows_outcome` | `CREATE INDEX catalog_import_rows_outcome ON ingest.catalog_import_rows USING btree (import_id, outcome)` |
| `ingest` | `catalog_import_rows` | `catalog_import_rows_pkey` | `CREATE UNIQUE INDEX catalog_import_rows_pkey ON ingest.catalog_import_rows USING btree (id)` |
| `ingest` | `catalog_import_rows` | `catalog_import_rows_product_idx` | `CREATE INDEX catalog_import_rows_product_idx ON ingest.catalog_import_rows USING btree (product_id)` |
| `ingest` | `catalog_import_rows` | `catalog_import_rows_variant_idx` | `CREATE INDEX catalog_import_rows_variant_idx ON ingest.catalog_import_rows USING btree (variant_id)` |
| `ingest` | `catalog_imports` | `catalog_imports_created_by_idx` | `CREATE INDEX catalog_imports_created_by_idx ON ingest.catalog_imports USING btree (created_by)` |
| `ingest` | `catalog_imports` | `catalog_imports_expiry` | `CREATE INDEX catalog_imports_expiry ON ingest.catalog_imports USING btree (expires_at) WHERE (phase = ANY (ARRAY['mapping'::text, 'settings'::text, 'confirm'::text]))` |
| `ingest` | `catalog_imports` | `catalog_imports_org_created` | `CREATE INDEX catalog_imports_org_created ON ingest.catalog_imports USING btree (organization_id, created_at DESC)` |
| `ingest` | `catalog_imports` | `catalog_imports_pkey` | `CREATE UNIQUE INDEX catalog_imports_pkey ON ingest.catalog_imports USING btree (id)` |
| `ingest` | `catalog_imports` | `catalog_imports_public_id_key` | `CREATE UNIQUE INDEX catalog_imports_public_id_key ON ingest.catalog_imports USING btree (public_id)` |
| `ingest` | `import_progress` | `idx_ingest_import_progress_organization_id` | `CREATE INDEX idx_ingest_import_progress_organization_id ON ingest.import_progress USING btree (organization_id)` |
| `ingest` | `import_progress` | `import_progress_pkey` | `CREATE UNIQUE INDEX import_progress_pkey ON ingest.import_progress USING btree (id)` |
| `ingest` | `import_progress` | `uq_import_progress_session` | `CREATE UNIQUE INDEX uq_import_progress_session ON ingest.import_progress USING btree (session_id)` |
| `ingest` | `import_rows` | `idx_import_rows_confidence_level` | `CREATE INDEX idx_import_rows_confidence_level ON ingest.import_rows USING btree (session_id, confidence_level)` |
| `ingest` | `import_rows` | `idx_ingest_import_rows_matched_product_id` | `CREATE INDEX idx_ingest_import_rows_matched_product_id ON ingest.import_rows USING btree (matched_product_id)` |
| `ingest` | `import_rows` | `idx_ingest_import_rows_org_created` | `CREATE INDEX idx_ingest_import_rows_org_created ON ingest.import_rows USING btree (organization_id, created_at DESC)` |
| `ingest` | `import_rows` | `import_rows_org_idx` | `CREATE INDEX import_rows_org_idx ON ingest.import_rows USING btree (organization_id)` |
| `ingest` | `import_rows` | `import_rows_pkey` | `CREATE UNIQUE INDEX import_rows_pkey ON ingest.import_rows USING btree (id)` |
| `ingest` | `import_rows` | `import_rows_session_idx` | `CREATE INDEX import_rows_session_idx ON ingest.import_rows USING btree (session_id, row_number)` |
| `ingest` | `import_sessions` | `idx_import_sessions_warehouse_id` | `CREATE INDEX idx_import_sessions_warehouse_id ON ingest.import_sessions USING btree (warehouse_id)` |
| `ingest` | `import_sessions` | `idx_ingest_import_sessions_file_upload_id` | `CREATE INDEX idx_ingest_import_sessions_file_upload_id ON ingest.import_sessions USING btree (file_upload_id)` |
| `ingest` | `import_sessions` | `idx_ingest_import_sessions_org_created` | `CREATE INDEX idx_ingest_import_sessions_org_created ON ingest.import_sessions USING btree (organization_id, created_at DESC)` |
| `ingest` | `import_sessions` | `import_sessions_org_idx` | `CREATE INDEX import_sessions_org_idx ON ingest.import_sessions USING btree (organization_id)` |
| `ingest` | `import_sessions` | `import_sessions_pkey` | `CREATE UNIQUE INDEX import_sessions_pkey ON ingest.import_sessions USING btree (id)` |
| `ingest` | `import_sessions` | `import_sessions_public_id_key` | `CREATE UNIQUE INDEX import_sessions_public_id_key ON ingest.import_sessions USING btree (public_id)` |
| `inventory` | `father_user_temparte_warehouses` | `father_user_temparte_warehouses_pkey` | `CREATE UNIQUE INDEX father_user_temparte_warehouses_pkey ON inventory.father_user_temparte_warehouses USING btree (id)` |
| `inventory` | `father_user_temparte_warehouses` | `idx_father_temp_org` | `CREATE INDEX idx_father_temp_org ON inventory.father_user_temparte_warehouses USING btree (organization_id)` |
| `inventory` | `father_user_temparte_warehouses` | `idx_father_temp_user` | `CREATE INDEX idx_father_temp_user ON inventory.father_user_temparte_warehouses USING btree (user_id)` |
| `inventory` | `father_user_temparte_warehouses` | `idx_inventory_father_user_temparte_warehouses_org_created` | `CREATE INDEX idx_inventory_father_user_temparte_warehouses_org_created ON inventory.father_user_temparte_warehouses USING btree (organization_id, created_at DESC)` |
| `inventory` | `stock_movements` | `idx_inventory_stock_movements_user_id` | `CREATE INDEX idx_inventory_stock_movements_user_id ON inventory.stock_movements USING btree (user_id)` |
| `inventory` | `stock_movements` | `stock_movements_org_idx` | `CREATE INDEX stock_movements_org_idx ON inventory.stock_movements USING btree (organization_id, created_at DESC)` |
| `inventory` | `stock_movements` | `stock_movements_pkey` | `CREATE UNIQUE INDEX stock_movements_pkey ON inventory.stock_movements USING btree (id)` |
| `inventory` | `stock_movements` | `stock_movements_stock_idx` | `CREATE INDEX stock_movements_stock_idx ON inventory.stock_movements USING btree (stock_id, created_at DESC)` |
| `inventory` | `stocks` | `idx_inventory_stocks_org_created` | `CREATE INDEX idx_inventory_stocks_org_created ON inventory.stocks USING btree (organization_id, created_at DESC)` |
| `inventory` | `stocks` | `stocks_org_idx` | `CREATE INDEX stocks_org_idx ON inventory.stocks USING btree (organization_id) WHERE (deleted_at IS NULL)` |
| `inventory` | `stocks` | `stocks_pkey` | `CREATE UNIQUE INDEX stocks_pkey ON inventory.stocks USING btree (id)` |
| `inventory` | `stocks` | `stocks_product_idx` | `CREATE INDEX stocks_product_idx ON inventory.stocks USING btree (product_id)` |
| `inventory` | `stocks` | `stocks_variant_idx` | `CREATE INDEX stocks_variant_idx ON inventory.stocks USING btree (product_variant_id)` |
| `inventory` | `stocks` | `stocks_warehouse_idx` | `CREATE INDEX stocks_warehouse_idx ON inventory.stocks USING btree (warehouse_id)` |
| `inventory` | `stocks` | `stocks_warehouse_variant_unique` | `CREATE UNIQUE INDEX stocks_warehouse_variant_unique ON inventory.stocks USING btree (warehouse_id, product_variant_id)` |
| `inventory` | `temp_warehouses` | `idx_inventory_temp_warehouses_created_by` | `CREATE INDEX idx_inventory_temp_warehouses_created_by ON inventory.temp_warehouses USING btree (created_by)` |
| `inventory` | `temp_warehouses` | `idx_inventory_temp_warehouses_father_id` | `CREATE INDEX idx_inventory_temp_warehouses_father_id ON inventory.temp_warehouses USING btree (father_id)` |
| `inventory` | `temp_warehouses` | `idx_inventory_temp_warehouses_org_created` | `CREATE INDEX idx_inventory_temp_warehouses_org_created ON inventory.temp_warehouses USING btree (organization_id, created_at DESC)` |
| `inventory` | `temp_warehouses` | `idx_inventory_temp_warehouses_organization_id` | `CREATE INDEX idx_inventory_temp_warehouses_organization_id ON inventory.temp_warehouses USING btree (organization_id)` |
| `inventory` | `temp_warehouses` | `temp_warehouses_pkey` | `CREATE UNIQUE INDEX temp_warehouses_pkey ON inventory.temp_warehouses USING btree (id)` |
| `inventory` | `warehouse_transfers` | `idx_inventory_warehouse_transfers_initiated_by` | `CREATE INDEX idx_inventory_warehouse_transfers_initiated_by ON inventory.warehouse_transfers USING btree (initiated_by)` |
| `inventory` | `warehouse_transfers` | `idx_inventory_warehouse_transfers_org_created` | `CREATE INDEX idx_inventory_warehouse_transfers_org_created ON inventory.warehouse_transfers USING btree (organization_id, created_at DESC)` |
| `inventory` | `warehouse_transfers` | `idx_inventory_warehouse_transfers_product_id` | `CREATE INDEX idx_inventory_warehouse_transfers_product_id ON inventory.warehouse_transfers USING btree (product_id)` |
| `inventory` | `warehouse_transfers` | `idx_inventory_warehouse_transfers_product_variant_id` | `CREATE INDEX idx_inventory_warehouse_transfers_product_variant_id ON inventory.warehouse_transfers USING btree (product_variant_id)` |
| `inventory` | `warehouse_transfers` | `transfers_from_idx` | `CREATE INDEX transfers_from_idx ON inventory.warehouse_transfers USING btree (from_warehouse_id)` |
| `inventory` | `warehouse_transfers` | `transfers_org_idx` | `CREATE INDEX transfers_org_idx ON inventory.warehouse_transfers USING btree (organization_id)` |
| `inventory` | `warehouse_transfers` | `transfers_to_idx` | `CREATE INDEX transfers_to_idx ON inventory.warehouse_transfers USING btree (to_warehouse_id)` |
| `inventory` | `warehouse_transfers` | `warehouse_transfers_pkey` | `CREATE UNIQUE INDEX warehouse_transfers_pkey ON inventory.warehouse_transfers USING btree (id)` |
| `inventory` | `warehouses` | `idx_inventory_warehouses_org_created` | `CREATE INDEX idx_inventory_warehouses_org_created ON inventory.warehouses USING btree (organization_id, created_at DESC)` |
| `inventory` | `warehouses` | `warehouses_branch_idx` | `CREATE INDEX warehouses_branch_idx ON inventory.warehouses USING btree (branch_id)` |
| `inventory` | `warehouses` | `warehouses_org_idx` | `CREATE INDEX warehouses_org_idx ON inventory.warehouses USING btree (organization_id) WHERE (deleted_at IS NULL)` |
| `inventory` | `warehouses` | `warehouses_pkey` | `CREATE UNIQUE INDEX warehouses_pkey ON inventory.warehouses USING btree (id)` |
| `inventory` | `warehouses` | `warehouses_public_id_key` | `CREATE UNIQUE INDEX warehouses_public_id_key ON inventory.warehouses USING btree (public_id)` |
| `org` | `branch_institutional_works` | `branch_institutional_works_pkey` | `CREATE UNIQUE INDEX branch_institutional_works_pkey ON org.branch_institutional_works USING btree (id)` |
| `org` | `branch_institutional_works` | `idx_branch_inst_works_work_id` | `CREATE INDEX idx_branch_inst_works_work_id ON org.branch_institutional_works USING btree (institutional_work_id)` |
| `org` | `branch_institutional_works` | `idx_branch_institutional_works_branch` | `CREATE INDEX idx_branch_institutional_works_branch ON org.branch_institutional_works USING btree (branch_id)` |
| `org` | `branch_institutional_works` | `idx_branch_institutional_works_cat` | `CREATE INDEX idx_branch_institutional_works_cat ON org.branch_institutional_works USING btree (work_category)` |
| `org` | `branch_institutional_works` | `uq_branch_institutional_work` | `CREATE UNIQUE INDEX uq_branch_institutional_work ON org.branch_institutional_works USING btree (branch_id, work_category)` |
| `org` | `branches` | `branches_city_idx` | `CREATE INDEX branches_city_idx ON org.branches USING btree (city_id)` |
| `org` | `branches` | `branches_one_main_per_org` | `CREATE UNIQUE INDEX branches_one_main_per_org ON org.branches USING btree (organization_id) WHERE (is_main AND (deleted_at IS NULL))` |
| `org` | `branches` | `branches_org_code_key` | `CREATE UNIQUE INDEX branches_org_code_key ON org.branches USING btree (organization_id, code) WHERE ((code IS NOT NULL) AND (deleted_at IS NULL))` |
| `org` | `branches` | `branches_org_idx` | `CREATE INDEX branches_org_idx ON org.branches USING btree (organization_id) WHERE (deleted_at IS NULL)` |
| `org` | `branches` | `branches_pkey` | `CREATE UNIQUE INDEX branches_pkey ON org.branches USING btree (id)` |
| `org` | `branches` | `branches_public_id_key` | `CREATE UNIQUE INDEX branches_public_id_key ON org.branches USING btree (public_id)` |
| `org` | `branches` | `idx_branches_manager` | `CREATE INDEX idx_branches_manager ON org.branches USING btree (manager_id)` |
| `org` | `branches` | `idx_branches_type` | `CREATE INDEX idx_branches_type ON org.branches USING btree (warehouse_type)` |
| `org` | `branches` | `idx_org_branches_org_created` | `CREATE INDEX idx_org_branches_org_created ON org.branches USING btree (organization_id, created_at DESC)` |
| `org` | `delivery_bands` | `delivery_bands_org_idx` | `CREATE INDEX delivery_bands_org_idx ON org.delivery_bands USING btree (organization_id) WHERE (is_active = true)` |
| `org` | `delivery_bands` | `delivery_bands_pkey` | `CREATE UNIQUE INDEX delivery_bands_pkey ON org.delivery_bands USING btree (id)` |
| `org` | `delivery_bands` | `idx_org_delivery_bands_org_created` | `CREATE INDEX idx_org_delivery_bands_org_created ON org.delivery_bands USING btree (organization_id, created_at DESC)` |
| `org` | `employee_institutional_works` | `employee_institutional_works_pkey` | `CREATE UNIQUE INDEX employee_institutional_works_pkey ON org.employee_institutional_works USING btree (id)` |
| `org` | `employee_institutional_works` | `idx_emp_inst_works_org` | `CREATE INDEX idx_emp_inst_works_org ON org.employee_institutional_works USING btree (organization_id) WHERE (deleted_at IS NULL)` |
| `org` | `employee_institutional_works` | `idx_emp_inst_works_unique` | `CREATE UNIQUE INDEX idx_emp_inst_works_unique ON org.employee_institutional_works USING btree (user_id, institutional_work_id) WHERE (deleted_at IS NULL)` |
| `org` | `employee_institutional_works` | `idx_emp_inst_works_user` | `CREATE INDEX idx_emp_inst_works_user ON org.employee_institutional_works USING btree (user_id) WHERE (deleted_at IS NULL)` |
| `org` | `employee_institutional_works` | `idx_emp_inst_works_work` | `CREATE INDEX idx_emp_inst_works_work ON org.employee_institutional_works USING btree (institutional_work_id) WHERE (deleted_at IS NULL)` |
| `org` | `employee_institutional_works` | `idx_org_employee_institutional_works_org_created` | `CREATE INDEX idx_org_employee_institutional_works_org_created ON org.employee_institutional_works USING btree (organization_id, created_at DESC)` |
| `org` | `institutional_work_connections` | `idx_inst_work_conn_from` | `CREATE INDEX idx_inst_work_conn_from ON org.institutional_work_connections USING btree (from_institutional_work_id)` |
| `org` | `institutional_work_connections` | `idx_inst_work_conn_to` | `CREATE INDEX idx_inst_work_conn_to ON org.institutional_work_connections USING btree (to_institutional_work_id)` |
| `org` | `institutional_work_connections` | `institutional_work_connections_pkey` | `CREATE UNIQUE INDEX institutional_work_connections_pkey ON org.institutional_work_connections USING btree (id)` |
| `org` | `institutional_work_connections` | `uq_inst_work_conn` | `CREATE UNIQUE INDEX uq_inst_work_conn ON org.institutional_work_connections USING btree (from_institutional_work_id, to_institutional_work_id)` |
| `org` | `institutional_works` | `idx_institutional_works_active` | `CREATE INDEX idx_institutional_works_active ON org.institutional_works USING btree (is_active) WHERE (deleted_at IS NULL)` |
| `org` | `institutional_works` | `idx_institutional_works_parent` | `CREATE INDEX idx_institutional_works_parent ON org.institutional_works USING btree (parent_id) WHERE (deleted_at IS NULL)` |
| `org` | `institutional_works` | `institutional_works_pkey` | `CREATE UNIQUE INDEX institutional_works_pkey ON org.institutional_works USING btree (id)` |
| `org` | `members` | `idx_org_members_invited_by` | `CREATE INDEX idx_org_members_invited_by ON org.members USING btree (invited_by)` |
| `org` | `members` | `idx_org_members_org_created` | `CREATE INDEX idx_org_members_org_created ON org.members USING btree (organization_id, created_at DESC)` |
| `org` | `members` | `idx_org_members_org_role` | `CREATE INDEX idx_org_members_org_role ON org.members USING btree (org_role_id) WHERE (org_role_id IS NOT NULL)` |
| `org` | `members` | `idx_org_members_role` | `CREATE INDEX idx_org_members_role ON org.members USING btree (org_role_id)` |
| `org` | `members` | `idx_org_members_role_key` | `CREATE INDEX idx_org_members_role_key ON org.members USING btree (role_key)` |
| `org` | `members` | `idx_org_members_user_org` | `CREATE INDEX idx_org_members_user_org ON org.members USING btree (user_id, organization_id) WHERE (status = 'active'::text)` |
| `org` | `members` | `members_active_idx` | `CREATE INDEX members_active_idx ON org.members USING btree (organization_id, is_active)` |
| `org` | `members` | `members_branch_idx` | `CREATE INDEX members_branch_idx ON org.members USING btree (branch_id)` |
| `org` | `members` | `members_employee_code_org_unique` | `CREATE UNIQUE INDEX members_employee_code_org_unique ON org.members USING btree (organization_id, employee_code) WHERE (employee_code <> ''::text)` |
| `org` | `members` | `members_org_idx` | `CREATE INDEX members_org_idx ON org.members USING btree (organization_id, status)` |
| `org` | `members` | `members_organization_id_user_id_key` | `CREATE UNIQUE INDEX members_organization_id_user_id_key ON org.members USING btree (organization_id, user_id)` |
| `org` | `members` | `members_pkey` | `CREATE UNIQUE INDEX members_pkey ON org.members USING btree (id)` |
| `org` | `members` | `members_user_idx` | `CREATE INDEX members_user_idx ON org.members USING btree (user_id, status)` |
| `org` | `organization_followers` | `idx_org_organization_followers_org_created` | `CREATE INDEX idx_org_organization_followers_org_created ON org.organization_followers USING btree (organization_id, created_at DESC)` |
| `org` | `organization_followers` | `idx_org_organization_followers_user_id` | `CREATE INDEX idx_org_organization_followers_user_id ON org.organization_followers USING btree (user_id)` |
| `org` | `organization_followers` | `org_followers_org_idx` | `CREATE INDEX org_followers_org_idx ON org.organization_followers USING btree (organization_id)` |
| `org` | `organization_followers` | `org_followers_unique` | `CREATE UNIQUE INDEX org_followers_unique ON org.organization_followers USING btree (organization_id, user_id)` |
| `org` | `organization_followers` | `organization_followers_pkey` | `CREATE UNIQUE INDEX organization_followers_pkey ON org.organization_followers USING btree (id)` |
| `org` | `organization_policies` | `idx_org_organization_policies_org_created` | `CREATE INDEX idx_org_organization_policies_org_created ON org.organization_policies USING btree (organization_id, created_at DESC)` |
| `org` | `organization_policies` | `org_policies_org_idx` | `CREATE INDEX org_policies_org_idx ON org.organization_policies USING btree (organization_id)` |
| `org` | `organization_policies` | `organization_policies_pkey` | `CREATE UNIQUE INDEX organization_policies_pkey ON org.organization_policies USING btree (id)` |
| `org` | `organization_reviews` | `idx_org_organization_reviews_order_id` | `CREATE INDEX idx_org_organization_reviews_order_id ON org.organization_reviews USING btree (order_id)` |
| `org` | `organization_reviews` | `idx_org_organization_reviews_org_created` | `CREATE INDEX idx_org_organization_reviews_org_created ON org.organization_reviews USING btree (organization_id, created_at DESC)` |
| `org` | `organization_reviews` | `idx_org_organization_reviews_product_id` | `CREATE INDEX idx_org_organization_reviews_product_id ON org.organization_reviews USING btree (product_id)` |
| `org` | `organization_reviews` | `idx_org_organization_reviews_responded_by` | `CREATE INDEX idx_org_organization_reviews_responded_by ON org.organization_reviews USING btree (responded_by)` |
| `org` | `organization_reviews` | `org_reviews_org_idx` | `CREATE INDEX org_reviews_org_idx ON org.organization_reviews USING btree (organization_id)` |
| `org` | `organization_reviews` | `org_reviews_user_unique` | `CREATE UNIQUE INDEX org_reviews_user_unique ON org.organization_reviews USING btree (organization_id, user_id)` |
| `org` | `organization_reviews` | `organization_reviews_pkey` | `CREATE UNIQUE INDEX organization_reviews_pkey ON org.organization_reviews USING btree (id)` |
| `org` | `organization_reviews` | `reviews_one_per_order` | `CREATE UNIQUE INDEX reviews_one_per_order ON org.organization_reviews USING btree (user_id, order_id) WHERE ((order_id IS NOT NULL) AND (deleted_at IS NULL))` |
| `org` | `organization_social_media` | `idx_org_organization_social_media_org_created` | `CREATE INDEX idx_org_organization_social_media_org_created ON org.organization_social_media USING btree (organization_id, created_at DESC)` |
| `org` | `organization_social_media` | `org_social_media_org_idx` | `CREATE INDEX org_social_media_org_idx ON org.organization_social_media USING btree (organization_id)` |
| `org` | `organization_social_media` | `organization_social_media_pkey` | `CREATE UNIQUE INDEX organization_social_media_pkey ON org.organization_social_media USING btree (id)` |
| `org` | `organizations` | `idx_org_organizations_approved_by` | `CREATE INDEX idx_org_organizations_approved_by ON org.organizations USING btree (approved_by)` |
| `org` | `organizations` | `organizations_commercial_register_key` | `CREATE UNIQUE INDEX organizations_commercial_register_key ON org.organizations USING btree (commercial_register) WHERE ((commercial_register <> ''::text) AND (deleted_at IS NULL))` |
| `org` | `organizations` | `organizations_name_trgm_idx` | `CREATE INDEX organizations_name_trgm_idx ON org.organizations USING gin (platform.normalize_arabic((name ->> 'ar'::text)) gin_trgm_ops)` |
| `org` | `organizations` | `organizations_number_key` | `CREATE UNIQUE INDEX organizations_number_key ON org.organizations USING btree (organization_number) WHERE ((organization_number IS NOT NULL) AND (deleted_at IS NULL))` |
| `org` | `organizations` | `organizations_owner_idx` | `CREATE INDEX organizations_owner_idx ON org.organizations USING btree (owner_id)` |
| `org` | `organizations` | `organizations_pkey` | `CREATE UNIQUE INDEX organizations_pkey ON org.organizations USING btree (id)` |
| `org` | `organizations` | `organizations_public_id_key` | `CREATE UNIQUE INDEX organizations_public_id_key ON org.organizations USING btree (public_id)` |
| `org` | `organizations` | `organizations_status_idx` | `CREATE INDEX organizations_status_idx ON org.organizations USING btree (status) WHERE (deleted_at IS NULL)` |
| `org` | `organizations` | `organizations_type_idx` | `CREATE INDEX organizations_type_idx ON org.organizations USING btree (type) WHERE (deleted_at IS NULL)` |
| `org` | `review_criteria` | `review_criteria_pkey` | `CREATE UNIQUE INDEX review_criteria_pkey ON org.review_criteria USING btree (key)` |
| `org` | `review_ratings` | `idx_org_review_ratings_criterion` | `CREATE INDEX idx_org_review_ratings_criterion ON org.review_ratings USING btree (criterion)` |
| `org` | `review_ratings` | `review_ratings_pkey` | `CREATE UNIQUE INDEX review_ratings_pkey ON org.review_ratings USING btree (review_id, criterion)` |
| `org` | `role_permissions` | `org_role_permissions_permission_idx` | `CREATE INDEX org_role_permissions_permission_idx ON org.role_permissions USING btree (permission_key)` |
| `org` | `role_permissions` | `role_permissions_pkey` | `CREATE UNIQUE INDEX role_permissions_pkey ON org.role_permissions USING btree (role_id, permission_key)` |
| `org` | `roles` | `idx_org_roles_org` | `CREATE INDEX idx_org_roles_org ON org.roles USING btree (organization_id)` |
| `org` | `roles` | `idx_org_roles_org_created` | `CREATE INDEX idx_org_roles_org_created ON org.roles USING btree (organization_id, created_at DESC)` |
| `org` | `roles` | `org_roles_created_by_idx` | `CREATE INDEX org_roles_created_by_idx ON org.roles USING btree (created_by)` |
| `org` | `roles` | `roles_pkey` | `CREATE UNIQUE INDEX roles_pkey ON org.roles USING btree (id)` |
| `org` | `roles` | `uq_org_roles_key` | `CREATE UNIQUE INDEX uq_org_roles_key ON org.roles USING btree (organization_id, key)` |
| `org` | `user_organizations` | `idx_user_orgs_cust` | `CREATE INDEX idx_user_orgs_cust ON org.user_organizations USING btree (customer_org_id) WHERE (deleted_at IS NULL)` |
| `org` | `user_organizations` | `idx_user_orgs_user` | `CREATE INDEX idx_user_orgs_user ON org.user_organizations USING btree (user_id) WHERE (deleted_at IS NULL)` |
| `org` | `user_organizations` | `idx_user_orgs_vendor` | `CREATE INDEX idx_user_orgs_vendor ON org.user_organizations USING btree (vendor_org_id) WHERE (deleted_at IS NULL)` |
| `org` | `user_organizations` | `uq_user_vendor_org` | `CREATE UNIQUE INDEX uq_user_vendor_org ON org.user_organizations USING btree (user_id, vendor_org_id) WHERE (deleted_at IS NULL)` |
| `org` | `user_organizations` | `user_organizations_pkey` | `CREATE UNIQUE INDEX user_organizations_pkey ON org.user_organizations USING btree (id)` |
| `platform` | `audit_log` | `audit_log_actor_idx` | `CREATE INDEX audit_log_actor_idx ON platform.audit_log USING btree (actor_user_id, created_at DESC)` |
| `platform` | `audit_log` | `audit_log_entity_idx` | `CREATE INDEX audit_log_entity_idx ON platform.audit_log USING btree (entity_type, entity_id, created_at DESC)` |
| `platform` | `audit_log` | `audit_log_org_idx` | `CREATE INDEX audit_log_org_idx ON platform.audit_log USING btree (organization_id, created_at DESC)` |
| `platform` | `audit_log` | `audit_log_pkey` | `CREATE UNIQUE INDEX audit_log_pkey ON platform.audit_log USING btree (id)` |
| `platform` | `translations` | `idx_translations_custom` | `CREATE INDEX idx_translations_custom ON platform.translations USING btree (is_custom)` |
| `platform` | `translations` | `idx_translations_namespace` | `CREATE INDEX idx_translations_namespace ON platform.translations USING btree (namespace)` |
| `platform` | `translations` | `translations_key_key` | `CREATE UNIQUE INDEX translations_key_key ON platform.translations USING btree (key)` |
| `platform` | `translations` | `translations_pkey` | `CREATE UNIQUE INDEX translations_pkey ON platform.translations USING btree (id)` |
| `promo` | `ad_clicks` | `ad_clicks_ad_idx` | `CREATE INDEX ad_clicks_ad_idx ON promo.ad_clicks USING btree (ad_id, created_at DESC)` |
| `promo` | `ad_clicks` | `ad_clicks_pkey` | `CREATE UNIQUE INDEX ad_clicks_pkey ON promo.ad_clicks USING btree (id)` |
| `promo` | `ad_clicks` | `idx_promo_ad_clicks_user_id` | `CREATE INDEX idx_promo_ad_clicks_user_id ON promo.ad_clicks USING btree (user_id)` |
| `promo` | `ad_impressions` | `ad_impressions_pkey` | `CREATE UNIQUE INDEX ad_impressions_pkey ON promo.ad_impressions USING btree (id)` |
| `promo` | `ad_impressions` | `ad_impressions_user_idx` | `CREATE INDEX ad_impressions_user_idx ON promo.ad_impressions USING btree (user_id)` |
| `promo` | `ad_impressions` | `idx_ad_impressions_ad` | `CREATE INDEX idx_ad_impressions_ad ON promo.ad_impressions USING btree (ad_id, created_at DESC)` |
| `promo` | `ad_plans` | `ad_plans_pkey` | `CREATE UNIQUE INDEX ad_plans_pkey ON promo.ad_plans USING btree (id)` |
| `promo` | `ads` | `ads_ad_plan_idx` | `CREATE INDEX ads_ad_plan_idx ON promo.ads USING btree (ad_plan_id)` |
| `promo` | `ads` | `ads_pkey` | `CREATE UNIQUE INDEX ads_pkey ON promo.ads USING btree (id)` |
| `promo` | `ads` | `ads_reviewed_by_idx` | `CREATE INDEX ads_reviewed_by_idx ON promo.ads USING btree (reviewed_by)` |
| `promo` | `ads` | `idx_ads_admin_status` | `CREATE INDEX idx_ads_admin_status ON promo.ads USING btree (admin_status, is_active, expires_at)` |
| `promo` | `ads` | `idx_ads_position_active` | `CREATE INDEX idx_ads_position_active ON promo.ads USING btree ("position") WHERE ((is_active = true) AND (admin_status = 'approved'::text))` |
| `promo` | `ads` | `idx_promo_ads_org_created` | `CREATE INDEX idx_promo_ads_org_created ON promo.ads USING btree (organization_id, created_at DESC)` |
| `promo` | `ads` | `idx_promo_ads_organization_id` | `CREATE INDEX idx_promo_ads_organization_id ON promo.ads USING btree (organization_id)` |
| `promo` | `highlight_section_items` | `highlight_section_items_pkey` | `CREATE UNIQUE INDEX highlight_section_items_pkey ON promo.highlight_section_items USING btree (id)` |
| `promo` | `highlight_section_items` | `highlight_section_items_section_idx` | `CREATE INDEX highlight_section_items_section_idx ON promo.highlight_section_items USING btree (section_id, display_order)` |
| `promo` | `highlight_section_items` | `idx_promo_highlight_section_items_offer_id` | `CREATE INDEX idx_promo_highlight_section_items_offer_id ON promo.highlight_section_items USING btree (offer_id)` |
| `promo` | `highlight_section_items` | `idx_promo_highlight_section_items_product_id` | `CREATE INDEX idx_promo_highlight_section_items_product_id ON promo.highlight_section_items USING btree (product_id)` |
| `promo` | `highlight_sections` | `highlight_sections_pkey` | `CREATE UNIQUE INDEX highlight_sections_pkey ON promo.highlight_sections USING btree (id)` |
| `promo` | `highlight_sections` | `highlight_sections_platform_slug_key` | `CREATE UNIQUE INDEX highlight_sections_platform_slug_key ON promo.highlight_sections USING btree (slug) WHERE (owner_type = 'platform'::text)` |
| `promo` | `highlight_sections` | `idx_promo_highlight_sections_org_created` | `CREATE INDEX idx_promo_highlight_sections_org_created ON promo.highlight_sections USING btree (organization_id, created_at DESC)` |
| `promo` | `highlight_sections` | `idx_promo_highlight_sections_organization_id` | `CREATE INDEX idx_promo_highlight_sections_organization_id ON promo.highlight_sections USING btree (organization_id)` |
| `promo` | `offer_clicks` | `idx_offer_clicks_created_at` | `CREATE INDEX idx_offer_clicks_created_at ON promo.offer_clicks USING btree (created_at)` |
| `promo` | `offer_clicks` | `idx_promo_offer_clicks_org_created` | `CREATE INDEX idx_promo_offer_clicks_org_created ON promo.offer_clicks USING btree (organization_id, created_at DESC)` |
| `promo` | `offer_clicks` | `idx_promo_offer_clicks_organization_id` | `CREATE INDEX idx_promo_offer_clicks_organization_id ON promo.offer_clicks USING btree (organization_id)` |
| `promo` | `offer_clicks` | `idx_promo_offer_clicks_user_id` | `CREATE INDEX idx_promo_offer_clicks_user_id ON promo.offer_clicks USING btree (user_id)` |
| `promo` | `offer_clicks` | `offer_clicks_offer_idx` | `CREATE INDEX offer_clicks_offer_idx ON promo.offer_clicks USING btree (offer_id, created_at DESC)` |
| `promo` | `offer_clicks` | `offer_clicks_pkey` | `CREATE UNIQUE INDEX offer_clicks_pkey ON promo.offer_clicks USING btree (id)` |
| `promo` | `offer_location_covers` | `idx_promo_offer_location_covers_city_id` | `CREATE INDEX idx_promo_offer_location_covers_city_id ON promo.offer_location_covers USING btree (city_id)` |
| `promo` | `offer_location_covers` | `idx_promo_offer_location_covers_org_created` | `CREATE INDEX idx_promo_offer_location_covers_org_created ON promo.offer_location_covers USING btree (organization_id, created_at DESC)` |
| `promo` | `offer_location_covers` | `idx_promo_offer_location_covers_organization_id` | `CREATE INDEX idx_promo_offer_location_covers_organization_id ON promo.offer_location_covers USING btree (organization_id)` |
| `promo` | `offer_location_covers` | `offer_location_covers_day_idx` | `CREATE INDEX offer_location_covers_day_idx ON promo.offer_location_covers USING btree (offer_id, day_of_week)` |
| `promo` | `offer_location_covers` | `offer_location_covers_pkey` | `CREATE UNIQUE INDEX offer_location_covers_pkey ON promo.offer_location_covers USING btree (id)` |
| `promo` | `offer_location_covers` | `uq_offer_location_city` | `CREATE UNIQUE INDEX uq_offer_location_city ON promo.offer_location_covers USING btree (offer_id, city_id)` |
| `promo` | `offer_packages` | `idx_offer_packages_tier` | `CREATE INDEX idx_offer_packages_tier ON promo.offer_packages USING btree (tier_level DESC, sort_order) WHERE (is_active = true)` |
| `promo` | `offer_packages` | `offer_packages_pkey` | `CREATE UNIQUE INDEX offer_packages_pkey ON promo.offer_packages USING btree (id)` |
| `promo` | `offer_products` | `idx_promo_offer_products_product_id` | `CREATE INDEX idx_promo_offer_products_product_id ON promo.offer_products USING btree (product_id)` |
| `promo` | `offer_products` | `idx_promo_offer_products_product_variant_id` | `CREATE INDEX idx_promo_offer_products_product_variant_id ON promo.offer_products USING btree (product_variant_id)` |
| `promo` | `offer_products` | `idx_promo_offer_products_variant_id` | `CREATE INDEX idx_promo_offer_products_variant_id ON promo.offer_products USING btree (variant_id)` |
| `promo` | `offer_products` | `offer_products_offer_idx` | `CREATE INDEX offer_products_offer_idx ON promo.offer_products USING btree (offer_id)` |
| `promo` | `offer_products` | `offer_products_pkey` | `CREATE UNIQUE INDEX offer_products_pkey ON promo.offer_products USING btree (id)` |
| `promo` | `offer_sponsorships` | `idx_offer_sponsorships_active` | `CREATE INDEX idx_offer_sponsorships_active ON promo.offer_sponsorships USING btree (item_type, item_id) WHERE ((status = 'active'::text) AND (admin_status = 'approved'::text))` |
| `promo` | `offer_sponsorships` | `idx_promo_offer_sponsorships_offer_id` | `CREATE INDEX idx_promo_offer_sponsorships_offer_id ON promo.offer_sponsorships USING btree (offer_id)` |
| `promo` | `offer_sponsorships` | `idx_promo_offer_sponsorships_org_created` | `CREATE INDEX idx_promo_offer_sponsorships_org_created ON promo.offer_sponsorships USING btree (organization_id, created_at DESC)` |
| `promo` | `offer_sponsorships` | `idx_promo_offer_sponsorships_organization_id` | `CREATE INDEX idx_promo_offer_sponsorships_organization_id ON promo.offer_sponsorships USING btree (organization_id)` |
| `promo` | `offer_sponsorships` | `idx_promo_offer_sponsorships_package_id` | `CREATE INDEX idx_promo_offer_sponsorships_package_id ON promo.offer_sponsorships USING btree (package_id)` |
| `promo` | `offer_sponsorships` | `offer_sponsorships_pkey` | `CREATE UNIQUE INDEX offer_sponsorships_pkey ON promo.offer_sponsorships USING btree (id)` |
| `promo` | `offer_sponsorships` | `offer_sponsorships_request_idx` | `CREATE INDEX offer_sponsorships_request_idx ON promo.offer_sponsorships USING btree (sponsorship_request_id)` |
| `promo` | `offer_sponsorships` | `offer_sponsorships_reviewed_by_idx` | `CREATE INDEX offer_sponsorships_reviewed_by_idx ON promo.offer_sponsorships USING btree (reviewed_by)` |
| `promo` | `offers` | `idx_promo_offers_approved_by` | `CREATE INDEX idx_promo_offers_approved_by ON promo.offers USING btree (approved_by)` |
| `promo` | `offers` | `idx_promo_offers_org_created` | `CREATE INDEX idx_promo_offers_org_created ON promo.offers USING btree (organization_id, created_at DESC)` |
| `promo` | `offers` | `idx_promo_offers_rejected_by` | `CREATE INDEX idx_promo_offers_rejected_by ON promo.offers USING btree (rejected_by)` |
| `promo` | `offers` | `offers_active_idx` | `CREATE INDEX offers_active_idx ON promo.offers USING btree (is_active, starts_at, expires_at) WHERE (deleted_at IS NULL)` |
| `promo` | `offers` | `offers_admin_status_idx` | `CREATE INDEX offers_admin_status_idx ON promo.offers USING btree (admin_status, is_active) WHERE (deleted_at IS NULL)` |
| `promo` | `offers` | `offers_branch_idx` | `CREATE INDEX offers_branch_idx ON promo.offers USING btree (branch_id) WHERE (deleted_at IS NULL)` |
| `promo` | `offers` | `offers_org_idx` | `CREATE INDEX offers_org_idx ON promo.offers USING btree (organization_id) WHERE (deleted_at IS NULL)` |
| `promo` | `offers` | `offers_pkey` | `CREATE UNIQUE INDEX offers_pkey ON promo.offers USING btree (id)` |
| `promo` | `offers` | `offers_public_id_key` | `CREATE UNIQUE INDEX offers_public_id_key ON promo.offers USING btree (public_id)` |
| `promo` | `sponsorship_purchases` | `idx_sponsorship_purchases_expires` | `CREATE INDEX idx_sponsorship_purchases_expires ON promo.sponsorship_purchases USING btree (expires_at) WHERE (status = 'active'::text)` |
| `promo` | `sponsorship_purchases` | `idx_sponsorship_purchases_org` | `CREATE INDEX idx_sponsorship_purchases_org ON promo.sponsorship_purchases USING btree (organization_id) WHERE (status = 'active'::text)` |
| `promo` | `sponsorship_purchases` | `sponsorship_purchases_approved_idx` | `CREATE INDEX sponsorship_purchases_approved_idx ON promo.sponsorship_purchases USING btree (approved_by)` |
| `promo` | `sponsorship_purchases` | `sponsorship_purchases_package_idx` | `CREATE INDEX sponsorship_purchases_package_idx ON promo.sponsorship_purchases USING btree (package_id)` |
| `promo` | `sponsorship_purchases` | `sponsorship_purchases_payment_idx` | `CREATE INDEX sponsorship_purchases_payment_idx ON promo.sponsorship_purchases USING btree (payment_id)` |
| `promo` | `sponsorship_purchases` | `sponsorship_purchases_pkey` | `CREATE UNIQUE INDEX sponsorship_purchases_pkey ON promo.sponsorship_purchases USING btree (id)` |
| `promo` | `sponsorship_purchases` | `sponsorship_purchases_public_id_key` | `CREATE UNIQUE INDEX sponsorship_purchases_public_id_key ON promo.sponsorship_purchases USING btree (public_id)` |
| `promo` | `sponsorship_requests` | `idx_sponsorship_requests_active` | `CREATE INDEX idx_sponsorship_requests_active ON promo.sponsorship_requests USING btree (item_type, item_id) WHERE ((status = 'active'::text) AND (admin_status = 'approved'::text))` |
| `promo` | `sponsorship_requests` | `idx_sponsorship_requests_admin` | `CREATE INDEX idx_sponsorship_requests_admin ON promo.sponsorship_requests USING btree (admin_status, status)` |
| `promo` | `sponsorship_requests` | `idx_sponsorship_requests_org` | `CREATE INDEX idx_sponsorship_requests_org ON promo.sponsorship_requests USING btree (organization_id)` |
| `promo` | `sponsorship_requests` | `sponsorship_requests_package_idx` | `CREATE INDEX sponsorship_requests_package_idx ON promo.sponsorship_requests USING btree (package_id)` |
| `promo` | `sponsorship_requests` | `sponsorship_requests_pkey` | `CREATE UNIQUE INDEX sponsorship_requests_pkey ON promo.sponsorship_requests USING btree (id)` |
| `promo` | `sponsorship_requests` | `sponsorship_requests_public_id_key` | `CREATE UNIQUE INDEX sponsorship_requests_public_id_key ON promo.sponsorship_requests USING btree (public_id)` |
| `promo` | `sponsorship_requests` | `sponsorship_requests_purchase_idx` | `CREATE INDEX sponsorship_requests_purchase_idx ON promo.sponsorship_requests USING btree (purchase_id)` |
| `promo` | `sponsorship_requests` | `sponsorship_requests_reviewed_idx` | `CREATE INDEX sponsorship_requests_reviewed_idx ON promo.sponsorship_requests USING btree (reviewed_by)` |
| `workflow` | `purchase_priority_engines` | `idx_workflow_purchase_priority_engines_org_created` | `CREATE INDEX idx_workflow_purchase_priority_engines_org_created ON workflow.purchase_priority_engines USING btree (organization_id, created_at DESC)` |
| `workflow` | `purchase_priority_engines` | `idx_workflow_purchase_priority_engines_organization_id` | `CREATE INDEX idx_workflow_purchase_priority_engines_organization_id ON workflow.purchase_priority_engines USING btree (organization_id)` |
| `workflow` | `purchase_priority_engines` | `idx_workflow_purchase_priority_engines_processed_by` | `CREATE INDEX idx_workflow_purchase_priority_engines_processed_by ON workflow.purchase_priority_engines USING btree (processed_by)` |
| `workflow` | `purchase_priority_engines` | `purchase_priority_engines_pkey` | `CREATE UNIQUE INDEX purchase_priority_engines_pkey ON workflow.purchase_priority_engines USING btree (id)` |
| `workflow` | `purchase_priority_engines` | `purchase_priority_engines_request_number_key` | `CREATE UNIQUE INDEX purchase_priority_engines_request_number_key ON workflow.purchase_priority_engines USING btree (request_number)` |
| `workflow` | `purchase_priority_engines` | `purchase_priority_public_id_key` | `CREATE UNIQUE INDEX purchase_priority_public_id_key ON workflow.purchase_priority_engines USING btree (public_id)` |
| `workflow` | `purchase_priority_engines` | `purchase_priority_user_idx` | `CREATE INDEX purchase_priority_user_idx ON workflow.purchase_priority_engines USING btree (user_id)` |
| `workflow` | `report_issues` | `idx_workflow_report_issues_order_id` | `CREATE INDEX idx_workflow_report_issues_order_id ON workflow.report_issues USING btree (order_id)` |
| `workflow` | `report_issues` | `idx_workflow_report_issues_org_created` | `CREATE INDEX idx_workflow_report_issues_org_created ON workflow.report_issues USING btree (organization_id, created_at DESC)` |
| `workflow` | `report_issues` | `idx_workflow_report_issues_organization_id` | `CREATE INDEX idx_workflow_report_issues_organization_id ON workflow.report_issues USING btree (organization_id)` |
| `workflow` | `report_issues` | `report_issues_pkey` | `CREATE UNIQUE INDEX report_issues_pkey ON workflow.report_issues USING btree (id)` |
| `workflow` | `report_issues` | `report_issues_user_idx` | `CREATE INDEX report_issues_user_idx ON workflow.report_issues USING btree (reported_by)` |
| `workflow` | `requests` | `idx_workflow_requests_from_user_id` | `CREATE INDEX idx_workflow_requests_from_user_id ON workflow.requests USING btree (from_user_id)` |
| `workflow` | `requests` | `requests_from_org_idx` | `CREATE INDEX requests_from_org_idx ON workflow.requests USING btree (from_org_id)` |
| `workflow` | `requests` | `requests_pkey` | `CREATE UNIQUE INDEX requests_pkey ON workflow.requests USING btree (id)` |
| `workflow` | `requests` | `requests_public_id_key` | `CREATE UNIQUE INDEX requests_public_id_key ON workflow.requests USING btree (public_id)` |
| `workflow` | `requests` | `requests_to_org_idx` | `CREATE INDEX requests_to_org_idx ON workflow.requests USING btree (to_org_id, status)` |
| `workflow` | `weekly_coverages` | `idx_weekly_coverages_branch_day` | `CREATE INDEX idx_weekly_coverages_branch_day ON workflow.weekly_coverages USING btree (branch_id, day_of_week)` |
| `workflow` | `weekly_coverages` | `idx_weekly_coverages_gov_city` | `CREATE INDEX idx_weekly_coverages_gov_city ON workflow.weekly_coverages USING btree (governorate_id, city_id)` |
| `workflow` | `weekly_coverages` | `idx_weekly_coverages_org_day_active` | `CREATE INDEX idx_weekly_coverages_org_day_active ON workflow.weekly_coverages USING btree (organization_id, day_of_week, is_active)` |
| `workflow` | `weekly_coverages` | `idx_workflow_weekly_coverages_branch_id` | `CREATE INDEX idx_workflow_weekly_coverages_branch_id ON workflow.weekly_coverages USING btree (branch_id)` |
| `workflow` | `weekly_coverages` | `idx_workflow_weekly_coverages_city_id` | `CREATE INDEX idx_workflow_weekly_coverages_city_id ON workflow.weekly_coverages USING btree (city_id)` |
| `workflow` | `weekly_coverages` | `idx_workflow_weekly_coverages_org_created` | `CREATE INDEX idx_workflow_weekly_coverages_org_created ON workflow.weekly_coverages USING btree (organization_id, created_at DESC)` |
| `workflow` | `weekly_coverages` | `idx_workflow_weekly_coverages_organization_id` | `CREATE INDEX idx_workflow_weekly_coverages_organization_id ON workflow.weekly_coverages USING btree (organization_id)` |
| `workflow` | `weekly_coverages` | `weekly_coverages_pkey` | `CREATE UNIQUE INDEX weekly_coverages_pkey ON workflow.weekly_coverages USING btree (id)` |

## A3. EXPLAIN (ANALYZE, BUFFERS) Profiling for Default Sorts

### `catalog.products` (Seq Scan)

```sql
EXPLAIN (ANALYZE, BUFFERS) SELECT * FROM catalog.products WHERE deleted_at IS NULL ORDER BY created_at DESC, id DESC LIMIT 25 OFFSET 0;
```

```
Limit  (cost=1577.23..1577.30 rows=25 width=429) (actual time=10.755..10.759 rows=25.00 loops=1)
  Buffers: shared hit=819
  ->  Sort  (cost=1577.23..1627.22 rows=19996 width=429) (actual time=10.754..10.756 rows=25.00 loops=1)
        Sort Key: created_at DESC, id DESC
        Sort Method: top-N heapsort  Memory: 50kB
        Buffers: shared hit=819
        ->  Seq Scan on products  (cost=0.00..1012.96 rows=19996 width=429) (actual time=0.011..5.709 rows=19996.00 loops=1)
              Filter: (deleted_at IS NULL)
              Buffers: shared hit=813
Planning:
  Buffers: shared hit=413 dirtied=2
Planning Time: 1.126 ms
Execution Time: 10.798 ms
```

### `commerce.orders` (Seq Scan)

```sql
EXPLAIN (ANALYZE, BUFFERS) SELECT * FROM commerce.orders WHERE 1=1 ORDER BY created_at DESC, id DESC LIMIT 25 OFFSET 0;
```

```
Limit  (cost=1.08..1.09 rows=4 width=232) (actual time=0.018..0.019 rows=4.00 loops=1)
  Buffers: shared hit=1
  ->  Sort  (cost=1.08..1.09 rows=4 width=232) (actual time=0.017..0.018 rows=4.00 loops=1)
        Sort Key: created_at DESC, id DESC
        Sort Method: quicksort  Memory: 26kB
        Buffers: shared hit=1
        ->  Seq Scan on orders  (cost=0.00..1.04 rows=4 width=232) (actual time=0.009..0.009 rows=4.00 loops=1)
              Buffers: shared hit=1
Planning:
  Buffers: shared hit=200
Planning Time: 0.536 ms
Execution Time: 0.044 ms
```

### `promo.offers` (Seq Scan)

```sql
EXPLAIN (ANALYZE, BUFFERS) SELECT * FROM promo.offers WHERE 1=1 ORDER BY created_at DESC, id DESC LIMIT 25 OFFSET 0;
```

```
Limit  (cost=1.02..1.02 rows=1 width=586) (actual time=0.019..0.020 rows=1.00 loops=1)
  Buffers: shared hit=1
  ->  Sort  (cost=1.02..1.02 rows=1 width=586) (actual time=0.018..0.019 rows=1.00 loops=1)
        Sort Key: created_at DESC, id DESC
        Sort Method: quicksort  Memory: 26kB
        Buffers: shared hit=1
        ->  Seq Scan on offers  (cost=0.00..1.01 rows=1 width=586) (actual time=0.011..0.011 rows=1.00 loops=1)
              Buffers: shared hit=1
Planning:
  Buffers: shared hit=157
Planning Time: 0.423 ms
Execution Time: 0.039 ms
```

### `identity.users` (Seq Scan)

```sql
EXPLAIN (ANALYZE, BUFFERS) SELECT * FROM identity.users WHERE 1=1 ORDER BY created_at DESC, id DESC LIMIT 25 OFFSET 0;
```

```
Limit  (cost=2.34..2.37 rows=12 width=254) (actual time=0.025..0.028 rows=20.00 loops=1)
  Buffers: shared hit=2
  ->  Sort  (cost=2.34..2.37 rows=12 width=254) (actual time=0.024..0.025 rows=20.00 loops=1)
        Sort Key: created_at DESC, id DESC
        Sort Method: quicksort  Memory: 32kB
        Buffers: shared hit=2
        ->  Seq Scan on users  (cost=0.00..2.12 rows=12 width=254) (actual time=0.009..0.012 rows=20.00 loops=1)
              Buffers: shared hit=2
Planning:
  Buffers: shared hit=146
Planning Time: 0.469 ms
Execution Time: 0.048 ms
```

### `platform.audit_log` (Seq Scan)

```sql
EXPLAIN (ANALYZE, BUFFERS) SELECT * FROM platform.audit_log WHERE 1=1 ORDER BY created_at DESC, id DESC LIMIT 25 OFFSET 0;
```

```
Limit  (cost=11.12..11.18 rows=25 width=193) (actual time=0.075..0.078 rows=25.00 loops=1)
  Buffers: shared hit=5
  ->  Sort  (cost=11.12..11.52 rows=160 width=193) (actual time=0.074..0.075 rows=25.00 loops=1)
        Sort Key: created_at DESC, id DESC
        Sort Method: top-N heapsort  Memory: 56kB
        Buffers: shared hit=5
        ->  Seq Scan on audit_log  (cost=0.00..6.60 rows=160 width=193) (actual time=0.027..0.048 rows=61.00 loops=1)
              Buffers: shared hit=5
Planning:
  Buffers: shared hit=70
Planning Time: 0.324 ms
Execution Time: 0.095 ms
```

### `org.organizations` (Seq Scan)

```sql
EXPLAIN (ANALYZE, BUFFERS) SELECT * FROM org.organizations WHERE 1=1 ORDER BY created_at DESC, id DESC LIMIT 25 OFFSET 0;
```

```
Limit  (cost=1.20..1.22 rows=8 width=372) (actual time=0.056..0.059 rows=14.00 loops=1)
  Buffers: shared hit=1
  ->  Sort  (cost=1.20..1.22 rows=8 width=372) (actual time=0.055..0.056 rows=14.00 loops=1)
        Sort Key: created_at DESC, id DESC
        Sort Method: quicksort  Memory: 32kB
        Buffers: shared hit=1
        ->  Seq Scan on organizations  (cost=0.00..1.08 rows=8 width=372) (actual time=0.010..0.011 rows=14.00 loops=1)
              Buffers: shared hit=1
Planning:
  Buffers: shared hit=183
Planning Time: 0.560 ms
Execution Time: 0.090 ms
```

### `inventory.stock` ()

```sql
EXPLAIN (ANALYZE, BUFFERS) SELECT * FROM inventory.stock WHERE 1=1 ORDER BY updated_at DESC, id DESC LIMIT 25 OFFSET 0;
```

```
Error: ERROR: relation "inventory.stock" does not exist (SQLSTATE 42P01)
```

### `billing.wallet_transactions` (Seq Scan)

```sql
EXPLAIN (ANALYZE, BUFFERS) SELECT * FROM billing.wallet_transactions WHERE 1=1 ORDER BY created_at DESC, id DESC LIMIT 25 OFFSET 0;
```

```
Limit  (cost=1.14..1.15 rows=6 width=134) (actual time=0.023..0.025 rows=11.00 loops=1)
  Buffers: shared hit=1
  ->  Sort  (cost=1.14..1.15 rows=6 width=134) (actual time=0.022..0.023 rows=11.00 loops=1)
        Sort Key: created_at DESC, id DESC
        Sort Method: quicksort  Memory: 27kB
        Buffers: shared hit=1
        ->  Seq Scan on wallet_transactions  (cost=0.00..1.06 rows=6 width=134) (actual time=0.011..0.012 rows=11.00 loops=1)
              Buffers: shared hit=1
Planning:
  Buffers: shared hit=47
Planning Time: 0.316 ms
Execution Time: 0.042 ms
```

### `billing.payments` (Seq Scan)

```sql
EXPLAIN (ANALYZE, BUFFERS) SELECT * FROM billing.payments WHERE 1=1 ORDER BY created_at DESC, id DESC LIMIT 25 OFFSET 0;
```

```
Limit  (cost=0.01..0.02 rows=1 width=256) (actual time=0.021..0.022 rows=0.00 loops=1)
  ->  Sort  (cost=0.01..0.02 rows=1 width=256) (actual time=0.020..0.020 rows=0.00 loops=1)
        Sort Key: created_at DESC, id DESC
        Sort Method: quicksort  Memory: 25kB
        ->  Seq Scan on payments  (cost=0.00..0.00 rows=1 width=256) (actual time=0.007..0.007 rows=0.00 loops=1)
Planning:
  Buffers: shared hit=87
Planning Time: 1.904 ms
Execution Time: 0.039 ms
```

### `billing.invoices` (Seq Scan)

```sql
EXPLAIN (ANALYZE, BUFFERS) SELECT * FROM billing.invoices WHERE 1=1 ORDER BY created_at DESC, id DESC LIMIT 25 OFFSET 0;
```

```
Limit  (cost=1.05..1.06 rows=3 width=118) (actual time=0.291..0.293 rows=3.00 loops=1)
  Buffers: shared hit=1
  ->  Sort  (cost=1.05..1.06 rows=3 width=118) (actual time=0.290..0.291 rows=3.00 loops=1)
        Sort Key: created_at DESC, id DESC
        Sort Method: quicksort  Memory: 25kB
        Buffers: shared hit=1
        ->  Seq Scan on invoices  (cost=0.00..1.03 rows=3 width=118) (actual time=0.280..0.281 rows=3.00 loops=1)
              Buffers: shared hit=1
Planning:
  Buffers: shared hit=106
Planning Time: 1.874 ms
Execution Time: 0.315 ms
```


## B. Templ Page UI Audit

| Page Template (`internal/ui/pages/`) | Has Table | Has B2BPagination | Identified Data Structs |
|---|:---:|:---:|---|
| `about.templ` | No | No | AboutPageData |
| `admin_adv_products.templ` | **Yes** | No | AdminAdvProductsData |
| `admin_analytics.templ` | **Yes** | No |  |
| `admin_approvals.templ` | **Yes** | No |  |
| `admin_askfor.templ` | **Yes** | No |  |
| `admin_audit.templ` | **Yes** | No |  |
| `admin_branches.templ` | **Yes** | No | AdminBranchesPageData |
| `admin_brands.templ` | **Yes** | No |  |
| `admin_categories.templ` | **Yes** | No |  |
| `admin_chat_history.templ` | **Yes** | ✅ Yes | AdminChatHistoryData |
| `admin_cities.templ` | **Yes** | No | AdminCitiesData |
| `admin_content.templ` | **Yes** | No |  |
| `admin_dashboard.templ` | **Yes** | No |  |
| `admin_developers.templ` | **Yes** | No |  |
| `admin_developers_diagnostics.templ` | **Yes** | No |  |
| `admin_documents.templ` | **Yes** | No |  |
| `admin_finance.templ` | **Yes** | No | AdminFinanceData |
| `admin_finance_subpages.templ` | **Yes** | No |  |
| `admin_full_user.templ` | **Yes** | No | AdminEmployeeActivitiesData, AdminUserDetailView |
| `admin_import_mapping.templ` | **Yes** | No |  |
| `admin_import_review.templ` | **Yes** | No |  |
| `admin_import_wizard.templ` | **Yes** | No |  |
| `admin_institutional.templ` | **Yes** | No |  |
| `admin_jobs.templ` | **Yes** | No | AdminJobView |
| `admin_match_decisions.templ` | **Yes** | ✅ Yes | AdminMatchDecisionsData |
| `admin_messages.templ` | **Yes** | No |  |
| `admin_monitoring.templ` | **Yes** | No |  |
| `admin_offers.templ` | **Yes** | No | AdminOffersData |
| `admin_orders.templ` | **Yes** | No | AdminOrdersData |
| `admin_org_detail.templ` | **Yes** | No | AdminOrgDetailData |
| `admin_organizations.templ` | **Yes** | No | AdminOrganizationsPageData, AdminEnterpriseHubData |
| `admin_page_control.templ` | **Yes** | No | SystemPagesView |
| `admin_plans.templ` | **Yes** | No | AdminPlansData |
| `admin_policies.templ` | **Yes** | No |  |
| `admin_product_detail.templ` | **Yes** | No |  |
| `admin_product_images_import.templ` | **Yes** | No |  |
| `admin_products.templ` | **Yes** | No |  |
| `admin_reference_crud.templ` | **Yes** | No |  |
| `admin_saving_products.templ` | **Yes** | ✅ Yes | AdminSavingProductsData |
| `admin_settings.templ` | No | No |  |
| `admin_settings_payment.templ` | No | No |  |
| `admin_settings_policies.templ` | No | No |  |
| `admin_settings_site.templ` | No | No |  |
| `admin_stocks.templ` | **Yes** | No |  |
| `admin_temp_warehouses.templ` | **Yes** | No | AdminTempWarehousesData |
| `admin_temp_warehouses_modals.templ` | **Yes** | No |  |
| `admin_translations.templ` | **Yes** | ✅ Yes | AdminTranslationsData |
| `admin_trash.templ` | **Yes** | No | TrashRowView |
| `admin_user_organizations.templ` | **Yes** | No | AdminUserOrgData |
| `admin_users.templ` | **Yes** | No | AdminUsersPageData, AdminUsersData |
| `admin_warehouses.templ` | **Yes** | ✅ Yes | AdminWarehouseRowView, AdminWarehouseDetailView |
| `admin_weekly_coverages.templ` | **Yes** | No | AdminWeeklyCoveragesData |
| `ai_consumption_logs.templ` | **Yes** | No |  |
| `auth.templ` | No | No |  |
| `auth_login.templ` | No | No |  |
| `compare_head_to_head.templ` | **Yes** | No | HeadToHeadPageData |
| `compare_mapping.templ` | **Yes** | No |  |
| `compare_market_benchmark.templ` | **Yes** | No | MarketBenchmarkPageData |
| `compare_market_intelligence.templ` | **Yes** | No | MarketIntelligencePageData |
| `compare_plans.templ` | No | No |  |
| `compare_results.templ` | **Yes** | No |  |
| `compare_tool.templ` | No | No |  |
| `component_gallery.templ` | **Yes** | ✅ Yes | ComponentGalleryProps |
| `contact.templ` | No | No |  |
| `content.templ` | No | No |  |
| `courier_delivery.templ` | No | No | CourierDeliveryData |
| `customer_branch_form.templ` | No | No | CustomerBranchFormData |
| `customer_branches.templ` | No | No | CustomerBranchesData |
| `customer_branches_employees.templ` | **Yes** | No |  |
| `customer_cart.templ` | No | No |  |
| `customer_catalog.templ` | No | No |  |
| `customer_catalog_filter.templ` | No | No |  |
| `customer_catalog_results.templ` | No | No |  |
| `customer_catalog_table.templ` | **Yes** | No |  |
| `customer_checkout.templ` | No | No |  |
| `customer_decision_memory.templ` | **Yes** | ✅ Yes | CustomerDecisionMemoryData |
| `customer_followed_suppliers.templ` | No | No |  |
| `customer_home.templ` | No | No |  |
| `customer_institutional.templ` | **Yes** | No |  |
| `customer_invoices.templ` | **Yes** | No |  |
| `customer_jobs.templ` | **Yes** | No | CustomerJobsData |
| `customer_jobs_modals.templ` | No | No |  |
| `customer_negotiation_modal.templ` | No | No |  |
| `customer_order_detail.templ` | **Yes** | No |  |
| `customer_order_detail_edit_modal.templ` | **Yes** | No |  |
| `customer_order_detail_script.templ` | No | No |  |
| `customer_orders.templ` | No | No |  |
| `customer_product_detail.templ` | No | No |  |
| `customer_product_detail_offers.templ` | **Yes** | No |  |
| `customer_saving.templ` | **Yes** | ✅ Yes | CustomerSavingPageData |
| `customer_saving_import_modal.templ` | **Yes** | No |  |
| `customer_saving_orders.templ` | **Yes** | No |  |
| `customer_saving_script.templ` | **Yes** | No |  |
| `customer_user_org_modals.templ` | No | No |  |
| `customer_user_organizations.templ` | No | No | CustomerUserOrgData |
| `document_error.templ` | No | No | DocumentUnavailableView |
| `error_page.templ` | No | No |  |
| `faq.templ` | No | No |  |
| `favorites.templ` | No | No |  |
| `home_hero_preview.templ` | **Yes** | No |  |
| `how_it_works.templ` | No | No |  |
| `invoice_printable.templ` | No | No |  |
| `invoice_printable_a4.templ` | **Yes** | No |  |
| `invoice_printable_thermal.templ` | No | No |  |
| `job_detail.templ` | No | No | JobDetailData |
| `jobs.templ` | No | No | JobItemView, JobsPageData |
| `market_discounts.templ` | No | No |  |
| `messages.templ` | No | No |  |
| `mfa_settings.templ` | No | No | MFASettingsViewData |
| `mfa_verify.templ` | No | No |  |
| `notifications.templ` | No | No |  |
| `notifications_dropdown.templ` | No | No |  |
| `offers.templ` | No | No | OfferCardData |
| `offers_detail.templ` | **Yes** | No | OfferDetailPageData |
| `onboarding.templ` | No | No |  |
| `onboarding_pending.templ` | No | No |  |
| `organization_documents.templ` | No | No |  |
| `organization_documents_modals.templ` | No | No |  |
| `password_reset.templ` | No | No |  |
| `pharmacy_dashboard.templ` | **Yes** | No |  |
| `pharmacy_dashboard_metrics.templ` | No | No |  |
| `pharmacy_dashboard_widgets.templ` | No | No |  |
| `platform_hardening.templ` | No | No |  |
| `promo_revenue.templ` | **Yes** | No | SponsorshipRequestsData, AdminOffersPackagesData |
| `promo_revenue_admin_subpages.templ` | **Yes** | No |  |
| `promo_revenue_vendor.templ` | **Yes** | No |  |
| `public_pages.templ` | No | No |  |
| `purchase_requests.templ` | **Yes** | No |  |
| `requests.templ` | **Yes** | No |  |
| `roles.templ` | No | No | RolesView, RoleEditView |
| `saving_import_review.templ` | **Yes** | No |  |
| `saving_import_wizard.templ` | **Yes** | No |  |
| `settings_employees.templ` | **Yes** | No |  |
| `settings_unified.templ` | No | No | UnifiedSettingsData |
| `smart_order.templ` | No | No | SmartOrderNewData |
| `smart_order_results.templ` | **Yes** | No | SmartOrderResultsData |
| `smart_order_results_row.templ` | No | No |  |
| `smart_order_review.templ` | **Yes** | No | SmartOrderReviewData |
| `smart_order_steps.templ` | **Yes** | No | SmartOrderMappingData, SmartOrderProgressData |
| `storefront.templ` | No | No |  |
| `subscription_gate.templ` | No | No | SubscriptionGateProps |
| `suppliers.templ` | No | No |  |
| `suppliers_map.templ` | No | No |  |
| `suppliers_profile.templ` | **Yes** | No |  |
| `team_import_wizard.templ` | **Yes** | No |  |
| `tenant_sessions.templ` | No | No | TenantSessionsViewData |
| `tenant_subscription.templ` | No | No |  |
| `tenant_team.templ` | **Yes** | No | TenantTeamView |
| `vendor_activities.templ` | **Yes** | No |  |
| `vendor_ads_wizard.templ` | No | No |  |
| `vendor_branch_form.templ` | No | No | VendorBranchFormData |
| `vendor_branches.templ` | No | No | VendorBranchesData |
| `vendor_catalog_select.templ` | **Yes** | No |  |
| `vendor_content.templ` | No | No |  |
| `vendor_coverage.templ` | No | No | VendorCoverageData |
| `vendor_coverage_table.templ` | **Yes** | No |  |
| `vendor_dashboard.templ` | **Yes** | No |  |
| `vendor_finance.templ` | **Yes** | No |  |
| `vendor_ingest.templ` | **Yes** | No |  |
| `vendor_ingest_ai.templ` | No | No |  |
| `vendor_ingest_results.templ` | **Yes** | No |  |
| `vendor_ingest_review.templ` | **Yes** | No |  |
| `vendor_ingest_stages.templ` | **Yes** | No |  |
| `vendor_institutional.templ` | **Yes** | No |  |
| `vendor_inventory.templ` | **Yes** | ✅ Yes | VendorInventoryData |
| `vendor_jobs.templ` | **Yes** | No | VendorJobsData |
| `vendor_offer_form.templ` | **Yes** | No | VendorOfferFormData |
| `vendor_offer_locations.templ` | No | No | VendorOfferLocationsData |
| `vendor_offers.templ` | No | No | VendorSpecialOffersData |
| `vendor_orders.templ` | **Yes** | No | VendorOrdersData |
| `vendor_organization.templ` | No | No |  |
| `vendor_product_editor.templ` | No | No | VendorVariantEditorData |
| `vendor_products.templ` | No | ✅ Yes | VendorVariantView, VendorVariantsData |
| `vendor_products_modals.templ` | No | No |  |
| `vendor_products_table.templ` | **Yes** | No |  |
| `vendor_roles.templ` | No | No |  |
| `vendor_saving.templ` | **Yes** | ✅ Yes | VendorSavingPageData |
| `vendor_saving_script.templ` | **Yes** | No |  |
| `vendor_team.templ` | **Yes** | No | TeamMemberView, VendorTeamData |
| `vendor_team_extensions.templ` | No | No |  |
| `vendor_transfers.templ` | **Yes** | No |  |
| `vendor_user_organizations.templ` | No | No | VendorUserOrgData |
| `vendor_warehouses.templ` | **Yes** | No | VendorWarehouseDetailData |
| `wallet.templ` | No | No | WalletViewData |
| `wallet_modals.templ` | No | No |  |
| `wallet_transactions_table.templ` | **Yes** | No |  |
| `wizard.templ` | No | No |  |
