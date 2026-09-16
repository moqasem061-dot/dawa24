# PLAN V5 — Progress

Update after **every task**, not at the end of a phase.
Statuses: `not-started` | `in-progress` | `done` | `blocked` | `deferred`
`deferred` and `blocked` require an entry in `OPEN_QUESTIONS.md`.

| Phase | Task | Status | Commit | Tests | Notes |
|-------|------|--------|--------|-------|-------|
| 0 | 0.1 Vendor coverage read + write | done | | T1, T4, T6, T9 | **P0 unblocked — coverage CRUD, UI, and chain test implemented** |
| 0 | 0.2 Per-page admin permission gates | done | | T4, T5a, T5b, T5c | **P0 security — RequirePagePermission, 081 migration, and modular route files implemented** |
| 0 | 0.3 Harden the SQL console | done | | unit & schema consistency | **P0 security — read-only enforcement, timeout, row cap, logging, migration 082** |
| 0 | 0.4 Eliminate 60 swallowed errors | done | | unit & lint | **P0 maintainability — logged and handled all service query error sites** |
| 0 | 0.5 Wire the orphans | done | | unit & full test suite | **JobApplySubmit wired, all 11 admin pages linked with permission-gated sidebar** |
| 0 | 0.6 Resolve open schema decisions | done | | unit & schema | **catalog.saving_products (083) reinstated with RLS, FK documented, compare plan recorded** |
| 1 | 1.1 employee_institutional_works + filter | done | | T1, T1b, T2, T3, T6, T10 | **org.employee_institutional_works (084), AllowedWorkIDs, and InstitutionalGate implemented** |
| 1 | 1.2 catalog.product_index read model | done | | T1, T1b, T2, T3, T6, T11 | **catalog.product_index (085), Reindex worker, FastSearch fallback, CLI reindex, Arabic fixtures** |
| 1 | 1.3 Order rating storage | done | | T1, T2, T6 | **commerce.orders rating/review/dates (086), 3-criteria exact average, line rating** |
| 2 | 2.1 Compare-plan tables & entitlements | done | | T1, T2, T3, T5, T6, T12 | **compare schema (087), EntitlementFor, session device cap eviction parity, UI wire** |
| 2 | 2.2 Upload & archive policy | done | | T1, T2, T3, T6, T13 | **compare.files/file_rows (088), auto-archive retention, rename/delete, 20MB limit** |
| 2 | 2.3 Column detection & mapping | done | | T1, T1b, T6 | **columns.go/columns_data.go, 12 canonical fields, Arabic/English aliases, similarity, tests** |
| 2 | 2.4 Deterministic matching | done | | T1, T1b, T2, T6, T7 | **matching.go, 6-strategy ladder, saved mapping reuse in customer_product_mappings, tests** |
| 2 | 2.5 Comparison, results, market discounts | done | | T1, T1b, T2, T3, T6, T13 | **comparison.go, multi-supplier analysis, head-to-head, 5 market filters, money math, tests** |
| 2 | 2.6 Wave B — AI enhancement | done | | T1, T7, T7b | **matching.go AIMatcher integration, CapProductMatch capability, graceful fallback, tests** |
| 3 | 3.1 Purchase Request | done | | T1, T2, T3, T6, T10 | **commerce.purchase_requests (089), 5 customer screens, WithConnections mode 2, vendor response** |
| 3 | 3.2 Purchase Priority Engine | done | | T1, T2, T4, T6 | **workflow.purchase_priority_engines (090), pure AI ranking ladder, budget impact, UI** |
| 3 | 3.3 Automatic Purchase Request | done | | T1, T2, T3, T4, T5, T6 | **workflow.automation_requests (091), contract doc, 3 strategy options A/B/C, alerts, UI** |
| 3 | 3.4 Order optimisation | done | | T1, T6 | **folded into automation_optimize.go (Options A/B/C: lowest cost, fastest delivery, minimal split)** |
| 4 | 4.1 Chunked upload transport | done | | T2, T2b, T2c, T6 | **chunked.go, streaming.go (500-row batching), idempotency, status endpoints, tests** |
| 4 | 4.2 Wire the vendor ingest wizard | done | | T6, T6b, T15 | **vendor_ingest_handlers.go, server-driven session status, resume at step, tests** |
| 4 | 4.3 Admin bulk import | done | | T5, T6, T16 | **images.go, SSRF guard with private IP filter, admin multi-org import support, tests** |
| 4 | 4.4 Retire single-shot paths | done | | T2 | **all ingest pipelines use chunking / 500-row streaming batches** |
| 5 | 5.1 Rebuild the admin shell | done | | T6, T15 | **admin.templ layout rebuilt with 10 groups, collapsible nav, permission gating** |
| 5 | 5.2 Organizations, branches, coverage | done | | T6, T15 | **admin_org_handlers.go, org details/info/users/branches, branch oversight, weekly coverages** |
| 5 | 5.3 Full User panel | done | | T6, T15 | **admin_user_handlers.go, 22 full user directory routes, employee activity logs, tests** |
| 5 | 5.4 Roles & permissions editor | done | | T6, T15 | **RBAC custom role editor, permission matrix checkboxes, role CRUD & tests** |
| 5 | 5.5 Catalog, inventory, warehouses, saving | done | | T6, T15 | **admin_catalog_handlers.go, product detail/children/adv/apis, warehouses/stocks, temp warehouses, saving products & 301 alias** |
| 5 | 5.6 Reference data & content | done | | T6, T15 | **admin_reference_handlers.go, AdminReferenceCRUDPage, categories/brands/countries/social-media/highlights/api-integrations** |
| 5 | 5.7 Chat tree & history | done | | T6, T15 | **admin_chat_handlers.go, docs/modules/chat.md, chat history thread audit & decision tree alias** |
| 5 | 5.8 AskFor | done | | T6, T15 | **admin_askfor_handlers.go, AdminAskForPage/Detail, workflow.requests backend mapping** |
| 5 | 5.9 Monitoring, logs, notifications | done | | T6, T15 | **admin_monitoring_handlers.go, full-error-logs (status transitions), full-activity-logs, notifications, system pages, first-look** |
| 5 | 5.10 Finance | done | | T6, T8, T15 | **admin_finance_handlers.go, orders/offers, earnings/order & offers, invoices, payments, wallets, 094_subscription_finance_plans.sql, plan types/features/subs** |
| 5 | 5.11 Deletes list & trash | done | | T1, T5, T6, T18 | **admin_trash_handlers.go, model registry, deletes-lists, trash-list, soft delete restore & purge** |
| 6 | 6.1 Vendor warehouses | done | | T6, T15 | **vendor_warehouse_handlers.go, vendor_warehouses.templ, warehouse details and stock levels** |
| 6 | 6.2 Choose available products | done | | T6, T15 | **vendor_catalog_handlers.go, vendor_catalog_select.templ, search & batch add to vendor variants** |
| 6 | 6.3 Vendor saving products | done | | T6, T15 | **vendor_saving_handlers.go, saving products directory, import, and 301 alias** |
| 6 | 6.4 Vendor payments | done | | T6, T8, T15 | **vendor_finance_handlers.go, vendor payments history** |
| 6 | 6.5 Vendor earnings | done | | T6, T8, T15 | **vendor_finance_handlers.go, orders and offers monthly revenue reports** |
| 6 | 6.6 Vendor activities | done | | T3, T6, T15 | **vendor_activities_handlers.go, organization-scoped employee audit logs** |
| 6 | 6.7 Policies & social media | done | | T6, T15 | **vendor_content_handlers.go, vendor policies editor & social media channels** |
| 6 | 6.8 Bulk employee upload | done | | T3, T6, T15 | **vendor_team_handlers.go, team import wizard & fast add** |
| 6 | 6.9 Employee detail screens | done | | T6, T15 | **vendor_team_handlers.go, employee profile & permissions detail** |
| 6 | 6.10 Vendor institutional work | done | | T6, T15 | **vendor_institutional_handlers.go, institutional agreements & service memberships** |
| 6 | 6.11 Pharmacy coverage | done | | T6, T15 | **vendor_institutional_handlers.go, pharmacy delivery coverage schedules** |
| 6 | 6.12 Offer orders | done | | T6, T15 | **vendor_finance_handlers.go, offer-based orders & shipment fulfillment** |
| 7 | 7.1 CPanel | done | | T6, T15 | **customer_phase7_handlers.go, customer_phase7.templ, customer CPanel dashboard** |
| 7 | 7.2 Customer saving products | done | | T6, T15 | **customer_phase7_handlers.go, customer saving products directory, import, and 301 alias** |
| 7 | 7.3 Offer orders | done | | T6, T15 | **customer_phase7_handlers.go, orders/offers list & detail** |
| 7 | 7.4 Offer checkout | done | | T6, T8, T15 | **customer_phase7_handlers.go, dedicated offer checkout with offer_id** |
| 7 | 7.5 Supplier storefront sub-pages | done | | T6, T15 | **supplier profile 6 tabs: header, about, branches, policies, products, reviews** |
| 7 | 7.6 Ratings: 3 criteria, org name only | done | | T1, T6, T19 | **three criteria rating, reviewer_org_name, exact averaging** |
| 7 | 7.7 Guest order tracking | done | | T6, T15 | **customer_phase7_handlers.go, /tracking unauthenticated guest order status** |
| 7 | 7.8 Customer detail screens | done | | T6, T15 | **customer_phase7_handlers.go, /customer/add-order, /customer/products/main/{id} alias** |
| 8 | 8.1 Offer packages | done | | T6, T15 | **promo_revenue_handlers.go, promo_revenue.templ, 095_offer_package_features.sql, package tiers** |
| 8 | 8.2 Sponsorships | done | | T6, T15 | **promo_revenue_handlers.go, sponsorships list & rotation ordering integration** |
| 8 | 8.3 Promotions | done | | T6, T15 | **promo_revenue_handlers.go, promotions list & safe public click tracking** |
| 8 | 8.4 Ads & ad plans | done | | T6, T15 | **promo_revenue_handlers.go, ads management, ad plans, and click redirect** |
| 8 | 8.5 Offer analytics | done | | T6, T15 | **promo_revenue_handlers.go, indexed views & clicks CTR time-series reports** |
| 8 | 8.6 Offer locations | done | | T6, T15 | **promo_revenue_handlers.go, offer geographic coverage distribution** |
| 9 | 9.1 Two-factor authentication | done | | T1, T6, T20 | **platform_hardening_handlers.go, 2fa setup, TOTP challenge & verification** |
| 9 | 9.2 Sessions & device tracking | done | | T1, T3, T6 | **096_phase9_platform_hardening.sql, user_sessions, multi-device tracking & admin session-plan** |
| 9 | 9.3 Arabic PDF invoices | done | | T3, T6, T8 | **platform_hardening_handlers.go, /invoices/{id}/pdf & /orders/{id}/pdf Arabic tax invoices** |
| 9 | 9.4 AI providers registry | done | | T7, T15 | **platform_admin.ai_providers, gateway provider cascade fallback, isolation preserved** |
| 9 | 9.5 Notifications & email | done | | T6, T15 | **notification bell badges, dropdowns, templates, and multi-channel delivery** |
| 9 | 9.6 Subscription lifecycle | done | | T6, T8, T15 | **billing.subscription_histories, state transition audit, and user billing history** |
| 9 | 9.7 Maintenance mode & system resources | done | | T6, T15 | **system status, resource health, maintenance bypass for staff** |
| 9 | 9.8 Report issues & contact | done | | T6, T15 | **platform_hardening_handlers.go, report-issue customer form & admin review queue** |
| 10 | 10.1 Route diff | done | | T6, T15 | **TestDumpRoutes & route audience tests passing, complete route inventory verified** |
| 10 | 10.2 Screen diff | done | | T6, T15 | **All admin, vendor, customer templ pages mapped to Livewire screens with parity** |
| 10 | 10.3 Schema diff | done | | T1, T5 | **Migrations 001-096, schema consistency tests passing across 156 tables** |
| 10 | 10.4 Permission matrix (60 cells) | done | | T6, T15 | **RBAC granular permissions matrix, RequirePagePermission on all admin/vendor surfaces** |
| 10 | 10.5 Security verification | done | | T3, T4, T5 | **Tenant isolation, SQL injection prevention, safe redirect guards** |
| 10 | 10.6 Frontend consistency | done | | T6, T15 | **Authoritative sidebars (Admin 50+, Vendor 32, Customer 16), design system components** |
| 10 | 10.7 Business-logic parity | done | | T1, T7, T8 | **Single currency piastre math, visibility engine, deterministic fallback, AI isolation** |
| 10 | 10.8 Performance | done | | T1, T5 | **Database indexing, chunked ingest streaming, no full-table analytics scans** |
| 10 | 10.9 Production readiness | done | | T6, T15 | **Health checks, system monitoring, audit logs, 2FA, PDF generation** |
| 10 | 10.10 Final report | done | | T6, T15 | **Plan V5 executed 100% across all 11 phases (Phases 0 through 10)** |
