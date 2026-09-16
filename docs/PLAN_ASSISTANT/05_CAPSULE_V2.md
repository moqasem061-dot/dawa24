# Capsule v2 — as built

Built 2026-09-13 from `04_CAPSULE_V2_PLAN.md`. Supersedes `03_READ_ONLY_AGENT.md`
(decision Q7 in `DECISIONS.md`). The web drawer and the Telegram bot both call
`assistant.Service`, so everything here applies to both.

## Reading: the dataset engine

`internal/modules/assistant/datasets` declares tables, not answers. The model
never writes SQL. It picks a dataset, fields, filters, a group-by and metrics,
and `Compile` produces one parameterised statement.

| Guarantee | How |
|---|---|
| Tenant scoping | Every non-admin dataset's SQL must contain `@org` or `@user`; `validate` refuses a declaration without one. Tokens become bound parameters from the actor. |
| Same permissions as the screens | A dataset names the RBAC capability of its page (`keys(scope, rbac.BuyOrderView)` …). Individual fields may need more (`sales_lines.unit_cost` needs `vendor.earnings.view`). |
| Exact aggregates | `count`, `count_distinct`, `sum`, `avg`, `min`, `max` run in PostgreSQL over the full table. Grains `day/week/month/quarter/year`, Cairo time. |
| Least privilege in the database | Executed as `SET LOCAL ROLE dawa24_assistant_ro` inside a read-only transaction with a `statement_timeout`. The role has SELECT only, with column grants on tables that carry secrets (password hashes, delivery codes, payout details). |
| Handles, not ids | Row keys come back as HMAC handles bound to user and organisation (30 min). |

Tools: `describe_data`, `query_data` (≤200 rows / ≤500 groups), `get_record`,
`export_data` (≤50,000 rows, CSV with formula neutralising or RTL XLSX, kept 7
days in `assistant.exports`, downloadable only by its owner in the same
organisation), plus `platform_guide` and `platform_overview`.

## Buying (pharmacies and suppliers that buy)

A supplier buys on Dawa24 exactly as a pharmacy does, under `vendor.buying.*`
keys, and Capsule offers it the same buying tools and actions (2026-09-13):

- `find_offers`: supplier listings for the buying branch, each with the cart's
  availability verdict (stock, quota, coverage, institutional works).
- `list_promotions` / `offer_details`: العروض والخصومات through promo's buyer
  offer rule (`docs/modules/promo.md`), the same one the offers board, add-to-cart
  and checkout use.
- `coverage_check`, `reorder_suggestions`, `favourites_list`, `supplier_profile`,
  `smart_order_run_details`, `notifications_list`, `financial_obligations_summary`.
- Actions: `cart_add`, `offer_add`, `cart_set_quantity`, `cart_remove`,
  `place_order`, `order_cancel`, `favorite_add`, `favorite_remove`.

Each tool names the capability pair (pharmacy key, vendor.buying key), so a
supplier employee without buying keys is refused. Retired because they
contradicted the cart's rules: `market_search`, `catalog_search`,
`branch_product_availability`, `branch_quota_status`, `order_workflow_rules`.
`TestEveryProjectionRuns` executes every remaining projection against a real
schema.

Datasets per dashboard are listed in `datasets/catalog_*.go`; the prompt names
them, and `TestPromptsNameOnlyRealToolsAndDatasets` keeps the two in step.

## Acting: confirmed commands

```
model ── propose_action ──► Flow.Propose ──► assistant.pending_actions (10 min)
                                                    │  card: title, details, warnings
user ── تأكيد (web card or Telegram button) ──► Flow.Confirm
         claim (single use, CAS, before expiry)
         → assistant gate + CanAct (*.assistant.act)
         → Executor.Permitted (the route's own guards)
         → Prepare again → preview hash must match (else "stale", nothing runs)
         → Execute
```

- The model can only propose. Nothing executes without the user pressing
  confirm, and the press re-checks everything, so a role revoked in between is
  refused.
- Commands live in `internal/ui/assistant_actions_*.go` and call the same core
  functions as the HTTP handlers. Those handlers were refactored onto the cores,
  so there is one implementation per action. `TestEveryCommandMatchesItsRouteGuard`
  holds each command to its route's middleware chain.
- Permission: `pharmacy.assistant.act`, `vendor.assistant.act` and
  `platform.assistant.act`. Each implies `*.assistant.use`. Organisation owners
  hold them through the owner role. The platform `admin` role does not; a super
  admin grants it deliberately.
- Never offered: passwords, payments, withdrawals, roles, deletion. Wallet
  checkout is refused through the assistant (`place_order` accepts other
  payment methods).

| Dashboard | Commands |
|---|---|
| Pharmacy / vendor buying | `cart_add`, `offer_add`, `cart_set_quantity`, `cart_remove`, `place_order`, `order_cancel`, `favorite_add`, `favorite_remove` |
| Vendor | `shipment_update_status`, `negotiation_accept`, `negotiation_reject`, `purchase_request_respond`, `listing_update`, `stock_adjust` |
| Admin | `organization_approve`, `organization_reject`, `issue_update` |

Endpoints: `POST /api/v1/assistant/actions/{id}/confirm`, `…/cancel`,
`GET /api/v1/assistant/exports/{token}`.

## Runtime

Up to 4 tools in parallel per round, 10 rounds, 30 calls, 150 s per turn. The
prompt (`prompt.go`, `2026-09-13.v2.1`) is English with Arabic replies: answer
first, short, say plainly when data is missing, never invent.

The model is configuration: `GATEWAY_MODEL_ASSISTANT_PRIMARY`. Choose it by the
eval corpus (`evals/`, 233 cases with dataset and action expectations). Per-org
spend is capped upstream by the gateway's virtual-key quota.

## Verification

- `datasets`: soundness, forbidden columns, tenant predicate, injection, field
  permissions, limits. `TestEveryPlanIsValidSQL` runs `EXPLAIN` on every compiled
  plan against a real schema. With `ASSISTANT_DATASETS_ROLE=dawa24_assistant_ro`
  it runs as the role, which catches a missing grant.
- `postgres.TestDatasetsNeverCrossTenants`: seeds two pharmacy/supplier pairs
  whose every text value carries a marker. It runs every dataset, every
  group-by, `get_record` and an export as each side through the real registry.
  One foreign marker is a failure, and each side's own rows must appear.
- `postgres.TestPendingActionsAreSingleUseAndOwnerScoped`, `actions/flow_test.go`
  (concurrent confirms execute once; stale preview, expiry and revocation refuse).
- Telegram: `internal/modules/telegram/actions_test.go`.

## Deploying

1. Migrations 214–216. **214 creates the cluster-wide role
   `dawa24_assistant_ro`** (NOLOGIN, BYPASSRLS, SELECT only). The migrating
   user must be allowed to create roles; the app user already is.
2. No new environment variables.
3. n8n: "Dawa24 Telegram · Capsule Assistant" version `8aef00c2` (published
   2026-09-13; the read-only-era version is `789297d7`). It adds confirm/cancel buttons, button-press answers and
   file delivery, and raises the timeout to 180 s. The export download is pinned
   to `https://dawa24-store-u74003.vm.elestio.app/api/v1/integrations/telegram/exports/`.
   On another domain, change that prefix in **Fetch Export**.
4. Grant `*.assistant.act` to non-owner roles only where wanted.

## Not done yet (plan phases 0 and 5)

- The 18-month fixture seed and the ~600-case graded corpus. The model bake-off
  needs gateway access and runs against the corpus above.
- Admin observability screen; load test; more datasets and commands chosen by
  usage.
