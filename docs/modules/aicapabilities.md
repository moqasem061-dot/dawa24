# Module: AI Capabilities

## Overview

The `aicapabilities` bounded context exposes intelligent domain augmentation (catalog product matching, column detection, search query expansion) strictly through the platform gateway (`internal/platform/gateway`).

## Invariants & Guarantees

1. **AI Provider Isolation:** Zero direct AI vendor SDKs or vendor names exist outside the platform gateway boundary.
2. **Deterministic Fallbacks:** When the AI gateway is disabled (`GATEWAY_ENABLED=false`) or encounters network timeouts/failures, the module instantly falls back to deterministic rule-based Arabic string normalization (`arabic.Similarity`) and keyword expansion.
3. **Black-Hole Resiliency:** Tested against unroutable destination IPs (RFC 5737 TEST-NET-1), ensuring zero panics or user-facing errors during upstream AI service interruptions.

## Unresolved Residue AI Enhancement (US9, US11, FR-060, FR-068)

AI enhancement operates strictly on the unresolved-but-resolvable residue following deterministic preparation across all five workflows (vendor import, savings import, smart order, catalog import, compare):

- **Eligible Population**: Rows in `needs_review` or `ambiguous` with available candidate options that are not human-locked and did not hit scoped knowledge.
- **Settled Rows Excluded**: Previously accepted or confirmed rows (`StateAccepted`, `StateConfirmed`) are settled and excluded from AI dispatch during ordinary runs.
- **Verification Pass**: Verification of settled rows is gated behind `Policy.AI.VerifyPass` (default `false`). When enabled, it counts toward the strict 3-attempt per question budget ceiling.
- **Per-Item Options Enforcement (FR-062)**: Every item restricts the model's choices to its explicit candidate shortlist (`Item.Options`). Any answer outside the offered options is rejected with `not_offered_for_row`.
- **Identity Gate Invariant**: All candidate answers must pass `Gate.Evaluate` (`Path = AI`). Hard conflicts (brand, form, concentration, release) are rejected.
- **Deterministic Independence**: If the Gateway is disabled or unreachable, the run transitions cleanly to `reviewable` with `ai_state = "off"`, and users can review and commit without blocking.


## Prompt v8 examples policy

Prompt v8 incorporates 10 normative few-shot examples defined in `internal/modules/aicapabilities/prompt_examples_v8.go`. The policy governing these examples:

1. **Synthetic and Leakage-Free:** All examples are completely synthetic. To prevent test set contamination, data leakage, and metric gaming, example product names, raw texts, and identifiers are forbidden from appearing in `testdata/matching/adversarial/cases.jsonl` or the held-out evaluation corpus. This invariant is enforced by automated unit tests (`TestPromptExamplesV8_NoLeakageInAdversarialOrHeldout`).
2. **Deterministic & Stable:** Examples are statically compiled into the binary rather than retrieved dynamically, ensuring 100% byte-reproducible prompt rendering across runs and cache hits.
3. **Comprehensive Category Coverage (FR-072):**
   - **Arabic shorthand:** Real-world Egyptian trade shorthand with price noise (`س.ج 45ج`), pack tokens, and colloquial abbreviations.
   - **Cross-script names:** Latin brand name matching an Arabic catalogue record.
   - **Strength distinction:** Hard numerical strength boundaries (e.g. 5 mg vs 10 mg); never rounding or picking the nearest match.
   - **Form distinction:** Critical pharmaceutical form divergence (cream vs ointment, tablet vs syrup).
   - **Pack distinction:** Exact pack count agreement when granularity `need="pack"` is required.
   - **Line extension:** Modifiers (Day/Night, Extra, Plus, Retard) indicate separate products.
   - **Manufacturer trap:** Shared manufacturer or distributor is never evidence of product identity.
   - **Prefix trap:** Shared leading prefixes (3–5 letters) do not justify conflating different brands.
   - **Unstated / ambiguous:** When catalogue variants differ in attributes omitted by the row, the model must abstain with `ambiguous`.
   - **Absent / no-candidate:** When the requested product is not represented in the catalogue window, the model must abstain with `absent`.
4. **Structured Evidence & Abstention:** Examples teach the model to populate the compact evidence mapping (`e`) and output correct abstention reasons (`x`), proving that abstention is a first-class, expected response.

