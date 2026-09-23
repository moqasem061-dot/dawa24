# Module: Notifications & Messaging

## Overview

The `notifications` bounded context handles multi-channel message dispatching (SMS, WhatsApp, Email, In-App), template rendering, and user notification inboxes.

## Schema Mapping

- **PostgreSQL Schemas:** `notifications`
- **Migrations:** `013_notifications.up.sql`
- **Tables Owned:**
  - `notifications.templates` — Localized message templates.
  - `notifications.logs` — Delivery audit logs and in-app feed records.

## Invariants & Rules

1. **Multi-Channel Dispatch:** Dispatches across `sms`, `whatsapp`, `email`, and `in_app` with tracked delivery states (`pending`, `sent`, `delivered`, `failed`).
2. **Tenant Isolation:** In-app notification feeds are isolated with `FORCE ROW LEVEL SECURITY`.
3. **One write point.** Every producer in `internal/ui` ends in
   `writeInAppNotification`, which writes the `notifications.logs` row. The
   Telegram and WhatsApp outboxes (`chatbridge`, polled by n8n) read those
   rows, so a notification that is written reaches the chat channels too, and
   one that is not written reaches nothing. There is no separate path to add.

## Who receives a notification

- **The permission is scoped to the recipient's dashboard.** An event names its
  audience with one dashboard's key; dispatch translates it through
  `rbac.Catalog.ScopedKey` for the organisation the row is about. A supplier
  buying (the shared buying surface) holds `vendor.buying.order.view`, never
  `pharmacy.order.view` — that key is not grantable in a vendor role — so its
  purchase notifications used to reach the owner only, and the chat outbox
  dropped even the buyer's own copy as `permission_revoked`. Wallet,
  organisation, document and subscription keys translate the same way in both
  directions. A key with no counterpart is left as is (owner and staff only).
- The organisation fan-out (`dispatchOrgBranchNotification`) keeps: active
  members only, branch-bound members only for their branch, couriers only for
  `vendor.delivery.view`, and owner / platform staff / holder of the key.
- The feed's unread query and the chat outbox re-check the stored key against
  the member's current grant, so the stored key must be one the member can
  hold. `TestScopedKeyResultsAreGrantable` holds every translation to that.

## Order placed

`commerce.Service.Checkout` announces every committed order through
`OnOrderPlaced` hooks (`order_hooks.go`). Both commerce services — the UI's and
the API's (JSON checkout, Smart Ordering) — are wired to
`UIHandler.AnnounceOrderPlaced`, so the checkout page, Capsule (browser and
Telegram), negotiation and Smart Ordering all notify the same way. Before
this, only the checkout page announced an order.

| Recipient | Event / key |
|---|---|
| The buyer who placed it, and buying-company members on the receiving branch | `order.placed`, `pharmacy.order.view` → `vendor.buying.order.view` for a supplier |
| Each fulfilling supplier's order team, on the fulfilling branch | `vendor.order.view` (skipped for a negotiation, which the supplier receives as `negotiation.offer`) |
| Platform staff | `admin.order.placed`, `commerce.order.view` |

A hook runs after the commit on a context that outlives the request, and a
panicking hook is logged, never turned into a failed checkout.
