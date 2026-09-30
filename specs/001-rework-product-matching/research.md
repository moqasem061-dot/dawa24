# Phase 0 Research: Product Matching Rework

Each entry lists **Decision**, **Rationale**, and **Alternatives considered**. Source observations come from the inspected working tree on 2026-09-30 (see [repository-context.md](repository-context.md)). Nothing here is a measured result. Every numeric default below is a *starting point* that the harness (R11, R17) must qualify.

---

## R1 — Entry-point inventory is a test, not a document

**Decision**: Maintain `internal/modules/catalog/matching/entrypoints.go`, a typed registry of every identity-assigning or grouping entry point. Each entry has an ID, workflow, owning file, profile and AI eligibility. `test/matching_entrypoints_test.go` fails the build when a call to `productmatch.(*Index).Match`, `productmatch.MatchAll`, `matchflow.Run`, or a grouping key function appears in a file not covered by a registry entry. Initial inventory from the source search:

| Workflow | Entry points (initial) |
|---|---|
| Vendor ingest | supplier wizard stage (`ingest/catalog_stage_run.go`), rerun/restage, admin-assisted org import, AI enhance (`ingest/catalog_enhance_run.go`) |
| Savings | customer direct upload, vendor upload, admin-assisted, staged wizard, per-row rematch (`ui/saving_products_match_choice.go`), AI (`ui/saving_products_ai.go`) |
| Smart Order | new run, resume, reprocess, manual rematch, foreground/background (`smartorder/pipeline/candidates.go`, `enhance.go`) |
| Master catalogue | preview/prepare, rerun, review, commit validation (`catalog/import_match*.go`, `importjobs/commit_worker.go`) |
| Compare | upload/remap/rerun, single-row ladder (`compare/matching.go:246`), batch (`:320`), AI (`matching_ai.go`), head-to-head grouping (`head_to_head_comparison.go:99`), multi-supplier grouping, corrections |

**Rationale**: FR-001 and SC-013 require complete coverage, and drift is the documented failure mode of this codebase (three copies of the prompt version, the ceilings and the planners). A test turns "we missed one" into a red build.

**Alternatives**: A markdown inventory rots. Grep in CI is fragile across refactors. The registry plus an AST-based test is cheap and survives renames.

---

## R2 — Orchestration lives in `catalog/matching`, consumed through ports

**Decision**: Create sub-package `internal/modules/catalog/matching` with `MatchingService` (contract: [matching-service-port.md](contracts/matching-service-port.md)). It owns the catalogue index cache, runs, the AI enhancement stage, knowledge, and policies. Each consumer module declares its own minimal port interface, and `cmd/server/wire_matching.go` adapts the service to each port. The savings AI loop and matcher (`internal/ui/saving_products_{ai,ai_plan,matcher,match_choice}.go`) move into `catalog` (savings rows are catalog-schema data). `internal/ui` keeps only handlers.

**Rationale**: `catalog` already owns `match_decisions`, `product_aliases` and the products, so knowledge and catalogue revision are local to it. Four consumers currently duplicate plan/apply glue (`*_enhance_plan.go`, `*_enhance_apply.go`), which is exactly where drift occurred. Module boundaries forbid direct imports, so ports plus composition-root adapters are the sanctioned pattern (AGENTS R5).

**Alternatives**:
- (a) A new `modules/matching` bounded context with its own schema. It is cleaner in theory, but it moves memory tables across schemas mid-flight and contradicts "no new modules" in AGENTS current state.
- (b) Keep orchestration per consumer and share only leaves. This is today's design, and it drifted.
- (c) A separate microservice. Rejected by ADR 0002.

---

## R3 — Identity facts, four-valued comparison, hard vetoes

**Decision**:
- Extract `IdentityFacts` per row and per product: brand tokens, ingredients (when present), strength components as an **ordered list of (value, unit, basis)** tied to ingredient position when known, concentration denominator, form, route, release marker, line-extension modifiers, identity letter, total content (volume/mass), pack structure (count, sub-unit, container), device size, manufacturer, and identifiers by namespace. Each fact carries `Provenance ∈ {original, corrected, inferred}` and `Certainty ∈ {explicit, parsed, uncertain}`. The existing extractors (`strength_set.go`, `quantity.go`, `identity_words.go`, `shape.go`, `annotations.go`) are wrapped rather than rewritten.
- `Compare(rowFacts, productFacts) → per-attribute {agree, conflict, unstated, uncertain}`.
- **Hard conflict classes** (veto on every path, at any score): strength component, combination/component count, line-extension modifier (both directions when the row states one), explicit form family (ordinary vs dissolving tablet, tablet vs capsule when both explicit, cream vs gel, …), route, release marker, identity letter, concentration (after supported unit normalization), device size/dimension, identifier-versus-description contradiction, and brand when both sides name different identifiable brands.
- **Pack is hard at pack granularity only.** A pack conflict blocks `pack`-granularity acceptance. It does not block `product`-granularity acceptance, and the decision records `pack: conflict|unstated` so pack-dependent actions stay blocked (FR-022, FR-038).
- **Soft signals** (ranking only): name similarity, manufacturer, price proximity, popularity. None of these can authorize acceptance alone (FR-024).
- The existing `conflictMass` ordering stays for **ranking** (it is good at putting the right sibling first). The accept decision moves to the gate. Candidates with hard conflicts stay in the shortlist with a visible reason (FR-032) but are `eligible=false`.
- Tablet/capsule: the current code (`formsConflict`) and prompt treat them as loosely equal. The new rule: when **both** sides state it explicitly and they differ, it is a conflict. When either side is unstated, it is unstated. A scoped curated correction (R6/R8) may repair known mislabelling (FR-021).

**Rationale**: The spec's central demand (US2, SC-002 = 100%) cannot be met by a multiplicative penalty. A single high-name-similarity candidate with a mass-0.45 pack conflict passes today. Separating *finding and ranking* from *accepting* (FR-026) keeps recall while making precision structural.

**Alternatives**:
- Raise masses to 1.0. This kills recall display and still leaves the identifier tiers unchecked.
- A learned classifier. It needs labelled data we do not have yet, is opaque to reviewers, and cannot give a 100% hard-conflict guarantee.

---

## R4 — Catalogue revision and a process-wide index cache

**Decision**: Add `catalog.catalog_revisions`, a single-row monotonic counter per catalogue scope. It is bumped by statement-level triggers on `catalog.products` insert/update/delete (only when identity-relevant columns change) and on curated fact/relationship tables. `MatchingService` keeps an LRU of `*productmatch.Index` keyed by `(scope, revision, normalizer_version)`: at most 2 per scope, built once and shared read-only across requests in a process. Every run records the revision it matched against (FR-028). A rematch checks the current revision; if it changed, the row is rematched against the new index and flagged when the decision changes. `ListCatalogMatchProducts(ctx, 50000)` (compare) and every other loader are replaced by one paginated full loader. A size above the configured ceiling becomes a visible `retrieval_incomplete` warning, never silent truncation (FR-027).

**Rationale**: Today each import rebuilds the index from scratch, which is the dominant fixed cost for small files (SC-005: 5 s for 100 rows on a 100k catalogue). The index is immutable after build, so sharing it is safe and needs no locking. The revision counter gives FR-028 consistency cheaply.

**Alternatives**:
- Postgres-side trigram retrieval per row. It violates FR-089's no-per-row-external-call intent at 50k rows and is slower than an in-memory index.
- An external search engine. That is new infrastructure and out of scope.
- A `max(updated_at)` fingerprint. It misses deletes and is racy.

---

## R5 — Identifier namespaces and validation

**Decision**: Three explicit namespaces: `gtin`, `supplier_code:<source_org_id>`, `catalog_code`. Column mapping declares one namespace per identifier column. There is no implicit copying: compare's `cleanSKU → Barcode` assignment and savings' "same column in both slots" are removed. GTIN requires lengths 8/12/13/14 with a valid mod-10 check digit; otherwise the value is `invalid_identifier` and is shown, not used. Leading zeroes are preserved as text end to end, and scientific-notation or truncated values are detected (`E+`, fewer digits than the column's modal length) and flagged. Supplier codes resolve only through knowledge entries scoped to that source (`match_knowledge` kind `supplier_code`, from confirmed past imports). They never resolve through `catalog.products.sku` unless the column is declared `catalog_code`. **All identifier hits then pass the gate** (R3). A hit whose description contradicts it produces a decision with state `needs_review` and conflict `identifier_vs_description` (FR-018).

**Rationale**: FR-015 to FR-018 and US1-2. The existing `WithIdentifiers` consent model is kept and extended with namespaces.

**Alternatives**: Keep identifier tiers as pre-scoring shortcuts. That is the observed failure mode, so it was rejected.

---

## R6 — Sellable pack granularity without a taxonomy rewrite

**Decision**:
- Derive `PackFacts` for canonical products from `units`, `small_unit`, `volume`, `unit` and the name at index time. The parser version is part of `normalizer_version`.
- Add `catalog.curated_identity_facts` (platform-curated per-product overrides for any fact, with an audit trail) and `catalog.pack_relationships` (e.g. `strip(10) × 3 = box(30)` for a product), which is admin-curated only.
- A `Decision` carries `granularity_achieved ∈ {none, product, pack}` and the consumer's `granularity_required`. Smart Order, savings updates, vendor stock/price and compare pricing require `pack`. The master catalogue update requires `product` plus explicit pack field review.
- Conversions (strip↔box) happen **only** through `pack_relationships` with exact integer arithmetic and `money.Amount` for unit prices (FR-038). Otherwise the price is "not directly comparable" (US6-3).

**Rationale**: There is no canonical sellable-pack entity (`product_variants` are org-owned offers). The spec's assumptions explicitly allow blocking pack-dependent actions rather than rebuilding the taxonomy.

**Alternatives**:
- A full canonical variant table now. It is large, touches every catalogue screen, and is out of scope.
- Treating pack as irrelevant. That is today's prompt rule and violates US2-3.

---

## R7 — Retrieval completeness and ranking

**Decision**: Keep the current multi-channel retrieval (tokens, variant keys, skeleton transliteration, trigram recall, exact name). Add:
- (a) An alias channel from `match_knowledge` (curated and human-confirmed aliases in scope).
- (b) Structured-fact retrieval (ingredient + strength) when the name is weak.
- (c) Measurement of `PoolLimit` (400) truncation. When the pool is truncated and the best candidate is not settled, widen once (×4) before reporting `retrieval_incomplete`.

Candidate recall@20 becomes the tuning metric for retrieval, separate from acceptance. Ordering is stable: ties break by product ID, and question dedup uses `signature.go`, so row order and concurrency cannot change results (FR-030). Thresholds (`DefaultMinStrong` 0.60, `blindNearIdentical`, `tieMargin`) become policy fields qualified per profile on the tuning split (FR-031). Scores are displayed as "match strength", never as a percentage-probability label.

**Rationale**: The engine's retrieval is already sophisticated. The measurable wins are the missing alias/fact channels and the silent-truncation risk. Treating recall and acceptance as separate metrics prevents "fixing" precision by hiding candidates.

**Alternatives**: Embeddings or a vector service. FR-089 forbids an embedding service for deterministic timing, and it is unmeasured value.

---

## R8 — Scoped knowledge replaces the decision cache

**Decision**: A new table, `catalog.match_knowledge` (data-model §3). Key: `(organization_id|NULL for platform, source_namespace, question_signature, kind)`, where `question_signature = sha256(normalized name ‖ decisive facts canonical form ‖ namespace-qualified identifiers ‖ normalizer_version)`. **The candidate list is no longer part of the key.** The entry stores `target_product_id`, `target_granularity`, `target_fact_fingerprint` (hash of the target's identity facts at decision time), `policy_version`, and for AI answers `ai_config_id` + `prompt_version` + `candidate_set_fingerprint`.

Kinds: `human_confirm`, `human_reject`, `curated_alias`, `curated_relationship`, `platform_fact`, `ai_answer`, `supplier_code`.

Precedence (compatible scope): `human_reject` > newer `human_confirm` > org `curated` > `platform_fact` > `ai_answer`. Every reuse passes the gate again. It is **stale** when the target's current fingerprint differs, the target is inactive or deleted, or the policy's `knowledge_compat` set excludes the entry's policy version. Negative AI answers apply only while the current candidate set ⊆ the stored `candidate_set_fingerprint` set (FR-056). AI answers are used only when AI policy is enabled (FR-057).

Migration: a backfill job copies `match_decisions` rows with `source ∈ {manual, admin, import}` as `human_confirm` candidates in `pending_revalidation` state. They are activated only after a gate re-check against the current catalogue with facts recomputed from `norm_name`. `source = ai` rows are imported as `ai_answer` with `legacy=true`. They are never used for automatic acceptance and only feed suggestions until re-asked. The old table stays read-only for one release, then a contract migration drops readers.

**Rationale**: The current key (name + candidate IDs + prompt version) both over-splits and under-splits:
- Over-splits: a catalogue addition changes the shortlist and orphans a human confirmation.
- Under-splits: two rows with the same name but different structured strength columns collide (spec example: same raw name, separate 10 mg/20 mg).

It also ignores target changes (FR-055).

**Alternatives**: Adding columns to `match_decisions`. Its unique index is on `(COALESCE(org,0), decision_key)`, which prevents holding kinds side by side, and the semantics of existing rows are mixed. A fresh table with explicit migration is safer under expand/contract.

---

## R9 — AI eligibility, batching and deadlines

**Decision**:
- **Eligibility (FR-060)**: send a question only if all of these hold. State ∈ {needs_review, ambiguous, no_eligible_candidate with ≥1 retrieved-but-unstated candidate}. At least one candidate is gate-eligible or has only `unstated` differences. No hard conflict is unresolved by source correction. It is not human-protected. It is not already answered by applicable knowledge. Settled rows are **not** sent (the "check" kind is removed from ordinary runs). An `audit` mode with its own budget samples accepted rows (FR-061).
- **Per-row eligibility (FR-062)**: each item carries its own option list, which includes recall-widened candidates. The shared window stays for token economy, but an answer outside the item's options is `not_offered_for_row` and is rejected.
- **Deadlines (FR-082/083)**: class by eligible distinct-question count: ≤100 → 30 s, ≤1000 → 90 s, >1000 → 180 s, measured from scheduling (enqueue timestamp) and including queue time. Per-attempt deadline = `min(15 s, remaining − 1 s)`. Proposals stop dispatching at 85% of the run deadline. Each question may be in ≤3 attempts total (proposal + retry + split + verification). The run publishes its terminal state within 2 s, enforced by a `time.AfterFunc` that writes state independently of in-flight goroutines.
- **Batch sizing**: items per attempt = `min(config.max_items, floor((15 s − p95_ttft) × p95_output_tps / tokens_per_answer))`, with a floor of 4 and an initial default of 16. `tokens_per_answer` is measured per configuration during qualification (the v8 compact answer targets ≤30 tokens). Concurrency per run defaults to 6, and per-org active AI runs are capped at 2 at enqueue. The worker uses a global semaphore with round-robin across orgs (FR-087).
- **Progressive application (FR-081)**: each validated attempt's answers are persisted immediately with a revision compare-and-set. The UI refreshes counts from saved state.
- **Verification pass**: `enhance_verify.go` stays behind policy flag `verify_pass`, default **off**. It is enabled per configuration only if the harness shows error reduction worth its cost (FR-079). Agreement between calls is never counted as correctness.

**Rationale**:
- A 170 s request timeout with batches of 60 is incompatible with a 15 s attempt deadline, so smaller batches at higher concurrency are the only path, and compact answers make them cheap.
- Routinely re-checking settled rows is the single largest spend. The spec's 90% no-call target (SC-014) forbids it.
- The window-wide choice (R-context row 4) is kept for recall, but only through per-row options, which preserves the useful property that neighbours' retrievals help while blocking neighbour contamination.

**Alternatives**:
- Streaming responses to beat deadlines. Gateway streaming for structured output is not assumed (FR-076).
- One row per request. Cost and latency explode.
- Keeping the check pass. It fails SC-014.

---

## R10 — Spending ceilings without provider pricing in the Store

**Decision**: Each qualified AI configuration records a `max_cost_per_attempt` (a `money.Amount` in micro-units), derived during qualification from the gateway's usage ledger for the worst-case input and output allowance. The run holds a budget `{max_requests, max_input_tokens, max_output_tokens, max_cost}`. Before dispatch it **reserves** `max_cost_per_attempt` from the remaining budget, and settles the actual cost from the gateway usage response (`usage.go`) after the attempt. When the actual cost is unavailable it settles at the reserved amount and marks the run `cost_known=false`. No reservation means no dispatch (FR-084). Org and operator ceilings live in the existing AI entitlement settings. Pricing stays in the Gateway (ADR 0005); the Store only reads usage results.

**Rationale**: This satisfies "remaining limit must cover bounded maximum attempt cost" without importing a price list. Unknown cost is never zero.

**Alternatives**:
- Gateway-side hard budget only. That surfaces as an opaque mid-run failure, which the current ceilings comment already notes.
- Post-hoc accounting. It can overrun.

---

## R11 — Statistics and threshold qualification

**Decision**: Use the **Clopper–Pearson exact one-sided 95% lower bound** for accepted precision, implemented in `matcheval` with the regularized incomplete beta function (tested against published tables). Thresholds are chosen on the **tuning split only** by maximizing coverage subject to precision lower bound ≥ target + 0.2 pp margin. They are then evaluated once on held-out. A held-out failure means the threshold is re-tuned on tuning data and the held-out is extended with fresh cases before re-evaluation (no peeking loop). Candidate recall@20, abstention and coverage are reported with Wilson intervals for readability.

**Rationale**: At 99.5% precision with about 1,000 accepted decisions, Wilson is slightly anti-conservative near the boundary. Clopper–Pearson never under-covers, which matches "insufficient sample is not a pass". The method is held constant across comparisons, as the SC rules require.

**Alternatives**: Bootstrap intervals are resampling-dependent and harder to reproduce. Bayesian beta posterior is fine but less familiar to reviewers.

---

## R12 — Reason codes and bilingual presentation

**Decision**: Replace the Arabic reason strings in `productmatch` (`match.go`, `match_decide.go`, `identity.go`, …) with `Reason{Code, Params}`. Codes are stable snake_case (`strength_conflict`, `pack_unstated`, `identifier_vs_description`, `retrieval_incomplete`, …) and listed in [identity-gate.md](contracts/identity-gate.md). UI renders them through the existing i18n catalogues in both languages. `MatchLevel.Label()` stays temporarily for legacy screens and is removed when the last consumer migrates. Review components are shared under `internal/ui/components/matchreview` with RTL-first layout, `dir="auto"`/`<bdi>` isolation for identifiers, keyboard navigation, `aria-live` count updates, and a 360 px layout.

**Rationale**: FR-049 and `check-hardcoded-arabic`. Codes also make reasons countable in reports (FR-105).

**Alternatives**: Bilingual strings in Go. They violate ADR 0018's split and the Makefile gate.

---

## R13 — Revisions, late answers, idempotency

**Decision**: Every consumer row gains `match_revision INT`. Any human edit, remap, rerun or policy change increments it. AI runs capture `(row_id, revision)` at planning time, and application is `UPDATE … SET … WHERE id=$1 AND match_revision=$2 AND ai_run_id=$3 AND human_locked=false`. Zero rows updated means `obsolete` and is counted, never retried. Commit consumes a `(row_id, revision)` pair and is idempotent through the owning workflow's existing commit keys. Concurrent reviewers use optimistic concurrency on the same revision column, and a conflict returns a bilingual notice (FR-048). Run liveness: `match_runs.heartbeat_at` is updated every 10 s. A River periodic job marks runs with no heartbeat for >45 s as `interrupted/recoverable` (SC-011 ≤60 s).

**Rationale**: This is the simplest mechanism that gives zero late-answer effects (SC-009) and does not depend on upstream cancellation.

**Alternatives**: Row locks held during AI. Holding them across a 180 s window blocks reviewers.

---

## R14 — Diagnostics and retention

**Decision**: Store structured attempt records (`catalog.match_ai_attempts`): timings, token counts, cost, outcome, validated answers and reason categories. **Do not store raw prompts or raw responses in production.** Store a SHA-256 of the rendered request and a truncated (≤2 KB) redacted response only for invalid or malformed outputs. Attempt-level detail is retained for **30 days** (a daily River job purges it). Run summaries and decision provenance live with the rows under existing retention. The evaluation harness may store full prompts locally for offline runs only. This is recorded as the "bounded planning decision" FR-100 requires. The product owner must confirm it before shadow mode starts (quickstart §8 checklist).

**Rationale**: Minimizes private data (FR-098) while keeping enough to debug malformed-output and cost issues.

**Alternatives**: Indefinite storage violates FR-100. No storage makes FR-104/SC-014 impossible.

---

## R15 — Policy versions, shadow mode and rollback

**Decision**: Add a `catalog.match_policies` table. Each policy is immutable and versioned, with thresholds, granularity per consumer, AI enablement per workflow/cohort, AI configuration ID, deadlines and budgets, `verify_pass`, and `knowledge_compat`. `catalog.match_policy_assignments` holds `(workflow, cohort) → active policy`, with an optional `shadow policy`. In shadow mode, the shadow policy runs after the active one on the same rows, writes only to `match_shadow_outcomes` (and never to user decisions), and feeds the comparison dashboard (FR-113 "observation without modifying user decisions"). Rollback flips an assignment pointer, audit-logged, which takes well under 5 minutes (SC-015). The cohort key is organization ID and can be allow-listed.

**Rationale**: Reversible, observable and cheap. It reuses the audit pattern already used for admin settings.

**Alternatives**: Env-var feature flags need a redeploy for rollback and cannot be per org.

---

## R16 — Prompt engineering method

**Decision**: Prompt v8 (contract: [ai-match-v8.md](contracts/ai-match-v8.md)):
- A short system prompt (target ≤ 900 tokens vs ~2,300 today) that states the identity definition, hard exclusions, "unstated ≠ agree", abstention as first-class, and that the data blocks are untrusted.
- Remove the "tablet = capsule" and "pack is weak" rules. Pack is judged only when `granularity=pack` is requested, with row pack facts supplied.
- Supply **structured row facts with provenance** (original / corrected / parsed-uncertain) instead of loose `*_hint` strings.
- Few-shot: 8–12 compact synthetic examples selected by a documented policy (coverage of Arabic shorthand, cross-script, strength/form/pack traps, manufacturer/prefix traps, absent target, hostile text). They are drawn from the **tuning** split's template families only and verified absent from held-out by family ID.
- Output is the compact v8 JSON (per-row `sel|null`, `abstain_reason` enum, per-attribute evidence status, and optional `conf` bucket `hi|mid|lo`). Confidence is **not** an acceptance input. Acceptance is gate + qualification.
- Untrusted text is wrapped in delimited blocks and escaped (pipe and newline neutralized), with a prompt-injection fixture set in the adversarial collection (FR-070).

Promotion: a candidate prompt must beat the incumbent on the tuning split on net AI resolution gain, **and** not lower AI-added precision's lower bound, **and** not raise mean output tokens by >20%. Length and style are not criteria (FR-080). `matchflow.PromptVersion` stays the single declared constant (`check-prompt-version`).

**Rationale**: The current prompt is long, partly contradicts the spec, and relies on self-reported confidence ("well calibrated", observed once). Evidence-structured output makes answers checkable by the gate and diagnosable.

**Alternatives**:
- Chain-of-thought or long reasons. These cost latency and FR-073 forbids requiring them.
- A per-provider prompt fork. FR-077 forbids it.

---

## R17 — Evaluation corpus and leakage control

**Decision**: The corpus lives **outside git** in an access-controlled bucket (`s3://…/matching-eval/<corpus_version>/`), with a manifest and SHA-256 in the repo (`testdata/matching/corpus.lock`). Format: JSONL cases (see [match-eval-cli.md](contracts/match-eval-cli.md)). Sampling: de-identified rows from each workflow's historical uploads via an `AsSystem` export command that strips org IDs, prices, quantities and supplier codes (supplier codes are replaced with namespace-preserving surrogates). Splitting is by **template family** (source org × file layout hash) and by **expression family** (question signature cluster), so no family crosses splits. Labelling: two reviewers on high-risk/disputed cases, with adjudication recorded, and unresolved cases excluded from accuracy but reported. An adversarial collection (synthetic, in git under `testdata/matching/adversarial/`) covers every edge-case table row and every normative example. Existing memory that predates the freeze is disclosed in the baseline report.

**Existing harness (reuse, do not duplicate)**: `test/corpus` already holds 24 real upload files, a catalogue snapshot (`catalogue.jsonl.gz`), derived labels (`CrossScriptLabels`, `SiblingLabels`), and committed result files (`baseline.json`, `accuracy.json`). The new metrics (`internal/shared/matcheval`) plug into it. Its labels become the first **tuning-split** cases. They are pseudo-labels derived from the catalogue itself, so they cannot count toward held-out gates. New de-identified held-out data stays out of git as described above.

**Rationale**: SC measurement rules. Keeping new real data out of git honours the isolation and privacy requirements.

**Alternatives**: Synthetic only. That establishes rule behaviour but cannot pass real-data gates, and the spec says so explicitly.

---

## Documentation drift to reconcile (FR-114)

The drift noted in repository-context is resolved by this plan's single sources:
- **Ceilings**: `matchflow` policy fields.
- **Memory scope**: R8.
- **Compare ladder**: routed through the service.

The per-module docs pages are rewritten in the slice that changes each workflow (S3), and `docs/modules/aicapabilities.md` in S6.
