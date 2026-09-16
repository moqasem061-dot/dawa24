# Module: Promo, Offers & Ads

## Overview

The `promo` bounded context handles promotional campaigns, vendor discount configurations, sponsorship tier packages, banner advertisements, and impression/click analytics.

## Schema Mapping

- **PostgreSQL Schemas:** `promo`
- **Migrations:** `009_promo.up.sql`
- **Tables Owned:**
  - `promo.offers` — Tenant-owned discount promotions (percentage or fixed).
  - `promo.offer_products` — Product mapping relationships.
  - `promo.offer_packages` & `promo.offer_sponsorships` — Paid promotion tiers and active vendor sponsorships.
  - `promo.ads` & `promo.ad_clicks` — Display advertisement banners and engagement tracking.

## Invariants & Rules

1. **Exact Minor Unit Pricing:** Percentages and fixed discounts are processed with `money.Amount` minor unit arithmetic (basis points for percentage discounts).
2. **Impression & Click Auditing:** Views and click counters are updated atomically without blocking customer reads.
3. **Tenant Row Level Security:** Vendor offers and sponsorships are isolated with `FORCE ROW LEVEL SECURITY`.

## Endpoints

- `GET /api/v1/promo/offers` — List all running promotions across the marketplace.
- `POST /api/v1/promo/offers` — Configure a new vendor discount offer.
- `GET /api/v1/promo/offers/{id}` — Get offer details and increment view count.
- `POST /api/v1/promo/offers/{id}/click` — Record click event on an offer.
- `GET /api/v1/promo/packages` — List available sponsorship tiers.
- `GET /api/v1/promo/ads` — List display advertisements.
- `POST /api/v1/promo/ads/{id}/click` — Log ad click analytics.

## Rebuild V2 changes

- **`promo.offers` is the only offer table** (062, 065). `admin_status`
  (`pending|approved|rejected`) gates commerce: storefront queries
  (`ListActiveOffers`, `ListOffersForProduct`, `ListOffersVisibleTo`) require
  `approved`. `min_order_amount` is the canonical minimum; `min_order_value`
  was dropped in 065.
- **Price authority:** `promo.EffectivePrice(listPrice, op, offer)` is the one
  resolver for what a pharmacy pays (custom_price > custom_discount_amount >
  custom_discount_percentage > offer-level discount). Order lines snapshot the
  result (063), never re-derive it.
- **Special offers merged (065):** `special_offers/_products/_locations` are
  gone. Rows migrated into `offers`/`offer_products`/`offer_location_covers`
  with `source = 'special'`; the vendor special-offer API reads only
  `source = 'special'`. Mapping contract:
  - status: `draft` → `is_draft`, `active` → `is_active`, else inactive;
  - discount fields → `discount_type`/`discount_value`;
  - dates → `starts_at`/`expires_at` (TIMESTAMPTZ);
  - **day_of_week shifted from legacy 1..7 to 0..6 (0 = Sunday)** — repo
    methods convert at the boundary (`+1`/`-1`).
- **Buyer offer rule (2026-09-13, replaces `ListOffersVisibleTo`):** one set of
  SQL predicates in `postgres/buyer_offers.go` (`offerRule`) decides which
  offers a buyer sees and may buy. `ListBuyerOffers` ANDs them (filtered,
  counted and paged in SQL) and `OfferVerdict` selects each as a column, so the
  offers board, offer detail, add-to-cart, checkout, both dashboards and Capsule
  cannot disagree. An offer is buyable when:
  1. it is live: not deleted, active, not a draft, `admin_status = approved`,
     inside its dates;
  2. its supplier is an approved `vendor|supplier|company|agency`;
  3. its branch, if it names one, still exists, is not inactive and is the
     supplier's (a deleted branch hides the offer; the supplier's offer card says
     so);
  4. it is not the buyer's own;
  5. (buying) a supplier branch it ships from holds an institutional work
     connected to the buyer branch's — the catalogue's rule;
  6. (buying) coverage for **today's weekday**: if the offer has approved active
     `offer_location_covers`, one for today whose city is the branch's or whose
     radius contains it; otherwise the supplier's weekly coverage, passed in as
     the sets `workflow.VendorBranchesServing` resolves (`OrgWideIDs` = rows with
     no branch, which cover branch-bound offers as `ServesPoint` does).
  Visitors browse rules 1–3. Sponsored offers (`item_type = 'offer'` only) rank
  first. Tested against a real schema by `TestBuyerOfferRule`.
- **Canonical projection:** `offerColumns` + `scanOffer` in
  `postgres/offers_products.go` — every offer SELECT reuses it.
