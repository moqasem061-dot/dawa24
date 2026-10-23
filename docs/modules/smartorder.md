# Module: Smart Ordering (نظام الطلب الذكي)

## Overview

Finalization rejects corrupt saved match decisions before order placement;
refresh cannot keep a supplier selection whose exact line amount overflows
(release hardening 2026-10-15). Missing legacy decisions still use the documented
revalidation path. Regression tests cover corrupt JSON and exact amount overflow.

A pharmacy uploads a spreadsheet of what it needs. The system resolves every row
to a catalogue product, finds the vendors who can actually deliver it, picks one
per line under the buyer's criteria, and hands back an order to review and place.

It is reached from the pharmacy's **Purchase Request (طلب الشراء)** area as a
third way to start a request, alongside the supplier directory and cross-supplier
catalogue search. Those two are untouched.

Spec: `specs/001-smart-ordering-system/`.

## Schema Mapping

- **PostgreSQL schema:** `smartorder`
- **Migrations:** `123_drop_automation_requests`, `124_smartorder`,
  `125_match_decisions`, `126_product_index_variant_stock`,
  `127_smartorder_files`, `132_smartorder_ai_enhance`,
  `236_smartorder_match_decision`
- **Tables owned:**
  - `smartorder.runs` — one end-to-end execution, its counters and totals.
  - `smartorder.run_config` — the configuration snapshot the run executed under.
  - `smartorder.column_mappings` — file columns to target fields, with the
    automatic guess kept alongside the confirmed mapping.
  - `smartorder.run_lines` — one row per imported spreadsheet row, including
    consumer decision fields and `offer_state`.
  - `smartorder.line_candidates` — every vendor offer considered, **including the
    rejected ones and why**.
  - `smartorder.line_selections` — the chosen candidate and what decided it.
  - `smartorder.run_events` — append-only progress and audit.
  - `smartorder.criteria_profiles` — the buyer's remembered defaults.
- **Tables shared with `catalog`:** `catalog.match_knowledge` (scoped decision
  memory and alias channel), `catalog.pack_relationships` (pack conversion).

## Invariants & Rules

1. **Identity is decoupled from offers.** Product identity resolution is strictly
   independent of supplier offer availability. Once a row is matched to a
   catalogue product (`matched_product_id`), the offer stage never alters or
   clears the product ID, and never substitutes an in-stock sibling product.
   If an identified product has no eligible offers, it is marked as
   `offer_state: identified_no_offer` and retained for review.
   Offer states are strictly partitioned: `offered`, `identified_no_offer`, and
   `blocked_pack`. Offer selection only receives accepted or confirmed matches.

2. **Pack conversion requires explicit relationships.** When a requested unit or
   pack differs from a supplier's offered pack, conversion is permitted only if
   an explicit entry exists in `catalog.pack_relationships` via
   `productmatch.Convert`. Without a certified relationship, the offer cannot
   be converted automatically and the line is marked `offer_state: blocked_pack`
   and `outcome: blocked_pack`. Converted quantities are exact integer units;
   money is always exact `money.Amount`.

3. **Match memory and corrections are tenant-scoped.** When a buyer corrects a
   product match (`CorrectMatch`), learned mappings are saved scoped to that
   buyer's `organization_id` in `catalog.match_knowledge` (`human_confirm` /
   `human_reject`). Alias generation tracks the verified source and never bleeds
   cross-tenant intent.

4. **Availability is never read from `catalog.product_index`.** Supplier offers
   come from `catalog.product_variants` joined to `inventory.stocks`. The read
   model's `variant_id` and `stock_quantity` were the literal NULL and 0 for all
   28,786 rows in production, so every query filtering on them returned nothing —
   silently, for every product. Migration 126 and `catalog/jobs/reindex_sql.go`
   fix the read model; this module still reads the authoritative tables.

5. **No stage issues a query per row.** Each deterministic tier is one query for
   the whole file; supplier offers for every matched product are one query. A
   ten-thousand-row import must stay inside five minutes, and per-row work is the
   single easiest way to lose that. `pipeline/retrieval` asserts the query count
   does not scale with row count.

6. **The deterministic engine is primary; AI only enhances what it left.**
   The engine runs to completion over the whole file first. What reaches the AI
   stage is exactly the two populations the buyer would otherwise see as
   `غير مطابق` and `مطلوب للمراجعة`: no match at all, or a match below the
   **0.850 cutoff**. A line at or above the cutoff is never sent and is never
   overwritten. With AI off the run still completes and produces a finalisable
   order.

7. **Retrieval for AI is separate from, and wider than, the scorer's shortlist.**
   `productmatch.Recall` unions three strategies — shared distinctive words,
   shared character trigrams, and the molecule — with the scorer's disqualifying
   penalties removed. This is the change that made the stage worth running:
   measured on a live 1,473-row file, the scorer's own shortlist was **empty for
   25% of review lines** (which the old adjudicator therefore skipped entirely)
   and averaged 2.2 candidates; recall leaves 0.1% empty at 12 candidates each,
   and puts products like `ابيليفاي 10مجم` in front of a line typed `ابليفاى
   10مجم` that shares no whole word with it.

8. **Catalogue window with per-item options enforcement (FR-062).** Each request
   packs a deduplicated catalogue window to minimize token overhead. However,
   each item enforces its own candidate shortlist (`Item.Options`). Answers
   outside the offered options are rejected with `not_offered_for_row`.

9. **The AI stage is bounded, cached and policy-governed (US9, US10).** Batch
   items are sized dynamically based on configuration limits (`max_items_per_attempt`,
   `max_input_tokens`, `max_output_tokens`), deadlines follow row-count classes
   (30s / 90s / 180s), and attempts are capped at $\le 3$ per question. Knowledge
   is cached in `catalog.match_knowledge` with fingerprint-based staleness checks.

10. **Nothing the model says is applied unchecked (Gate.Evaluate).** Answers
    are validated against `Gate.Evaluate` (`Path = AI`). Hard identity conflicts
    refuse the answer and retain the row for human review:
    - **strength** — different dosages or concentrations are hard conflicts;
    - **line extension** — بلس, اكسترا, نايت, فورت, ريتارد, SR/CR/XR are separate products;
    - **dosage form** — explicit tablet vs explicit capsule is an intentional hard conflict
      (`ConflictForm`), preserving clinical precision across the catalogue;
    - **shared distinctive word** — manufacturer names and non-specific descriptors
      never count as identity agreement.

11. **Gate Acceptance replaces raw confidence scores.** Model self-reported scores
    are not calibrated probabilities; acceptance is strictly governed by
    `Gate.Evaluate` and policy qualification thresholds.

12. **The prompt is generated, never authored by a model.**
    `aicapabilities.RenderEnhanceInput` is a pure function of the request — no
    clock, no map iteration, no randomness — conforming to prompt v8 and contracts/ai-match-v8.md.


13. **Chain-of-thought is switched off for this capability.** The default model
    is a reasoning model: its `reasoning` output is billed and drawn from the
    same `max_tokens` budget as the answer. Two hundred product matches reasoned
    through individually exceed any output ceiling, and when they do the model
    returns an EMPTY answer with `finish_reason: "length"` — a total, silent
    failure that looks like a model with nothing to say. `gateway.budget.think`
    is false for everything except the human-facing chat.

14. **Supplier selection is strict priority within a tolerance band.** The
   highest-priority enabled criterion decides, but only among candidates within
   the tolerance (default 5%) of the cheapest eligible net price. When the band
   rejects a criterion's winner, the line names the skipped supplier and by how
   much it missed. Ties break deterministically, ending in `vendor_org_id`, so
   re-running an unchanged file selects the same suppliers.

15. **Eligibility checks are ordered, and the order is the design.** own_org →
   inactive → institutional → coverage → stock → min_qty. The **first** failure
   is reported, because it is the most actionable: an offer both out of stock and
   outside coverage is a coverage problem, and saying "out of stock" sends the
   buyer hunting for a supplier they did not need to find.

16. **Nothing is silently substituted or dropped.** Finalisation re-verifies every
   line: `Matcher.Revalidate` checks product match against current catalogue
   revision, ensuring state remains accepted/confirmed and granularity satisfies
   pack. Any catalogue change, revision drift, or changed supplier stock/coverage
   halts order placement and returns the changed lines to review (`stale`). Quantity
   cells that cannot be read (`"2-3"`, negatives, garbage) produce no quantity and
   a note, never a guess.

17. **The review cart is not the shopping cart.** No path in this module reads or
   writes `commerce.carts` or `commerce.cart_items`. An abandoned import must not
   leave items in a cart the buyer believes is empty.

18. **One order path.** Finalisation goes through `commerce.Service.Checkout`, so
   multi-vendor shipment partitioning, order numbering, status history and the
   documents gate are identical to an ordinary order.

19. **Money is `money.Amount` throughout.** Net prices, bands and line totals are
    integer minor-unit arithmetic. The one exception is `runs.ai_cost_estimate`,
    which is USD telemetry and deliberately not `money.Amount`.

20. **Row level security** on all seven tenant-owned tables via
    `platform.tenant_visible(organization_id)`. Cross-tenant catalogue reads use
    `database.AsSystem` with the marketplace justification stated at the call
    site. A run belonging to another organisation is Not Found, never Forbidden.

## Module boundaries

`smartorder` imports no other module. Coverage, Corporate Operations, branch
locations and order placement arrive as narrow function types
(`smartorder/adapters.go`) that the composition root fills in
(`cmd/server/smartorder.go`, `cmd/worker/smartorder.go`). The matching engine
lives in `internal/shared/productmatch`, promoted there from
`modules/ingest/engine` so `ingest`, `compare` and `smartorder` share one
implementation.

## Web routes

| Route | Purpose |
|---|---|
| `GET /customer/smart-order/new` | Step 1 — upload and configuration |
| `POST /customer/smart-order` | Create the run |
| `GET/POST /customer/smart-order/{id}/mapping` | Step 2 — confirm columns |
| `GET /customer/smart-order/{id}/progress` | Step 3 — live progress |
| `GET /customer/smart-order/{id}/results` | Step 4 — matching and supplier results |
| `GET /customer/smart-order/{id}/review` | Step 5 — the dedicated review cart |
| `POST …/lines/{lineID}/{quantity,supplier,remove}` | Review edits |
| `POST /customer/smart-order/{id}/finalize` | Re-verify and place |

A JSON API mirroring these lives in `internal/modules/smartorder/http`.

## Known gaps

- **Gateway budget.** Both organisations sit on a plan budgeted at 0.05 USD per
  week, which one large run exhausts. The AI toggle renders disabled until an
  organisation has a virtual key; an adequately budgeted plan is an owner action.
- **No embedding model** is published on the Gateway (`/v1/embeddings` → 404), so
  the semantic recall channel is the structural identity key rather than vectors.
  The `CandidateProvider` seam is where an embedding implementation would go.
- **Uploaded files are held in memory** between step 1 and step 2, with a TTL. On
  a multi-instance deployment a buyer can land elsewhere and be asked to re-upload.
  The chunked-upload path in `ingest` is the answer when that starts to matter.
- **History** shares the import screen rather than having its own view.
- **Results export** (FR-041) is not implemented.

## Live progress repair (2026-10-23)

The SVG progress ring passes its actual card as the lifecycle owner. Its
rendering shim has no DOM `isConnected` property; treating that shim as the
owner stopped all updates before opening a stream. The shared progress client
now recovers from disconnects, frozen streams and prolonged network outages
without a manual refresh. JSON polling responses are not cached. A browser
regression executes the actual template initializer and observes the automatic
reload when the run completes.
