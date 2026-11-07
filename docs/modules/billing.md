# Module: Billing, Payments & Entitlements

## Overview

The `billing` bounded context manages user and tenant wallets with an append-only double-entry transaction ledger, payment records, unified subscription plans, and feature entitlement gating.

## Schema Mapping

- **PostgreSQL Schemas:** `billing`
- **Migrations:** `007_billing.up.sql`
- **Tables Owned:**
  - `billing.wallets` — User balance account header.
  - `billing.wallet_transactions` — Append-only ledger recording credits and debits.
  - `billing.payment_integrations` — Configuration for payment processors.
  - `billing.payments` — Payment records.
  - `billing.plans` & `billing.plan_features` — Subscription tiers and entitlements.
  - `billing.subscriptions` — Active user subscriptions (unifying the 4 legacy subscription systems from defect D7).

## Invariants & Rules

1. **Immutable Wallet Ledger:** Wallet balances are strictly projections calculated from the append-only `wallet_transactions` ledger. No mutable balance columns exist.
2. **Strict Overdraft Rejection:** Any withdrawal or charge that would reduce a wallet below zero is rejected with `422 Unprocessable Entity` (`wallet.insufficient_funds`).
3. **Legacy D7 Subscription Unification:** All four legacy subscription tables (`subscriptions`, `subscription_plans`, `subscription_users`, `subscription_histories`) collapse into a unified model with `source_system` and `source_id` provenance columns.
4. **Entitlement Resolution:** Application features verify access via `billing.Service.CheckEntitlement(ctx, userID, featureKey)`.
5. **Direct money moves need `billing.admin` (security fix 2026-09-24):** REST direct deposit, withdrawal, subscription activation and invoice settlement no longer accept "any staff"; the actor must hold `billing.admin`.

- **Platform payment channels show only their own credentials (2026-09-27):**
  `PlatformPaymentMethod.ScopeFieldsToProvider` keeps bank fields for `bank`,
  the IPA handle and phone for `instapay`, the phone for `wallet`, and nothing
  for `card`/`cash`. It runs on admin save and again before the wallet top-up
  modal renders, because the admin form only hid the other types' inputs and
  stored their values.

## Endpoints

- `GET /api/v1/billing/wallet` — Inspect user balance.
- `POST /api/v1/billing/wallet/deposit` — Credit wallet funds.
- `POST /api/v1/billing/wallet/withdraw` — Debit wallet funds.
- `GET /api/v1/billing/plans` — List active subscription plans.
- `POST /api/v1/billing/subscriptions` — Activate plan subscription.
- `GET /api/v1/billing/entitlements/{key}` — Query feature access entitlement.

## Recording invoice payments (security fix 2026-09-24)

`RecordInvoicePayment` accepts only the issuing organization (`invoices.organization_id`) or staff (organization 0). Recording a payment marks the invoice and its order paid, and the billed customer (`customer_org_id`) used to be accepted, so a pharmacy could settle its own debt by typing an amount. Admin payment routes require `billing.invoice.manage`, not the view permission.

Invoice print and Excel/Word exports (`/invoices/{id}/*`, `/orders/{id}/invoice/*`) check that the caller is staff, the issuer, the billed customer, the buyer or a fulfilling vendor (`internal/ui/invoice_access.go`). Orders are read as system, so these pages previously printed any company's order. An invoice synthesised from an order is filed only when its issuer is known; it no longer defaults to organization 1.

## Deposit rejection repair (2026-10-23)

The admin rejection lookup qualifies `d.id` after joining wallets. The previous
unqualified `id` made PostgreSQL reject every rejection attempt as ambiguous.
The pending deposit is locked before transition; rejection and its audit entry
commit together. No credit transaction is created. A real PostgreSQL regression
checks the saved reason, reviewer, audit, unchanged ledger and refusal to reject
twice or approve after rejection.

## Order wallet refunds (Batch 1B, S03)

- `Repository.CreditOrderRefund` credits back, inside the caller's transaction, the wallet money an
  order is still net-debited. The commerce repository calls it with the order row locked. It is
  wired in the composition root (`WithWalletRefunder`), so commerce never imports billing.
- The amount is taken from the ledger, never from the order total: debits of the order
  (`withdrawal`, reference `order`), less admin reversals of those debits (`reverses_transaction_id`),
  less earlier order credits (`deposit`, reference `refund`). A negative result credits nothing.
- The wallet row is locked (`SELECT ... FOR UPDATE`) before the balance is read, so refunds for one
  wallet serialise with each other and with the admin finance refund.
- A key (`idempotency_key`, migration 247) is stored on the credit row. The partial unique index
  refuses a second credit with the same key with a conflict. Old rows have no key and are unaffected.
- An order debited from two wallets is refused with an internal error and needs review. The
  current code debits one wallet per order.
- Admin finance refund (`AdminRefundTransaction`) of an order debit now locks the wallet and
  refuses the reversal when the order's remaining net debit does not cover it
  (`transaction.already_refunded`). Other transactions are unchanged.
- Known limit (S01, closed by Batch 2): ordinary debits and credits now take the wallet lock too
  (see below).

## Wallet ledger concurrency and wallet-paid purchases (Batch 2, S01)

- Every ledger write locks the wallet row (`lockWalletRow`) in the same transaction as its balance
  check: `RecordTransaction`, subscription renewal, deposit approval, withdrawal request and approval,
  admin adjustment and refund. Two writers on one wallet run one after the other. A wallet can
  not go negative and no debit is lost. Tests: `wallet_lock_integration_test.go` (20 rounds each).
- `PurchaseWalletSubscription` writes the subscription and its debit in one transaction. The
  caller's expected current subscription must still be current, or the purchase is refused with
  `subscription.changed`. A failed debit leaves no active subscription.
- `DebitOrderShipment` (commerce's shipment charge) and `WithdrawInTx` (promo's package payment) run
  inside the caller's transaction, so the debit commits with the caller's write or not at all.
  Ledger key `order-shipment-debit:<order>:<shipment>` is unique (migration 247).

## Session plans (retired 2026-11-07)

Session plans are retired. No page or route sells them: `POST /settings/security/plan/{id}`,
its handler, the wallet debit (`DebitSessionPlanInTx`) and the purchase service are removed.
`/admin/session-plan*` redirects to `/admin/plans`.

- **The sign-in limit comes from the organisation's subscription plan only**
  (`billing.plans.max_login_sessions`, plus the organisation's extra devices), as before.
- **Correction (C1).** Earlier notes said a purchased session plan raised the buyer's sign-in
  limit. That was never true: the stored value was not used at login. Nothing was ever
  charged to a wallet by a page, so no refund is due.
- **Kept as data.** `identity.session_plans`, `identity.user_security.max_login_sessions` (no
  longer written by the password path, and the security upsert no longer touches it), and the
  read fallback in `identity/service.go`. The permission keys `billing.session_plan.*` remain.
- **Not enforced.** `max_devices` is still not enforced anywhere. That is a separate ticket.

## Post-fix changes (2026-11-07)

- **Lock order (B1).** Any path that moves wallet money for a subscription locks the wallet
  first, then the subscription rows. `RenewSubscription` and `PurchaseWalletSubscription` follow
  that order, so they cannot deadlock. Covered by a 20-round test on real Postgres.
- **Renewal ledger (B8, D4).** A renewal's debit is written by `recordLedgerRow`, the same writer
  as every other debit. A failed pending-withdrawal query now fails the renewal instead of being
  ignored. A short wallet on renewal is refused with `wallet.insufficient_funds` (a validation
  error now, was a conflict before; the message is the ledger's bilingual one).
- **Invoice payments (A6).** Recording an invoice payment no longer rewrites the status of a wallet
  order, whose status comes from its shipments.
- **Wallet helper.** `latestBalance` no longer takes a `forUpdate` argument (it was always false).
