# Module: Organizations & Tenant Management (`org`)

## Overview

The `org` bounded context manages organization tenants (suppliers, pharmacies, pharmacy chains), physical branch networks, staff membership roles, vendor customer ratings/reviews, following relationships, and organizational policies.

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
