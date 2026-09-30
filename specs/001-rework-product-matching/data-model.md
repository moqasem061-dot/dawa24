# Phase 1 Data Model: Product Matching Rework

This file maps the spec's Key Entities to concrete in-memory types and Postgres tables. Migrations are numbered from **230** (the last applied is 229). All of them are `BEGIN; … COMMIT;` expand-only, and backward-compatible with the deployed image (ADR 0004). Carry Arabic `COMMENT ON` text for every new column.

## 1. In-memory types (leaf packages)

### 1.1 `productmatch.Fact` / `IdentityFacts` — *Identity Evidence*

| Field | Type | Notes |
|---|---|---|
| `Brand` | `[]Token` | distinctive name tokens (after noise removal), script-tagged |
| `Ingredients` | `[]string` | when a scientific/active column exists |
| `Strengths` | `[]Strength{Value decimal-string, Unit, Basis(per_unit/per_volume), Component int}` | **ordered**, never sorted; `Component=-1` when the association is unknown |
| `Concentration` | `*Ratio{Num Strength, Den Quantity}` | `250 mg/5 mL` ≡ `50 mg/mL` via exact rational normalization |
| `Form` | `FormFamily` + `FormDetail` | e.g. `tablet/odt`, `tablet/film`, `capsule`, `cream`, `gel` |
| `Route` | enum | oral, iv, im, topical, ophthalmic, … |
| `Release` | enum | ordinary, sr, xr, mr, cr, la, … |
| `Modifiers` | `[]string` | plus/extra/forte/night/… (from `identity_words.go`) |
| `Letter` | string | identity letter (`بتنوفيت ان`) |
| `TotalContent` | `*Quantity` | 100 mL, 30 g |
| `Pack` | `*PackFacts{Count int, SubUnit string, Container string}` | `2×10` kept structured, never collapsed to 20 |
| `DeviceSize` | string | |
| `Manufacturer` | string | ranking only |
| `Identifiers` | `[]Identifier{Namespace, Value, Valid bool}` | text-preserving |

Every fact has `Prov ∈ {original, corrected, inferred}` and `Cert ∈ {explicit, parsed, uncertain}`.

**Validation**: numeric parsing is bounded (≤12 significant digits, ≤3 multiplication factors). Overflow or malformed input yields `uncertain` with reason `numeric_uninterpretable`, never a panic (FR-014, edge cases). Price-annotated numbers (`س.ج 141ج`) are classified as `money` evidence and excluded from strength/pack (FR-010).

### 1.2 `productmatch.Comparison` — *Candidate Assessment*

`map[Attribute]Outcome` where `Outcome ∈ {agree, conflict, unstated, uncertain}`, plus `Hard []ConflictKind`, `RetrievalBasis []Channel` (identifier/knowledge/exact/token/variant/skeleton/trigram/facts), `Rank int`, `Strength float64` (ranking score, not a probability), `Eligible bool`, `Reasons []Reason`.

### 1.3 `matchflow.RowDecision` — *Match Decision* (persisted as JSON v1; see [decision-record.md](contracts/decision-record.md))

| Field | Type |
|---|---|
| `V` | `1` |
| `State` | `accepted \| confirmed \| needs_review \| ambiguous \| no_candidate \| invalid_input \| excluded \| unavailable` |
| `Basis` | `identifier \| knowledge_human \| knowledge_curated \| knowledge_platform \| exact \| similarity \| ai \| reviewer` |
| `ProductID` | `*int64` |
| `Granularity` | `{Required, Achieved}` ∈ `none/product/pack` |
| `Pack` | `agree/conflict/unstated/needs_relationship` + optional `RelationshipID` |
| `Candidates` | ≤5 `{ProductID, Rank, Eligible, Diff map[attr]outcome, ReasonCodes}` |
| `Reasons` | `[]Reason{Code, Params}` |
| `Flags` | `retrieval_incomplete`, `catalog_changed`, `stale_knowledge`, `identifier_vs_description`, … |
| `Versions` | `{Policy, Normalizer, CatalogRevision, Prompt?, AIConfig?}` |
| `Revision` | int (mirrors row `match_revision`) |
| `AI` | `{State, Proposal?, Outcome, AttemptIDs}` where State ∈ `off/ineligible/queued/running/completed/stopped/failed/obsolete` |
| `Actor` | for `confirmed`: `{UserID, At, Corrections[]}` |

### 1.4 `matchflow.QuestionSignature`

`sha256(normalizedName ‖ canonical(decisive facts) ‖ namespace-qualified identifiers ‖ normalizer_version)`. It is used for (a) in-run dedup (FR-034) and (b) the knowledge key (R8). Two rows with the same text but different structured strength columns get different signatures.

## 2. State machines

### 2.1 Primary decision state (per row, per revision)

```text
            parse fail ──► invalid_input ──(source correction)──► [re-evaluate]
 source row ─┤
            parse ok ──► retrieve ──► none retrieved ─────────────► no_candidate
                            │           catalogue unavailable ───► unavailable ──(retry)──► [re-evaluate]
                            ▼
                     gate over candidates
                       ├─ unique eligible, policy passes ─────────► accepted
                       ├─ several eligible, not separable ────────► ambiguous
                       └─ otherwise ──────────────────────────────► needs_review
 reviewer: select eligible candidate ─────────────────────────────► confirmed
 reviewer: hard conflict present → must record fact correction → re-evaluate → confirmed
 reviewer/rule: exclude ──────────────────────────────────────────► excluded
 any edit/remap/policy change: revision++ ; automation for older revision ⇒ obsolete
```

Invariant: exactly one primary state per `(row, revision)`. A rerun that yields a non-accepted state clears `matched_product_id` (FR-041).

### 2.2 AI state (orthogonal)

`off → ineligible` or `off → queued → running → {completed | stopped | failed}`. Any state becomes `obsolete` when the row revision changes. AI can move the primary state only from `needs_review/ambiguous/no_candidate` to `accepted`, and only via the gate. It never moves a state out of `accepted`/`confirmed`. A contradiction becomes a flag, `ai_disagrees`, which creates a review concern (FR-065).

### 2.3 Match run

`preparing → reviewable → enhancing → reviewable(enhanced|partial) → committed`, with side exits `interrupted(recoverable)`, `failed`, `cancelled`. `reviewable` is written only after all deterministic decisions are saved (FR-091).

## 3. Postgres tables

### 3.1 `catalog.catalog_revisions` (230) — platform, not tenant-owned
| column | type | |
|---|---|---|
| `scope` | TEXT PK | `'master'` (extensible) |
| `revision` | BIGINT NOT NULL | bumped by statement triggers on identity-relevant changes to `catalog.products`, `curated_identity_facts`, `pack_relationships` |
| `updated_at` | TIMESTAMPTZ | |

### 3.2 `catalog.curated_identity_facts` (231) — platform-curated
`id, product_id FK, attribute TEXT, value JSONB, reason TEXT, created_by, created_at, revoked_at, revoked_by`. Partial unique `(product_id, attribute) WHERE revoked_at IS NULL`. Written only by catalogue admins (permission `catalog.identity.curate`).

### 3.3 `catalog.pack_relationships` (231) — platform-curated
`id, product_id FK, from_unit TEXT, to_unit TEXT, factor INT CHECK (factor > 0), created_by, created_at, revoked_at`. Exact integer factors only.

### 3.4 `catalog.match_knowledge` (232) — tenant-owned (nullable org = platform)
| column | type | notes |
|---|---|---|
| `id` | BIGINT identity | |
| `organization_id` | BIGINT NULL FK | NULL ⇒ platform scope |
| `source_namespace` | TEXT NOT NULL | `'*'` or `supplier:<org>` or declared namespace |
| `question_signature` | TEXT NOT NULL | R8 |
| `kind` | TEXT CHECK in (`human_confirm`,`human_reject`,`curated_alias`,`curated_relationship`,`platform_fact`,`ai_answer`,`supplier_code`) | |
| `target_product_id` | BIGINT NULL FK | NULL = "none" (AI negative) |
| `target_granularity` | TEXT | product/pack |
| `target_fact_fingerprint` | TEXT | for staleness |
| `candidate_set_fingerprint` | TEXT NULL | negative AI answers only |
| `policy_version`, `normalizer_version` | TEXT | |
| `ai_config_id`, `prompt_version` | TEXT NULL | `ai_answer` only |
| `display_text` | TEXT | normalized source text, for management UI |
| `status` | TEXT CHECK in (`active`,`pending_revalidation`,`stale`,`revoked`) | |
| `legacy` | BOOL DEFAULT false | migrated from `match_decisions` |
| `created_by`, `created_at`, `revoked_by`, `revoked_at`, `last_used_at` | | |

Unique: `(COALESCE(organization_id,0), source_namespace, question_signature, kind) WHERE status IN ('active','pending_revalidation')`.
RLS: FORCE. Policy `platform.tenant_visible(organization_id)` for org rows. Platform rows are read only through `KnowledgeRepo.LookupPlatform` under `database.AsSystem`, filtered by `decision_memory_preferences.use_platform_memory`. **Isolation test**: org A cannot read org B rows, and platform rows never include org-sourced text or supplier codes (FR-054).

### 3.5 `catalog.match_policies` / `catalog.match_policy_assignments` (233) — platform config
- Policies: `version TEXT PK, body JSONB (thresholds, granularity map, ai{enabled, config_id, deadlines, budgets, verify_pass, audit_rate}, knowledge_compat[]), created_by, created_at`. They are immutable: there are no UPDATE grants.
- Assignments: `(workflow, cohort) PK, active_version FK, shadow_version FK NULL, changed_by, changed_at`, with an audit trail in `catalog.match_policy_assignment_log`.

### 3.6 `catalog.ai_match_configurations` (233) — platform config
`id TEXT PK` (e.g. `pm-route-a-v8`), `gateway_route TEXT` (opaque alias), `prompt_version`, `contract_version`, `native_structured BOOL`, `max_items_per_attempt INT`, `max_input_tokens`, `max_output_tokens`, `max_cost_per_attempt NUMERIC(12,6)`, `qualified_cohorts JSONB` (workflow × language × row-type), `status CHECK in ('candidate','shadow','qualified','disabled')`, `qualification_report_uri`, audit columns. No provider or model names (G2).

### 3.7 `catalog.match_runs` (234) — tenant-owned
`id, organization_id, workflow, entry_point, consumer_ref TEXT` (e.g. `ingest.catalog_imports:123`), `source_revision INT, policy_version, catalog_revision, normalizer_version, state, stage_timings JSONB, counts JSONB` (primary states, excluded, invalid, distinct questions, knowledge reuse), `ai JSONB` (eligible, attempted, accepted_new, review_flags, abstained, rejected{reason→n}, failed, unattempted, obsolete, requests, tokens, cost, cost_known, deadline_class, ended_reason), `heartbeat_at, created_by, created_at, finished_at`. FORCE RLS, plus an isolation test.

### 3.8 `catalog.match_ai_attempts` (234) — tenant-owned, 30-day retention (R14)
`id, run_id FK, organization_id, attempt_no, kind` (`proposal/retry/split/verify/audit`), `question_count, started_at, deadline_at, ended_at, outcome` (`ok/partial/malformed/timeout/auth/quota/unsupported/transient`), `input_tokens, output_tokens, cost NUMERIC(12,6) NULL, request_sha256, invalid_excerpt TEXT NULL (≤2KB)`, `answers JSONB` (validated, compact). FORCE RLS, plus an isolation test. A purge job runs daily.

### 3.9 `catalog.match_shadow_outcomes` (234) — tenant-owned
`run_id, row_ref, shadow_policy_version, decision JSONB, created_at`. It is never read by consumers and exists only for comparison reports. FORCE RLS, plus an isolation test.

### 3.10 Consumer row columns (235–239, one migration per workflow)

Added to: `ingest.catalog_import_rows`, `catalog.import_staging_rows`, `smartorder.run_lines`, `platform.import_run_rows` (staged savings), `catalog.saving_products` (direct savings path), `compare.file_rows`.

| column | type | default |
|---|---|---|
| `match_state` | TEXT NULL (CHECK 8 states) | NULL = legacy row |
| `match_basis` | TEXT NULL | |
| `match_granularity` | TEXT NULL | |
| `match_revision` | INT NOT NULL | 0 |
| `human_locked` | BOOL NOT NULL | false |
| `ai_state` | TEXT NULL | |
| `match_run_id` | BIGINT NULL | |
| `match_decision` | JSONB NULL | Decision v1 |
| `baseline_decision` | JSONB NULL | deterministic Decision before AI |

Existing columns (`matched_product_id`, `match_level`, snapshot columns from 120) **keep being written** during transition, derived from the new decision, so the old image still reads valid data (expand/contract). Partial index `(run/import id, match_state)` for review filters. The consumer's existing RLS covers the new columns. Each migration's isolation test re-asserts the cross-tenant zero-row property on that table.

### 3.11 Retired later (contract migration, S7)
`catalog.match_decisions` reads are removed one release after knowledge backfill completes and SC-012 passes. The table is then dropped in a subsequent migration. `product_aliases` rows with trusted sources are migrated into `match_knowledge` as `curated_alias`.

## 4. Relationships

```text
catalog.products 1─* curated_identity_facts, pack_relationships, match_knowledge(target)
match_policies 1─* match_policy_assignments ; ai_match_configurations referenced by policy body
match_runs 1─* match_ai_attempts, match_shadow_outcomes
match_runs 1─* consumer rows (via match_run_id)          # per consumer table
consumer row ── match_decision.ProductID ─► catalog.products
```

## 5. Validation rules traced to requirements

| Rule | FR |
|---|---|
| Identifier used only when column is mapped, the namespace is declared, and the tier is chosen | 015–017 |
| GTIN check digit and length valid, or the value is ignored as authority | 016 |
| Every accept path calls `Gate.Evaluate` | 019 |
| Hard conflict ⇒ `Eligible=false`; reviewer must record a correction first | 018, 045 |
| `Granularity.Achieved ≥ Required` before any consumer business action | 022, 038 |
| AI answer must ∈ the row's options, and pass gate and qualification | 062–063, 074 |
| Knowledge reuse ⇒ scope compatible, not stale, gate passes | 052–057 |
| Apply guarded by `match_revision` + `human_locked` | 048, 088 |
| Each source row is counted once in the primary totals | 094 |
