# Module: Workflow

## Overview

The `workflow` bounded context manages background purchasing priority optimization, geographic branch route coverage, and defect/support ticket tracking.

## Schema Mapping

- **PostgreSQL Schemas:** `workflow`
- **Migrations:** `010_workflow.up.sql`, `090_purchase_priority_engine_columns.up.sql`
- **Tables Owned:**
  - `workflow.purchase_priority_engines` — Bulk order supplier selection and AI ranking calculations.
  - `workflow.weekly_coverages` — Daily branch delivery routes and coverage zones.
  - `workflow.report_issues` — Customer issue and defect reports.

## Invariants & Rules

1. **Purchasing Optimization:** Supports constraint weighting across highest discount, lowest price, fastest delivery, and preferred suppliers.
2. **Weekly Route Schedules:** Delivery schedules per branch are isolated per tenant with `FORCE ROW LEVEL SECURITY`.
3. **Branch Weekly Locations Decision (Phase 0 Task 0.1.1):** In Laravel, `branch_weekly_locations` and `weekly_coverages` represented overlapping models. In the Go schema, `workflow.weekly_coverages` (with spatial extensions from migration 050) completely captures all necessary attributes: `organization_id`, `branch_id`, `city_id`, `day_of_week` (0 = Sunday .. 6 = Saturday), `coverage_from`, `coverage_to`, `address`, `latitude`, `longitude`, `distance_meters`, and `is_active`. Therefore, a separate `branch_weekly_locations` table is redundant and omitted.
4. **Day of Week Numbering:** Go standardizes on `0 = Sunday` through `6 = Saturday` (aligned with Go's `time.Weekday`).
5. **Coverage Window & Visibility:** `promo/postgres/visibility.go` performs spatial radius checks using `platform.distance_meters` against active weekly coverage records for the matching weekday.

## Purchase Priority Engine (محرك أولوية الشراء — Plan V5 Task 3.2)

### 1. Status Lifecycle
- Transitions: `pending` -> `processing` -> `completed` (or `failed`).
- Request number generation format: `PPE-YYYY-RANDOM8` (e.g. `PPE-2026-AB12CD34`).

### 2. Candidate Filtering (`getProductsByPriorities`)
- Reads from `catalog.product_index` where `status = 'active'` and `stock_quantity > 0`.
- **Institutional Work Filter (Simple Mode 0):** `ARRAY_LENGTH(institutional_work_ids, 1) IS NULL OR institutional_work_ids && $authorizedWorkIDs`. Unrestricted products are visible to all pharmacies under Simple mode.
- **Budget Constraint:** `price <= budget OR (price - discount) <= budget`.
- **Preferred Suppliers:** `organization_id = ANY($preferredSuppliers)`.
- **Limit:** 1,000 items.

### 3. AI Scoring & Ranking Formula (`applyAIRanking`)
The composite score $S \in [0, 100]$ evaluates each candidate product against enabled preferences:

- **Discount Score (0 - 30 pts):**
  - $\text{Discount } \% \ge 50\% \implies 30 \text{ pts}$
  - $\text{Discount } \% \ge 30\% \implies 25 \text{ pts}$
  - $\text{Discount } \% \ge 20\% \implies 20 \text{ pts}$
  - $\text{Discount } \% \ge 10\% \implies 15 \text{ pts}$
  - $\text{Discount } \% \ge 5\% \implies 10 \text{ pts}$
  - $\text{Discount } \% > 0\% \implies 5 \text{ pts}$
  - Otherwise $0 \text{ pts}$.

- **Price Score (0 - 30 pts):**
  - If $\text{maxPrice} = \text{minPrice} \implies 15 \text{ pts}$
  - Else $\text{pos} = \frac{\text{maxPrice} - \text{finalPrice}}{\text{maxPrice} - \text{minPrice}}$; $\text{score} = \text{round}(\text{pos} \times 30, 2)$.

- **Delivery Score (0 - 25 pts):**
  - $\text{Delivery } \le 1 \text{ day} \implies 25 \text{ pts}$
  - $\text{Delivery } \le 2 \text{ days} \implies 20 \text{ pts}$
  - $\text{Delivery } \le 3 \text{ days} \implies 15 \text{ pts}$
  - $\text{Delivery } \le 5 \text{ days} \implies 10 \text{ pts}$
  - $\text{Delivery } \le 7 \text{ days} \implies 5 \text{ pts}$
  - Otherwise $0 \text{ pts}$.

- **Preferred Supplier Score (0 - 15 pts):**
  - If $\text{organization\_id} \in \text{PreferredSuppliers} \implies 15 \text{ pts}$, else $0 \text{ pts}$.

### 4. Recommendations & Budget Impact (`generateRecommendations`)
- Ranks candidate products descending by total score.
- Iterates over candidates, deducting final price from remaining budget.
- Produces up to top 20 recommendations with budget impact (`price`, `remaining_after`, `percentage_of_budget`) and human-readable recommendation reasons.

### 5. Web UI & API Endpoints
- `GET /customer/purchase-priority` — Priority configuration form and run history.
- `POST /customer/purchase-priority/run` — Execute engine and render recommendations.
- `GET /customer/purchase-priority/{id}` — Recommendation details & budget breakdown.

