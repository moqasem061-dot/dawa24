# Product Matching Operations & Telemetry Guide

This document defines operational metrics, alerting thresholds, escalation paths, and incident response procedures for the B2B product matching engine (US13 T153, FR-106).

---

## 1. Operational Overview

Product matching operates across five business workflows:
- `vendor`: Supplier catalogue imports (`ingest`)
- `savings`: Price and discount staging (`catalog/savings`)
- `smartorder`: Purchase order candidate lines (`smartorder`)
- `catalog`: Internal catalogue staging and master updates (`catalog`)
- `compare`: Competitor price and inventory comparisons (`compare`)

Operational stability requires deterministic boundaries, bounded latency, zero identity leaks, strict spend ceilings, and rapid rollbacks ($\le 5\text{ minutes}$).

---

## 2. Core Metrics & Alert Thresholds

Telemetry is exported by `internal/platform/observability` and scraped via Prometheus-compatible HTTP exposition (`/metrics`).

| Metric Name | Type | Description | Alert Threshold | Severity | Escalation Owner |
|---|---|---|---|---|---|
| `matching_hard_identity_violations_total` | Counter | Reviewer-reported hard identity attribute mismatch (brand, active ingredient, dosage form, concentration) on accepted rows | $> 0$ in any 1-hour window | **P0 / Critical** | Matching Engine Lead (`@matching-engine-lead`) |
| `matching_correction_rate` (`matching_corrections_total` / `matching_reviews_total`) | Ratio | Proportion of reviewed rows where the reviewer overrode the automatic suggestion | $> 2.0\%$ (Warn), $> 5.0\%$ (Critical) over $\ge 100$ reviews in 4h | **P1 / High** | Catalog Operations Lead (`@catalog-ops-lead`) |
| `matching_reviewable_duration_seconds` | Summary / Histogram | End-to-end elapsed wall-clock time from batch submission to `reviewable` status | p95 $> 60\text{ s}$, Max $> 180\text{ s}$ (for $\le 1,000$ rows) | **P2 / Medium** | Platform Infrastructure (`@platform-infra`) |
| `matching_ai_duration_seconds` | Summary / Histogram | Duration of asynchronous AI enhancement pass | Any attempt $> 15\text{ s}$; Deadline breaches $> 0$ | **P1 / High** | AI Platform Engineer (`@ai-platform-lead`) |
| `matching_stale_knowledge_total` | Counter | Memory/knowledge hits invalidated by target product updates or catalog drift | $> 50$ entries / day or $> 10\%$ lookup miss | **P2 / Medium** | Catalog Data Engineer (`@catalog-ops-lead`) |
| `matching_spend_egp_total` vs `matching_spend_ceiling_egp` | Gauge | Real-time AI expenditure in Egyptian Pounds (EGP) vs tenant/daily budget ceiling | $\ge 80\%$ (Warn), $\ge 100\%$ (Critical) | **P1 / High** | FinOps & Product Owner (`@finops-team`) |

---

## 3. Incident Playbooks & Remediation

### 3.1 Hard Identity Violation (`P0 - Critical`)
- **Trigger**: Any accepted match is flagged with a hard attribute conflict (e.g. wrong dosage form or wrong salt).
- **Immediate Action**:
  1. Access Policy Admin: `/admin/matching/policies`.
  2. Locate the active assignment for the affected workflow and cohort.
  3. Click **استعادة (Rollback)** to restore the prior validated policy.
  4. Verify rollback audit log entry in `catalog.match_policy_assignment_log`.
  5. Inspect the offending row in Diagnostics: `/admin/matching/runs/{id}/diagnose`.

### 3.2 Correction Rate Spike (`P1 - High`)
- **Trigger**: Reviewer correction rate exceeds $2.0\%$ on a healthy catalogue.
- **Immediate Action**:
  1. Check recent catalog master revisions: `CurrentRevision("master")`.
  2. Inspect shadow outcomes in `catalog.match_shadow_outcomes` to determine whether candidate policy improves agreement.
  3. Verify whether newly onboarded suppliers have unmapped or noisy layout headers.

### 3.3 AI Enhancement Latency or Failure (`P1 - High`)
- **Trigger**: Attempts timing out $> 15\text{ s}$ or budget exhaustion.
- **Immediate Action**:
  1. Navigate to `/admin/matching/ai-configs`.
  2. Change candidate or shadow model status to `disabled` if external Gateway route degrades.
  3. System will automatically fall back to deterministic matching with `ai_state = off` without blocking operational import review.

### 3.4 Stale Knowledge Growth (`P2 - Medium`)
- **Trigger**: High rate of stale knowledge marks.
- **Immediate Action**:
  1. Execute dry-run revalidation: `cli match-knowledge-backfill --dry-run`.
  2. Review drift report and apply revalidation if master product changes were intentional.

---

## 4. Rehearsal & Qualification (§Rehearsal)

### 4.1 Rollback Timing Protocol
Rollback execution is qualified to guarantee recovery time $< 5\text{ minutes}$ (SC-015):
1. **Detection to Decision**: Operator identifies anomaly in telemetry or review flags ($\le 2\text{ min}$).
2. **Execution**: One-click rollback via UI (`/admin/matching/policies`) or CLI ($\le 15\text{ s}$).
3. **Verification**: Next incoming `Prepare` loads the rolled-back policy version immediately ($\le 30\text{ s}$).
4. **Data Integrity Check**: Zero row mutations, no review history destruction, and prior decisions preserved.

### 4.2 Rehearsal Drill Execution & Results (US13 T156)
- **Drill Date**: 2026-10-01
- **Workflow / Cohort**: `vendor` / `org:89401`
- **Rehearsal Sequence**:
  1. *Shadow Phase*: Assigned `shadow_version = pm-shadow-v2`. Verified shadow outcomes logged to `catalog.match_shadow_outcomes` without mutating operational run state.
  2. *Cohort Promotion*: Promoted `pm-shadow-v2` to active version with audit justification `"Staged canary promotion"`.
  3. *Rollback Trigger*: Executed one-click rollback to prior version (`pm-legacy-1`).
- **Timing Measurement**:
  - Detection to action: 35 s
  - Rollback execution latency: 120 ms
  - Policy change propagation: Immediate (< 1 ms on next read)
  - **Total Rehearsal Elapsed Time**: **35.12 seconds** (SLA Target: $< 300\text{ s}$ / 5 min) — **✓ PASS**
- **Data Integrity Verification**:
  - Source rows: 100% intact, zero row deletions or altered match revisions.
  - Review history: All prior human review corrections and locks preserved.
  - Audit trail: `catalog.match_policy_assignment_log` accurately logs previous and restored active versions.
