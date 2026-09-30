# Implementation Plan: Product Matching Rework

**Branch**: `001-rework-product-matching` (work lands on `main` in slices; no long-lived branch) | **Date**: 2026-09-30 | **Spec**: [spec.md](spec.md)

**Input**: Feature specification from `specs/001-rework-product-matching/spec.md`, plus the source investigation in [repository-context.md](repository-context.md).

## Summary

Rebuild product identity as **one gate, many consumers**. Today the deterministic engine (`internal/shared/productmatch`) is strong at retrieval and ranking but has three structural gaps that no amount of tuning or prompting will fix:

1. **Shortcuts skip the identity check.** The barcode and code tiers in `Index.Match` return before strength, form or pack are consulted. Exact names, saved decisions, manual choices and AI each call a different subset of checks.
2. **Contradiction is a ranking penalty, not a veto.** `conflictMass` × `survival` keeps a pack or figure contradiction (mass < 0.80) accept-eligible when it is the only candidate. Pack identity has no granularity of its own.
3. **The AI stage is tuned for thoroughness, not for bounded value.** It uses a 170 s request timeout, 8–14 min runs, routine re-checking of settled rows, prompt rules the spec rejects ("pack is weak", "tablet = capsule"), self-reported confidence as the acceptance floor, and a memory key that ignores row facts.

The approach, in order of leverage:

- **Measure first.** Build an offline evaluation harness (`cmd/cli match-eval`) and a frozen, leakage-safe corpus. Record the current engine's baseline before any tuning (FR-107). Every later change is judged on that harness.
- **Build a single identity gate** (`productmatch.Gate`). Structured identity facts with provenance and a four-valued comparison (agree/conflict/unstated/uncertain). Hard conflicts are vetoes on **every** acceptance path. Decisions are granularity-aware (product vs sellable pack).
- **Use a shared decision record** (`matchflow.RowDecision`, versioned JSON plus four indexed columns) that every consumer persists. This covers the 8 primary states, basis, deterministic baseline, AI contribution, row revision and reason codes. Consumers keep their own tables and commit logic.
- **Move matching behind one service port** owned by `catalog` (it already owns memory and aliases), with a process-wide catalogue index cache keyed by catalogue revision. All five workflows call it through adapters in `cmd/server`, so the savings AI loop leaves `internal/ui`.
- **Replace memory with scoped knowledge.** Key on the question's identity signature, not the candidate list. Store a target fact fingerprint for invalidation. Separate human confirm/reject, curated, platform and AI-answer kinds. Old rows are revalidated, never trusted blindly.
- **Rework the AI stage.** It applies only to unresolved-but-resolvable questions, uses per-row eligible candidates, runs 30/90/180 s run deadlines, a 15 s attempt deadline, at most 3 attempts per question, and a pre-dispatch cost reservation. A compact v8 answer contract is validated app-side for every model. The prompt is rewritten and promoted only on harness evidence. Qualified configurations are a registry the operator can flip in under 5 minutes.
- **Roll out behind policy versions**: offline → shadow → limited cohort → broad, per workflow. Deterministic improvements can ship while AI stays off.

## Technical Context

**Language/Version**: Go 1.26.4 (module `github.com/moqasem061-dot/dawa24`)

**Primary Dependencies**: chi v5 (HTTP), templ + HTMX + Alpine CSP (UI), pgx v5 (Postgres), River v0.43 (jobs), existing `internal/platform/gateway` (capability-based AI; no provider SDKs). No new third-party dependency is required. Statistics (Clopper–Pearson bound) are implemented in-repo under `internal/shared/matcheval`.

**Storage**: PostgreSQL (schemas `catalog`, `ingest`, `smartorder`, `compare`, `platform`, `ai`), with RLS via `platform.tenant_visible`. `pg_trgm` is already installed. Catalogue retrieval stays in process: an in-memory `productmatch.Index` cached per catalogue revision (research R4).

**Testing**: `go test -race` table-driven unit tests; integration tests against real Postgres (`make test-integration`); tenant-isolation tests per new tenant-owned table (CI gate); gateway-disabled fallback test per AI path; offline evaluation harness plus frozen corpus for quality gates; load profiles through `cmd/loadseed` plus a new `match-bench` sub-command.

**Target Platform**: Linux containers (one image, `server` / `worker` / `cli` binaries), deployed via `deploy/hostinger/docker-compose.yml`.

**Project Type**: Modular monolith web service with server-rendered UI.

**Performance Goals** (SC-005, SC-008, SC-009): reviewable deterministic results in ≤5 s / 15 s / 60 s / 5 min for 100 / 1k / 10k / 50k rows, p95, against a 100k-product reference catalogue. With 5 organizations running concurrently, stay within 2×. One-row rematch ≤1 s p95. AI run deadlines are 30/90/180 s with a 15 s per-attempt deadline and terminal state published ≤2 s after the deadline. AI adds ≤1 s p95 to reviewable time.

**Constraints**: Money stays `money.Amount`/`NUMERIC`. Provider isolation. Deterministic fallback for every capability. RLS through `db.InTx`/`db.InReadTx`. No module-to-module imports. ≤400 lines per Go file. Migrations must be expand/contract and backward-compatible with the deployed image. UI copy must be bilingual and RTL-first. No hard-coded user-facing Arabic in Go (`check-hardcoded-arabic`), so reason strings become reason codes.

**Scale/Scope**: 5 workflows and ~17 entry points (inventory in research R1). ~13.5k LOC in `productmatch`, ~2.2k in `matchflow`, ~1.1k in `aicapabilities`. Reference catalogue of 100k canonical products. Files up to 50k rows. Evaluation corpus of ≥6,000 questions (≥4,000 held out).

**Resolved unknowns** (details in [research.md](research.md)): confidence-bound method (R11), catalogue revision mechanism (R4), pack granularity without a taxonomy rewrite (R6), memory key and migration (R8), AI batch sizing under a 15 s attempt deadline (R9), cost reservation without provider pricing in the Store (R10), diagnostic retention (R14), rollout switch (R15).

**External dependencies, not plan unknowns**: an authorized de-identified corpus and two domain labellers; budgeted access to ≥2 gateway model routes; a recorded reference environment. The plan provides the procedure and tooling for each. Gates that cannot be evaluated without them stay *unverified*, never *passed* (SC rules).

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

`.specify/memory/constitution.md` is an unfilled template, so the operative constitution is **AGENTS.md** plus the accepted ADRs (0002–0005, 0011, 0018). Gates:

| # | Gate (source) | Pre-design | Post-design | How the design satisfies it |
|---|---|---|---|---|
| G1 | Money never `float64` (AGENTS R1) | PASS | PASS | Identity comparison never uses price. Pack conversions and cost ceilings use `money.Amount` and integer micro-units (R10). The existing `MatchCandidate.PublicPrice` string display is unchanged. Scores stay `float64` because they are not money. |
| G2 | No provider names outside gateway (R2, ADR 0005) | PASS | PASS | The AI configuration registry stores opaque gateway **route aliases** (`product.match:route-a`). Model capability flags come from the existing `gateway.Capabilities` call. `check-provider-isolation` is unchanged. |
| G3 | Deterministic fallback per capability (R3) | PASS | PASS | FR-004: every workflow completes with AI off. AI state is a separate attribute and never blocks review or commit. A gateway-disabled test exists per consumer. |
| G4 | Tenant queries via `db.InTx`/`InReadTx`, `AsSystem` greppable (R4, ADR 0003) | PASS | PASS | New tenant-owned tables (`catalog.match_knowledge`, `catalog.match_runs`, `catalog.match_ai_attempts`) get `organization_id` plus FORCE RLS plus `platform.tenant_visible`. Platform-scope knowledge reads go through one `AsSystem` repository method with a scope filter. |
| G5 | Module boundaries (R5, ADR 0002) | PASS | PASS | Shared types live in `internal/shared/{productmatch,matchflow,matcheval}` (leaves). The service lives in `modules/catalog`. Consumers depend on a port interface they declare themselves, adapted in `cmd/server`. The savings orchestration moves out of `internal/ui` into catalog. |
| G6 | ≤400 lines/file (R6) | PASS | PASS | New code is split by concern (`gate.go`, `facts_*.go`, `decision.go`, `deadline.go`, …). The current ≥390-line files (`row_price.go`, `identity.go`) are split before they grow. |
| G7 | Platform is source of truth; compatibility (R7) | PASS | PASS | Product IDs, order numbers and imported rows are unchanged. Behaviour changes are recorded in `docs/modules/{catalog,ingest,smartorder,compare,aicapabilities}.md` (FR-114). |
| G8 | Migrations expand/contract, never edit applied (ADR 0004) | PASS | PASS | New columns are nullable or defaulted, and old columns stay written during transition. `match_decisions` is retired only in a later contract migration after one release of dual-read (R8). |
| G9 | Tenant isolation test per tenant table (Testing) | PASS | PASS | Listed in data-model per table. Cross-tenant read ⇒ 0 rows. |
| G10 | AI feature has a gateway-disabled test (Testing) | PASS | PASS | One per consumer plus the engine-level test (quickstart §3). |
| G11 | Money assertions exact (Testing) | PASS | PASS | Pack conversion and savings/compare totals are exact-value tests. |
| G12 | Arabic matching vocabulary is not translatable text (ADR 0018) | PASS | PASS | Vocabulary stays in Go data. User-facing *reasons* move from Arabic string literals to reason codes rendered through i18n (`internal/ui` catalogues) (R12). |

**Result**: no violations, so Complexity Tracking is empty. One deliberate addition, the matching service port in `catalog`, is explained in R2 because it centralizes rather than adds a layer.

## Project Structure

### Documentation (this feature)

```text
specs/001-rework-product-matching/
├── spec.md                 # Feature specification (input)
├── repository-context.md   # Source investigation (input)
├── plan.md                 # This file
├── research.md             # Phase 0: decisions R1–R16
├── data-model.md           # Phase 1: entities, tables, state machines
├── quickstart.md           # Phase 1: validation scenarios
├── contracts/
│   ├── identity-gate.md        # productmatch facts/compare/gate API
│   ├── decision-record.md      # persisted decision schema v1 + consumer columns
│   ├── matching-service-port.md# catalog-owned service consumed via ports
│   ├── ai-match-v8.md          # request rendering + answer contract + validation
│   └── match-eval-cli.md       # corpus format, eval/bench commands, report
├── checklists/requirements.md
└── tasks.md                # Phase 2 (/speckit-tasks) — not created here
```

### Source Code (repository root)

```text
internal/shared/productmatch/          # deterministic engine (leaf)
├── identity_facts.go, identity_facts_extract.go  # IdentityFacts with provenance (orig/corrected/inferred)
├── identity_compare.go                # 4-valued per-attribute comparison
├── gate.go, gate_policy.go            # THE acceptance gate + policy/granularity
├── identifiers_ns.go                  # namespaces: barcode(GTIN check), supplier code, catalogue code
├── pack.go                            # sellable-pack facts, relationship lookups
├── match.go, match_decide.go          # identifier tiers routed through gate; hard vetoes
├── signature.go                       # question identity signature (dedup + memory key)
└── reasons.go                         # reason codes (no UI strings)

internal/shared/matchflow/             # shared contracts (leaf)
├── row_decision.go                    # RowDecision record v1 (NOTE: existing Decision = AI answer), states, basis, AI state
├── deadline.go, budget.go             # class deadlines, attempt caps, cost reservation
├── engine*.go                         # reworked runner (no routine check pass)
├── contract_v8.go                     # answer contract types + validator
└── knowledge.go                       # scoped knowledge port (replaces memory.go/cachekey.go)

internal/shared/matcheval/             # NEW leaf: corpus, metrics, Clopper–Pearson, report

internal/modules/catalog/
├── matching/                          # NEW: MatchingService (prepare, rematch, enhance, knowledge ops)
│   ├── service.go, index_cache.go, runs.go, enhance.go, knowledge.go
│   └── postgres/                      # match_runs, match_ai_attempts, match_knowledge, policies
├── import_match*.go                   # master-catalogue consumer → service
└── saving_*                           # savings orchestration moved here from internal/ui

internal/modules/aicapabilities/
├── enhance_prompt_v8.go, enhance_decode_v8.go   # new prompt + tolerant decoder
└── enhance_verify.go                            # retained, disabled by default policy

internal/modules/ingest/catalog_*.go             # vendor consumer → port
internal/modules/smartorder/pipeline/*.go        # order consumer → port; identity≠offer
internal/modules/compare/matching*.go, head_to_head*.go, multi_supplier*.go  # consumer + grouping gate
internal/ui/saving_products_*.go                 # thin handlers only
internal/ui/components/matchreview/              # shared bilingual review components

cmd/cli/match_eval.go, match_bench.go            # harness entrypoints
cmd/server/wire_matching.go                      # port adapters
db/migrations/230_…–23x_…                        # see data-model.md
docs/modules/*.md                                # FR-114 updates
testdata/matching/                               # synthetic fixtures ONLY (real corpus stays out of git)
```

**Structure Decision**: Keep the existing modular monolith. Identity logic stays in the `shared/productmatch` leaf so every consumer and the harness run byte-identical rules. Orchestration, persistence and knowledge move into a `catalog/matching` sub-package exposed through a port. Consumers keep ownership of their rows, business eligibility and commit (spec §Scope table).

## Delivery slices

Independently shippable, in order. Each slice ends with harness numbers and docs updated.

| Slice | Content | Unblocks | Ship gate |
|---|---|---|---|
| S0 Measure | `matcheval`, corpus format, `match-eval`/`match-bench`, entry-point inventory test, **baseline report of current engine** | everything | Baseline recorded (FR-107) |
| S1 Identity core | Facts/compare/gate, identifier namespaces, hard vetoes, pack granularity, reason codes; engine-level only | S2 | SC-002 on synthetic + adversarial set; no recall regression |
| S2 Decision record + service | `matchflow.RowDecision`, `catalog/matching` service, index cache, run table, per-consumer columns (expand) | S3 | Equal-context replay harness (SC-013) green |
| S3 Consumers | One workflow per PR: vendor → savings (all paths) → Smart Order → master catalogue → compare incl. grouping | S4 | Per-workflow SC-001/003/005/009/010/011 |
| S4 Review UX | Shared bilingual review components, filters, bulk preview, conflict-correction flow, concurrency notices | S5 | SC-006 observation protocol |
| S5 Knowledge | `match_knowledge`, revalidation, migration from `match_decisions`, management screens | S6 | SC-012 |
| S6 AI rework | Eligibility, deadlines, budgets, v8 contract, prompt v8, config registry, shadow mode | S7 | SC-007/008/014 per configuration |
| S7 Rollout | Policy versions, cohort flags, dashboards/alerts, rollback rehearsal, contract migration | done | SC-015 |

## Complexity Tracking

No constitution violations. Nothing to justify.
