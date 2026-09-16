# CLOSEOUT PROMPT — ALL REMAINING WORK (Antigravity / Gemini)

Everything still open on the Dawa24 rebuild. This supersedes every earlier phase, wave and
addendum prompt.

**Start from:** `phase-6-completion` at `1e249b1`. Verify with `git log --oneline -1` before
touching anything.

Branches, in order, each cut from the previous:
`phase-6-reorg` → `phase-7-regression` → `phase-7-audit`

**Three report gates.** Stop and report at each. Do not run two without review between.

---

## 0. STATE — VERIFY THESE BEFORE YOU START

If any number differs, **stop and report it**; the tree has moved under you.

```
inline styles (pages+layouts)      81
raw <dialog> in pages               0
!important in CSS                   3
undefined CSS classes             235
user-facing Arabic in Go            0     (internal/ui and cmd both zero)
oversized Go files (>400 lines)     0
templates over 1000 lines           0
deadcode findings                 211
shell CSS bytes               167,151
flat .go files in internal/ui     245     (64 of them _test.go)
ADRs                               18
go build / go vet / go test ./...   pass (70 packages)
```

### What is already done — do not redo any of it

Phases 0–5 complete. Phase 6 Tasks 2–5 complete. Phase 7 Task 2 complete.

- **Authorization** settled: audience gates, tenant/admin permission scopes, API gates,
  guard tests. **Do not change it.** Report bugs, do not fix them.
- **Account lifecycle asserted**: `internal/ui/account_lifecycle_matrix_test.go` covers seven
  organization states across both audiences, plus money-movement writes and audience
  separation. It was verified to go red when `RequireApproved` is removed. **Phase 7 Task 2
  is done — do not rewrite it.**
- **Test harness fixed**: `newTestRouter` in `internal/ui/handlers_test.go` now mirrors
  production's three shared tiers. It previously mounted all four shared registrars in one
  ungated group, so it could not see the bug it existed to catch.
- **English requirement complete**: `internal/ui` and `cmd` measure zero. The 226 Arabic
  literals remaining in `internal/modules` are matching dictionaries, not text — see ADR 0018
  before touching them.
- **CSS recovery**: 233 classes that the inline-style conversion left with no rule were
  defined. See ADR 0017.

---

## 1. NON-NEGOTIABLE RULES

1. **Measure, then set the ceiling to the measured number.** Never below it.
2. **Paste raw command output. Never transcribe a number by hand.**
3. **Mark every stated gate MET or MISSED with its number.** A skipped task is a discrepancy
   and must be reported as one.
4. **THE STOP RULE.** If a quote or count does not match, stop that item, report it, move on.
5. **No new `go.mod` dependencies. No database. No SQL. No migrations.**
6. **Do not delete or rewrite existing comments.** Past tense when a cause is removed.
7. **`!important` stays at 3. Breakpoints stay 640 / 768 / 1024.**
8. **Name commits for what they do.** Several commits in this repo are named `fasf`, `gdfgdf`,
   `KHL;`. The diff is recoverable; the reasoning is not.
9. Read `.impeccable.md` before UI work. The **counter test** governs: one hand, mid-range
   Android, bright light, interrupted.

### Gate commands — `make` is not installed; run these and paste output

```bash
gofmt -l ./cmd ./internal
go vet ./...
go test ./... -count=1
grep -oh 'style="' internal/ui/pages/*.templ internal/ui/layouts/*.templ | wc -l
grep -oh '!important' internal/ui/static/css/*.css | wc -l
find ./cmd ./internal -name '*.go' -not -name '*_templ.go' -exec wc -l {} + | grep -v " total$" | awk '$1>400' | wc -l
cat internal/ui/static/css/{tokens,base,layout,components,foundations,utilities,app}.css | wc -c
```

`templ` is at `/c/Users/mydwa/go/bin/templ`. Regenerate only files you edit and commit the
generated `*_templ.go` — they are deliberately tracked.

---

# PHASE 6 TASK 1 — REORGANISE `internal/ui` BY AUDIENCE

The only Phase 6 task left. **245 flat `.go` files** sit beside `pages/`, `components/`,
`layouts/`, `static/`. Group them the way the routes are already gated, so a reader answering
"what can a vendor do" opens one directory.

The filenames already carry the grouping: 62 start `admin_`, 40 `vendor_`, 25 `customer_`.

```
internal/ui/admin/      admin_* handlers, routes, view models
internal/ui/vendor/     vendor_*
internal/ui/customer/   customer_*
internal/ui/shared/     settings, documents, wallet, invoices, notifications, messages
internal/ui/public/     public pages, auth, onboarding, storefront
internal/ui/view/       the domain -> template mapping now in *_models.go
```

`pages/`, `components/`, `layouts/`, `static/` do not move.

### How to do it without breaking anything

1. **One audience per commit.** Six commits, not one. A 245-file move in a single commit is
   not reviewable and cannot be bisected.
2. **No behaviour change in the same commit as a move.** Move files, fix imports, run tests,
   commit. Anything else is a separate commit.
3. **Test files move with the code they test.**
4. `UIHandler` methods currently live across all these files on one receiver. Keep the single
   receiver — splitting the type is a different change and is not in scope. If Go's package
   rules make that impossible for a given file, **stop and report** rather than inventing a
   new type.
5. Run `go test ./... -count=1` after **every** commit, not at the end.

If the single-receiver constraint makes a clean six-way split impossible, **say so and stop**.
Report the shape that is actually achievable. A partial reorganisation that is honest beats a
forced one that fragments the handler type.

### Also in this phase

**Decide the orphaned package.** `internal/modules/smartorder/http` is imported by nothing.
`RegisterRoutes`, `Handler`, `Reviewer`, the events endpoint and the review endpoints are all
unreachable. The smartorder *service* is wired — `cmd/server/smartorder.go` composes it and the
UI drives it — but its JSON API was never mounted.

**Do not delete it and do not wire it.** Both are product calls. Report what the package
exposes, what would have to happen to mount it, and what would be lost by removing it.

**GATE: report and stop.**

---

# PHASE 7 — REGRESSION SAFETY NET

This phase exists so the CSS work in the next one is safe. Do it in this order; the ordering
is the point.

### TASK 1 — Visual regression

Every component from the Phase 3 contract: button, input, select, checkbox/radio, card, table,
modal, badge, empty state, skeleton, error state, pagination, tabs, dropdown, toast.

Each captured in **both densities** (`comfortable` / `is-compact`), **both themes**, and
**both directions** (RTL and LTR). That is the matrix — do not shortcut it to light-LTR only,
because the platform's default user is dark-capable, Arabic, and on a phone.

Build it as a static component gallery page served by the app under a staff-only route, then
capture it. Any capture tool is acceptable; **if none can run here, say so plainly and stop**
— do not fake a baseline. A visual regression suite whose baseline was never rendered is worse
than none, because the next phase will trust it.

### TASK 2 — Permission matrix

Every role × every route, asserted, as a table a human can read. `test/rbac_guard_test.go`
already checks that gate keys exist and that their scope matches the route's audience; this is
the complement — what each role can actually *reach*.

Follow the shape of `internal/ui/account_lifecycle_matrix_test.go`: a declared table, the
assertion reading off it, and the expected status code stated per row rather than inferred.

**Verify it can fail.** Remove one gate, confirm red, restore. Report the failure output. A
test never seen failing proves nothing — that standard has caught two real problems in this
project already.

### TASK 3 — Accessibility

Keyboard-only traversal of all three dashboards: every interactive element reachable, visible
focus, no trap outside a modal, Escape closes what it should. Screen reader on the sidebar,
top bar and modals. Contrast in both themes — AA on body text, AAA on numbers and data.

Report failures as a list with the file and the fix, and **fix the ones that are one-line
attribute changes**. Anything structural goes in the report for the next pass.

### TASK 4 — Real devices — the counter test

Mid-range Android and iOS Safari, RTL and LTR, at 375px.

Complete a full order **one-handed**: find a product, compare suppliers, add to cart, check
out. Then the same on the vendor side: receive the order, mark it shipped.

**Report where it is awkward, not only where it breaks.** This is the acceptance test for the
whole rebuild — the platform exists for a pharmacist at a counter being interrupted. If a step
needs two hands or a second attempt, that is a finding.

If no device is available, say so. Do not simulate it and call it done.

**GATE: report and stop.**

---

# PHASE 7 (CONTINUED) — CSS AUDIT AND CLOSEOUT

**Do not start this until the visual regression baseline from the previous phase exists.**
That ordering was got wrong once already: the shell CSS dedupe was attempted four times
without a safety net and moved 2 KB in total, because doing it blind across 136 pages is not
safe and everyone knew it.

### TASK 1 — Reset the shell CSS target, then meet it

Shell CSS is **167,151 bytes**. The old 90 KB target was set against a stylesheet that was
missing 468 classes' worth of rules — part of the apparent progress toward it was styling that
had gone absent (ADR 0017).

1. **Set a target you can defend** against the current, complete stylesheet. State the number
   and the reasoning.
2. Then meet it. The weight is `components.css` (~86 KB), `foundations.css` (~21 KB) and
   `utilities.css` (~21 KB), which overlap heavily. With `@layer` declaring the order, the
   duplicates can be deleted rather than out-shouted.
3. Move rules used by only one surface into that surface's sheet — `admin.css`, `vendor.css`,
   `customer.css`, `public.css` already exist for this.
4. **Run the visual regression suite after every deletion batch.** That is what it is for.

### TASK 2 — Lower `check-undefined-classes`

235 remain. The largest entries are Alpine state names inside `:class` bindings — `activeTab`
(29), `activeCategory`, `filterTab`, `policyFilter`, `siteSubTab` — which are never CSS
selectors.

Teach the gate to skip tokens that appear only inside `:class` / `x-bind:class` attributes,
then lower the ceiling to the real number. Report both figures.

### TASK 3 — `deadcode`

`check-deadcode` sits at 211 with nothing deleted, deliberately: `deadcode` does not parse
`.templ` files, so a symbol it calls unreachable may be reached from a template.

Go through the list. For each finding, establish whether it is genuinely unreachable **before**
deleting. Anything ambiguous goes in the report, not in a delete commit. Lower the ceiling to
whatever survives.

### TASK 4 — Final residual report

Every gate, current value against ceiling, as raw output. Then the residual list: **every
target not reached, its number, and why.** That list is the handover document for whoever picks
this up next, and it is more valuable than a clean summary that hides the gaps.

**GATE: all green, or every gap named with a number.**

---

## REPORTING — three times

Each report contains:

1. Per task: `DONE` / `PARTIAL` / `STOPPED`, files touched, what changed and why.
2. Raw pasted output for every gate command.
3. Every stated gate marked **MET** or **MISSED**, with its number.
4. Discrepancies, including anything you did not do.
5. `git diff --stat` against the parent branch.
6. `git branch --show-current` and `git log --oneline -1` after committing — another session
   has moved HEAD in this repository between tool calls before.

Phase-specific: the reorg report names any file that could not move and why, plus the
smartorder/http findings · the regression report names anything that could not be run on this
machine · the closeout report carries the residual list.

**Stop at each of the three gates.**
