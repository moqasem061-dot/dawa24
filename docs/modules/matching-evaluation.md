# Matching Evaluation & Labelling Workflow

## Overview & Privacy Guarantees

Dawa24 evaluates product-matching precision, recall, and quality gates against real-world upload data without compromising pharmacy privacy or commercial secrecy:

1. **Zero Raw Uploads in Git:** Customer order files, vendor price lists, and real uploaded spreadsheets never enter the git repository.
2. **De-Identification:** The `match-corpus-export` command drops all organization IDs, prices, costs, and quantities, and replaces supplier codes with deterministic, namespace-preserving SHA-256 surrogates (`sc_<hash>`).
3. **External Corpus Storage:** Ground-truth corpus files and label files reside in access-controlled object storage (`s3://dawa24-eval-corpus/` or a secure local staging directory outside the repository root).
4. **Git Pinning:** Only the SHA-256 fingerprints of the catalogue snapshot, tuning split, and held-out manifest are tracked in `testdata/matching/corpus.lock`.

---

## Labelling Protocol

To establish statistically sound ground truth for quality gates (SC-001 through SC-004, SC-007, SC-014), cases are labelled according to the following protocol:

### 1. Dual Independent Labelling
Every case sampled from real uploads is independently evaluated by two qualified domain experts (licensed pharmacists):
- **Resolvable (`bool`):** Does the raw text describe an actual pharmaceutical product present in the catalogue?
- **Targets (`[]int64`):** The exact canonical product ID(s) matching the requested item.
- **Granularity (`product` | `pack`):** Whether identity requires product-level match or exact pack size/content match.
- **Expectation (`expect`):**
  - `accept`: Clear factual identity match.
  - `abstain`: Molecule/brand missing or completely unrecognizable.
  - `conflict`: Hard contradiction present (e.g. 500 mg vs 1000 mg, tablet vs syrup).
  - `ambiguous`: Insufficient distinguishing evidence between sibling variants.
- **Difficulty Tags:** Characterizes specific edge cases (`arabic_shorthand`, `cross_script`, `concentration_equivalence`, `pack_sensitive`, `combination_order`, etc.).

### 2. Agreement & Direct Resolution
When both labellers agree on `resolvable`, target IDs, and expectation:
- The case is recorded with `labellers: 2`, `disputed: false`.
- The case is directly included in evaluation splits.

---

## Adjudication & The Dispute Rule

When the two independent labellers disagree:

### 1. Adjudication Procedure
- The case is escalated to the **Senior Clinical Adjudicator**.
- The adjudicator inspects Egyptian Drug Authority (EDA) registration records, official packaging, and manufacturer specifications.
- If factual evidence proves a definitive match, the adjudicator records the decision, logs the rationale, and marks `disputed: false`.

### 2. The Dispute Rule (FR-108, Research R17)
- If contradictory evidence cannot be definitively resolved (e.g. ambiguous local trade naming, contradictory manufacturer packaging revisions, or undecidable dosage forms):
  - The case is classified as **Disputed** (`disputed: true`).
  - **Exclusion Rule:** Disputed cases are **strictly excluded from accuracy, precision, and coverage denominators** (`NDisputedExcluded`).
  - **Transparency Invariant:** Disputed cases MUST still be tracked and reported in summary statistics (`n_disputed_excluded`). They are never hidden or quietly dropped.

---

## Split Integrity & Leakage Prevention

Cases are partitioned using `match-corpus-split`:
- **Template Family (`tf-<hash>`):** Derived from `hash(source_org_id, file_layout)`.
- **Expression Family (`ef-<hash>`):** Derived from `hash(QuestionSignature)`.
- **Family-Disjoint Invariant:** No template family and no expression family may cross splits between `tuning` and `heldout`. Transitive graph clusters of shared families are placed entirely within one split.
- **Verification:** CI and benchmark runners reject any split where `set(tuning) ∩ set(heldout) != ∅`.

---

## Corpus Lock & Manifest

`testdata/matching/corpus.lock` records cryptographic digests ensuring reproducible benchmarks:
- `baseline_sha`: Git commit hash of the legacy baseline comparison binary.
- `catalogue_sha256`: SHA-256 of the frozen canonical catalogue snapshot.
- `tuning_cases_sha256`: SHA-256 of the tuning cases split.
- `heldout_manifest`: URI and SHA-256 of the held-out manifest and associated label files.
