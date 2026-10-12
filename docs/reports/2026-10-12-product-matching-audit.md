# Product matching audit — 12 October 2026

Scope: vendor file parsing, canonical decisions, staging projections, manual
review, stored rows and commit revalidation. AI-disabled operation remains the
baseline. No production database was inspected or changed.

## Root causes and repairs

1. **Invented confidence.** `ingest/catalog_stage_run.go` replaced candidate
   scores with `1.0`, independent of decision state. A suggestion with an
   identity conflict therefore displayed 100% beside “لن يستورد”. Preserve the
   canonical score; repair the display of existing rows from saved decisions.
   This changes no product link and does not automatically approve suggestions.
2. **Welded shorthand.** `2.5مجم30ق` was not expanded because the parser only
   understood a token beginning with a count and ending with its abbreviation.
   The remaining `ق` was read as an identity marker, creating a false conflict.
   Separate a dosage unit from its following pack count before expansion;
   retain tests proving a genuinely different strength is still rejected.
3. **Divergent name preparation.** Fact extraction stripped alternative-product
   clauses, while the scorer could read their words and figures as identity.
   Both now remove those clauses. Explicit form comparisons remain unsettled.
4. **Dilution bypass.** Production scoring could compare drug mass and ignore
   the concentration denominator. Typed mass/volume ratios now participate in
   the same conflict model used by scoring, memory, reviewers and AI. Different
   vial amounts retain their distinct identity even with equal density.
5. **Version drift.** Index revisions returned a hardcoded normalizer version.
   They now use the same constant as question signatures, so changed parsing
   does not silently reuse old normalization keys.

## Review and commit checks

Unsettled suggestions keep `matched_product_id` null; a high score alone does
not make a row eligible. Manual review remains explicit. Exclusions, source
revisions and revalidation are retained. The current commit path compares the
achieved granularity to the decision's required granularity (product for this
workflow); older module prose describing an unconditional pack requirement was
not an accurate description of the current code.

## Evaluation

The unchanged HEAD was evaluated from a temporary source snapshot, with the
same corpora and CLI production `Index.Decide` path as the changed version.

The 11,912-case Egyptian benchmark baseline accepted 8,876 rows, correctly
accepted 8,811, and had 65 wrong acceptances (99.27% precision; 88.44%
resolvable coverage). The changed engine retains this result. The three
reported compact/alternative-name regressions pass separately, as do different
strength, different dilution, distinct vial amount and compared-form guards.

The adversarial baseline had 6 wrong acceptances among 30 accepted rows and
candidate recall 97.30%. The audit repairs improve this suite; the generated
`reports/2026-10-12-matching-audit.json` and explanation contain the final
case-by-case result.

**The global matching quality gates are not all green.** The Egyptian
benchmark already fails the configured precision and candidate-recall gates.
The adversarial fixture also exposes unresolved bilingual descriptions and
policy/label differences for reordered combination doses and undeclared
supplier codes. These are not permission to auto-accept every suggestion or
silently treat supplier codes as master-catalog identifiers. They require a
separate measured policy/data qualification change before claiming launch-ready
matching precision. The display/parsing defects reported here are repaired;
this report does not certify perfect matching of unseen files.

Database/container validation was stopped and the isolated test container and
its storage removed at the user's request. Verification uses unit tests,
source checks, corpus evaluation and database-free browser fixtures.
