# PHASES 4 → 7 — REMAINING WORK (Antigravity / Gemini)

**This document supersedes `docs/PHASE_4_AND_5_PROMPT.md`.** Work only from this one.

Branches, in order:
`phase-4-chrome` → `phase-5-conversion` → `phase-6-structure` → `phase-7-verification`,
each from the previous. Phase 4 branches from `phase-3-design-system`.

**Report at every gate: Phase 4, then Phase 5 once per wave (four waves), then Phase 6,
then Phase 7. Eight reports. Never run two gates without review between them.**

---

## 0. PHASE 3 REVIEW — STRONG WORK, ONE BROKEN BUILD

Verified on `da48c82`:

| Claim | Verified | Note |
|---|---|---|
| `!important` 668 → 3 | **YES** | Target was ≤20. You hit 3. Exceptional |
| `tokens.css` rebuilt in OKLCH | **YES** | 126 `oklch()` declarations |
| Readex Pro + Spline Sans Mono | **YES** | Correctly wired in `base.templ` |
| Breakpoints collapsed | **YES** | Only 640 / 768 / 1024 remain — better than the 4 asked for |
| `@layer` cascade declared | **YES** | |
| New ratchets added | **YES** | |

The `!important` result is the most valuable thing achieved in any phase so far. 668 down to
3, with each survivor defended (`[x-cloak]`, `dialog:not([open])`, `.drawer-overlay.hidden`)
— those three are legitimately pre-script-initialisation hiding rules that must not lose.
That is the structural fix that makes every later phase cheaper.

### Correction 1 — you shipped a failing build

```
inline styles measured: 3619
ceiling you set:        3606
```

`make check-inline-styles` **fails**. You lowered the ceiling below the actual value.

This is the **fifth consecutive phase** where a reported number was better than reality, and
the first where it breaks CI:

| Phase | Reported | Actual |
|---|---|---|
| 1.5 | 85 oversized files | 101 |
| 2 | 85 oversized files | 101 |
| 2 | 3,988 inline styles | 4,001 |
| 2 | CSS within budget | 155 KB |
| 3 | 3,606 inline styles | **3,619** |

The instruction to paste raw output rather than transcribe numbers was given before Phase 3
and was not followed. **It is now a hard requirement: a report containing a hand-typed metric
is incomplete and will be sent back.** Paste the terminal output. Every time.

### Correction 2 — Task 0a was skipped and reported as absent discrepancies

§1 of the Phase 4/5 prompt added a Phase 3 Task 0a: get the shell CSS under 90 KB, and check
whether `glass.css` is still needed now that Phase 2 removed the aesthetic it served.

Measured now:

```
tokens + base + layout + glass + components + foundations + utilities + app = 148,439 bytes
glass.css = 8,068 bytes — byte-identical to Phase 2, untouched
```

The shell is ~145 KB against a 90 KB target, and your report said *"No discrepancies with the
prompt instructions."* A skipped task is the largest possible discrepancy.

I want to be precise about why this matters more than the number: **the Phase 2 gate already
missed this, I flagged it, I re-issued it as Task 0a, and it was skipped again silently.** A
gate that can be passed over twice without appearing in a report is not a gate. Both
corrections are Phase 4 Task 0.

---

## 1. RULES — ALL PHASES

1. **THE STOP RULE.** Verify every quote and number before acting. If it does not match,
   stop that task, record it under Discrepancies, move on.
2. **Paste raw command output.** No transcribed numbers. Non-negotiable now.
3. **Every stated gate is marked MET or MISSED with its number.** A missed gate reported
   honestly is acceptable. A missed gate not mentioned is not.
4. **Never lower a ceiling below the measured value.** Measure first, then set the ceiling to
   that number.
5. **No new `go.mod` dependencies. No database. No SQL. No migrations.**
6. **No authorization changes.** Phases 0–1.5 are settled. Report bugs, do not fix them here.
7. **Do not delete or rewrite existing comments.** Rewrite to past tense when a cause is
   removed; never drop the reasoning.
8. **No new `.md` files.** Report in chat.
9. Read `.impeccable.md` before any UI work. The **counter test** governs: one hand,
   mid-range Android, bright light, interrupted.

### Ratchet commands — `make` is unavailable to you, run these directly and paste output

```bash
gofmt -l ./cmd ./internal
go vet ./...
go test ./... -count=1
grep -oh 'style="' internal/ui/pages/*.templ internal/ui/layouts/*.templ | wc -l
find ./cmd ./internal -name '*.go' -not -name '*_templ.go' -exec wc -l {} + | grep -v " total$" | awk '$1>400' | wc -l
grep -oh '!important' internal/ui/static/css/*.css | wc -l
cat internal/ui/static/css/{tokens,base,layout,components,foundations,utilities,app}.css | wc -c
```

Note the last command omits `glass.css` — if it still exists after Task 0, add it back.

---

# PHASE 4 — CHROME, NAVIGATION, MODALS

### TASK 0 — Phase 3 corrections

**0a.** Measure inline styles, then set the ceiling to that exact number. Paste both.

**0b.** Get the shell CSS under 90 KB. Phase 3's `@layer` work is what makes this reachable —
with the order declared, duplicated rules can be deleted rather than out-shouted. Concretely:

- `glass.css` (8,068 bytes, untouched since Phase 2) styles a glass aesthetic Phase 2 largely
  removed. Determine whether anything still references it. If not, delete it. If something
  does, fold the survivors into `components` and delete the file.
- `foundations.css` (20.7 KB) and `utilities.css` (12.5 KB) overlap `components.css`
  (67.8 KB). Under a declared cascade the overlap is now safely removable.
- `customer.css` is 41.7 KB — larger than most shells. Split per surface (catalogue, cart,
  orders) and load per route, as Phase 2 did for the shells.

Paste `wc -c` before and after. If 90 KB is unreachable without breaking something, **report
the number you reached and why** — an honest 110 KB beats a broken 90 KB. What is not
acceptable is the task going unmentioned again.

---

### TASK 1 — One dashboard top bar

**Verify before editing.** One CSS rule serves two unrelated jobs:

```css
.public-navbar, .top-navbar { … }
```

The marketing navbar wants height and a brand lockup; the dashboard bar sits above a data
table and wants to be compact and quiet. They currently share height, padding, sticky
positioning and shadow. Then each shell builds its contents differently:

| Shell | Left | Right | Layout via |
|---|---|---|---|
| Public | Brand + 5 nav links | Auth actions, mobile toggle | `.top-navbar-brand` |
| Admin | Page title | Lang, theme, user chip, logout | `.top-navbar-start/end` |
| Vendor | Page title | Lang, theme, static badge, logout | `.top-navbar-start/end` |
| Pharmacy | Title + branch selector | Lang, theme, cart, badge, logout | **inline `style=`** |

**Required:** one `DashboardTopBar` with a declared contract — title/breadcrumb slot, a
contextual slot (branch selector, org switcher), and a fixed action cluster (search,
notifications, user menu). `.site-header` becomes a separate class for the public navbar
sharing **no** rules with it; delete the combined selector. Four implementations become two.
No inline styles in any shell. Density-aware: `compact` for admin and vendor, `comfortable`
for pharmacy.

---

### TASK 2 — A real user menu

Phase 0 replaced the hardcoded "Super Admin" with the real actor — the minimum. Build the
control properly: name, role, organisation, organisation switcher where the actor belongs to
several, settings, logout.

Keyboard accessible, Escape closes, focus returns to the trigger, and not a `<div>`
pretending to be a menu. A sheet on mobile, not a dropdown overflowing the viewport. Wire the
switcher to the existing `/org/switch/{id}` in Tier B — do not invent an endpoint.

---

### TASK 3 — One modal system

35 uses of the native `<dialog>`-based `components.Modal`; **38 hand-rolled
`.modal-overlay` divs** driven by `window.openModal`. The native one is correct — real top
layer, focus trap, Escape and backdrop for free. Roughly half the platform's dialogs are
currently keyboard-inaccessible and behave differently under the same trigger.

1. Migrate all 38 to `components.Modal`.
2. Delete `window.openModal` / `window.closeModal` and the `.modal-overlay` CSS.
3. Preserve the `app.js` comment explaining the backdrop-filter cost, rewritten to past tense.
4. **Migrate in batches of ~8 and verify each batch.** A 38-file commit is not reviewable.

---

### TASK 4 — The modal contract

Sizes from tokens, never ad-hoc widths. Body scroll lock with exact restore; the existing
open-count logic in `app.js` handles nesting — keep it. **Full-bleed sheet below 768 px** — a
centred dialog with margins is unusable one-handed. Focus to the first interactive element on
open, back to the trigger on close. Escape and backdrop close; a form mid-edit warns before
discarding. **Destructive actions require typed confirmation**, not just a red button — this
platform moves medicine and money. Loading and error states via `components.Modal`'s existing
`State` field, not per-page spinners.

---

### TASK 5 — The sidebar

- **Off-canvas drawer below 1024 px**, opened from the top bar, with focus trap, Escape,
  backdrop dismiss, focus returned to trigger.
- **Collapsed icon rail** above 1024 px with accessible tooltips, state persisted.
- Move the scroll-preservation and active-link resolution logic out of `admin.templ`'s inline
  `<script>` into the shell JS module, **behaviour identical**. The active-link resolution is
  load-bearing: it highlights the right link when the route does not exactly match an href.

The sidebar renders from the RBAC registry. **Do not change what it renders or how visibility
is decided** — that is settled. Presentation only.

---

### TASK 6 — Navigation IA review — REPORT ONLY

For each of the three dashboards, report: sections and items in order with the gating
permission; any label that does not describe its page; any item duplicating another route;
any section with one item or more than about seven.

**Change nothing.** Navigation changes alter muscle memory and are the product owner's call.

---

### TASK 7 — Ratchets

`check-modal-legacy` = 0 (`modal-overlay`, `window.openModal`); `check-topbar-impls` = 2;
lower `check-inline-styles` by what the shells removed.

**GATE:** ratchets hold, tests pass, three dashboards work at 375 px and 1440 px in both
themes, RTL and LTR, keyboard-only, zero legacy modals. **Report and stop.**

---

# PHASE 5 — PAGE CONVERSION AND ENGLISH

Four waves. **Report after each and stop.**

### The English requirement

English is confirmed as a real requirement. **1,246 Arabic string literals are hardcoded in
Go**, tracked by `make check-hardcoded-arabic`. The language toggle appears in every navbar
and promises what the platform cannot deliver.

Every literal moves to `internal/shared/i18n`, called via `i18n.T(lang, key)`. This is
per-page work inside each wave, not a separate sweep — converting a page means converting its
strings. Follow the key convention already in `internal/shared/i18n`; read it first and report
what you found.

**Do not machine-translate.** Where no verified English exists, add the key with the Arabic
present, mark the English as needing translation, and list those keys. A wrong English string
in a medicine platform is worse than an obviously missing one.

### Per-page checklist — a page is not done until every line is true

1. Inline `style=` replaced with Phase 3 classes.
2. Ad-hoc markup replaced with Phase 4 components; no new one-off card/table/modal markup.
3. Loading, empty and error states present. Empty states **teach the interface**.
4. Verified at **375 px**. Tables become card lists below 768 px, not sideways scrollers.
5. Keyboard traversable, visible focus everywhere.
6. Both themes checked.
7. RTL and LTR both checked.
8. Arabic literals moved to `i18n`.
9. Touch targets ≥ 44 px in `comfortable`.
10. No `!important` added — the count stays at 3. No new breakpoint.

### Waves

- **Wave 1 — daily drivers.** Three dashboards, customer catalogue, cart, checkout, order
  detail, settings. Phase 3 converted five of these; verify they still hold after Phase 4's
  chrome changes, then finish the rest.
- **Wave 2 — admin heavy screens.** Approvals, organisations, finance, catalog inventory,
  users, roles, cities, developers. `admin_catalog_inventory.templ` is **2,736 lines** and
  `admin_organizations.templ` is 1,460 — **decompose into per-tab and per-section components
  as part of conversion.** Converting in place just moves the problem.
- **Wave 3 — vendor operations.** Offers, inventory, ingest, warehouses, team, transfers,
  coverage, finance.
- **Wave 4 — public and long tail.** Marketing, auth, onboarding, jobs, compare, remainder.

### Ratchets — lower after every wave

Measure, then set `check-inline-styles` and `check-hardcoded-arabic` to the measured values,
committed with the wave. **This is the wave's definition of done.** Targets by end of
Phase 5: inline styles **≤ 300**, Arabic literals in Go **0**.

---

# PHASE 6 — STRUCTURE AND DEAD CODE

### TASK 1 — Reorganise `internal/ui` by audience

51,908 lines of handlers sit flat beside `pages/`, `components/`, `layouts/`. Group them the
way the routes are already gated, so a reader answering "what can a vendor do" opens one
directory:

`internal/ui/admin/`, `vendor/`, `customer/`, `shared/`, `public/`, plus `view/` for the
domain→template mapping now spread across `*_models.go`. `pages/`, `components/`, `layouts/`,
`static/` unchanged.

**No behaviour change in the same commit as a move.** Move first, verify tests, then change
anything else. Keep the moves reviewable — one audience per commit.

### TASK 2 — Split oversized Go files

**101 files exceed the project's own 400-line limit** (`check-file-size`). Split by concern,
not by line count: read handlers, write handlers, and view-model mapping are natural seams.
Lower `check-file-size-count` to the measured value as you go.

Target: **0**. If you cannot reach it, report the real number and the files that resisted.

### TASK 3 — Decompose oversized templates

14 templates exceed 1,000 lines. Wave 2 handles the two largest; this task finishes the rest.

### TASK 4 — Dead code

`golangci-lint`'s `unused` does not catch exported symbols nothing calls.

Add `golang.org/x/tools/cmd/deadcode` to `make check` with its own ratchet, run it, and
remove what it finds. **Read each finding before deleting** — a symbol may be referenced from
a `.templ` file that `deadcode` does not parse. Anything ambiguous goes in the report, not in
a delete commit.

Target: **0**.

### TASK 5 — Documentation

Update `AGENTS.md` for the new structure. Add ADRs in `docs/adr/` for: the `@layer` order, the
`DashboardTopBar` contract, the API gate pattern (`RequireAPIPermission` and why it returns
403 where the UI returns 404), and the tenant-vs-admin permission namespace split — the last
one exists specifically because confusing the two broke supplier fulfilment in Phase 0, and
it must be written down.

**GATE:** all ratchets hold, `deadcode` clean, tests pass. **Report and stop.**

---

# PHASE 7 — VERIFICATION

### TASK 1 — Permission matrix

Every role × every route, asserted, as a table a human can read. It must be obvious from the
source what each role can reach. This supersedes ad-hoc gate tests as the canonical statement
of who can do what.

### TASK 2 — Account lifecycle

`pending` → `under_review` → `approved` → `suspended` → `rejected`, asserting exactly what
each state reaches on both the UI and API surfaces. This is the product owner's original
concern and gets its own dedicated test.

### TASK 3 — Visual regression

The component library in both themes and both directions. Every component from the Phase 3
contract, every variant, both densities.

### TASK 4 — Real devices

Mid-range Android and iOS Safari, RTL and LTR, at 375 px. **The counter test, executed for
real:** complete a full order — find a product, compare suppliers, add to cart, check out —
one-handed. Report where it is awkward, not just where it breaks.

### TASK 5 — Accessibility

Keyboard-only traversal of all three dashboards. Screen reader on the sidebar, top bar and
modals. Contrast verified in both themes — AA on body, AAA on data.

### TASK 6 — Load

The ten heaviest endpoints with the Phase 2 Lighthouse budget enforced. Lighthouse was
**NOT RUN** in Phase 2 for lack of a deployment — if one is available now, run it and report
real numbers against the budget. If not, say so plainly again.

### TASK 7 — Final ratchet report

Every gate, current value against ceiling, as raw output. Plus the residual list: every
target not reached, its number, and why.

**GATE:** all green, or every gap named with a number.

---

## REPORTING — eight times, separately

Each report contains:

1. **Per task / per page** — `DONE` / `PARTIAL` / `STOPPED`, files touched, what and why.
2. **Raw pasted command output** for every ratchet. Not transcribed.
3. **Every stated gate marked MET or MISSED**, with its number.
4. **Discrepancies** — including any task you did not do. A skipped task is a discrepancy.
5. **Deferred** — noticed and deliberately untouched.
6. **Diff summary** — `git diff --stat` against the parent branch.

Phase-specific additions: Phase 4 Task 6 IA findings · Phase 5 untranslated keys per wave ·
Phase 6 ambiguous `deadcode` findings · Phase 7 the residual list.

**Stop at every gate.**
