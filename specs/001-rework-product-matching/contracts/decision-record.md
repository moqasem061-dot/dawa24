# Contract: Persisted Decision Record v1

Every consumer row table stores the same shape (data-model §3.10). The JSON is produced **only** by `matchflow.RowDecision.MarshalJSON` and read only by `matchflow.ParseRowDecision`, which rejects unknown major versions.

```json
{
  "v": 1,
  "state": "needs_review",
  "basis": "similarity",
  "product_id": 48213,
  "granularity": {"required": "pack", "achieved": "product"},
  "pack": {"outcome": "unstated"},
  "candidates": [
    {"product_id": 48213, "rank": 1, "eligible": true,
     "diff": {"strength": "agree", "form": "agree", "pack": "unstated"},
     "reasons": ["similarity"]},
    {"product_id": 48214, "rank": 2, "eligible": false,
     "diff": {"strength": "conflict"}, "reasons": ["strength_conflict"]}
  ],
  "reasons": [{"code": "pack_unresolved"}],
  "flags": ["retrieval_incomplete"],
  "versions": {"policy": "pm-2026.10.1", "normalizer": "n7", "catalog_revision": 91822},
  "revision": 3,
  "ai": {"state": "completed", "outcome": "abstained", "attempts": [9912]},
  "actor": null
}
```

## Column projection (written in the same statement as the JSON)

| Column | Derived from |
|---|---|
| `match_state` | `state` |
| `match_basis` | `basis` |
| `match_granularity` | `granularity.achieved` |
| `matched_product_id` (existing) | `product_id` **iff** state ∈ {accepted, confirmed}, else NULL (FR-041). Suggestions live only in JSON. |
| `match_level` (existing, legacy) | mapping: identifier→`barcode`/`code`; exact→`exact`; accepted similarity→`strong`; needs_review→`review`; ambiguous→`ambiguous`; else `none` |
| `ai_state` | `ai.state` |

`baseline_decision` holds the deterministic decision before any AI change and is written once per revision (FR-064).

## Write rules

- `INSERT/UPDATE … WHERE match_revision = $expected` (optimistic). Human writes set `human_locked=true` and increment the revision. AI writes also require `human_locked=false AND match_run_id=$run`.
- A consumer's commit reads `(matched_product_id, match_granularity, match_revision)`, revalidates through the service (`Revalidate`), and rejects on a revision mismatch or `granularity < required` (FR-093).
