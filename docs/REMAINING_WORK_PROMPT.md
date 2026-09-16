# REMAINING WORK — EXECUTION PROMPT (Antigravity / Gemini)

Everything left on the Dawa24 rebuild: the rest of Phase 4, then Phases 5, 6 and 7.
This document is your complete specification. It supersedes all earlier phase prompts.

**Repository:** `F:\Dawa 24\dawa24-store`
**Start from:** `main` at `2724875`. Verify with `git log --oneline -1` before doing anything.

Branches, in order, each from the previous:
`phase-4-modals` → `phase-5-conversion` → `phase-6-structure` → `phase-7-verification`

---

## 0. WARNING — ANOTHER SESSION IS ACTIVE IN THIS REPOSITORY

The reflog shows branch checkouts, merges and commits made by someone other than the
previous agent, arriving *between* its tool calls. It cut a branch from the wrong commit
once and lost a verified file edit once because of this.

**Before every commit:** run `git branch --show-current` and confirm you are where you think
you are. **After every commit:** run `git log --oneline -1` and confirm your commit is the
tip. If either check surprises you, **stop and report** — do not "fix" it by force-pushing,
resetting, or re-committing over someone else's work.

Note that a branch named `phase-4-chrome` exists at commit `2c99607 "gs"`. **That is not
yours and not part of this plan. Do not touch it.**

---

## 1. WHAT IS ALREADY DONE

Phases 0 through 3, plus Phase 4 Tasks 0–2, are complete and merged into `main`. **Do not
redo any of it.** Summary so you do not re-litigate settled decisions:

| Area | State |
|---|---|
| Authorization | Settled. Audience gates, tenant/admin permission split, API gates, guard tests. **Do not change.** |
| Performance | `transition: all` = 0; `backdrop-filter` = 2 (modal only); `app.js` 60 KB → 16 KB; CSS split into shell + surface bundles |
| Design tokens | `tokens.css` rebuilt in OKLCH, tinted neutrals, density tokens, both themes |
| Typography | Readex Pro + Spline Sans Mono |
| Cascade | `@layer reset, tokens, base, layout, components, utilities` — every sheet layered except `app.css`, which is deliberately unlayered for hide/show primitives |
| `!important` | **3**, each defended. Keep it there. |
| Breakpoints | 640 / 768 / 1024 only |
| Top bar | `components.DashboardTopBar` — one dashboard header, `.site-header` for public. `glass.css` deleted. |
| Account menu | `components.UserMenu` — real actor, permission-gated org link |

### Verified baseline — measure these yourself before you start and confirm they match

```
inline styles (pages+layouts):  3616
!important in CSS:              3
oversized Go files (>400):      101
unlayered stylesheets:          0   (app.css excepted by design)
modal-overlay occurrences:      38  across 22 templates
window.openModal / openModal(:  26
components.Modal uses:          16
Arabic literals in Go:          1270
templates over 1000 lines:      15
page templates total:           136
shell CSS bytes:                ~150,900
```

If a number differs, **report it before proceeding** — it means the tree moved.

---

## 2. RULES — ALL PHASES

1. **THE STOP RULE.** Where this prompt quotes code or a number, verify it first. If it does
   not match, stop that task, record it under Discrepancies, move on. Do not improvise.
2. **Paste raw command output. Never transcribe a number by hand.** Five consecutive phases
   were reported with figures better than reality, always flattering. A report containing a
   hand-typed metric is incomplete and will be sent back.
3. **Every stated gate is marked MET or MISSED with its number.** A missed gate reported
   honestly is fine. A missed gate not mentioned is not.
4. **Never lower a ceiling below the measured value.** Measure, then set the ceiling to that
   number. Phase 3 set 3606 against a real 3619 and shipped a failing build.
5. **A skipped task is a discrepancy and must be reported as one.** Phase 3 skipped its
   Task 0a and reported "no discrepancies."
6. **No new `go.mod` dependencies. No database. No SQL. No migrations.**
7. **No authorization changes.** Report bugs; do not fix them here.
8. **Do not delete or rewrite existing comments.** They document real past bugs. When a cause
   is removed, rewrite to past tense — never drop the reasoning.
9. **No new `.md` files.** Report in chat.
10. **Read `.impeccable.md` before any UI work.** The **counter test** governs every decision:
    one hand, mid-range Android, bright light, interrupted.

### Gate commands — `make` is not installed; run these directly and paste the output

```bash
gofmt -l ./cmd ./internal
go vet ./...
go test ./... -count=1
grep -oh 'style="' internal/ui/pages/*.templ internal/ui/layouts/*.templ | wc -l
find ./cmd ./internal -name '*.go' -not -name '*_templ.go' -exec wc -l {} + | grep -v " total$" | awk '$1>400' | wc -l
grep -oh '!important' internal/ui/static/css/*.css | wc -l
grep -rho 'modal-overlay' --include=*.templ internal/ui | wc -l
```

`templ` is at `/c/Users/mydwa/go/bin/templ`. Regenerate only the files you edit:
`/c/Users/mydwa/go/bin/templ generate -f <path>`. Commit the generated `*_templ.go` —
they are deliberately tracked (the Dockerfile does not run `templ generate`).

---

# PHASE 4 (REMAINDER) — MODALS AND SIDEBAR

### TASK 3 — Collapse two modal systems into one

**38 `.modal-overlay` occurrences across 22 templates**, driven by `window.openModal`
(26 call sites), coexist with 16 uses of the native `<dialog>`-based `components.Modal`.

The native one is correct: real top layer, focus trap, Escape and backdrop for free. The
legacy one has none of that, so roughly half the platform's dialogs are keyboard-inaccessible
and behave differently under the same trigger. This is the highest-value consistency win left.

1. Migrate all 38 to `components.Modal` (see `internal/ui/components/modal.templ` —
   `ModalProps` already carries `ID`, `Title`, `State`, `Size`).
2. Delete `window.openModal` / `window.closeModal` and the `.modal-overlay` CSS.
3. Preserve the comment in `app.js` explaining the old backdrop-filter cost — rewritten to
   past tense, since Phase 2 removed the cause.
4. The modal rules in `components.css` (~line 222) carry `!important` on nearly every
   declaration. With `@layer` in place they should not need it. Remove them **without
   raising the `!important` count above 3.**

**Migrate in batches of about 8 templates, verifying each batch before continuing.** A single
38-file commit is not reviewable and will be rejected.

---

### TASK 4 — The modal contract

Specify once in `modal.templ`, then hold everywhere:

- Sizes from tokens (`sm`/`md`/`lg`/`xl`/`full`); never ad-hoc widths.
- Body scroll lock on open, exact restore on close. The open-count logic already in `app.js`
  handles two modals correctly — keep it.
- **Full-bleed sheet below 768px.** A centred dialog with margins is unusable one-handed.
- Focus to the first interactive element on open; back to the trigger on close.
- Escape and backdrop close. A form mid-edit warns before discarding.
- **Destructive actions require typed confirmation**, not just a red button. This platform
  moves medicine and money.
- Loading and error states via the existing `State` field, not per-page spinners.

---

### TASK 5 — The sidebar

- **Off-canvas drawer below 1024px.** The toggle already exists in the top bar:
  `components.DashboardTopBar` renders a button carrying `data-drawer-toggle`,
  `aria-controls="app-sidebar"` and `aria-expanded`. **Wire it up** — give the sidebar
  `id="app-sidebar"`, add a focus trap, Escape to close, backdrop dismiss, and return focus
  to the trigger. Keep `aria-expanded` in sync.
- **Collapsed icon rail above 1024px** with accessible tooltips, state persisted.
- Move the scroll-preservation and active-link resolution logic out of the inline `<script>`
  in `admin.templ` into the shell JS module, **behaviour identical**. The active-link
  resolution is load-bearing: it highlights the right link when the route does not exactly
  match an `href`.

The sidebar renders from the RBAC registry. **Do not change what it renders or how
visibility is decided** — that is settled and correct. Presentation only.

---

### TASK 6 — Navigation IA — REPORT ONLY, CHANGE NOTHING

For each of the three dashboards, report: sections and items in order with the gating
permission; any label that does not describe its page; any item duplicating another route;
any section with one item or more than about seven.

Navigation changes alter muscle memory and are the product owner's call, not yours.

---

### TASK 7 — Ratchets

Add `check-modal-legacy` (0 occurrences of `modal-overlay` or `window.openModal`) and wire it
into the `check` target. Lower `check-inline-styles` to the measured value.

**PHASE 4 GATE:** all ratchets hold; tests pass; three dashboards work at 375px and 1440px, in
both themes, RTL and LTR, keyboard-only; zero legacy modals. **Report and stop.**

---

# PHASE 5 — PAGE CONVERSION AND ENGLISH

**Four waves. Report after each and stop.** 136 page templates; Phase 3 converted five.

### English is a real requirement

**1,270 Arabic string literals are hardcoded in Go** (the count grew from 1,246 — it is not
being held). The language toggle appears in every header and promises what the platform
cannot deliver.

Every literal moves to `internal/shared/i18n`, called via `i18n.T(lang, key)`. This is
per-page work inside each wave, not a separate sweep — converting a page means converting its
strings. **Read the existing key convention in `internal/shared/i18n` first and report what
you found.**

There is one temporary exception: `tr(lang, ar, en)` in
`internal/ui/components/dashboard_topbar.templ`, added deliberately so two shell strings did
not invent a third convention. **Fold it into the i18n catalogue during Wave 1 and delete it.**

**Do not machine-translate.** Where no verified English exists, add the key with the Arabic
present, mark the English as needing translation, and list those keys in your report. A wrong
English string in a medicine platform is worse than an obviously missing one.

### Per-page checklist — a page is not done until every line is true

1. Inline `style=` replaced with Phase 3 classes.
2. Ad-hoc markup replaced with components. No new one-off card/table/modal markup.
3. Loading, empty and error states present. Empty states **teach the interface**.
4. Verified at **375px**. Tables become card lists below 768px, not sideways scrollers.
5. Keyboard traversable, visible focus everywhere.
6. Both themes checked.
7. RTL and LTR both checked.
8. Arabic literals moved to `i18n`.
9. Touch targets ≥ 44px in `comfortable` density.
10. No `!important` added — the count stays at **3**. No new breakpoint.

### Waves

- **Wave 1 — daily drivers.** Three dashboards, customer catalogue, cart, checkout, order
  detail, settings. Verify Phase 3's five still hold after Phase 4's chrome changes.
- **Wave 2 — admin heavy screens.** Approvals, organisations, finance, catalog inventory,
  users, roles, cities, developers. `admin_catalog_inventory.templ` is **2,736 lines** and
  `admin_organizations.templ` is 1,460 — **decompose into per-tab and per-section components
  as part of conversion.** Converting in place just moves the problem.
- **Wave 3 — vendor operations.** Offers, inventory, ingest, warehouses, team, transfers,
  coverage, finance.
- **Wave 4 — public and long tail.** Marketing, auth, onboarding, jobs, compare, remainder.

### Shell CSS — finish what Phase 2 and 4 could not

Shell CSS is **~150,900 bytes against a 90 KB target — MISSED three times.** It was correctly
deferred to here: the remaining weight is `components.css`, and splitting it by surface needs
this conversion to know which surface uses what.

As each wave converts pages, move the component rules only that surface uses out of
`components.css` into its surface sheet. **Report shell bytes after every wave.** Target
90 KB by the end of Wave 4. If unreachable, report the real number and why.

### Ratchets — lower after every wave

Measure, then set `check-inline-styles` and `check-hardcoded-arabic` to the measured values,
committed with the wave. **This is the wave's definition of done.** Targets by end of
Phase 5: inline styles **≤ 300**, Arabic literals in Go **0**.

---

# PHASE 6 — STRUCTURE AND DEAD CODE

### TASK 1 — Reorganise `internal/ui` by audience

~52,000 lines of handlers sit flat beside `pages/`, `components/`, `layouts/`. Group them the
way routes are already gated, so a reader answering "what can a vendor do" opens one
directory: `internal/ui/admin/`, `vendor/`, `customer/`, `shared/`, `public/`, plus `view/`
for the domain→template mapping now spread across `*_models.go`. `pages/`, `components/`,
`layouts/`, `static/` unchanged.

**No behaviour change in the same commit as a move.** Move, verify tests, then change
anything else. One audience per commit.

### TASK 2 — Split oversized Go files

**101 files exceed the project's own 400-line limit.** Split by concern, not line count: read
handlers, write handlers, and view-model mapping are the natural seams. Lower
`check-file-size-count` to the measured value as you go. Target **0**; report what resisted.

### TASK 3 — Decompose oversized templates

**15 templates exceed 1,000 lines.** Wave 2 handles the two largest; finish the rest here.

### TASK 4 — Dead code

`golangci-lint`'s `unused` does not catch exported symbols nothing calls. Add
`golang.org/x/tools/cmd/deadcode` to `make check` with its own ratchet, run it, remove what it
finds.

**Read each finding before deleting** — a symbol may be referenced from a `.templ` file that
`deadcode` does not parse. Anything ambiguous goes in the report, not in a delete commit.

### TASK 5 — Documentation

Update `AGENTS.md`. Add ADRs in `docs/adr/` for: the `@layer` order; the `DashboardTopBar`
contract; the API gate pattern and why it returns 403 where the UI returns 404; and **the
tenant-vs-admin permission namespace split** — that one exists because confusing
`commerce.*` (admin-scope) with `vendor.*` (tenant-scope) broke supplier fulfilment in
Phase 0, and it must be written down where the next engineer will find it.

**PHASE 6 GATE:** all ratchets hold, `deadcode` clean, tests pass. **Report and stop.**

---

# PHASE 7 — VERIFICATION

### TASK 1 — Permission matrix
Every role × every route, asserted, as a table a human can read. This becomes the canonical
statement of who can do what.

### TASK 2 — Account lifecycle
`pending` → `under_review` → `approved` → `suspended` → `rejected`, asserting exactly what
each state reaches on both UI and API. This was the product owner's original concern and gets
its own dedicated test.

### TASK 3 — Visual regression
Every component from the Phase 3 contract, every variant, both densities, both themes, both
directions.

### TASK 4 — Real devices
Mid-range Android and iOS Safari, RTL and LTR, at 375px. **Execute the counter test for real:**
complete a full order — find a product, compare suppliers, add to cart, check out — one-handed.
Report where it is *awkward*, not only where it breaks.

### TASK 5 — Accessibility
Keyboard-only traversal of all three dashboards. Screen reader on sidebar, top bar and modals.
Contrast verified in both themes: AA on body, AAA on data.

### TASK 6 — Performance
Lighthouse was **NOT RUN** in Phase 2 and 3 for lack of a deployment, so the budget added in
Phase 2 has never been enforced. If a staging URL exists now, run it against the public home
page, customer catalogue, `/admin/organizations` and the cart, and report real numbers against
the budget (perf ≥ 80 mobile, LCP ≤ 2.5s, TBT ≤ 300ms, CLS ≤ 0.1). If not, say so plainly.

### TASK 7 — Final ratchet report
Every gate, current value against ceiling, as raw output, plus the residual list: every target
not reached, its number, and why.

**PHASE 7 GATE:** all green, or every gap named with a number.

---

## REPORTING — nine times, separately

Phase 4 · Phase 5 Waves 1–4 · Phase 6 · Phase 7. Never run two gates without review between.

Each report contains:

1. **Per task / per page** — `DONE` / `PARTIAL` / `STOPPED`, files touched, what and why.
2. **Raw pasted command output** for every gate. Not transcribed.
3. **Every stated gate marked MET or MISSED**, with its number.
4. **Discrepancies** — including any task you did not do.
5. **Deferred** — noticed and deliberately untouched.
6. **Diff summary** — `git diff --stat` against the parent branch.
7. **Branch check** — `git branch --show-current` and `git log --oneline -1` after committing,
   per §0.

Phase-specific: Phase 4 Task 6 IA findings · Phase 5 untranslated keys and shell bytes per
wave · Phase 6 ambiguous `deadcode` findings · Phase 7 the residual list.

**Stop at every gate.**
