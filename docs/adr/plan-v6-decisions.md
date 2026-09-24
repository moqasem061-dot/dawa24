# PLAN V6 — Decisions

Q1 (does Laravel have it?) and Q2 (does another Go screen do it?) for every
screen touched. See `00_MASTER.md` §A.2.

## User List Screens (`/admin/users`, `/admin/full-user`, `/admin/customer-list`, `/admin/vendor-list`, `/admin/admin-list`, `/admin/admins`)
**Q1 Laravel:** Has `Admin/FullUser/Index`, `CustomerList`, `VendorList`, `AdminList`.
**Q2 Duplicate:** Differ only by user role / classification filter.
**Decision:** Merge into canonical `/admin/users` with tabbed filtering (`?type=customer`, `?type=vendor`, `?type=staff`, `?type=new`), issue 301 redirects for all legacy aliases.
**Reason:** Single unified user repository and handler with clean client/server tab strip.

---

## Organization Aliases (`/admin/vendors`, `/admin/suppliers`)
**Q1 Laravel:** Has `Admin/Organizations`.
**Q2 Duplicate:** Duplicates `/admin/organizations`.
**Decision:** Canonical `/admin/organizations?type=vendor`, issue 301 redirects for `/admin/vendors` and `/admin/suppliers`.
**Reason:** Clean canonical entity model with filtered view.

---

## Sponsorship & Saving Product Misspelling Aliases (`/admin/offer-sponsorships`, `/admin/saveing-products`)
**Q1 Laravel:** Laravel registers both monetization aliases and has legacy typo `saveing-products`.
**Q2 Duplicate:** Exact duplicates of `/admin/offers-packages/sponsorships` and `/admin/saving-products`.
**Decision:** 301 redirect `/admin/offer-sponsorships` → `/admin/offers-packages/sponsorships` and `/admin/saveing-products` → `/admin/saving-products`.
**Reason:** Preserves legacy bookmark reachability while keeping routing clean.

---

## Policies Management (`/admin/settings` tab 3 vs `/admin/policies`)
**Q1 Laravel:** Has dedicated `Admin/Policies.php` with version history and publishing status.
**Q2 Duplicate:** Tab 3 in `/admin/settings` contained a conflicting inline editor with fabricated fallback text.
**Decision:** `/admin/policies` is canonical. Settings tab 3 replaced with summary overview and direct link; `POST /admin/settings/policy` deleted.
**Reason:** Single source of truth with audit versioning and publish status.

---

## Trash Lists & Deletes Lists (`/admin/deletes-lists`, `/admin/trash-list`)
**Q1 Laravel:** Has `DeletesListsIndex` and `TrashListIndex`.
**Q2 Duplicate:** `deletes-lists/{model}/{id}` duplicated `trash-list/{model}`.
**Decision:** `/admin/deletes-lists` lists tables with soft-delete support; `/admin/trash-list` lists trashed records; 301 alias on deep links.
**Reason:** Clean separation of schema metadata vs deleted record restoration.
