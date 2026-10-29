# Module: Organizations & Tenant Management (`org`)

## Overview

The `org` bounded context manages organization tenants (suppliers, pharmacies, pharmacy chains), physical branch networks, staff membership roles, vendor customer ratings/reviews, following relationships, and organizational policies.

## Map reliability (2026-10-29)

Branch pickers wait for both local Leaflet CSS and JavaScript before initializing. Platform maps use the canonical HTTPS OpenStreetMap tile host, and the shared loader installs a bilingual tile-failure notice with a retry action. The generic product-image fallback leaves Leaflet's managed tiles and markers to its own loading/error lifecycle instead of permanently hiding them. Pickers retain resize/visibility handling and guard against repeated initialization. Local browser checks cover real tile loading and simulated network failure/recovery; production network availability still depends on the client's connection to OpenStreetMap.

## Schema Mapping

- **PostgreSQL Schema:** `org`
- **Migrations:** `003_organizations.up.sql`, `014_enhancements.up.sql`, `015_org_extensions.up.sql`, `077_institutional_works.up.sql`, `079_institutional_work_connections.up.sql`, `084_employee_institutional_works.up.sql`
- **Tables Owned:**
  - `org.organizations` — Core tenant entity with approval status and credit terms.
  - `org.branches` — Physical branch and warehouse locations.
  - `org.members` — Staff association and RBAC role assignment.
  - `org.organization_reviews` — Customer ratings (1-5) and review texts.
  - `org.organization_followers` — User following relationships for promotional alerts.
  - `org.organization_social_media` — Verified social media channels.
  - `org.organization_policies` — Public terms, refund policies, and shipping rules.
  - `org.user_organization_numbers` — Official contact registries.
  - `org.institutional_works` — Hierarchical corporate classifications and enterprise branches.
  - `org.institutional_work_connections` — Directional visibility matrix (`from_institutional_work_id` $\rightarrow$ `to_institutional_work_id`).
  - `org.employee_institutional_works` — User-to-institutional work assignments with tenant `organization_id` for RLS.

## Invariants & Rules

1. **Tenant Isolation:** All tenant tables enforce PostgreSQL `FORCE ROW LEVEL SECURITY` with `platform.tenant_visible(organization_id)`. `org.employee_institutional_works` carries an explicit `organization_id` column for RLS isolation.
2. **Single Main Branch:** Each organization has at most one branch with `is_main = true`. Creating a new main branch automatically unsets the flag on existing branches.
3. **Approval Lifecycle:** Organizations transition strictly through `pending` $\rightarrow$ `approved` / `rejected` / `suspended`.
4. **Institutional Filter Modes (Plan V5 Phase 1 Task 1.1):**
   - **Mode Simple (Customer Dashboard / Catalogue):** `visible(product) := (product.institutional_work_ids ∩ user.works ≠ ∅) OR (product.institutional_work_ids IS NULL/empty)`. Unrestricted products are visible to all users.
   - **Mode WithConnections (Purchase Requests / Guided Procurement):** `visible(product) := product.institutional_work_ids ∩ {to | (from, to) ∈ connections, from ∈ user.works} ≠ ∅`. Unrestricted products are strictly invisible in this mode.
   - Cross-module resolution is mediated via `catalog.InstitutionalGate` and `promo.InstitutionalGate` without cyclic module imports.
5. **One institutional work per pharmacy branch:** the customer branch form is a single choice among the pharmacy works, and `resolveCustomerInstitutionalWork` keeps the first submitted value (falling back to the pharmacy work when none is sent). Admin and vendor branch forms still accept several.

## Admin documents and licenses (2026-10-12)

The organizations list and detail link to `/admin/organizations/{id}/documents`.
It renders the same requirements, upload, replacement, request and preview
components used by the organization itself. `hr.document.view` grants a read-only
page; `hr.document.update` grants uploads. The server resolves the target from
the route, requires platform staff, and ignores posted organization IDs.
It retains the real administrator as uploader/audit actor and never changes
the administrator's session or grants a tenant role. New uploads remain pending.
Document requests must belong to the target and match the uploaded document
type; the repository also enforces that binding at write time.

## User-organization links

Edit, delete, approve and reject address a link by id and run as system, so every handler first calls `Service.AuthorizeUserOrgLink`: the customer side must own the link (`user_id`), the vendor side must be its `vendor_org_id`. A link the caller does not own reads as not found (security fix 2026-09-24).

## Team management limits (security fix 2026-09-24)

- A team manager assigns only roles whose permissions they hold themselves (`resolveTeamRole`); the owner and staff are not bounded. The fallback is the least-privileged assignable role.
- Adding an employee by email reuses an existing account only when it is not platform staff and belongs to no other organization (`accountManagedByOrg`, vendor and pharmacy screens and team import). The team edit screen changes the person's own name and phone only under the same condition.

## Organization list layout (2026-10-23)

The admin organization table reserves a readable name column and a fixed avatar.
Wide content scrolls inside the table wrapper on phones instead of compressing
Arabic names into one letter per line. Browser fixtures cover 390, 768 and 1440
pixel widths in both themes.
