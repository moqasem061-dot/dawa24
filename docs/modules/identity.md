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
6. **Registration never grants a platform role (security fix 2026-09-24):** `RegisterInput.Role`, `EmailVerifiedAt` and `PhoneVerifiedAt` are `json:"-"`; `Register` produces `user` or `job_seeker` only. Staff are created solely by `CreateStaffUser`. Previously `POST /api/v1/auth/register` accepted `"role":"super_admin"`.
7. **Staff targets need the owner (security fix 2026-09-24):** admin password reset, MFA reset, suspend, reactivate, detail edit and role assignment on a staff account (`admin_guard.go`) require an active `super_admin` actor; granting `super_admin` requires one too; nobody changes their own role.
8. **MFA (security fix 2026-09-24):** the `dawa24_mfa_pending` cookie is signed with a key derived from `SESSION_SECRET` (was a constant in source), and failed codes count toward the same per-account lockout as passwords.
9. **Tenant switch rebuilds the actor (security fix 2026-09-24):** `ResolveTenant` applies the full RBAC grant of the requested organization (`tenant_switch.go`); it used to swap only `Permissions`, leaving the previous organization's permission set, owner flag and branch in force.
10. **Registration rejects existing email addresses before OTP and account creation:** `/auth/email/send-otp` and the registration submit check the normalized address across tenants. A lookup error stops registration rather than issuing an OTP; the database unique index remains the race-safe final guard.
11. **Duplicate-email feedback is visible at the email field:** the registration page renders the localized conflict message outside the hidden OTP panel and offers a direct sign-in link. A duplicate is blocked both when requesting an OTP and when submitting registration.
12. **Session stability (2026-09-30):** Redis idle expiry removes the session and its user/organization indexes without revalidating it. The previous expiry/deletion recursion could exhaust the server stack. Activity updates compare the stored session before writing and preserve its existing expiry, so delayed requests cannot resurrect revoked sessions or overwrite newer activity.
13. **Dependency failures preserve login (2026-09-30):** failed user lookups and unavailable Redis connections return a retryable error without deleting the session, clearing its cookie, or caching the user as inactive. Confirmed missing, deleted, suspended, expired and revoked accounts still lose access. The active-user cache has a hard 4,096-entry bound.
14. **Browser activity (2026-09-30):** server-rendered session metadata enables activity tracking with HttpOnly cookies. Trusted input is synchronized to `/api/v1/auth/me` at most once every 30 seconds per tab; idle checks send `X-Dawa-Background: 1` and never renew the session. Session-specific storage shares activity across tabs, with an in-memory fallback when storage is blocked. Only a server-confirmed 401 redirects to sign-in; network/dependency failures retain the current page. Programmatic scrolling and background polling do not count as user activity.
15. **Phone ownership (2026-09-30):** public and profile Telegram OTP handlers compare the phone returned by the Gateway with the requested phone, reject consumed requests, and refuse mock verification in production. Profile verification responses and public OTP responses are uncacheable.
16. **Blocked browser storage (2026-09-30):** theme and sidebar preferences keep an in-memory fallback. Storage exceptions cannot interrupt initialization of mobile navigation, and early theme selection tolerates blocked storage.

## Registration address repair (2026-10-12)

The two browser geocoders declared the same global `let`, preventing the
registration script from loading. Registration now uses the shared map lookup.
Generated suggestions retain building/house number, street, neighborhood,
quarter, suburb, district, city, governorate, postcode and country where the
map data supplies them, with duplicate components removed. Lookups use the
address layer and building detail, abort superseded requests and ignore late
responses. They update only the originating form and preserve edits made while
a lookup is in flight. Selecting a new pin explicitly requests a fresh generated
address, even after typing a previous address manually (repair 2026-10-14).
The address hint asks the registrant to review and add building/floor/landmark
details; reverse geocoding cannot supply details absent from map data.

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

