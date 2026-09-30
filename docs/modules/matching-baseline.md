# Product Matching Deterministic Baseline Report

- **Date Recorded**: 2026-09-30
- **Baseline Git SHA**: `1ec88e4f53f9d5a86511a74e79d6830013d90fd9`
- **Corpus**: `test/corpus` (Snapshot: `test/corpus/catalogue.jsonl.gz`)
- **Split**: `tuning` (`test/corpus/cases_tuning.jsonl.gz`)
- **Engine**: `current` (prior to Gate and Policy rework)

## Baseline Metrics

- **Total Cases**: 32,179
- **Resolvable Cases**: 32,179
- **Accepted**: 32,157 (Correct: 31,855)
- **Precision**: 99.06% (95% Clopper–Pearson LB: 98.97%)
- **Coverage (Resolvable)**: 98.99%
- **Candidate Recall@20**: 99.92%
- **Hard Conflict Violations**: 0

## Quality Gate Status (Baseline)

| Gate | Description | Status |
|---|---|---|
| SC-001 | Precision >= 99.5% with 95% LB >= 99.0% | ✗ FAIL (99.06% / 98.97%) |
| SC-002 | Hard conflict protection (100%) | ✓ PASS (0 violations) |
| SC-003 | Correct coverage >= 80% of resolvable rows | ✓ PASS (98.99%) |
| SC-004 | Candidate recall@20 >= 98% | ✓ PASS (99.92%) |

## Observations

1. The current engine has excellent retrieval recall (99.92% recall at 20) and broad coverage (98.99%).
2. However, precision on the tuning set falls short of the non-negotiable 99.5% threshold (99.06% vs 99.5%), failing SC-001.
3. This establishes the quantitative reference baseline (FR-107). Phase 2B/2C/2D and Phase 3 improvements will be measured directly against this baseline to verify that gate interventions improve precision to >= 99.5% without regressing recall.
