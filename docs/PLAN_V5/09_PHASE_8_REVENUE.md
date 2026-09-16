# PHASE 8 — Revenue Surfaces: Offer Packages, Sponsorships, Promotions, Ads

**Depends on:** Phase 5 (admin shell, permission gates), Phase 6 (vendor shell).
**Tasks:** 6.

## Why this phase exists

**Nine Go tables are fully built and no route touches them:**

`promo.offer_packages` · `promo.offer_sponsorships` · `promo.offer_promotions` ·
`promo.offer_views` · `promo.offer_clicks` · `promo.offer_location_covers` ·
`promo.ads` · `promo.ad_plans` · `promo.ad_clicks`

These are the platform's monetisation surfaces. The schema work is done; the
product does not exist. Laravel has 26 routes across them (11 admin
offers-packages + 15 ads/ad-plans/session-plans) plus 5 vendor routes.

Laravel services to port: `OfferSponsorshipService` (194 lines),
`OfferRotationEngine` (110), `OfferAnalyticsService` (71),
`OfferViewTrackingService` (87), `OfferClickTrackingService` (53),
`OfferLocationService` (291).

---

## TASK 8.1 — Offer packages (باقات العروض)

### Inspect
```bash
cat F:/Dawa\ 24/Laravel/app/Livewire/Admin/OffersPackages/{Index,PackagesIndex,PackagesShow}.php
cat F:/Dawa\ 24/Laravel/app/Livewire/Employee/OffersPackages/{Index,PackagesShow}.php
cat F:/Dawa\ 24/Laravel/app/Models/Offers/*.php
sed -n "/CREATE TABLE \`offer_packages\`/,/ENGINE=/p"        F:/Dawa\ 24/u924222867_Testv5.sql
sed -n "/CREATE TABLE \`offer_package_features\`/,/ENGINE=/p" F:/Dawa\ 24/u924222867_Testv5.sql
```

`offer_package_features` is missing in Go (audit §6.1) — add it.

### Screens
| Route | Audience | Laravel |
|---|---|---|
| `/admin/offers-packages` | admin | `OffersPackages/Index` — the hub with stats |
| `/admin/offers-packages/packages` | admin | `PackagesIndex` |
| `/admin/offers-packages/packages/{id}` | admin | `PackagesShow` |
| `/vendor/offers-packages` | vendor | `Employee/OffersPackages/Index` |
| `/vendor/offers-packages/{id}` | vendor | `Employee/OffersPackages/PackagesShow` |

Vendor view: available packages, what each grants, current subscription, purchase.
Admin view: package CRUD, feature CRUD, subscriber list.

**Permissions** (Laravel keys → Go keys, mapped in Phase 0 Task 0.2.3):
`offer_plans_view` / `_create` / `_update` / `_delete` / `_manage`.

---

## TASK 8.2 — Sponsorships (الرعايات)

### Screens
| Route | Audience | Laravel |
|---|---|---|
| `/admin/offer-sponsorships` | admin | `OffersPackages/SponsorshipsIndex` |
| `/admin/offer-sponsorships/{id}` | admin | `SponsorshipsShow` |
| `/admin/offers-packages/sponsorships` (+ `{id}`) | admin | same components — Laravel registers both paths; alias them |
| `/vendor/offers-packages/sponsorships` | vendor | `Employee/…/SponsorshipsIndex` |
| `/vendor/offers-packages/sponsorships/{id}` | vendor | `Employee/…/SponsorshipsShow` |

### Business logic — `OfferSponsorshipService` (194 lines)
Read it and record in `docs/modules/promo.md`:
- what sponsoring an offer does to its ranking
- the sponsorship lifecycle (request → payment → active → expired)
- whether sponsorship is per-offer or per-organization
- the interaction with `org.organizations.is_sponsored`, which
  `ListOffersVisibleTo` already orders by (`ORDER BY vo.is_sponsored DESC`)

**The ordering hook already exists in the canonical visibility query.** Whatever
sponsorship does to ranking must flow through that column or an equivalent —
`00_MASTER.md` §0.14 forbids forking `visibility.go`.

### `OfferRotationEngine` (110 lines)
Sponsored offers rotate so one vendor does not permanently own the top slot.
Read the algorithm and reproduce it. It affects the visibility query's ordering,
so implement it as a deterministic, testable ranking input — not a `random()` in
SQL, which is untestable.

---

## TASK 8.3 — Promotions (الحملات الترويجية)

| Route | Audience | Laravel |
|---|---|---|
| `/admin/offers-packages/promotions` (+ `{id}`) | admin | `PromotionsIndex`, `PromotionsShow` |
| `/vendor/offers-packages/promotions` | vendor | `Employee/…/PromotionsIndex` |

Public click tracking already exists in Laravel:
`GET /promotions/track-click/{offer}/{promotion?}` (`routes/web.php:213`).
Reproduce it as a public route that records to `promo.offer_clicks` and
redirects. **Rate-limit it** and guard against open redirect — the destination
must come from the stored promotion, never from a query parameter.

---

## TASK 8.4 — Ads & ad plans (إدارة الإعلانات)

### Screens
| Route | Audience | Laravel |
|---|---|---|
| `/admin/ads` | admin | `Ads/Index` |
| `/admin/ads/create` | admin | `Ads/Create` |
| `/admin/ads/{id}` | admin | `Ads/Show` |
| `/admin/ads/{id}/edit` | admin | `Ads/Edit` |
| `/admin/ads/{id}/action` | admin | `Ads/Action` — inspect what "action" means (approve/reject?) |
| `/admin/ad-plan` (+ create, `{id}`, `{id}/edit`) | admin | `AdPlans/*` |
| `/vendor/ads` | vendor | `Employee/Ads/Index` |
| `/vendor/ads/add` | vendor | `Employee/Ads/Create` |
| `/vendor/ads/{id}` | vendor | `Employee/Ads/Show` |
| `/vendor/ads/{id}/edit` | vendor | `Employee/Ads/Edit` |
| `GET /ads/click/{ad}` | **public** | `routes/web.php:198` — click tracking + redirect |

### Requirements
- Ad creative upload through `internal/platform/storage`
- Placement/slot model — read `Models/Ads/*` for what placements exist
- Approval workflow (`admin_status` mirrors the offers pattern)
- Budget/scheduling from `ad_plans`
- Public click endpoint: record to `promo.ad_clicks`, then redirect.
  **Open-redirect guard**: the target comes from the stored ad row only.
  Rate-limit per IP.
- Ads render on the public home page — wire them into `HomePage`, matching where
  Laravel places them (read `resources/views/ads/`).

**Permission:** `ads_view` → `promo.ad.view` etc.

---

## TASK 8.5 — Offer analytics (views & clicks)

| Route | Laravel |
|---|---|
| `/admin/offers-packages/views` (+ `{id}`) | `ViewsIndex`, `ViewsShow` |
| `/admin/offers-packages/clicks` (+ `{id}`) | `ClicksIndex`, `ClicksShow` |

Tables `promo.offer_views` / `promo.offer_clicks` exist. `POST /offers/{id}/click`
is already registered publicly — **verify it writes to `promo.offer_clicks`**.
View tracking (`OfferViewTrackingService`, 87 lines) is missing: reproduce it,
including its deduplication rule (per session? per IP per day? read the service).

Analytics screens: time-series charts, per-offer breakdown, per-organization
totals, export. Vendors see their own; admin sees all.

**Performance:** these tables grow fast. Add appropriate indexes and consider a
rollup table if Laravel has one. Do not aggregate raw rows on every page load —
if a `/admin/analytics` query scans millions of rows, it will time out.

---

## TASK 8.6 — Offer locations

| Route | Laravel |
|---|---|
| `/admin/offers/locations` | `AdminOffersLocationsIndex` |
| `/admin/offers/{id}/locations` | `AdminOfferLocations` |
| `/vendor/offers/locations` | `EmployeeOffersLocationsIndex` |
| `/vendor/offers/{id}/locations` | ✅ exists |

`OfferLocationService` (291 lines) — read it. `promo.offer_location_covers`
exists. The missing `offer_importants` table (audit §6.1) has geographic columns
(`city_id`, `address_ar/en`, `latitude`, `longitude`, `radius`, `day_of_week`,
`time_from`, `time_to`, `status`, `admin_status`) — **determine whether
`offer_importants` is actually the offer-location table under a confusing name**,
and reconcile it with `offer_location_covers` before adding anything.

---

## PHASE 8 COMPLETION GATE

```bash
make check && go test ./internal/modules/promo/... -race
```

- [ ] All nine previously-dead promo tables are read and written by real routes
- [ ] `offer_package_features` added
- [ ] Sponsorship ranking flows through the existing `visibility.go` ordering — the query is still not forked
- [ ] Rotation is deterministic and unit-tested (no `random()` in SQL)
- [ ] Public click endpoints are rate-limited and cannot be used as open redirects
- [ ] Analytics queries have indexes and do not scan raw rows per page load
- [ ] Vendor and admin sidebars carry entries 16–19 (vendor) and Group 6 (admin)
- [ ] `PROGRESS.md` updated for 8.1–8.6
