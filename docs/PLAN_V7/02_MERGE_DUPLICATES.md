# PHASE 2 — Merge the nine duplicate clusters

**Depends on:** Phase 1.
**This phase must reduce routes and pages.** Record before/after in `PROGRESS.md`.

Use the merge procedure in `00_MASTER.md` for every task. Log each in
`MERGE_LOG.md`: survivor, feature diff, what was ported, what was dropped.

---

## TASK 2.1 — Settings: 7 screens → 1

**Audit §1.2. The user has stated the preference: keep the OLD tabbed page.**

### Survivor
`/settings?tab=…` → `pages.UnifiedSettingsPage` in `settings_unified.templ`.

### To be merged in and then deleted
| Route | Template func | Data it currently loads |
|---|---|---|
| `/settings/profile` | `SettingsProfile` | `*identity.User` |
| `/settings/addresses` | `SettingsAddresses` | addresses, cities, history |
| `/settings/security` | `SettingsSecurity` | sessions, session plans |
| `/settings/organization` | `SettingsOrganization` | org, branches, members |
| `/settings/preferences` | `SettingsPreferences` | preferences |
| `/settings/payment-methods` | `SettingsPaymentMethods` | **nothing — data-less** |

### 2.1.1 Feature diff first

Before deleting anything, produce the table in `MERGE_LOG.md`:

| Feature | In old tabbed page | In new sub-page | Action |
|---|---|---|---|
| profile edit | ? | ? | |
| addresses + history | ? | ? | |
| active sessions list | ? | ? | |
| session revoke | ? | ? | |
| **max concurrent sessions / session plans** | ? | ✅ | **port to old** |
| **notification preferences** | ? | ✅ | **port to old** |
| org data | ? | ✅ | |
| branches | ? | ✅ | **do not port — see Task 2.2** |
| members / roles | ? | ✅ | |
| payment methods | ? | shell | connect during the merge |
| password change | ? | ? | |
| account deletion request | ? | ? | |

The user specifically named **notification features** and **maximum simultaneous
sessions** as things to keep from the new implementation. Those are mandatory
ports.

### 2.1.2 Merge

1. Add the missing tabs/sections to `settings_unified.templ`, reusing the
   existing tab mechanism — **do not introduce a second tab component**.
   `settings.templ`'s `settingsTabs` is deleted with the file.
2. Move the data loading into `SettingsIndex`. It already loads user, sessions,
   payment methods, wallet, transactions — extend it, and replace every
   `err == nil` swallow it contains with real error handling.
3. `SettingsPaymentMethods(lang, dir)` is data-less: connect it while merging
   (`billing.user_payment_methods` + `billing.platform_payment_methods`).
4. 301 every old sub-path to its tab:
   `/settings/profile` → `/settings?tab=profile`, etc.
5. Delete `internal/ui/pages/settings.templ` and its six handlers.
6. Keep every **POST** route working — forms post to
   `/settings/profile`, `/settings/password`, `/settings/addresses`, … and those
   endpoints must survive the merge. Only the **GET pages** collapse.

### 2.1.3 Sidebar

Customer sidebar already points at `/settings?tab=wallet`. Vendor sidebar points
at `/settings/employees` — repoint it. Audit every reference:
```bash
grep -rn "/settings/" internal/ui/ --include=*.templ | grep -v "301\|Redirect"
```

### 2.1.4 Tests

- D1: each tab renders its seeded data
- D2: each form still writes (profile, password, address, preference, payment method, session revoke)
- assert each old sub-path returns **301**
- assert `grep -rn "SettingsProfile\|SettingsSecurity" internal/ui/` returns nothing but the 301s

---

## TASK 2.2 — Branches: 3 write paths → 2

**Audit §1.3.**

### Delete outright
```
POST /settings/organization/branch              → SettingsBranchCreateSubmit
POST /settings/organization/branch/{id}/delete  → SettingsBranchDeleteSubmit
```
plus the branch list section inside `SettingsOrganization`.

`SettingsBranchCreateSubmit` invents branch codes
(`"BR-" + time.Now().UnixNano()%100000`) — check for and report rows it created:
```sql
SELECT id, code, organization_id, created_at FROM org.branches WHERE code LIKE 'BR-%';
```

### Survivors
`/customer/branches` (pharmacy pickup/delivery points) and `/vendor/branches`
(supplier branches + coverage). These are the real systems.

### The Organization tab
Keeps org data and members. For branches it shows a **read-only count with a link**:
> الفروع: 4 — [إدارة الفروع](/customer/branches)

No add/edit/delete there. One management screen per concept.

### Also fix while here
`VendorBranchDeleteSubmit` is a no-op that reports success
(`_ = id` then `"تم حذف الفرع بنجاح"`). Connect it to `orgSvc.DeleteBranch`
with the org scope, or delete the button. Same for `VendorTeamToggleSubmit`.

### Tests
- D2: creating a branch works from `/customer/branches` and `/vendor/branches`
- D2b: deleting a branch **actually deletes it**
- D3: vendor B cannot delete vendor A's branch; the row survives
- assert `POST /settings/organization/branch` returns **404**

---

## TASK 2.3 — Policies: standalone page → settings tab

**Audit §1.7. The user wants the good editor inside the tab.**

1. Move the `/admin/policies` interface (version list, editor, publish) into
   `/admin/settings` tab 3, replacing the current inline editor.
2. **Delete the hardcoded Arabic default policy text** at
   `admin_settings.templ:150-155`. If a default is genuinely needed, it belongs
   in a seed migration.
3. Keep `POST /admin/policies` and `POST /admin/policies/{id}/publish` as the
   write endpoints — they support versioning, which the settings-tab endpoint did
   not. Delete `POST /admin/settings/policy`.
4. 301 `/admin/policies` → `/admin/settings?tab=policies`.
5. Remove the `/admin/policies` sidebar entry.

### Tests
- D1: seed two policy versions → the tab lists both
- D2: publishing from the tab sets `is_active` in the database
- assert `POST /admin/settings/policy` returns 404
- assert the settings page body does **not** contain the old hardcoded text

---

## TASK 2.4 — User lists: 6 screens → 1

**Audit PART 2 #1.**

Survivor: `/admin/users` with a tab strip
(الكل · العملاء · الموردون · المشرفون · عملاء جدد) driven by `?type=`.

Before merging, read the Laravel components to see whether the lists differ in
**columns**, or only in a filter:
```bash
cat "F:/Dawa 24/Laravel/app/Livewire/Admin/FullUser/CustomerList.php"
cat "F:/Dawa 24/Laravel/app/Livewire/Admin/FullUser/VendorList.php"
cat "F:/Dawa 24/Laravel/app/Livewire/Admin/FullUser/AdminList.php"
```
If columns differ, one handler + one template with a column set chosen by tab.
Never six templates.

301: `/admin/full-user`, `/admin/customer-list`, `/admin/vendor-list`,
`/admin/admin-list`, `/admin/admins` → `/admin/users?type=…`.
Detail/edit routes keep their own paths under `/admin/users/{id}`.

---

## TASK 2.5 — The remaining five clusters

| Cluster | Survivor | 301 from |
|---|---|---|
| **Organizations** | `/admin/organizations` (with `?type=`) | `/admin/vendors`, `/admin/suppliers` |
| **Sponsorships** | `/admin/offers-packages/sponsorships` | `/admin/offer-sponsorships` |
| **Saving products** | `/admin/saving-products` | `/admin/saveing-products` |
| **Trash** | `/admin/trash-list` (+ `{model}`, `{model}/{id}`) | `/admin/deletes-lists*` — **first read both Laravel components and confirm they are not genuinely different**; record the finding |
| **Analytics** | `/admin/analytics` with tabs for views and clicks | `/admin/offers-packages/views`, `/clicks` |

For each: 301 the alias, delete the duplicate handler and template, update the
sidebar, and verify `grep -rn "<dead path>" internal/` shows only the 301.

---

## TASK 2.6 — Catalog screens: 4 → 2

**Audit PART 2 #7.**

| Route | Data today | Decision |
|---|---|---|
| `/admin/products` | real | **survivor** — the master catalog |
| `/admin/product-child` | ? | inspect: if it is `catalog.product_variants` for all vendors, keep as `/admin/products/variants`; if it duplicates `/admin/products`, delete |
| `/admin/adv-products` | **none** | inspect Laravel `AdvProducts.php` — if it is a filter over products, delete and add the filter |
| `/admin/apis-products` | **none** | inspect Laravel `ApisProducts.php` — same question |

Rule: a filter is a query parameter, not a screen.

### Naming — fix the collision the user reported

One Arabic term per concept, used in sidebar, page title, breadcrumb and empty
state:

| Concept | Table | Canonical Arabic |
|---|---|---|
| master catalog item | `catalog.products` | **الأدوية والمستحضرات** |
| vendor sellable variant | `catalog.product_variants` | **أصناف الموردين** |
| category | `catalog.categories` | **التصنيفات** |
| brand | `catalog.brands` | **الشركات المصنعة** |

Then:
```bash
grep -rn "الأدوية المعتمدة\|الأصناف الأساسية\|التصنيفات والشركات" internal/ui/
```
and make every occurrence agree.

---

## PHASE 2 GATE

```bash
make check && DATABASE_URL="..." go test ./... -race
```

- [ ] Routes **decreased** — record before/after
- [ ] Pages **decreased** — record before/after
- [ ] Nine clusters → nine single screens; every alias is a 301, not a live duplicate
- [ ] `MERGE_LOG.md` has a feature diff for each merge
- [ ] Notification features and max-concurrent-sessions are in the unified settings page
- [ ] Only two branch creation paths remain
- [ ] `grep -rn "SettingsProfile\|SettingsSecurity\|SettingsOrganization" internal/ui/` finds only redirects
- [ ] One Arabic term per catalog concept, everywhere
