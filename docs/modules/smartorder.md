# Module: Smart Ordering (نظام الطلب الذكي)

## Overview

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
  `127_smartorder_files`, `132_smartorder_ai_enhance`
- **Tables owned:**
  - `smartorder.runs` — one end-to-end execution, its counters and totals.
  - `smartorder.run_config` — the configuration snapshot the run executed under.
  - `smartorder.column_mappings` — file columns to target fields, with the
    automatic guess kept alongside the confirmed mapping.
  - `smartorder.run_lines` — one row per imported spreadsheet row.
  - `smartorder.line_candidates` — every vendor offer considered, **including the
    rejected ones and why**.
  - `smartorder.line_selections` — the chosen candidate and what decided it.
  - `smartorder.run_events` — append-only progress and audit.
  - `smartorder.criteria_profiles` — the buyer's remembered defaults.
- **Tables shared with `catalog`:** `catalog.match_decisions` (AI decision
  cache), `catalog.product_aliases` (learned alias channel). Both are catalogue
  knowledge rather than buyer data, and are deliberately **not** tenant-scoped.

## Invariants & Rules

1. **Availability is never read from `catalog.product_index`.** Supplier offers
   come from `catalog.product_variants` joined to `inventory.stocks`. The read
   model's `variant_id` and `stock_quantity` were the literal NULL and 0 for all
   28,786 rows in production, so every query filtering on them returned nothing —
   silently, for every product. Migration 126 and `catalog/jobs/reindex_sql.go`
   fix the read model; this module still reads the authoritative tables.

2. **No stage issues a query per row.** Each deterministic tier is one query for
   the whole file; supplier offers for every matched product are one query. A
   ten-thousand-row import must stay inside five minutes, and per-row work is the
   single easiest way to lose that. `pipeline/retrieval` asserts the query count
   does not scale with row count.

3. **The deterministic engine is primary; AI only enhances what it left.**
   The engine runs to completion over the whole file first. What reaches the AI
   stage is exactly the two populations the buyer would otherwise see as
   `غير مطابق` and `مطلوب للمراجعة`: no match at all, or a match below the
   **0.850 cutoff**. A line at or above the cutoff is never sent and is never
   overwritten. With AI off the run still completes and produces a finalisable
   order.

4. **Retrieval for AI is separate from, and wider than, the scorer's shortlist.**
   `productmatch.Recall` unions three strategies — shared distinctive words,
   shared character trigrams, and the molecule — with the scorer's disqualifying
   penalties removed. This is the change that made the stage worth running:
   measured on a live 1,473-row file, the scorer's own shortlist was **empty for
   25% of review lines** (which the old adjudicator therefore skipped entirely)
   and averaged 2.2 candidates; recall leaves 0.1% empty at 12 candidates each,
   and puts products like `ابيليفاي 10مجم` in front of a line typed `ابليفاى
   10مجم` that shares no whole word with it.

5. **One catalogue window per request, shared by every item in it.** Each item
   names its own shortlist by id, but the model may answer with any id in the
   window — the correct product is often present because it was retrieved for a
   different line. De-duplicating the union is what makes a whole file fit in one
   or a few calls: 13,453 candidate references become 10,294 catalogue rows, and
   that 1,473-row file goes from fifteen requests to eight. Ordinary files take
   one.

6. **The AI stage is bounded, cached and re-checked.** At most 200 items per
   request, 12 requests and 8 minutes per run, 4 in flight; duplicate lines are
   collapsed into one question. Every decision is cached on
   `sha256(normalised text ‖ sorted candidate ids ‖ prompt version)`, so a
   recurring file costs almost nothing the second time.

7. **Nothing the model says is applied unchecked.** `productmatch.IdentityConflict`
   re-derives, from the catalogue's own record, whether the two things can be the
   same product at all — and refuses the answer if they cannot. It exists because
   the reported failure was not hallucination but plausibility: brand families in
   this catalogue differ by a single word, and a model reading "بانادول 24 قرص"
   against "بانادول اكسترا 24 قرص" sees four words of agreement. Four checks, all
   measured against live data:

   - **strength** — three doses of one brand are three products; combinations
     compare as ratios, and units only conflict when both sides state the same one;
   - **line extension** — بلس, اكسترا, نايت, فورت, ريتارد, ديسكملت, SR/CR/XR and the
     rest are the product, not a description of it;
   - **dosage form** — tablets and capsules are one class because pharmacies write
     them interchangeably, and everything else is distinct;
   - **shared distinctive word** — some word of the line must correspond to some
     word of the product, spelling allowed to differ, with manufacturer names
     excluded from both sides. That exclusion is the point: "ابيفيناك حقن /ايبيكو"
     was matched to "سيفوتاكس 500مجم فيال ايبيكو" in production, two different
     drugs whose only shared word names the company that makes both.

   On a live 1,004-line residue the guards refused 133 of the model's answers —
   strength 42, line extension 47, evidence 39, form 5. A refusal costs one line
   of manual work; a false acceptance ships the wrong medicine.

8. **The confidence floor is 0.80, and it is the model's own judgement.** Its
   confidence is sharply bimodal on real data — 0.95 for what it is sure of — and
   the answers in the seventies were the ones sharing only a category suffix
   (ديرم, بيد). Taking it at its word removes that class for almost no loss.

9. **The prompt is generated, never authored by a model.**
   `aicapabilities.RenderEnhanceInput` is a pure function of the request — no
   clock, no map iteration, no randomness — because the decision cache keys on
   the question asked. The answer is strict JSON with a JSON-schema response
   format where the Gateway supports one, and a prompt that specifies the same
   shape in full where it does not.

10. **Chain-of-thought is switched off for this capability.** The default model
    is a reasoning model: its `reasoning` output is billed and drawn from the
    same `max_tokens` budget as the answer. Two hundred product matches reasoned
    through individually exceed any output ceiling, and when they do the model
    returns an EMPTY answer with `finish_reason: "length"` — a total, silent
    failure that looks like a model with nothing to say. `gateway.budget.think`
    is false for everything except the human-facing chat.

11. **Supplier selection is strict priority within a tolerance band.** The
   highest-priority enabled criterion decides, but only among candidates within
   the tolerance (default 5%) of the cheapest eligible net price. When the band
   rejects a criterion's winner, the line names the skipped supplier and by how
   much it missed. Ties break deterministically, ending in `vendor_org_id`, so
   re-running an unchanged file selects the same suppliers.

12. **Eligibility checks are ordered, and the order is the design.** own_org →
   inactive → institutional → coverage → stock → min_qty. The **first** failure
   is reported, because it is the most actionable: an offer both out of stock and
   outside coverage is a coverage problem, and saying "out of stock" sends the
   buyer hunting for a supplier they did not need to find.

13. **Nothing is silently substituted or dropped.** Finalisation re-verifies every
   line; a line that changed blocks the whole order and is named. Quantity cells
   that cannot be read (`"2-3"`, negatives, garbage) produce no quantity and a
   note, never a guess.

14. **The review cart is not the shopping cart.** No path in this module reads or
   writes `commerce.carts` or `commerce.cart_items`. An abandoned import must not
   leave items in a cart the buyer believes is empty.

15. **One order path.** Finalisation goes through `commerce.Service.Checkout`, so
   multi-vendor shipment partitioning, order numbering, status history and the
   documents gate are identical to an ordinary order.

16. **Money is `money.Amount` throughout.** Net prices, bands and line totals are
    integer minor-unit arithmetic. The one exception is `runs.ai_cost_estimate`,
    which is USD telemetry and deliberately not `money.Amount`.

17. **Row level security** on all seven tenant-owned tables via
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
