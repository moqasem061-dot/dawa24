# PLAN V6 — Chain Audit

Six-link chain per screen (`00_MASTER.md` §A.5). All six cells must be non-empty.
A template signature of `(lang, dir string)` is a broken link 6.

| Screen | 1 Table | 2 Repository | 3 Interface | 4 Service | 5 Handler | 6 Template takes data |
|---|---|---|---|---|---|---|
| `/vendor/payments` | `billing.payments` | `billing/postgres/payments.go` | `billing/repository.go` | `billing/service.go` | `vendor_finance_handlers.go` | `[]*billing.Payment` |
| `/admin/invoices` | `billing.invoices` | `billing/postgres/invoices.go` | `billing/repository.go` | `billing/service.go` | `admin_finance_handlers.go` | `[]*billing.Invoice` |
| `/admin/payments` | `billing.payments` | `billing/postgres/payments.go` | `billing/repository.go` | `billing/service.go` | `admin_finance_handlers.go` | `[]*billing.Payment` |
| `/admin/wallets` | `billing.wallets` | `billing/postgres/wallets.go` | `billing/repository.go` | `billing/service.go` | `admin_finance_handlers.go` | `[]*billing.Wallet` |
| `/admin/plans-info` | `billing.subscription_plans` | `billing/postgres/plans.go` | `billing/repository.go` | `billing/service.go` | `admin_finance_handlers.go` | `[]*billing.Plan` |
| `/admin/plans/subscriptions` | `billing.subscriptions` | `billing/postgres/subscriptions.go` | `billing/repository.go` | `billing/service.go` | `admin_finance_handlers.go` | `[]*billing.Subscription` |
| `/vendor/activities` | `platformadmin.audit_logs` | `platform_admin/postgres/audit.go` | `platform_admin/repository.go` | `platform_admin/service.go` | `vendor_activities_handlers.go` | `[]*platformadmin.AuditLog` |
| `/vendor/saving-products` | `catalog.saving_products` | `catalog/postgres/saving_products.go` | `catalog/repository.go` | `catalog/service.go` | `vendor_saving_handlers.go` | `[]*catalog.SavingProduct` |
| `/vendor/institutional-work` | `org.institutional_works` | `org/postgres/institutional.go` | `org/repository.go` | `org/service.go` | `vendor_institutional_handlers.go` | `[]*org.InstitutionalWork` |
| `/admin/countries` | `platformadmin.countries` | `platform_admin/postgres/reference.go` | `platform_admin/repository.go` | `platform_admin/service.go` | `admin_reference_handlers.go` | `[]pages.ReferenceItem` |
| `/admin/social-media` | `platformadmin.site_settings` | `platform_admin/postgres/settings.go` | `platform_admin/repository.go` | `platform_admin/service.go` | `admin_reference_handlers.go` | `[]pages.ReferenceItem` |
| `/admin/highlight-sections` | `platformadmin.content_blocks` | `platform_admin/postgres/content.go` | `platform_admin/repository.go` | `platform_admin/service.go` | `admin_reference_handlers.go` | `[]pages.ReferenceItem` |
| `/admin/api-integrations` | `platformadmin.gateway_settings` | `platform_admin/postgres/settings.go` | `platform_admin/repository.go` | `platform_admin/service.go` | `admin_reference_handlers.go` | `[]pages.ReferenceItem` |
| `/catalog` | `catalog.products` | `catalog/postgres/search.go` | `catalog/repository.go` | `catalog/service.go` | `customer_handlers.go` | `pages.CatalogPageData` |
| `/cart` | `commerce.carts` | `commerce/postgres/cart.go` | `commerce/repository.go` | `commerce/service.go` | `customer_handlers.go` | `*commerce.Cart` |
| `/orders` | `commerce.orders` | `commerce/postgres/orders.go` | `commerce/repository.go` | `commerce/service.go` | `customer_handlers.go` | `[]*commerce.Order` |
| `/orders/{id}` | `commerce.orders` | `commerce/postgres/orders.go` | `commerce/repository.go` | `commerce/service.go` | `customer_handlers.go` | `*commerce.Order, []*commerce.OrderHistory` |
| `/notifications` | `notifications.logs` | `notifications/postgres/logs.go` | `notifications/repository.go` | `notifications/service.go` | `customer_handlers.go` | `[]*notifications.NotificationLog` |
| `/admin/users` | `identity.users` | `identity/postgres/users.go` | `identity/repository.go` | `identity/service.go` | `admin_user_handlers.go` | `[]*identity.User` |
| `/admin/organizations` | `org.organizations` | `org/postgres/orgs.go` | `org/repository.go` | `org/service.go` | `admin_org_handlers.go` | `[]*org.Organization` |
| `/admin/branches` | `org.branches` | `org/postgres/branches.go` | `org/repository.go` | `org/service.go` | `admin_org_handlers.go` | `[]*org.Branch` |
| `/admin/roles` | `org.roles` | `org/postgres/roles.go` | `org/repository.go` | `org/service.go` | `admin_user_handlers.go` | `[]*org.Role` |
| `/admin/weekly-coverages` | `workflow.weekly_coverages` | `workflow/postgres/coverage.go` | `workflow/repository.go` | `workflow/service.go` | `admin_org_handlers.go` | `[]*workflow.CoverageView` |
| `/admin/stocks` | `inventory.stocks` | `inventory/postgres/stocks.go` | `inventory/repository.go` | `inventory/service.go` | `admin_catalog_handlers.go` | `[]*inventory.Stock` |
| `/admin/warehouses` | `inventory.warehouses` | `inventory/postgres/warehouses.go` | `inventory/repository.go` | `inventory/service.go` | `admin_catalog_handlers.go` | `[]*inventory.Warehouse` |
