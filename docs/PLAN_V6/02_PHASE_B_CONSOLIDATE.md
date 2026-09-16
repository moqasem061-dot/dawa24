# PHASE B — Consolidate: one screen per job

**Depends on:** Phase A complete.
**Principle:** shrink the surface before connecting it. Connecting a screen you
are about to delete is pure waste.

**Current state:** 420 routes · 186 admin routes · 24 admin sidebar entries ·
97 pages · 7 duplicate clusters. Laravel — the reference — has ~50 admin sidebar
entries in 10 groups, and every one of them is reachable.

**Target:** every route is either reachable from navigation or deliberately
parameterised (`/{id}`, `/{id}/edit`). Zero screens that duplicate another
screen's job.

---

## TASK B.1 — Consolidate the six user-list screens

REVIEW §1.3. Go currently has six:

| Route | Handler | Purpose |
|---|---|---|
| `/admin/users` | `AdminUsersPage` | flat user list |
| `/admin/full-user` | `AdminFullUserPage` | "unified panel" |
| `/admin/customer-list` | | customers |
| `/admin/vendor-list` | | vendors |
| `/admin/admin-list` | | staff |
| `/admin/admins` | | staff (again) |

### B.1.1 Inspect Laravel first

```bash
cat "F:/Dawa 24/Laravel/app/Livewire/Admin/FullUser/Index.php"
cat "F:/Dawa 24/Laravel/app/Livewire/Admin/FullUser/CustomerList.php"
cat "F:/Dawa 24/Laravel/app/Livewire/Admin/FullUser/VendorList.php"
cat "F:/Dawa 24/Laravel/app/Livewire/Admin/FullUser/AdminList.php"
cat "F:/Dawa 24/Laravel/app/Livewire/Admin/Users.php"
cat "F:/Dawa 24/Laravel/app/Livewire/Admin/Admins.php"
```

**The question to answer in `DECISIONS.md`:** do these Laravel screens differ in
**columns and actions**, or only in a `type` filter? Read the `render()` query
and the Blade table headers of each.

- If they differ only by filter → **one screen, one tab strip.**
- If a list genuinely has different columns (e.g. staff shows `role` and
  `last_login`; customers show `organization` and `orders_count`) → keep the
  tabs but share **one handler and one template** with a column set chosen by the
  tab.

### B.1.2 Target shape

```
/admin/users                 → tabbed list  (الكل | العملاء | الموردون | المشرفون | عملاء جدد)
/admin/users/{id}            → detail
/admin/users/{id}/edit       → edit
/admin/users/new?type=…      → create
```

Redirect the retired paths (301), do not delete them — Laravel's URLs may be
bookmarked and the sidebar spec references them:
`/admin/full-user` → `/admin/users`
`/admin/customer-list` → `/admin/users?type=customer`
`/admin/vendor-list` → `/admin/users?type=vendor`
`/admin/admin-list`, `/admin/admins` → `/admin/users?type=staff`
`/admin/full-user/new-clients` → `/admin/users?type=new`

### B.1.3 Tests

- **D1**: seed a customer, a vendor and a staff user; each tab shows exactly its
  own set and not the others
- **D3**: the list is `AsSystem` (admin, cross-tenant by design) — assert the
  `AsSystem` call carries a justifying comment
- assert each retired path returns **301** to the right target

---

## TASK B.2 — Collapse the organization aliases

`/admin/organizations`, `/admin/vendors`, `/admin/suppliers` all map to
`AdminOrganizationsPage`.

Keep `/admin/organizations` as canonical with a `?type=` filter.
301 the other two. Remove them from any navigation.

- [ ] One canonical route; two 301s; sidebar shows one entry

---

## TASK B.3 — Collapse the sponsorship aliases

`/admin/offer-sponsorships` and `/admin/offers-packages/sponsorships` render the
same thing. Laravel registers both, so keep both **as a route + 301**, not as two
live screens. Canonical: `/admin/offers-packages/sponsorships` (it sits inside the
monetisation group).

Same treatment for `/admin/saveing-products` → 301 → `/admin/saving-products`.
The misspelling is Laravel's; preserve reachability, not duplication.

- [ ] Each alias is a 301, not a second registered page handler

---

## TASK B.4 — Resolve the policies duplication (your example)

REVIEW §1.1. Two UIs write to `platform_admin.policies`:

| | Screen | Write path |
|---|---|---|
| A | `/admin/settings` tab 3 — inline editor | `POST /admin/settings/policy` |
| B | `/admin/policies` — version list | `POST /admin/policies`, `POST /admin/policies/{id}/publish` |

### B.4.1 Inspect Laravel

```bash
cat "F:/Dawa 24/Laravel/app/Livewire/Admin/Policies.php"
cat "F:/Dawa 24/Laravel/app/Livewire/Admin/Settings.php"
grep -rn "privacy_policies\|PrivacyPolicy" "F:/Dawa 24/Laravel/app/Livewire/Admin/"
```

Laravel has `/admin/policies` as its own screen **and** `/admin/settings`. Confirm
whether Laravel's settings screen also edits policies, or only `Policies.php`
does.

### B.4.2 Decision (record it, with the evidence)

Recommended: **`/admin/policies` is canonical.** It is the screen Laravel has,
and it carries versioning + publish, which the settings tab cannot express.

- Remove policy editing from `/admin/settings` tab 3
- Replace that tab's content with a short summary (active policy per key, last
  updated, edited-by) and a link to `/admin/policies`
- Delete `POST /admin/settings/policy`
- **Delete the hardcoded default policy text** in
  `internal/ui/pages/admin_settings.templ:150-155`. That text is a fabricated
  dataset (Phase A §A.3 class) — the real default belongs in a seed migration if
  it is needed at all.

### B.4.3 Tests

- **D1**: seed a policy version; `/admin/policies` shows it; `/admin/settings`
  shows its summary
- **D2**: publishing from `/admin/policies` changes `is_active` in the database
- assert `POST /admin/settings/policy` returns **404**
- assert the settings page body does **not** contain the old hardcoded Arabic text

---

## TASK B.5 — Resolve the catalog naming collision (your other example)

REVIEW §1.2. Three catalog-shaped admin screens with inconsistent labels:

| Route | Sidebar label | Page title | What it lists |
|---|---|---|---|
| `/admin/products` | كتالوج الأدوية المعتمدة | **دليل الأدوية والأصناف الأساسية** | `catalog.products` |
| `/admin/product-child` | *(not in sidebar)* | المنتجات الفرعية | `catalog.product_variants` |
| `/admin/categories` | التصنيفات والشركات | | `catalog.categories` (+ brands?) |

Plus `/admin/adv-products` and `/admin/apis-products`, both rendering zero data.

### B.5.1 Fix the naming first

**One concept, one Arabic name, everywhere** — sidebar, page title, breadcrumb,
empty state. Decide the vocabulary and write it into `DECISIONS.md`:

| Concept | Table | Canonical Arabic |
|---|---|---|
| master catalog item | `catalog.products` | **الأدوية والمستحضرات** |
| vendor's sellable variant | `catalog.product_variants` | **أصناف الموردين** |
| category | `catalog.categories` | **التصنيفات** |
| brand / manufacturer | `catalog.brands` | **الشركات المصنعة** |

Then apply it. `grep -rn "الأدوية المعتمدة\|الأصناف الأساسية" internal/ui/` and
make every occurrence agree.

### B.5.2 Decide the fate of `adv-products` and `apis-products`

```bash
cat "F:/Dawa 24/Laravel/app/Livewire/Admin/AdvProducts.php"
cat "F:/Dawa 24/Laravel/app/Livewire/Admin/ApisProducts.php"
```

Both exist in Laravel, so both survive Q1. Answer Q2: is `AdvProducts` a filtered
view of products (advertised ones)? Is `ApisProducts` products sourced from
`api_integrations`?

- If either is a **filter over `catalog.products`** → delete the separate screen,
  add the filter to `/admin/products`, 301 the path.
- If genuinely distinct → keep and connect in Phase C.

### B.5.3 Separate categories from brands

If `/admin/categories` currently shows both, split them: `/admin/categories` and
`/admin/brands` (both exist in Laravel). One screen showing two unrelated tables
is why the naming is confusing.

- [ ] One Arabic term per concept, used consistently in every file
- [ ] `adv-products` / `apis-products` decided with evidence
- [ ] Categories and brands separated

---

## TASK B.6 — Merge deletes-lists and trash-list

Six routes over one (fabricated) dataset:
`/admin/deletes-lists`, `/{model}`, `/{model}/{id}`,
`/admin/trash-list`, `/{model}`, `/{model}/{id}`.

```bash
cat "F:/Dawa 24/Laravel/app/Livewire/Admin/DeletesListsIndex.php"
cat "F:/Dawa 24/Laravel/app/Livewire/Admin/TrashListIndex.php"
```

**Read both and state the difference.** They look near-identical. Likely: one
lists tables that *support* soft delete; the other lists rows that *are* deleted.

- If that is the difference → keep both, but they share one model registry
  (built in Phase C Task C.7).
- If there is no real difference → keep `/admin/trash-list`, 301 the other three.

- [ ] The difference is documented, or the duplicate is retired

---

## TASK B.7 — Rebuild the admin navigation

REVIEW §2.9: **186 admin routes, 24 sidebar entries, and one sidebar link
(`/admin/notifications`) is a dead target that 404s.**

### B.7.1 Fix the dead links first

```bash
# regenerate the dead-target list (00_MASTER §A.5 has the command)
```
Current dead targets: `/admin/import`, `/admin/notifications`,
`/customer/branches/active`, `/ready`.

Each is either a typo (fix the href) or a missing route (register it or remove
the link). `/ready` is a health endpoint outside the UI mux — exclude it
explicitly in the scan rather than leaving it as noise.

### B.7.2 Restructure into Laravel's 10 groups

`PLAN_V5/06_PHASE_5_ADMIN.md` §5.2 carries the authoritative group structure and
Arabic labels extracted from
`Laravel/resources/views/components/layouts/admin.blade.php`. **That mapping is
still correct — reuse it.**

Rules:
1. Every **top-level** route (no `{id}`) appears in the sidebar, or is deleted.
2. Every entry is permission-aware: `if actor.Can("<key>") { … }`, so a staff
   member never sees a link that 404s for them.
3. Detail routes (`/{id}`, `/{id}/edit`) are reached from their list, not the
   sidebar.
4. Groups are collapsible; the group containing the active route starts open.

### B.7.3 Produce the reachability table

`docs/PLAN_V6/NAVIGATION.md`, one row per admin route:

| Route | Reachable from | Permission | Status |
|---|---|---|---|
| `/admin/products` | sidebar → المنتجات والمخزون | `catalog.product.view` | ✅ |
| `/admin/products/{id}` | products list row | `catalog.product.view` | ✅ |
| `/admin/adv-products` | — | — | ❌ orphan → decide |

**Every route needs a non-empty "Reachable from".** An orphan is either linked or
deleted.

### B.7.4 Group the vendor sidebar too

REVIEW §2.11: 32 flat entries. Laravel groups them. Apply the same grouping as
`PLAN_V5/00_MASTER.md` §0.7.3, in sections:

المنشأة والفروع · الكتالوج والمخزون · العروض والتسويق · الطلبات والمالية ·
الأدوات · المحتوى والسياسات

### B.7.5 Tests

- a script asserts every registered top-level route appears in
  `NAVIGATION.md` with a non-empty "Reachable from"
- the dead-target scan returns **0** (excluding `/ready`)
- **D3**: an actor without a permission does not receive the link in the HTML

---

## TASK B.8 — Delete every screen Laravel does not have

Run Q1 (`00_MASTER.md` §A.2) across all 97 pages:

```bash
ls internal/ui/pages/*.templ | sed 's|.*/||;s|\.templ||' > /tmp/go_pages.txt
# for each, search Laravel for the concept
```

Anything with no Laravel counterpart and no explicit product decision gets
deleted. Record each in `DELETED.md` with the search that proved absence.

**Do not delete** the Go-specific infrastructure pages: `error_page`,
`onboarding`, `onboarding_pending`, `password_reset`, `notifications_dropdown`,
`settings_unified`, `shell_for` consumers.

---

## PHASE B COMPLETION GATE

```bash
make check && go test ./... -race
```

- [ ] Zero duplicate clusters: one screen per job, aliases are 301s
- [ ] Policies has exactly one editor; the hardcoded default text is gone
- [ ] Catalog vocabulary is consistent across sidebar, titles and empty states
- [ ] `NAVIGATION.md` covers every admin route with a non-empty "Reachable from"
- [ ] Dead-target scan = 0
- [ ] Admin sidebar has Laravel's 10 groups; vendor sidebar is grouped
- [ ] `DECISIONS.md` records Q1/Q2 for every screen touched
- [ ] Route count **decreased**. Record the before/after number in `PROGRESS.md`.

**If the route count went up during Phase B, you did it wrong.**
