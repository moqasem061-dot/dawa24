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
