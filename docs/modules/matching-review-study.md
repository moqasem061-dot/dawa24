# Product Matching Review UX Observation Protocol (SC-006)

This document formalizes the usability and observational study protocol to satisfy **SC-006** (Reviewer Effort and Uncertainty Resolution) of the Product Matching Rework specification (`specs/001-rework-product-matching/spec.md`).

---

## 1. Objective & Success Criteria

The purpose of this study is to measure the efficiency, accuracy, and accessibility of human review across all matching workflows (Supplier Ingest, Smart Order, Savings Lists, Central Catalog Admin, and Comparison Tool) following the rollout of the unified `matchreview` components and keyboard navigation system.

### Success Criteria (SC-006 Non-Negotiables)

1. **Unassisted Task Completion**: $\ge 90\%$ of assigned tasks complete from file upload through valid commit without facilitator intervention or procedural assistance.
2. **Review Efficiency Gain**: Median active review time on matched-complexity files drops by at least **$30\%$** compared to the pre-rework baseline (legacy Livewire/table review).
3. **No Quality Regression**: Mistaken confirmations must not increase over baseline ($\le 0.5\%$ error rate on golden verification test items).
4. **Keyboard-Only Accessibility**: $100\%$ of core review actions (navigate, inspect, select, correct, exclude, undo) are completable using only keyboard navigation in both Arabic (RTL) and English (LTR).
5. **Mobile / Narrow Viewport Usability**: At $360\text{ px}$ display width, no critical identity fact (brand, form, concentration, pack size, code) or actionable button is obscured, clipped, or inaccessible.

---

## 2. Participant Cohort

The study requires a minimum of **eight (8)** representative participants partitioned across personas and primary working languages:

| ID | Persona | Typical Role / Experience | Language & Direction | Target Workflows |
|---|---|---|---|---|
| **P1** | Community Pharmacist | Pharmacy owner / buyer (10+ daily orders) | Arabic (RTL) | Smart Order, Savings Import |
| **P2** | Hospital Pharmacist | Formulary purchaser | English (LTR) | Smart Order, Compare Results |
| **P3** | Wholesale Vendor Ingest Staff | Supplier catalog entry operator | Arabic (RTL) | Vendor Ingest Review |
| **P4** | Distributor Catalog Manager | Data specialist managing price lists | English (LTR) | Vendor Ingest Review |
| **P5** | Platform Operations Specialist | Internal admin supporting onboarding | Arabic (RTL) | Admin Import, Diagnostics |
| **P6** | Platform Data Auditor | Central catalog curator | English (LTR) | Admin Import, Diagnostics |
| **P7** | Pharmacy Assistant | Regular pharmacy ordering staff | Arabic (RTL) | Smart Order, Savings Import |
| **P8** | Supplier Operations Lead | Regional pharmaceutical supplier | Arabic (RTL) | Vendor Ingest, Compare Results |

*Note: In accordance with repository conventions, at least 5 of the 8 participants must evaluate the interface in Arabic (`dir="rtl"`).*

---

## 3. Test Fixtures & Task Scenarios

Each participant executes three standardized review tasks on randomized, matched-complexity datasets containing known edge cases (dosage conflicts, combination drugs, concentration ambiguity, brand collisions).

### Task 1: Supplier Ingest Batch Review (Vendor Workflow)
- **Dataset**: `test/corpus/files/vendor-30.xlsx` (30 lines with ambiguous concentrations, trade names, and unit discrepancies).
- **Goal**: Review unconfirmed items, apply corrections for flagged conflicts, select eligible candidates, and commit the import.
- **Key Observation Points**: Candidate diff inspection, reason chips clarity, inline attribute correction form.

### Task 2: Smart Order Uncertainty Adjudication (Pharmacy Workflow)
- **Dataset**: 40-item simulated pharmacy purchase order with 6 unmatched items and 4 multi-candidate ambiguities.
- **Goal**: Resolve unmatched items via central catalog search and keyboard selection (1–5), exclude non-deliverable lines (`x`), and advance to order creation.
- **Key Observation Points**: Keyboard-driven navigation flow, speed of candidate selection, zero mouse usage.

### Task 3: Multi-User Concurrency & Conflict Notice
- **Scenario**: Two reviewers simulate concurrent review on the same staged batch. Reviewer A commits an attribute change on row 12 while Reviewer B is viewing row 12. Reviewer B triggers a select/correct action.
- **Goal**: Verify that the row re-renders with the bilingual revision conflict notice banner (`RevisionConflictNoticeBilingual`), displays the fresh server state, and preserves Reviewer B's scroll position and filters without full page reload.

---

## 4. Accessibility & Constraint Verifications

### 4.1 Keyboard Navigation Check
Participants P1, P3, P5, and P7 execute at least one full workflow using **keyboard only**:
- `j` / `↓`: Move focus to next row.
- `k` / `↑`: Move focus to previous row.
- `Enter`: Toggle candidate details / diff view.
- `1` – `5`: Select candidate 1 through 5 directly.
- `x`: Exclude row from import / batch.
- `Tab` / `Shift+Tab`: Navigate modal and inline form fields.
- `Escape`: Close active dropdown or candidate drawer.

**Pass Criteria**: Zero required mouse interactions. Every action triggers visual focus rings adhering to design tokens.

### 4.2 360-Pixel Viewport Verification
Conducted on Google Chrome Device Emulation ($360 \times 640\text{ px}$, DPR 3.0) and physical mobile test devices:
- Verify horizontal table scroll containers or responsive card views do not break page-level viewport width.
- Ensure product name, concentration, and pack unit do not truncate critical digits (using `<bdi>` and tabular numerals).
- Action buttons must maintain a minimum touch target size of $44 \times 44\text{ px}$ (or equivalent accessible padding).

---

## 5. Metrics & Data Collection Sheet

| Metric | Baseline (Legacy Monolith) | Target (Rework) | Measured Value | Status |
|---|---|---|---|---|
| **Unassisted Completion Rate** | $68\%$ | $\ge 90\%$ | *Recorded per session* | - |
| **Median Review Time / Ambiguous Row** | $28.4\text{ s}$ | $\le 19.8\text{ s}$ ($-30\%$) | *Recorded per session* | - |
| **Erroneous Confirmation Rate** | $2.1\%$ | $\le 0.5\%$ | *Audit of committed rows* | - |
| **Keyboard-Only Completion** | $0\%$ (Not supported) | $100\%$ | *Pass / Fail* | - |
| **360 px Viewport Invariant** | Failed (Horizontal clipping) | $100\%$ Accessible | *Pass / Fail* | - |

### Observational Logging Format

```tsv
SessionID	ParticipantID	Workflow	Language	Device/Width	InputMode	ActiveTimeSec	CompletedTasks	Interventions	ErrorCount	Notes
S-01	P1	SmartOrder	ar	Desktop	Keyboard-only	312	3/3	0	0	j/k navigation smooth
S-02	P2	Compare	en	Desktop	Mouse+Key	245	3/3	0	0	Diff table clear
S-03	P3	VendorIngest	ar	360px	Touch+Key	410	3/3	0	0	No clipped facts
```

---

## 6. Execution & Sign-off Procedure

1. **Pre-study Briefing**: Explain think-aloud protocol to participant without guiding UI interactions.
2. **Timed Session**: Facilitator logs timing and interventions silently.
3. **Post-Task Audit**: Verification against golden catalog records to calculate precision.
4. **Sign-off**: Formal acceptance requires review and sign-off by Product Lead and Engineering Lead.
