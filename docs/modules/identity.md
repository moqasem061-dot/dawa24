# Module: Identity & Authentication

## Overview

The `identity` bounded context manages user accounts, authentication, password verification, session lifecycles, and unified Role-Based Access Control (RBAC).

## Schema Mapping

- **PostgreSQL Schemas:** `identity`, `profile`
- **Migrations:** `002_identity.up.sql`
- **Tables Owned:**
  - `identity.users` — Core identity only (name, email, password hash, role, status).
  - `identity.user_security` — Login attempts, lockout timestamps, IP tracking.
  - `identity.user_mfa` — TOTP 2FA secret and confirmation status.
  - `identity.user_identities` — Federated third-party provider logins.
  - `identity.roles` & `identity.permissions` — Unified RBAC.
  - `identity.role_permissions` & `identity.user_roles` — Role assignments.
  - `identity.kyc_records` — Regulated compliance verification.
  - `identity.account_deletion_requests` — Account deletion audit workflow.
  - `profile.user_profiles` & `profile.user_preferences` — Extended profile metadata.

## Invariants & Rules

1. **Bcrypt Compatibility:** PHP/Laravel `$2y$` hashes and Go `$2a$` hashes are verified seamlessly. Existing user passwords require no forced resets.
2. **Account Lockout:** 5 consecutive failed login attempts lock the account for 15 minutes.
3. **Session Store:** Stored in Redis under `session:<token>` and `user_sessions:<userID>` sets.
4. **Tenant Resolution:** The middleware extracts tenant ID from `X-Dawa-Org-ID` header or session `ActiveOrgID`, and sets `database.WithTenant(ctx, orgID)`.
5. **Unified RBAC:** A user's effective permissions are the union of their platform roles (`identity.user_roles`), default role (`users.role`), and organization membership roles (`org.members`).

## Endpoints

- `POST /api/v1/auth/register` — Create new user account.
- `POST /api/v1/auth/login` — Authenticate and receive session cookie/token.
- `POST /api/v1/auth/logout` — Invalidate session.
- `GET /api/v1/auth/me` — Inspect current session and permissions.

## Admin Page Permission Mapping (Phase 0 Task 0.2)

| Laravel Permission Key | Go Permission Key (`<module>.<resource>.<action>`) | Protected Admin Routes |
|------------------------|---------------------------------------------------|------------------------|
| `products_view` | `catalog.product.view` | `GET /admin/products`, `GET /admin/products/sample.*` |
| `products_import` | `catalog.product.update` | `POST /admin/products/{id}/edit`, `POST /admin/products/{id}/status`, `POST /admin/products/import` |
| `categories_view` | `catalog.category.view` / `catalog.category.update` | `GET /admin/categories`, `POST /admin/categories*` |
| `brands_view` | `catalog.brand.view` / `catalog.brand.update` | `GET /admin/brands`, `POST /admin/brands*` |
| `organizations_view` | `org.organization.view` | `GET /admin/organizations*` |
| `organization_users_view` | `identity.user.view` | `GET /admin/users*` |
| `organization_branches_view` | `org.branch.view` | `GET /admin/branches*`, `GET /admin/coverage*` |
| `users_view` | `identity.user.view` | `GET /admin/users` |
| `users_update` | `identity.user.update` | `POST /admin/users/{id}/*` |
| `new_clients_view` | `identity.user.view` | `GET /admin/users/new-clients` |
| `want_delete_view` | `identity.user.view` | `GET /admin/users/deletion-requests` |
| `orders_view` | `commerce.order.view` | `GET /admin/orders*` |
| `invoices_view` | `billing.invoice.view` | `GET /admin/invoices*` |
| `payments_view` | `billing.payment.view` | `GET /admin/payments*`, `GET /admin/wallet-transactions*` |
| `ads_view` | `promo.ad.view` | `GET /admin/ads*` |
| `ad_plans_view` | `promo.ad_plan.view` | `GET /admin/ad-plans*` |
| `session_plans_view` | `billing.session_plan.view` | `GET /admin/session-plans*` |
| `plans_view` | `billing.subscription_plan.view` | `GET /admin/plans*` |
| `warehouses_view` | `inventory.warehouse.view` | `GET /admin/warehouses*` |
| `stocks_view` | `inventory.stock.view` | `GET /admin/stocks*` |
| `job_offers_view` | `hr.job.view` | `GET /admin/jobs*`, `GET /admin/job-applications*` |
| `documents_view` | `hr.document.view` | `GET /admin/documents*` |
| `settings_view` | `platform.setting.view` | `GET /admin/settings*` |
| `admin_roles_view` | `identity.admin_role.view` | `GET /admin/admin-roles*` |
| `supplier_roles_view` | `org.role.view` | `GET /admin/supplier-roles*`, `GET /admin/roles*` |
| `activity_logs_view` | `platform.activity_log.view` | `GET /admin/activity-logs*` |
| `error_logs_view` | `platform.error_log.view` | `GET /admin/error-logs*` |
| `sql-console-developer` | `platform.developer.sql` | `GET/POST /admin/developers/sql*` |

