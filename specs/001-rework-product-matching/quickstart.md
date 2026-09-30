# Quickstart: Validating the Product Matching Rework

Runnable scenarios proving each slice works. Details live in the contracts and [data-model.md](data-model.md). This file only says what to run and what to expect.

## 0. Prerequisites

```bash
make up
```

```bash
make migrate
```

```bash
make match-baseline
```

- The corpus is fetched to `.cache/matching-eval/` using the manifest in `testdata/matching/corpus.lock`. Access is authorized operators only. Without it, only the synthetic and adversarial scenarios run, and real-data gates report `insufficient_sample`.
- The AI scenarios need a gateway route per configuration in `catalog.ai_match_configurations`, or the built-in simulator (`--ai-sim`).

## 1. Baseline is recorded before tuning (S0, FR-107)

```bash
go run ./cmd/cli match-eval --corpus .cache/matching-eval --split heldout --engine baseline --out reports/baseline.json --md reports/baseline.md
```

Expected: the report includes the baseline git SHA, corpus SHA and memory disclosure, with per-workflow/language cohorts. Commit only the **markdown summary without case text** to `docs/modules/matching-baseline.md`.

## 2. Hard identity protection on every path (S1, SC-002)

```bash
go test ./internal/shared/productmatch/... -run 'Gate|Identity|Identifier|Pack' -race
```

```bash
go run ./cmd/cli match-eval --corpus testdata/matching --split adversarial --engine current --out reports/adv.json
```

Expected: exit 0 and `hard_conflict_violations: []` for each path (identifier, knowledge, exact, similarity, reviewer, ai-sim). Every normative example in spec §"Normative identity examples" appears as a named test case.

## 3. AI off / unavailable: every workflow completes (SC-009, G3/G10)

```bash
go test ./internal/modules/... -run 'GatewayDisabled|AIOff' -race
```

Expected, per consumer (vendor, savings ×3 paths, smartorder, catalog, compare):
- the run reaches `reviewable`, rows can be confirmed and committed, and 0 remote calls are recorded
- `ai_state=off`
- no `ai_answer` knowledge is applied

## 4. Equal context gives equal decisions across consumers (SC-013)

```bash
go test ./test/... -run 'MatchingReplay|MatchingEntrypoints' -race
```

Expected: the same fixture file through every inventoried entry point yields identical `(state, product_id, reasons)`. The entry-point registry test is green, and it fails if a new `Match`/`MatchAll`/`matchflow.Run` call site is unregistered.

## 5. Deterministic latency (SC-005)

```bash
go run ./cmd/cli match-bench --catalog .cache/matching-eval/cat-ref-100k --sizes 100,1000,10000,50000 --runs 30 --orgs 1 --state cold --out reports/bench-cold.json
```

Repeat with `--state warm` and `--orgs 5`. Expected: p95 ≤ 5 s / 15 s / 60 s / 300 s; with 5 orgs, ≤ 2×; one-row rematch p95 ≤ 1 s. The report records the environment fingerprint, and results from a different environment are not comparable.

## 6. Bounded AI (SC-008, SC-014)

```bash
go run ./cmd/cli match-bench --catalog .cache/matching-eval/cat-ref-100k --sizes 100,1000,5000 --runs 30 --ai-sim slow --out reports/ai-slow.json
```

Run with each of `--ai-sim fast|slow|down|budget|malformed`. Expected:
- every run is terminal by 30/90/180 s + 2 s, with no attempt over 15 s
- ≤3 attempts per question, and no ceiling is exceeded
- `unattempted`, `abstained`, `rejected` and `accepted_new` are counted separately
- `late_answer_effects = 0`

## 7. Concurrency and recovery (SC-009, SC-011)

```bash
go test ./internal/modules/catalog/matching/... -run 'Revision|Obsolete|Heartbeat|Resume|Idempotent' -race
```

Expected:
- a human edit during AI ⇒ the AI answer is `obsolete`, with 0 effect
- a worker killed mid-run ⇒ the run is `interrupted/recoverable` within 60 s
- a repeated commit ⇒ one business effect
- counts reconcile 1:1 with source rows

## 8. Isolation and knowledge (SC-010, SC-012)

```bash
make test-integration
```

(This includes the `*Isolation*` and `*Knowledge*` tests.) Expected:
- every new tenant table returns 0 rows cross-tenant
- org A's confirmation has no effect for org B
- revoke, target fact change and policy change ⇒ `stale`/blocked
- a platform promotion contains no supplier code, price or org text

Before shadow mode, the product owner signs off on the retention decision (research R14) in `docs/modules/catalog.md`.

## 9. AI qualification per configuration (SC-007)

```bash
go run ./cmd/cli match-eval --corpus .cache/matching-eval --split heldout --engine current --ai --config pm-route-a-v8 --budget budgets/qual.json --out reports/qual-route-a.json
```

Expected: net gain ≥ 20% per enabled workflow, AI-added precision lower bound ≥ the workflow threshold, 0 hard-conflict violations, and cost per verified gain ≤ the declared maximum. Repeat for a second configuration without native structured output. A configuration that fails stays `disabled`, or is qualified only for the passing cohorts.

## 10. Rollout and rollback (SC-015)

1. Set a shadow policy for one workflow and cohort. Confirm that `match_shadow_outcomes` fills and user decisions are unchanged.
2. Promote to a limited cohort, then observe for 7 days on the dashboards (identity violations, correction growth, latency, stale knowledge, spend).
3. Rollback drill: flip the assignment back. Expected: completes in under 5 minutes, and all source rows, review history and committed records are intact.

## Review UX observation (SC-006)

Follow `docs/modules/matching-review-study.md` (written in slice S4): 8+ participants covering customers, vendors, admins and Arabic-first users, with tasks in AR-RTL and EN. Check keyboard-only completion and 360 px width. Record completion rate and median active review time against the baseline.
