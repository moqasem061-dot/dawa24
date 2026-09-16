# PLAN V7 — Decisions

## Settings: keep the tabbed page (Task 2.1)
**Evidence:** `/settings?tab=…` already had profile, wallet, security,
organization, payments and preferences tabs. The six sub-pages in
`settings.templ` rendered the same data through a second tab component.
**Decision:** keep the tabbed page; merge in the concurrent-session cap, which
was the only thing the sub-pages had that it lacked. Six 301s.
**Note:** `/settings/employees` stays a page — it is a management screen (staff
list, branch-manager assignment, account creation), not a settings tab.

## Branches: two write paths, not three (Task 2.2)
**Evidence:** `SettingsBranchCreateSubmit` invented codes
(`"BR-" + time.Now().UnixNano()%100000`) when the form omitted one.
**Decision:** deleted. `/customer/branches` and `/vendor/branches` are the
management screens; the settings Organization tab shows org data and members
only.

## Policies: the editor moves into the tab (Task 2.3)
**Evidence:** two editors over `platform_admin.policies` with different write
endpoints, plus a hardcoded Arabic default policy text in the settings template.
**Decision:** `/admin/policies`'s editor becomes `AdminPoliciesPanel`, rendered
in the settings Policies tab. Versioning and publish are preserved because the
standalone page's endpoints survive; `POST /admin/settings/policy` is gone.
The hardcoded text is deleted — a default belongs in a seed migration, not a
template.

## Trash: one screen, discovered model list (Task 2.5)
**Evidence:** deletes-lists and trash-list were the same screen twice, over a
hardcoded slice, with restore/purge that logged and reported success.
**Decision:** trash-list survives. The model list comes from
`information_schema` (any table with `deleted_at` appears without a code
change) and counts are real queries. deletes-lists is a 301.

## Translations: removed (Task 3.1)
**Evidence:** `platform_admin.translations` was written by one screen and read
by nothing; every user-facing string is an inline bilingual branch.
**Decision:** removed the whole vertical and dropped the table (migration 097).
`internal/shared/i18n` and the lang/dir plumbing stay — the platform is still
bilingual, it just has no unused override store.

## cPanel: removed (Task 3.2)
**Evidence:** `/customer/cpanel` linked to `/customer/branches`,
`/settings/organization` and `/wallet` — all three already in the sidebar.
**Decision:** 301 to `/customer/dashboard`. Laravel has a CPanel, but parity is
not a reason to keep a screen whose only function is duplicating navigation.

## Brand ↔ category: many-to-many (Task 4.1) — DELIBERATE DEVIATION
**Evidence:** Laravel's `brands` table is
`id, name, description, image, status, timestamps` — no category link at all,
and `Brand.php` has no category relationship. So there is nothing to port.
**Decision:** new `catalog.brand_categories` join table, because the segments
the platform sells (أدوية, مستحضرات تجميل, مستلزمات طبية) are routinely spanned
by one manufacturer, which a single `category_id` on brands could not express.
Backfilled from what products already assert, plus root categories for brands
with no products yet so nothing vanishes from a selector.
**Enforced server-side** in `CreateProduct`/`UpdateProduct`: the client-side
filter is convenience, not the rule.

## Retracted: layouts/pharmacy.templ is not dead
**Evidence:** I scanned for `PharmacyShell`; the file defines `CustomerShell`,
which `shell_for.templ` dispatches to. The compiler caught the deletion.
**Correction applied** to `AUDIT_V7_CONSOLIDATION.md` PART 4.
