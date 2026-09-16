# The buying surface

## Overview

The catalogue, the supplier directory, the offers board, the purchase request,
Smart Ordering, the cart, the checkout and the buyer's own orders are **one
surface used by two dashboards**. A pharmacy buys through them. A supplier
sells — and also buys, from other suppliers, through exactly the same screens.

There is one implementation of each page and there must stay one. A second copy
under `/vendor/*` would be a second set of coverage rules, a second cart, and a
second place for the self-supply refusal to be forgotten.

## Where it lives

| Concern | File |
|---|---|
| Which page is which, and the key each dashboard grants it under | `internal/platform/rbac/capability.go` |
| Who may be on the surface at all | `authctx.RequireBuyer` |
| Which page of it, per caller | `authctx.RequireCapability` |
| The routes | `internal/ui/buying_routes.go` (+ `RegisterSmartOrderRoutes`) |
| Whose stock is the caller's own | `internal/ui/buying_context.go` |
| The supplier's sidebar section | `rbac.vendorBuyingNav` (شراء المنتجات) |

## Invariants

1. **A company never buys from itself.**
   `commerce.CheckAvailability` refuses `CustomerOrgID == VendorOrgID` with
   `ReasonOwnOrganization`, before it loads anything. Every purchase surface
   calls it, so cart-add, cart render, catalogue cards and checkout
   re-validation all inherit the refusal.

   Smart Ordering has enforced the same invariant since it was written, as
   `smartorder.ReasonOwnOrg`.

   Refusing is the control. The listings *also* filter, because the requirement
   is that a supplier never sees its own stock offered back to it: catalogue
   cards and promo rows (`offersForProduct`), the offers board and one offer
   (`OffersPage`, `OfferDetailPage`), the supplier directory, the followed list,
   the supplier profile, and the cart (`commerce.Service.GetCart`).

2. **A cart line is hidden, not deleted.**
   A cart belongs to a user, and a user may be a member of two companies. A line
   added while acting for a pharmacy is a perfectly good line; it is only
   unbuyable while the same person is acting for the supplier that sells it, and
   it comes back when they switch companies. `GetCart` takes the buying
   organisation and omits its own lines, so checkout sees exactly what the screen
   shows — a hidden line that checkout could still see would fail the whole order
   over something nobody could find.

3. **One capability, two keys.**
   A route on this surface is gated on a `rbac.Capability`, never on a
   permission string: the capability resolves to `pharmacy.*` for a pharmacist
   and `vendor.buying.*` for a supplier's buyer. Keys stay namespaced by
   dashboard so `Catalog.Restrict` remains a real boundary — a supplier's owner
   cannot grant a `pharmacy.` key by any route.

4. **The receiving branch decides.**
   Coverage, distance and orderability are all computed against the branch the
   caller is buying for (`buyingBranchID`, `buyingBranchCoords`). The selector is
   in both shells, because a supplier restocking has warehouses to receive into
   just as a pharmacy has branches.

## URLs

The buying pages keep the `/customer/` prefix they were written with
(`/customer/catalog`, `/customer/purchase-request`, `/customer/smart-order/*`).
That prefix is a URL, not an audience: what decides who may open one is
`RequireBuyer` plus the capability gate, and nothing else. Renaming them would
have touched a hundred template links for no behavioural gain.

`/catalog`, `/offers`, `/suppliers` remain **public**, guarded by the
anti-scraping meter, for signed-out browsing and search engines. Both sidebars
point at the gated `/customer/*` copies instead, because a permission cannot
hide a link whose destination anyone may open.

## Permissions added

`vendor.buying.*` — catalog.view, purchase_request.view/create,
smart_order.view/run, cart.use, order.view/create/update, offer.view,
supplier.view/follow, review.write, favorite.view/manage.

A supplier's owner holds them from the moment the catalogue declares them
(`IsOrgOwner` resolves to `KeysFor(scope)` on every request). The starter
`org_manager` role is seeded with the full set, `org_accountant`,
`org_warehouse` and `org_pharmacist` with read-only slices. **Roles that already
exist keep the grants they have**: `EnsureCompanyRoles` writes grants once, at
creation, so an owner's edits are never undone — which means an existing
company's custom roles must be granted the new keys by hand in the role editor.

## Audits

- `internal/ui/sidebar_route_sync_test.go` — every sidebar item, in every
  dashboard, requested through the real router by a caller holding its
  permission and by one holding everything else. This is what "the route and the
  sidebar item are synchronised" means, checked rather than asserted.
- `internal/ui/route_guard_audit_test.go` — every route registration in
  `internal/ui` (discovered, not globbed by filename) sits behind a permission
  gate or is listed with a reason.
- `internal/platform/rbac/rbac_test.go` — both keys of every capability are
  declared and grantable in their own scope.
- `internal/ui/audience_separation_test.go` — the shared pages are reachable by
  a supplier who holds the buying grants and refused to one who does not.
