# Repository Context for the Product Matching Specification

**Inspected**: 2026-09-30  
**Base commit**: `ccf5d076976f29678b7a1eb58dda8815c2cb657a`  
**Checkout**: `main`, with existing unrelated uncommitted work  
**Specification**: [spec.md](spec.md)

This is a read-only source investigation supporting the specification, not a runtime benchmark, implementation plan, security audit, or assertion that all potential failure paths have been reproduced. Observations describe inspected working-tree files; documentation sometimes describes earlier revisions. No production data or live AI requests were used. Existing code and module documentation were left unchanged.

## Constraints and Spec Kit setup

- [AGENTS.md](../../AGENTS.md) supplies current project invariants: Arabic-first bilingual output, exact money, tenant isolation, module boundaries, deterministic AI fallback, gateway/provider isolation, compatible migrations, and the current platform as source of truth.
- [Architecture decisions](../../docs/adr/decisions.md) were consulted, especially ADR 0002–0005 on boundaries, tenancy, deployment compatibility, and capability-based AI, and ADR 0018 distinguishing Arabic matching vocabulary from translatable UI copy.
- [Constitution](../../.specify/memory/constitution.md) contains template placeholders rather than ratified project principles. It was not modified or treated as additional approved requirements.
- `specify preset resolve spec-template` resolved [.specify/templates/spec-template.md](../../.specify/templates/spec-template.md), from the core layer. Mandatory section ordering was preserved and the core template was the initial file.
- [Initialization options](../../.specify/init-options.json) select sequential feature numbering. There was no `specs/` directory at inspection, so this invocation created `001-rework-product-matching`. Old documentation referencing another `specs/001-...` folder does not imply that folder currently exists.
- [Extensions](../../.specify/extensions.yml) contains hooks for planning, tasks, and implementation only. There are no `before_specify` or `after_specify` hooks to dispatch; no branch hook ran. The feature-directory pointer was saved in [.specify/feature.json](../../.specify/feature.json).

## Existing ownership and integration surfaces

| Area | Inspected evidence | Implication for the specification |
|---|---|---|
| Deterministic engine | [match.go](../../internal/shared/productmatch/match.go), [match_decide.go](../../internal/shared/productmatch/match_decide.go), [identity.go](../../internal/shared/productmatch/identity.go), [identifiers.go](../../internal/shared/productmatch/identifiers.go) | Rework an existing shared capability. Identity checks must cover all acceptance paths, not only AI proposals. |
| Shared AI orchestration | [contract.go](../../internal/shared/matchflow/contract.go), [engine.go](../../internal/shared/matchflow/engine.go), [ceilings.go](../../internal/shared/matchflow/ceilings.go), [engine_ask.go](../../internal/shared/matchflow/engine_ask.go), [engine_check.go](../../internal/shared/matchflow/engine_check.go) | Shared batching and verification already exist. The spec demands measured bounded behavior rather than another orchestration copy. |
| Prompt and verification | [enhance_prompt.go](../../internal/modules/aicapabilities/enhance_prompt.go), [enhance_verify.go](../../internal/modules/aicapabilities/enhance_verify.go), [AI module docs](../../docs/modules/aicapabilities.md) | Prompt changes must be evaluated together with evidence, validation, configuration limits, and acceptance rules. |
| Vendor ingest | [ingest docs](../../docs/modules/ingest.md), existing `catalog_*` staging/enhancement integration references | Preserve staged review/commit, all vendor/admin paths, progress, and deterministic availability. |
| Savings | [saving_products_ai.go](../../internal/ui/saving_products_ai.go), saving import/matcher/administrator entry-point references | Test direct and staged paths plus customer/vendor/admin variants; comments record previous wiring drift. |
| Smart Order | [Smart Order docs](../../docs/modules/smartorder.md), [enhance_plan.go](../../internal/modules/smartorder/pipeline/enhance_plan.go), resolve/candidate/apply references | Separate identity from offers, stock, unit conversion, and checkout; preserve deterministic completion. |
| Master catalogue | [catalogue docs](../../docs/modules/catalog.md), [import_match.go](../../internal/modules/catalog/import_match.go) | Stronger update protection, reversible preparation, explicit commit, and identifier-only row compatibility. |
| Comparison | [compare docs](../../docs/modules/compare.md), [matching.go](../../internal/modules/compare/matching.go), [matching_ai.go](../../internal/modules/compare/matching_ai.go), head-to-head and multi-supplier references | Audit row matching and later equivalence grouping; a shared scorer alone does not cover every association. |
| Scoped memory | [cachekey.go](../../internal/shared/matchflow/cachekey.go), [match_decisions_scope.go](../../internal/modules/catalog/postgres/match_decisions_scope.go), [match_decisions_mutations.go](../../internal/modules/catalog/postgres/match_decisions_mutations.go) | Respect existing private/platform memory distinction, explicit promotion, precedence, and invalidation. |

## Concrete observations motivating requirements

| Observation from inspected source | Interpretation and limit | Requirements |
|---|---|---|
| `matchflow` uses a 170-second request timeout and a 14-minute hard stage limit. Profiles specify 8 minutes for orders, 12 minutes for base/savings/catalogue, and 14 minutes for vendors, with 120–200 planned requests depending on profile. | Confirms that long waits are permitted by current policy. It does not measure observed latency or establish which layer dominates a real request. | FR-081–089, SC-005/008/009 |
| Proposal calls can retry and split recursively; verification has additional attempts and splits. | Request ceilings must include actual attempts across both stages. This inspection does not claim to prove an actual spending overrun. | FR-075, 079, 083–086, 101–103 |
| The 2026-09-27 AI module documentation describes sending rows with candidates, including accepted name matches, for checking. The profile plausibility floor is zero. | Routine broad checking can consume latency and budget. New policy prioritizes unresolved questions and makes accepted-match audits explicit; improvement must be measured. | FR-059–068 |
| Proposal prompts allow selection of any product in the request catalogue window, not only a row's initial options. | This can recover retrieval misses, but batch membership must not establish row eligibility or justify reuse under a different evidence window. The specification permits broader candidates only after row-specific validation. | FR-055, 062–063 |
| Proposal prompt calls pack size weak evidence. Verification says pack size is not identity and treats tablet/capsule wording as equivalent. | These are existing prompt rules, not claims of clinical equivalence. The rework deliberately distinguishes canonical identity from sellable pack and treats explicit form differences conservatively. | FR-020–023, 038, 069–073 |
| Profiles apply AI confidence floors of 0.80, or 0.90 for catalogue imports. Source comments describe confidence as well-calibrated based on earlier observations. | Model self-confidence is not portable qualification evidence. Require held-out precision, cohort-specific validation, and independent labels. | FR-031, 063, 076, 079–080, SC-001/007 |
| `DecisionKey` hashes normalized name, sorted distinct candidate IDs, and prompt version. | That function alone does not encode structured row facts, candidate content changes, or model configuration. Repository-level scope is separate; no cross-tenant exploit is inferred merely from the key shape. | FR-052–057 |
| Comparison's per-row ladder returns early on an exact normalized name. Its batch path loads up to 50,000 candidates, copies `cleanSKU` into a barcode field, and uses default identifier options. | These are distinct pathways deserving tests. Because default identifier tiers are off, the barcode field assignment alone does not prove an active wrong-barcode match. Cover completeness, evidence plumbing, and early acceptance consistently. | FR-001–003, 015–019, 027, 040–041 |
| Comparison also uses product keys and short-brand grouping in head-to-head/multi-supplier logic. | Matching scope includes equivalence/grouping consumers; replacing only import scoring could leave inconsistent comparisons. | FR-001, 022, 040, 111 |
| Current shared match defaults show a 0.60 strong threshold and 0.25 review floor; older module docs describe different cutoffs. | Ranking scores and historical prose are not validated probability estimates. Do not assume one threshold's meaning is consistent everywhere without evidence. | FR-031, 105, 114 |
| Catalogue docs record a previously fixed pack-multiplication panic affecting multiple consumers, and include a second-import regression. | Preserve such regression scenarios. The spec does not claim the fixed panic still exists. | FR-014, 111 |
| Existing scale tests exercise synthetic 20,000-product catalogues and 25,000-row imports, including a loose five-minute guard. | Useful regression starting points, not evidence of production precision, representative AI value, or the proposed 100,000-item qualification targets. | FR-107–112, SC-001–008 |

## Documentation drift that planning must reconcile

- Older ingest/Smart Order sections describe smaller request ceilings and AI restricted to unresolved rows; newer AI documentation and orchestration settings differ.
- Smart Order documentation describes shared match decisions as non-tenant-scoped catalogue knowledge; current memory management exposes organization-owned and explicitly promoted platform scopes.
- Compare documentation's older ladder/AI description does not fully describe the current shared-engine and pair-check flow.
- The spec does not copy these contradictions as future requirements. FR-114 requires source, screens, defaults, and affected module documentation to agree when the behavior is implemented.

## Scope classification beyond the five workflows

A repository-wide reference search also found `assistant`, temporary-warehouse staging, and team-import code. Inspected assistant use was a feature-attribution constant for chat, not an additional product matcher. Inspected warehouse/team references used shared layout/header analysis. These references alone do not justify rebuilding those features. Planning must still inventory any downstream product-identity assignment reached from them and include it if it participates in the five workflows. This distinction keeps “all consumers” comprehensive without expanding the task to every import or AI feature.

## Decisions intentionally left to technical planning

The spec does not choose retrieval algorithms, storage/index layouts, worker topology, request serialization, provider integrations, migrations, or concrete production prompt wording. Planning must choose these against the identity and measurable outcome requirements. It must also freeze benchmark fixtures/reference capacity, select the confidence-bound method, set actual spending ceilings before qualification, and resolve any missing retention duration before collecting new payloads.

No implementation, migration, live model qualification, production corpus sampling, or runtime test execution occurred during specification creation. Those activities are required to demonstrate the future acceptance criteria, not to claim the specification itself is complete.
