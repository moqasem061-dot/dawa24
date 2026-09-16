# PLAN V5 — Open Questions

Record every decision you had to infer rather than read. Format:

```markdown
## Q<N> — <one-line question>

**Phase/Task:** <N.M>
**Laravel evidence read:** <file:line, what it showed>
**Interpretations considered:**
1. …
2. …
**Chosen:** <which, and why>
**If wrong, what changes:** <blast radius>
**Status:** open | resolved-by-<who/what>
```

Do not stall a phase on an open question. Implement your best interpretation,
record it here, and continue. Never mark a feature complete-but-stubbed.

---

## Q1 — Does the compare engine keep its own plan family, or reuse billing.plans?

**Phase/Task:** 0.6.3, resolved in 2.1
**Laravel evidence read:** `compare_discount_plans`, `_plan_features`,
`_plan_requests`, `_plan_subscriptions`, `_plan_subscription_users`,
`_user_sessions` exist as a distinct family in `u924222867_Testv5.sql`.
Go's `compare_handlers.go:53` subscribes against `billing.plans` with a
`"compare"` string.
**Interpretations considered:**
1. Keep the string shortcut; add limits as billing plan features.
2. Build the separate `compare` schema Laravel has.
**Chosen:** 2 — the shortcut cannot express the 8/22 archive limits or the
device cap, both of which `/what-in` advertises.
**If wrong, what changes:** migration 087 and the entitlement service.
**Status:** resolved — see Phase 2 Task 2.1. Migration 087 must also move any
subscriptions taken through the interim `billing.plans` path.

---

## Q2 — Should commerce.orders.user_address_id reference live user_addresses or immutable user_address_histories?

**Phase/Task:** 0.6.2
**Laravel evidence read:** `MainOrder.php:201` defines `belongsTo(UserAddress::class, 'user_address_id')`. `UserAddress.php:127` logs every creation/update/deletion to `UserAddressHistory`.
**Interpretations considered:**
1. Point to `identity.user_addresses(id)` directly (matches Laravel FK target).
2. Point to `identity.user_address_histories(id)` snapshot (as done in Go migration 063).
**Chosen:** 2 — An order is a legal point-in-time commercial transaction. Referencing `user_address_histories` guarantees that later updates to a user's delivery address do not retroactively alter the delivery address recorded on historical order receipts.
**If wrong, what changes:** Foreign key definition in `commerce.orders`.
**Status:** resolved — documented in `docs/modules/commerce.md`.

---

## Q3 — Should catalog.saving_products be reinstated alongside promo offers?

**Phase/Task:** 0.6.1
**Laravel evidence read:** `SavingProduct.php` model with `qty`, `price`, `organization_id`, `user_id`, and `product_id`. The `/what-in` page lists منتجات التوفير as a separate pillar from promo offers.
**Interpretations considered:**
1. Keep saving products dropped and treat all discounts as promo offers.
2. Reinstate `catalog.saving_products` as a dedicated customer/vendor savings tracking table with RLS.
**Chosen:** 2 — Reinstate `catalog.saving_products` via migration `083_saving_products.up.sql`.
**If wrong, what changes:** Drop `catalog.saving_products`.
**Status:** resolved — migration 083 created and documented in `docs/modules/catalog.md`.

