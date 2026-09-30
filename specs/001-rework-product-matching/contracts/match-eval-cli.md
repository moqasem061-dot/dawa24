# Contract: Evaluation and Benchmark CLI

Binary: `cmd/cli`. The sub-commands are offline and deterministic unless `--ai` is given. They never write to tenant tables.

## Corpus case (JSONL, one per line)

```json
{
  "id": "c-000123",
  "split": "tuning|heldout|adversarial",
  "template_family": "tf-9a1c",
  "expression_family": "ef-33d0",
  "workflow": ["vendor","savings"],
  "language": "ar|en|mixed",
  "row": {"cells": {"name": "…", "strength": "…", "code": "…"}, "mapping": {"name": 0, "strength": 1},
          "identifiers": [{"column": "code", "namespace": "supplier_code"}]},
  "required_granularity": "pack",
  "label": {"resolvable": true, "targets": [48213], "granularity": "pack",
            "expect": "accept|abstain|conflict|ambiguous", "difficulty": ["pack_sensitive","close_strength"],
            "labellers": 2, "disputed": false},
  "catalog_snapshot": "cat-2026-10-01"
}
```

The catalogue snapshot is a separate JSONL of `MasterProduct` rows plus curated facts and relationships, pinned by SHA-256 in `testdata/matching/corpus.lock`.

## Commands

```text
cli match-eval  --corpus <dir> --split heldout|tuning|adversarial
                --engine baseline|current [--policy <file|version>]
                [--ai --config <ai_match_configuration id> --budget <file>]
                --out <report.json> [--md <report.md>]

cli match-bench --catalog <snapshot> --sizes 100,1000,10000,50000 --runs 30
                --orgs 1|5 --state cold|warm [--ai-sim fast|slow|down|budget]
                --out <bench.json>

cli match-corpus-export  --workflow <w> --since <date> --limit <n> --out <dir>   # AsSystem; de-identifies (R17)
cli match-corpus-split   --in <dir> --out <dir> --seed <n>                       # family-disjoint split
cli match-knowledge-backfill --dry-run|--apply                                   # R8 migration
```

`--engine baseline` runs the matcher at the git revision recorded in the corpus lock. It is built once into `bin/match-baseline` by `make match-baseline`, so baseline and candidate share inputs byte-for-byte.

## Report (JSON; the markdown version is a rendering of it)

Per `{overall, workflow, language, difficulty, identifier_present, pack_sensitive}`:
- `n_cases`, `n_resolvable`, `n_disputed_excluded`
- `accepted`, `accepted_correct`, `precision`, `precision_lb95` (Clopper–Pearson)
- `coverage_resolvable`, `coverage_all_rows`, `recall_at_20`, `review_workload`
- `hard_conflict_violations` (listed by case ID and path), `absent_or_ambiguous_accepted`
- `unavailable_vs_none_confusions`
- AI block: `gain_new_correct`, `new_wrong`, `lost_correct`, `net_gain_rate`, `ai_added_precision_lb95`, `attempts`, `tokens`, `cost`, `cost_known`, `cost_per_verified_gain`, `zero_gain_reasons{}`
- `gates{SC-001..SC-004, SC-007, SC-014}: pass|fail|insufficient_sample` — **`insufficient_sample` is never rendered as pass**

Bench report: per size and load condition, p50/p95/max reviewable time and stage breakdown (read, parse, catalogue/index, match, save). Plus index build time, cache hit/miss, the environment fingerprint (CPU, RAM, Postgres version, catalogue SHA), and AI simulation outcomes (deadline adherence, terminal-state publish latency, late-answer effects = 0).

## Exit codes

`0` all requested gates pass; `2` a gate failed; `3` insufficient sample; `1` usage or IO error. CI runs `match-eval --split adversarial` on every PR touching `productmatch`, `matchflow` or `aicapabilities` (the adversarial set is in git, so there is no private data).
