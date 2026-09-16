# PHASE 7 — Customer Dashboard Completion

**Depends on:** Phase 1 (institutional filter), Phase 2 (compare), Phase 3
(purchase request), Phase 4 (ingest).
**Tasks:** 8.

Laravel's customer sidebar has 16 entries; Go has 11 working. This phase closes
the gap and adds the customer-facing detail screens Laravel has and Go flattened.

Authoritative sidebar: `00_MASTER.md` §0.7.3.

---

## TASK 7.1 — CPanel (position 2)

| Route | Laravel |
|---|---|
| `/customer/cpanel` | `Customer/Cpanel` |

"إعدادات المنشأة والحساب" per `/what-in`. **Inspect whether this duplicates
`/settings`** — Go already has a unified settings surface. If CPanel is a
dashboard-style summary of account + organization state rather than a settings
form, build it. If it is genuinely the same as `/settings`, alias it and record
the decision rather than building a duplicate.

---

## TASK 7.2 — Customer saving products (position 7)

| Route | Laravel |
|---|---|
| `/customer/saving-products` | `Customer/SavingProducts` |
| `/customer/saving-products/import` | `Customer/SavingProductsImport` |
| `/customer/saving-products/{id}` | `Customer/SavingProductShow` |

Per `/what-in`: "تتبع فروق الأسعار والخصومات الحية مع البحث بالأسماء المعربة
JSON" — live price-delta tracking with Arabic-name search.

That means the screen is **not** just a list: it compares the customer's saved
products against current supplier prices and shows the delta. Read
`Customer/SavingProducts.php` for the exact comparison and reproduce it.

Arabic-name search uses the Phase 1 normaliser.
Import via Phase 4. Keep the `/saveing-products` 301 alias.

---

## TASK 7.3 — Offer orders (position 11)

| Route | Laravel |
|---|---|
| `/orders/offers` | `Customer/OfferOrders` |
| `/orders/offers/{id}` | `Customer/ShowOfferOrder` |

Same decision as Phase 6 Task 6.12 — likely a filtered view of `/orders`.
Be consistent with whatever was decided there.

---

## TASK 7.4 — Offer checkout

| Route | Laravel |
|---|---|
| `/offers/{id}/checkout` | `Customer/OfferCheckout` |

Laravel has a **dedicated checkout for an offer**, separate from the cart
checkout. Go has `/checkout` only. Given the order model is offer-centric
(`commerce.orders.offer_id`), verify how Go's `/checkout` currently sets
`offer_id` — if it cannot, this screen is the missing path and must be built.

Requirements: offer summary, product lines with offer pricing, quantity limits,
`min_order_amount` enforcement, delivery branch + address selection, the
documents gate (`docsGate` already blocks checkout on missing documents),
totals via `money.Amount` (T8).

---

## TASK 7.5 — Supplier storefront sub-pages

Laravel splits the supplier profile into components:
`Customer/Supplier/{Header,About,OurBranches,Policies,Products,Reviews}`.
Go has one flat `/suppliers/{id}`.

Rebuild `/suppliers/{id}` as a tabbed page (`@components.Tabs`) with those six
sections, each reproducing its Laravel component:

| Tab | Source | Notes |
|---|---|---|
| نظرة عامة | `Header` + `About` | logo, trade name, rating, follow button |
| المنتجات | `Products` | institutional filter (Simple), coverage-aware |
| الفروع | `OurBranches` | map + list |
| السياسات | `Policies` | from Phase 6 Task 6.7 |
| التقييمات | `Reviews` | see Task 7.6 |
| — | — | contact / quote request |

---

## TASK 7.6 — Ratings: three criteria, organization name only

This closes the two complaints from AUDIT_V3 PART 5 that are still open on the
UI side. The schema is already in place (migration 075, seeded criteria).

### 7.6.1 Three-criteria rating UI

Required criteria (exact Arabic labels):
- تقييم المندوب
- تقييم السرعة
- تقييم التعامل والجودة

Each is a **5-star input**. `@components.RatingStars` exists — check whether it
supports input mode or only display, and extend it if needed.

On submit:
- write one `org.review_ratings` row per criterion
- set `org.organization_reviews.rating` = the average of the three
- set `commerce.orders.rating` / `rated_at` = the same average (Phase 1 Task 1.3)

Product/supplier review displays show **the average**, with a breakdown on hover
or expand.

### 7.6.2 Organization name only

`ListReviewsByOrg` currently selects **no name at all**, while the domain type
carries a `UserName` field.

Required: join `org.organizations` and select `trade_name`. **Remove the
`UserName` field from the domain type** — leaving a personal-name field
populated invites someone to render it later. The reviewer is identified by
their organization, never by their personal name.

Write a test asserting the review projection contains no personal-name field.

### 7.6.3 Review submission route

`internal/ui/components/review_modal.templ` posted to `/api/v1/reviews`, which
was never registered. `POST /reviews/submit` now exists in
`RegisterCustomerRoutes` — **verify the modal points at it** and that the round
trip works end to end.

### 7.6.4 Tests

- T1: averaging is exact (Phase 1 Task 1.3 already requires this)
- T6: submitting three ratings creates three rows and one averaged scalar
- T6b: the rendered review shows the organization trade name and no personal name
- T19: a review cannot be submitted twice for the same order (**verify Laravel's
  rule first** — if Laravel allows editing, port that)

---

## TASK 7.7 — Guest order tracking

| Route | Laravel |
|---|---|
| `/tracking` | `Front/GuestOrderTracking` |

Public, signed-out. A visitor enters an order number and sees status.

**Security:** an order number alone must not expose customer details. Read what
Laravel reveals and match it, but if Laravel leaks personal data, record the
concern in `docs/modules/commerce.md` and still port it (rule 7) — then raise it
as a post-cutover fix. Add rate limiting regardless: this is an unauthenticated
enumeration surface.

---

## TASK 7.8 — Customer detail & remaining screens

| Route | Laravel | Notes |
|---|---|---|
| `/customer/add-order` | `Customer/AddOrder` | inspect — a manual order entry path? |
| `/customer/products/main/{id}` | `CustomerShowProducts` | Laravel routes both `/products/{id}` and `/products/main/{id}` to the same component — reproduce or alias |
| `/customer/automation-previous` | Phase 3 | |
| `/customer/job-opportunities/{id}` | `ShowJobOpportunities` | Go has `/jobs/{id}` public — verify the customer-authenticated variant adds anything (apply button state, saved jobs) |
| `/customer/favorite` | ✅ `/favorites` | verify parity of filters |
| `/customer/notifications` | ✅ `/notifications` | |

Also reproduce Laravel's customer header components:
`Partials/{Header,TopBar,GlobalSearch,HeaderCounts,Notifications,LanguageSwitcher,Footer}`.
`HeaderCounts` in particular drives the cart/notification/favourite badges —
verify Go's shell shows the same counts and that they update.

---

## PHASE 7 COMPLETION GATE

```bash
make check && go test ./... -race
```

- [ ] All 16 customer sidebar entries present, in Laravel's order
- [ ] Three-criteria rating works end to end; reviews show organization name only
- [ ] `UserName` is gone from the review projection
- [ ] Supplier profile has all six Laravel sections
- [ ] Offer checkout sets `commerce.orders.offer_id` correctly
- [ ] Guest tracking is rate-limited
- [ ] Header counts match Laravel's
- [ ] Dead-target scan = 0
- [ ] `PROGRESS.md` updated for 7.1–7.8
