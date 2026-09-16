# FINAL PROMPT — WAVE 4, PHASE 6, PHASE 7

Everything remaining on the Dawa24 rebuild, in one document. This supersedes all earlier
phase and wave prompts.

**Start from:** `phase-5-conversion` at `f335edc`. Verify with `git log --oneline -1`.
Branches: `phase-5-wave-4` → `phase-6-structure` → `phase-7-verification`.

**Three report gates, not nine** — after Wave 4, after Phase 6, after Phase 7. This is a
deliberate compression to finish faster. The measurement discipline below is **not**
compressed; it is what makes moving fast safe.

---

## 0. WAVE 3 — ACCEPTED

| Target | Before | After | Ceiling |
|---|---|---|---|
| inline styles | 2657 | **2170** | must match |
| Arabic literals | 1223 | **966** | must match |
| raw `<dialog>` | 31 | **20** | must match |
| `!important` | 3 | **3** | held |

Track 2 moved **257 literals**, clearing the 250 target. Three waves running with every
ceiling honest. Keep that going — it is the only reason this can now be compressed.

---

## 1. NON-NEGOTIABLE RULES

Fast does not change any of these. They exist because each one has already gone wrong once.

1. **Measure, then set the ceiling to the measured number.** Never below it. Phase 3 and
   Wave 1 both shipped failing builds this way.
2. **Paste raw command output. Never transcribe a number.**
3. **Mark every gate MET or MISSED with its number.** A skipped task is a discrepancy and
   must be reported as one.
4. **THE STOP RULE.** If a quote or count does not match, stop that item, report it, move on.
   Do not improvise.
5. **No authorization changes.** Phases 0–1.5 are settled. Report bugs; do not fix them.
6. **No new `go.mod` dependencies except `deadcode` in Phase 6. No database. No SQL.**
7. **Do not delete or rewrite existing comments.** Past tense when a cause is removed.
8. **`!important` stays at 3. No new breakpoints beyond 640 / 768 / 1024.**
9. Read `.impeccable.md` before UI work. The **counter test** governs: one hand, mid-range
   Android, bright light, interrupted.
10. **Name your commits for what they do.** `FGASF`, `KHL;`, `gdfgdf` cost nothing today and
    an hour in six months.

### Gate commands — `make` is not installed; run these and paste output

```bash
gofmt -l ./cmd ./internal
go vet ./...
go test ./... -count=1
grep -oh 'style="' internal/ui/pages/*.templ internal/ui/layouts/*.templ | wc -l
grep -oh '<dialog' internal/ui/pages/*.templ | wc -l
grep -oh '!important' internal/ui/static/css/*.css | wc -l
find ./cmd ./internal -name '*.go' -not -name '*_templ.go' -exec wc -l {} + | grep -v " total$" | awk '$1>400' | wc -l
pat=$(printf '\330'); LC_ALL=C grep -rc "\"[^\"]*$pat[^\"]*\"" --include='*.go' internal/ui internal/modules cmd | grep -v "_test\|_templ\|/i18n/" | awk -F: '{s+=$2} END{print s+0}'
cat internal/ui/static/css/{tokens,base,layout,components,foundations,utilities,app}.css | wc -c
```

`templ` is at `/c/Users/mydwa/go/bin/templ`. Regenerate only files you edit; commit the
generated `*_templ.go` (they are deliberately tracked).

---

# WAVE 4 — FINISH THE CONVERSION

**74 page templates still carry inline styles.** Heaviest first: `suppliers` 138,
`customer_saving` 119, `smart_order_review` 100, `customer_jobs` 100, `auth` 100, `jobs` 89,
`wallet` 77, `invoice_printable` 75.

**20 raw `<dialog>` blocks remain**, spread across `promo_revenue`, `customer_saving`,
`admin_brands`, `suppliers`, `customer_branches`, `admin_products`, `tenant_subscription`,
`compare_mapping`, `admin_plans`, `admin_institutional`, `admin_audit`.

### Track 1 — pages

Convert all remaining pages. Same ten-item checklist, per page:

1. Inline `style=` → Phase 3 classes.
2. Ad-hoc markup → components. Every `<dialog>` goes through `components.Modal`.
3. Loading, empty, error states present. Empty states **teach the interface**.
4. Verified at 375px. Tables become card lists below 768px, not sideways scrollers.
5. Keyboard traversable, visible focus.
6. Both themes.
7. RTL and LTR.
8. Arabic literals → `i18n.T(lang, key)`.
9. Touch targets ≥ 44px in `comfortable` density.
10. No `!important` added, no new breakpoint.

**Targets: inline styles ≤ 300, raw `<dialog>` = 0.**

If a page genuinely cannot reach 0 inline styles — a print stylesheet like
`invoice_printable`, a dynamically computed position — leave the style inline **with a
`var(--token)` value, never a literal**, and list it in your report with the reason.

### Track 2 — module strings

**966 Arabic literals remain**, still concentrated outside pages:
`catalog/import_rows.go` 55, `ingest/domain.go` 46, `smartorder/pipeline/query.go` 45,
`compare/columns_data.go` 41, `compare/comparison.go` 36.

**Target: ≤ 200 by end of Wave 4.**

Same care as Wave 3: a log line or internal identifier is **not** user-facing and must not
become an i18n key. Report anything you leave alone and why. If `depguard` blocks a module
from importing `i18n`, **stop and report** — do not weaken the boundary in `.golangci.yml`.

### Shell CSS — decide, do not carry it silently

Shell CSS is **149,028 bytes** against a 90 KB target. It has moved ~2 KB across four
attempts. Make one focused attempt: as pages convert, move component rules used by only one
surface out of `components.css` into that surface's sheet.

Then **report the number and state plainly whether 90 KB is reachable.** If it is not, say so
and propose the number that is. A target carried unmet to Phase 7 measures nothing — retiring
it honestly is a better outcome than four more silent misses.

### Ratchets

Lower `check-inline-styles`, `check-modal-handwritten`, `check-hardcoded-arabic` to measured
values. Add `check-shell-css` at whatever byte count you land on.

**GATE: report and stop.**

---

# PHASE 6 — STRUCTURE AND DEAD CODE

### TASK 1 — Reorganise `internal/ui` by audience

~52,000 lines of handlers sit flat beside `pages/`, `components/`, `layouts/`. Group them the
way routes are already gated: `internal/ui/admin/`, `vendor/`, `customer/`, `shared/`,
`public/`, plus `view/` for the domain→template mapping now spread across `*_models.go`.
`pages/`, `components/`, `layouts/`, `static/` unchanged.

**No behaviour change in the same commit as a move.** Move, verify tests, then change
anything else. One audience per commit — five commits, not one.

### TASK 2 — Split oversized Go files

**101 files exceed the project's own 400-line limit.** Split by concern: read handlers, write
handlers, view-model mapping are the natural seams. Lower `check-file-size-count` as you go.
Target **0**; report what resisted and why.

### TASK 3 — Decompose oversized templates

**14 templates exceed 1,000 lines**, largest `promo_revenue.templ` at 2,006. Use the Wave 2
pattern that worked: split by subject into focused files, and verify every `templ` function
survives the move. That check is what made the `admin_catalog_inventory` split safe.

### TASK 4 — Dead code

`golangci-lint`'s `unused` misses exported symbols nothing calls. Add
`golang.org/x/tools/cmd/deadcode` to `make check` with its own ratchet, run it, remove what it
finds.

**Read every finding before deleting.** A symbol may be referenced from a `.templ` file that
`deadcode` does not parse. Anything ambiguous goes in the report, not in a delete commit.

### TASK 5 — Documentation

Update `AGENTS.md`. Add ADRs in `docs/adr/` for:
- the `@layer` order;
- the `DashboardTopBar` contract;
- the API gate pattern and why it returns 403 where the UI returns 404;
- **the tenant-vs-admin permission namespace split** — `commerce.*` is admin-scope,
  `vendor.*` / `pharmacy.*` are tenant-scope. Confusing them broke supplier fulfilment in
  Phase 0 and cost a whole correction phase. Write it where the next engineer will find it.

**GATE: report and stop.**

---

# PHASE 7 — VERIFICATION

### TASK 1 — Permission matrix
Every role × every route, asserted, as a table a human can read. This becomes the canonical
statement of who can do what.

### TASK 2 — Account lifecycle
`pending` → `under_review` → `approved` → `suspended` → `rejected`, asserting exactly what each
state reaches on both UI and API. This was the product owner's original concern; give it its
own test.

### TASK 3 — Visual regression
Every component from the Phase 3 contract: every variant, both densities, both themes, both
directions.

### TASK 4 — Real devices — the counter test, executed
Mid-range Android and iOS Safari, RTL and LTR, at 375px. Complete a full order — find a
product, compare suppliers, add to cart, check out — **one-handed**. Report where it is
*awkward*, not only where it breaks. This is the acceptance test for the whole rebuild.

### TASK 5 — Accessibility
Keyboard-only traversal of all three dashboards. Screen reader on sidebar, top bar, modals.
Contrast in both themes: AA on body, AAA on data.

### TASK 6 — Performance
Lighthouse has been **NOT RUN** since Phase 2, so the budget has never been enforced. If a
staging URL exists, run it against the public home page, customer catalogue,
`/admin/organizations` and the cart: perf ≥ 80 mobile, LCP ≤ 2.5s, TBT ≤ 300ms, CLS ≤ 0.1.
If not, say so plainly — do not estimate.

### TASK 7 — Final report
Every gate, current value against ceiling, raw output. Then the **residual list**: every target
not reached, its number, and why. That list is the handover document for whoever picks this up
next.

**GATE: all green, or every gap named with a number.**

---

## REPORTING — three times

Each report:

1. Per task / per page: `DONE` / `PARTIAL` / `STOPPED`, files touched, what and why.
2. Raw pasted output for every gate.
3. Every gate marked MET or MISSED with its number.
4. Discrepancies, including anything you did not do.
5. `git diff --stat` against the parent branch.

Wave 4 adds: Track 1 and Track 2 separately; strings judged non-user-facing; any `depguard`
block; the shell CSS verdict.
Phase 6 adds: ambiguous `deadcode` findings; templates whose split you could not verify.
Phase 7 adds: the residual list.

**Stop at each of the three gates.**
