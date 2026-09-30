# Contract: Identity Gate (`internal/shared/productmatch`)

This is the single acceptance authority. **Every** path that can associate a row with a product calls `Gate.Evaluate` before its result is stored as `accepted` or `confirmed`. That covers identifier, knowledge, exact, similarity, reviewer and AI paths. The entry-point test (research R1) and a dedicated test that greps for writers of `match_state='accepted'` enforce this.

## Go surface (signatures are normative; bodies are not)

```go
// Facts
func ExtractRowFacts(row *Row, m *Mapping, corrections []Correction) IdentityFacts
func (p *MasterProduct) Facts() IdentityFacts            // computed at index build, incl. curated overrides
func CompareFacts(row, product IdentityFacts) Comparison // 4-valued per attribute + Hard []ConflictKind

// Policy
type Granularity uint8 // None, Product, Pack
type Policy struct {
    Version            string
    MinStrong          float64 // qualified per profile (R11); not a probability
    MinReview          float64
    TieMargin          float64
    BlindNearIdentical float64
    Required           Granularity
    AllowOmittedAttrs  bool    // FR-023: only when uniquely identified
    Identifiers        IdentifierPolicy // namespaces declared + chosen (R5)
}

// Gate
type Path uint8 // Identifier, Knowledge, Exact, Similarity, Reviewer, AI
type Proposal struct {
    Path      Path
    ProductID int64
    Siblings  []int64      // other eligible candidates, for uniqueness/ambiguity
    Evidence  *AIEvidence  // Path==AI only: per-attribute status from the answer
}
type Verdict struct {
    Eligible    bool
    State       matchflow.State   // accepted | needs_review | ambiguous | ...
    Achieved    Granularity
    Pack        PackOutcome
    Hard        []ConflictKind
    Reasons     []Reason
}
func (g *Gate) Evaluate(rowFacts IdentityFacts, p Proposal, pol Policy) Verdict

// Index (unchanged shape, new semantics)
func (idx *Index) Match(row *Row, pol Policy) matchflow.RowDecision   // identifier tiers → Gate
func (idx *Index) Revision() (catalog int64, normalizer string)
```

## Gate rules (evaluated in order; the first failure determines the verdict)

1. **Existence and eligibility.** The product exists in this index revision, is active, and is visible to the scope. Otherwise → `not_eligible_target`.
2. **Hard conflicts.** Any `Hard` from `CompareFacts` ⇒ `Eligible=false`, state `needs_review`, reasons listed. This applies to all paths including `Identifier` (the identifier-vs-description rule) and `Reviewer`. A reviewer path requires that a `corrected` fact removes the conflict first.
3. **Identifier path extras.** The namespace is declared for the column, the value is valid (GTIN check digit), and it is unique within the namespace. Otherwise the verdict falls back to similarity evaluation. It never silently passes.
4. **Uniqueness.** If another eligible sibling is not separable on stated facts ⇒ `ambiguous`, with `missing_attributes` listed (FR-023, US2-4). A tie-break may order siblings, but it never accepts one.
5. **Omitted attributes.** `unstated` on strength/form/release is allowed only if rule 4 passed **and** `pol.AllowOmittedAttrs`. Otherwise → `needs_review` with reason `attribute_unstated`.
6. **Blind similarity** (existing `settleable`). Letters-only evidence cannot accept (keeps today's measured rule).
7. **Threshold.** Similarity/Exact paths require `Strength ≥ MinStrong`. AI requires a qualified configuration for the cohort. Knowledge requires non-stale, compatible entries. Reviewer has no threshold.
8. **Granularity.** `Achieved = Pack` only if pack facts agree or a `pack_relationships` entry bridges them. If `Achieved < Required`, state `needs_review` with reason `pack_unresolved`. The product ID may still be recorded as a suggestion.

## Hard conflict kinds

`strength`, `strength_component`, `combination`, `modifier`, `modifier_unstated_in_row`*, `form`, `route`, `release`, `letter`, `concentration`, `dimension`, `brand`, `identifier_vs_description`, `pack` (only when `Required=Pack`).

\* `modifier_unstated_in_row` (the catalogue names a line extension the row omits) is hard **only when a plain sibling exists in the index**. Otherwise it is `unstated`. This preserves today's measured behaviour, where the extended product is the only family member.

## Reason codes (stable; rendered through i18n)

`identifier_gtin`, `identifier_catalog_code`, `identifier_supplier_code`, `identifier_invalid`, `identifier_duplicate`, `identifier_vs_description`, `exact_name`, `similarity`, `similarity_letters_only`, `knowledge_human`, `knowledge_curated`, `knowledge_platform`, `knowledge_stale`, `knowledge_rejected`, `ai_selected`, `ai_abstained`, `ai_rejected_not_offered`, `ai_rejected_conflict`, `ai_rejected_unqualified`, `ai_disagrees`, `strength_conflict`, `component_conflict`, `combination_conflict`, `modifier_conflict`, `form_conflict`, `route_conflict`, `release_conflict`, `letter_conflict`, `concentration_conflict`, `dimension_conflict`, `brand_conflict`, `attribute_unstated`, `pack_unresolved`, `pack_conflict`, `pack_needs_relationship`, `ambiguous_siblings`, `no_candidate`, `retrieval_incomplete`, `catalog_unavailable`, `catalog_changed`, `invalid_row`, `excluded_header`, `excluded_total`, `excluded_blank`, `excluded_user`, `numeric_uninterpretable`.

Each code has `{ar, en}` entries in the UI catalogue. A test fails when a code lacks either language.

## Determinism guarantees

- Same `(facts, policy, index revision)` produces a byte-identical `Decision` (candidate order, reasons and flags) regardless of row order or `MatchAll` worker count. This is property-tested with shuffles and `workers ∈ {1, 4, 16}`.
- No clock, no randomness and no map-iteration order in output.
