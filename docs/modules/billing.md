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

## Endpoints

- `GET /api/v1/billing/wallet` — Inspect user balance.
- `POST /api/v1/billing/wallet/deposit` — Credit wallet funds.
- `POST /api/v1/billing/wallet/withdraw` — Debit wallet funds.
- `GET /api/v1/billing/plans` — List active subscription plans.
- `POST /api/v1/billing/subscriptions` — Activate plan subscription.
- `GET /api/v1/billing/entitlements/{key}` — Query feature access entitlement.
