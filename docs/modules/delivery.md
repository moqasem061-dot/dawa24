# إدارة الشحنات — the delivery representative

## Overview

A supplier's parcel is carried to a pharmacy by a **مندوب توصيل**, a delivery
representative. Until now that person had no account. The supplier opened
`/delivery`, typed a waybill number, and shared the resulting link.

That page authenticated nobody. Anyone holding the tracking number could read
the receiving pharmacy's address and phone number, see the cash due at the door,
and submit the form that closed the order. It also had no notion of *whose*
parcel it was, so a supplier could not say who was carrying what, and a courier
could not be shown their own work.

The representative is now an **organization role** — `org_courier`, seeded into
every supplier company — and the page is a screen on the supplier's own
dashboard, at `/vendor/delivery`. A parcel is assigned to a person; that person
signs in and sees the parcels assigned to them, oldest assignment first.

## Where it lives

| Concern | File |
|---|---|
| The role, and that it is supplier-only | `rbac.OrganizationRoles` (`org_courier`, `TenantScopes`) |
| What the role holds | `rbac.orgRoleGrants[ScopeVendor]["org_courier"]` |
| The three permissions | `rbac.vendorDeliveryPerms` |
| The sidebar section | `rbac.vendorDeliveryNav` (إدارة الشحنات) |
| Seeding the role into companies that predate it | `rbac.SeedMissingSystemRoles` |
| Assignment columns | `db/migrations/188_shipment_courier_assignment.up.sql` |
| Who is carrying what | `commerce/postgres/courier_assign_repo.go` |
| The four board queues | `commerce/postgres/courier_queue_repo.go` |
| Ownership rules | `commerce/courier_service.go` |
| What the courier must collect | `commerce.OrderShipment.CourierCollection` |
| Which employees are couriers | `org.Repository.ListMembersHolding` |
| Routes | `ui.registerVendorDeliveryRoutes` |
| The board and the parcel screen | `ui/pages/vendor_delivery*.templ` |

## The three permissions

They separate three different jobs, and the separation is the point.

| Key | Who | What it opens |
|---|---|---|
| `vendor.delivery.view` | a مندوب, a dispatcher | the board |
| `vendor.delivery.update` | a مندوب | advancing and closing **their own** parcels |
| `vendor.delivery.assign` | a dispatcher, an owner | deciding whose round a parcel is on |

Neither of the last two implies the other. A courier cannot move work onto their
own round; a dispatcher who never leaves the office cannot sign for a delivery
they did not make.

`.assign` implies `vendor.order.view`, because the assignment control also lives
on أوامر التوريد, where the parcel was created — a grant that revealed a control
on a page the holder could not open would be a grant that does nothing.

## Invariants

1. **A parcel is shown to, and closable by, the representative it was assigned
   to.**
   `Service.GetCourierShipment(ctx, shipmentID, orgID, mustOwn)` is the single
   gate. A مندوب passes their own user id and is confined to their round; a
   dispatcher passes zero and supervises the whole board. Every courier action —
   opening the screen, advancing the parcel, the handover — goes through it, so
   the rule is stated once rather than three times.

2. **Ownership by the supplier is a SQL predicate, not a check on the result.**
   `GetVendorShipment` and `AssignShipmentCourier` both carry
   `organization_id = $2` in their `WHERE`. A member of company A asking for
   company B's shipment id gets "not found", which is the only answer that does
   not confirm the parcel exists.

3. **The assignment date is set by the database.**
   It is what the queue is ordered by, so a clock supplied by a request would let
   a parcel jump the queue. Reassigning re-stamps it: the new courier has been
   holding the parcel since they were handed it, not since it left the warehouse.
   Unassigning clears it, so a parcel returned to the pool does not carry a stale
   age into somebody else's round.

4. **A courier cannot mark a parcel delivered by asserting it.**
   `AdvanceCourierShipment` refuses `delivered` and `completed` outright. Closing
   a parcel needs the pharmacy's own six-digit code, through
   `CompleteCourierDelivery`. The status buttons are a claim the courier makes
   about themselves; the handover is a claim about the pharmacy.

5. **What the courier is told to collect is what the ledger records.**
   `OrderShipment.CourierCollection()` is the one statement of the rule — wallet
   orders owe the delivery fee and nothing more, cash-on-delivery owes the whole
   invoice, everything else owes nothing. The parcel screen renders it and the
   completion transaction records it. They used to be two copies, and a wallet
   order with free shipping told the courier to collect nothing while the ledger
   wrote the delivery fee.

6. **The assignable list is resolved by permission, not by role key.**
   `ListMembersHolding(orgID, "vendor.delivery.view")` finds the members whose
   company role grants the portal. A supplier that renames مندوب توصيل, or builds
   "مندوب المنطقة الشرقية" from scratch and grants it the delivery keys, has made
   a delivery representative — a query for `role_key = 'org_courier'` would not
   find them and would silently omit half the team. The posted value is
   re-resolved against the same list on submit, so a forged form cannot assign a
   parcel to another company's employee.

7. **A courier's landing page is their portal.**
   `org_courier` deliberately does not hold `vendor.dashboard.view`: the supplier
   dashboard reports the company's sales. `ui.dashboardLanding` walks the sidebar
   the shell would render and takes its first real link, so a courier lands on
   إدارة الشحنات and an ordinary member still lands on the dashboard.

## The four queues

| Queue | Predicate | Order |
|---|---|---|
| `mine` | assigned to the caller, still open | assignment date, oldest first |
| `completed` | assigned to the caller, closed | delivered date, newest first |
| `unassigned` | nobody is carrying it, still open | order date, oldest first |
| `all` | every open parcel in the company | assignment date, oldest first |

The last two require `.assign`. Asking for one without it is a stale bookmark
rather than an attack, so the handler falls back to the caller's own round: they
are not forbidden the page, only that view of it.

The open queues sort ascending because that is what "what should I deliver
first" means. Completed work sorts descending because it is a receipt, not a
queue. Unassigned parcels have no assignment date, so they fall back to when the
order arrived — the same rule applied to the only timestamp that exists.

## Legacy behaviour preserved

The handover itself is unchanged: the same six-digit PIN, the same five-attempt
lockout for fifteen minutes, the same parent-order synchronisation when the last
shipment of an order is delivered. `VerifyAndCompleteDelivery` in
`commerce/postgres/delivery_verification_repo.go` is the same transaction it
always was; what changed is that its caller now proves who is asking.

`GET /delivery` answers `301` to `/vendor/delivery`, so links a supplier already
shared do not dead-end. `POST /delivery/verify` is gone — it is the endpoint that
let a waybill number close an order.
