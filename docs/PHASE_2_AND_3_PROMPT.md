# PHASE 2 & 3 — EXECUTION PROMPT (Antigravity / Gemini)

Two phases in one document. **Phase 2 is performance. Phase 3 is the design system
foundation.** They are sequential and gated: do not begin Phase 3 until Phase 2's gate
passes and you have reported Phase 2.

Repository: `F:\Dawa 24\dawa24-store`
Branches: `phase-2-performance` from `phase-1-5-tenant-permissions`, then
`phase-3-design-system` from `phase-2-performance`.

Report **twice** — once when Phase 2 is complete, once when Phase 3 is. Do not merge the
two reports.

---

## 0. PHASE 1.5 REVIEW — ACCEPTED

The regression fix is correct: `vendor.order.update`, staff bypass intact, and the comment
recording why `commerce.order.*` was wrong is exactly the right artefact to leave behind.

**Three things you did that I want to name, because they are the behaviours that make this
work:**

- **Task 3 evidence.** You produced the real red-test output showing
  `TransitionShipmentStatus by owning vendor with permission` failing at 403 *before* the
  fix. That is proof, not assertion.
- **Task 4.** Twenty-one routes, each in exactly one bucket, with real reasoning — including
  admitting `RequireAPITenantPermission` cannot cleanly gate multi-audience routes without
  rejecting staff. That is a genuine architectural finding, not an excuse.
- **Task 2 coverage.** You stated plainly which call sites static scanning cannot classify
  and why (in-handler `actor.Can()` inside multi-audience handlers needs data-flow
  analysis). An honest partial check beats a complete-looking one that guesses.

**Two corrections, both minor, carried into Phase 2 Task 0.**

1. You reported oversized Go files as **85**. The measured value is **101** (ceiling 101 —
   it passes, but only just). Re-check that command; you likely excluded `_test.go` files,
   which the Makefile gate does not.
2. `internal/platform/authctx/testhelper.go` has no `//go:build` tag and is not a `_test.go`
   file, so `ActorForRole` — test scaffolding — **compiles into the production binary** and
   drags `rbac.Default()` into the runtime path. Fix in Task 0.

**One route-count note:** you found 21 non-admin commerce routes where I said 22. You are
right and you showed your enumeration. Good.

---

## 1. RULES FOR BOTH PHASES

1. **THE STOP RULE.** Where this prompt quotes code or cites a number, verify it first. If
   it does not match — stop that task, record it under Discrepancies, move on. Phase 1.5
   exists because a non-executable instruction was absorbed instead of reported.
2. **Report `NOT RUN` honestly.** You did this correctly last time. Keep doing it.
3. **No new dependencies in `go.mod`.** Front-end tooling for Phase 2 (Lighthouse CI) is
   configured in CI only — no bundler, no framework, no npm build step for the app itself.
   The platform is server-rendered templ and stays that way.
4. **No database connection.** No migrations, no SQL, no schema changes.
5. **No behaviour changes.** These phases must not alter what any page *does*, only how
   fast it does it and how it looks. If a performance or styling change would alter
   behaviour, stop and report.
6. **Do not delete or rewrite existing comments.**
7. **Do not touch authorization.** Phases 0–1.5 are settled. If you find an authorization
   bug, report it; do not fix it here.
8. **Do not convert all 136 pages.** That is Phase 5. Phase 3 builds the foundation and
   proves it on a named short list. Converting more is scope creep and will be rejected.
9. **No new `.md` files.** Report in chat.

### Ratchets — run these directly, `make` is not installed for you

```bash
gofmt -l ./cmd ./internal
go vet ./...
go test ./... -count=1
grep -oh 'style="' internal/ui/pages/*.templ internal/ui/layouts/*.templ | wc -l    # ceiling 4001
find ./cmd ./internal -name '*.go' -not -name '*_templ.go' -exec wc -l {} + | grep -v " total$" | awk '$1>400' | wc -l   # ceiling 101
grep -c '!important' internal/ui/static/css/*.css                                   # Phase 3 target
```

---

# PHASE 2 — PERFORMANCE

## Why this phase exists

The platform is uniformly laggy — pages, modals, dashboards, sidebar. Uniform lag points at
the shell, not at individual pages, and the shell is where the causes are. Measured
baseline:

| Fact | Value |
|---|---|
| CSS loaded on every route | 8 files, ~233 KB |
| JS loaded on every route | `app.js`, 60 KB, plus HTMX and Alpine |
| `backdrop-filter` rules | 25, including on the `position: sticky` navbar |
| `transition: all` declarations | 32 |
| `CapsuleAssistant` rendered into every page `<body>` | 1,566 templ lines / 67 KB generated |

The primary user is on a **mid-range Android phone in a pharmacy**. That device, not your
laptop, is the target.

---

### TASK 0 — Carry-forward corrections

**0a.** Move `ActorForRole` out of the production build. Preferred: a new package
`internal/platform/authctx/authctxtest` importable by tests across the repo. A `//go:build`
tag is acceptable if that proves cleaner. Update the call sites in
`shipment_authorization_test.go` and `api_gates_test.go`.

**0b.** Re-run the oversized-file count with the exact command above and report the real
number.

---

### TASK 1 — Measure before you change anything

**Nothing else in Phase 2 starts until this is done.** Every remaining task must be able to
cite a before and after.

Add Lighthouse CI to `.github/workflows/ci.yml` as a separate job, with a **mobile** config
and CPU/network throttling representative of a mid-range Android — not desktop defaults.

Budget, enforced as assertions:

| Metric | Budget |
|---|---|
| Performance score (mobile) | ≥ 80 |
| Largest Contentful Paint | ≤ 2.5 s |
| Total Blocking Time | ≤ 300 ms |
| Cumulative Layout Shift | ≤ 0.1 |
| CSS transferred per route | ≤ 90 KB |
| JS transferred per route | ≤ 120 KB total including HTMX + Alpine |

Run it against at least four routes covering the real span: the public home page, the
customer catalogue, an admin table page (`/admin/organizations`), and the customer cart.

**If the budget cannot be met by the end of Phase 2, do not weaken the budget.** Report the
gap and what remains. A budget adjusted to fit the result measures nothing.

If Lighthouse CI cannot run in this environment, say so and **still record before/after
transferred-bytes per route** by measuring the served assets directly. Some measurement is
mandatory; the specific tool is not.

---

### TASK 2 — Lazy-load the Capsule assistant

**File:** `internal/ui/layouts/base.templ`, `internal/ui/components/capsule_assistant.templ`

`Base` ends its `<body>` with `@components.CapsuleAssistant()`. That renders a 1,566-line
Alpine subtree — drag-and-drop, file upload, message list, eight transitions — into every
page, including the signed-out marketing site and every admin screen. Alpine must walk and
initialise all of it before the page is interactive. It is the largest fixed cost on every
navigation.

Required:

1. The shell renders only the trigger button — target under 40 lines, no `x-data` beyond
   what the button itself needs.
2. The panel loads over HTMX on first open, then stays in the DOM for the session.
3. **Gate it by audience.** Render nothing for signed-out visitors and for platform staff
   unless you can demonstrate they use it. Check where it is actually reachable before
   deciding; report what you found.
4. Behaviour on open must be unchanged — same panel, same handlers, same drag-and-drop.

Report the before/after byte count of the rendered `<body>` for one representative page.

---

### TASK 3 — Remove `backdrop-filter` from scrolling surfaces

**Files:** `static/css/layout.css` (~line 644), `glass.css`, `components.css`

`.public-navbar, .top-navbar` carries `backdrop-filter: blur(16px)` and is
`position: sticky`. The browser therefore re-blurs the region behind the header **on every
scroll frame, on every page, in all four shells**. This is the sidebar and page-scroll lag.

Someone already diagnosed this — `static/js/app.js` around line 279 carries a comment
stating `backdrop-filter` is "the most expensive property in this stylesheet". The
diagnosis was correct and was applied only to modals.

Required:

- Remove `backdrop-filter` from every sticky, fixed, or scroll-adjacent surface. Replace
  with a solid token background plus a 1px border. The visual result should read as
  *cleaner*, not degraded.
- Keep blur **only** on the modal overlay and the assistant panel, where the content behind
  is static.
- Once background blurs are gone from the page, the modal-open workaround in `app.js` that
  disables `.glass-panel` blurs may be redundant. Check whether it still has a job. If it
  does not, remove it **and its comment's now-obsolete claim** — but preserve the historical
  explanation, rewritten to past tense. Do not silently delete the reasoning.
- Target: **at most 2** `backdrop-filter` rules remain.

---

### TASK 4 — Replace `transition: all`

32 declarations across `components.css` (20), `layout.css` (10), `foundations.css` (1),
`glass.css` (1). `transition: all` makes the browser watch every animatable property,
including layout-triggering ones, and on the navbar it compounds with the blur.

Replace each with the properties it actually means — almost always some of
`background-color`, `border-color`, `color`, `opacity`, `transform`, `box-shadow`. Never
transition `width`, `height`, `padding`, `margin`, `top`/`left`.

Add `@media (prefers-reduced-motion: reduce)` honouring if it is not already present.

Target: **zero** `transition: all`.

---

### TASK 5 — Split the CSS

233 KB on every route, including the marketing home page. Split into:

- **Shell bundle** — tokens, reset, base, layout, navigation, modal, form, button, table,
  badge. Loaded everywhere.
- **Surface bundles** — admin, vendor, customer, public. Loaded per audience.
- **Page bundles** — only for genuinely heavy one-off screens (the compare tool, the import
  wizards, the map picker).

The existing `Asset()` content-hash resolver in `internal/ui/layouts/assets.go` already
handles cache-busting; extend it, do not replace it. The shells are the natural place to
declare which bundles a page needs.

Do not introduce a bundler. Splitting files and loading the right ones is sufficient and
keeps the build as it is.

Target: **≤ 90 KB CSS** on a dashboard route.

---

### TASK 6 — Split `app.js`

60 KB parsed on every page load by every visitor. It contains, among other things, the
three-step registration stepper, a password strength meter, Leaflet glue, reverse geocoding
against Nominatim, and a hardcoded table of Egyptian city coordinates.

Split into:

- **Shell module** — theme, sidebar, tabs, dropdowns, modal manager, CSRF injection, HTMX
  glue, flash notices. Loaded everywhere. Target ≤ 22 KB.
- **Route modules** — registration/onboarding, maps and geocoding (Leaflet already
  demand-loads; follow that pattern), import wizards, the compare tool.

Keep the existing CSRF interception and scroll-restoration behaviour intact — both are
load-bearing and both are documented in comments. Preserve those comments.

---

### TASK 7 — Long lists and tables

Apply `content-visibility: auto` with an appropriate `contain-intrinsic-size` to repeated
row and card elements on the heaviest lists (admin catalogue inventory, organisations,
customer catalogue). Measure the effect; if it does not help on a given page, do not apply
it there. Report per-page results.

Do not implement virtualisation in this phase.

---

### TASK 8 — Query audit (static analysis only, no database)

You have **no database access**. Do not attempt to connect.

Read the repository layer for the ten heaviest dashboard screens. Look for:

- Queries issued inside a loop over rows (the N+1 shape).
- `SELECT *` where a handful of columns are used.
- Pagination applied in Go after loading a full result set.
- Filters on columns that no migration in `db/migrations/` creates an index for.

**Produce a report table only. Change no SQL.** Each row: file:line, the shape found, the
screen it affects, and the proposed fix. Index recommendations must cite the migration file
you checked. This becomes a later phase's work once someone with database access can verify
against real cardinality.

---

### PHASE 2 GATE

All ratchets hold. `go test ./... -count=1` passes. `backdrop-filter` ≤ 2.
`transition: all` = 0. CSS ≤ 90 KB and JS ≤ 22 KB (excluding HTMX/Alpine) on a dashboard
route, with before/after numbers. **Report Phase 2 and stop for review before starting
Phase 3.**

---

# PHASE 3 — DESIGN SYSTEM FOUNDATION

## Read this first

`.impeccable.md` exists in the project root. **Read it in full before writing any CSS.** It
is the source of design truth — users, brand personality, anti-references, typography and
the five design principles. It was written from a structured design interview with the
product owner. Where this prompt and that file could conflict, that file wins on matters of
taste; this prompt wins on matters of scope.

The single most important line in it: **the counter test.** If a pharmacist cannot complete
the task one-handed, on a mid-range Android, in bright light, while being interrupted, it
is not done.

## What Phase 3 is and is not

**Is:** the token layer, the cascade, typography, breakpoints, the icon dispatcher, and a
documented component contract — proven on a short list of screens.

**Is not:** converting 136 pages. That is Phase 5. Phase 3 touches the pages named in
Task 7 and no others.

---

### TASK 1 — Declare the cascade with `@layer`

There are **670 `!important` declarations**: `components.css` 212, `utilities.css` 195,
`foundations.css` 181, `layout.css` 53, `base.css` 26, `app.css` 3. That is not a styling
problem, it is a cascade with no defined order — every fix had to shout louder than the
last, which is the mechanical reason a change on one page never generalises.

Declare the order once, at the top of the first stylesheet loaded:

```css
@layer reset, tokens, base, layout, components, utilities;
```

| Layer | Holds |
|---|---|
| `reset` | Normalisation, box-sizing, RTL defaults |
| `tokens` | Colour, type, space, radius, z-index, motion, density |
| `base` | Element defaults, typography |
| `layout` | Shell, sidebar, top bar, page container, grid |
| `components` | Button, card, table, modal, form, badge, nav |
| `utilities` | Single-purpose helpers — win by position, never by `!important` |

Move existing rules into layers and delete each `!important` as its layer lands. Work layer
by layer, verifying visually as you go — do not delete all 670 in one commit.

**Target: under 20**, each surviving one carrying a comment justifying it. If you cannot
reach 20 without breaking something, stop at the number you can defend and report the
remainder with reasons. An honest 60 beats a broken 20.

---

### TASK 2 — Rebuild the token layer in OKLCH

**File:** `static/css/tokens.css`

Per `.impeccable.md`: the clinical blue **stays** as the brand hue and is refined, not
replaced. Rebuild it properly.

Requirements:

- Express colour in `oklch()`. As lightness approaches white or black, **reduce chroma** —
  high chroma at extreme lightness looks garish.
- **Tint the neutrals toward the brand hue** at a chroma around 0.005–0.01. Pure grey reads
  as unconsidered; a neutral carrying a trace of the brand hue reads as chosen. No pure
  `#000` or `#fff` anywhere.
- Semantic colour (success / warning / danger / info) is **separate from the accent** and
  does not count as the accent. Verify each is distinguishable under the common forms of
  colour blindness — this platform moves medicine and money.
- **Both themes are first-class.** Light is the default; dark gets equal design effort, not
  a naive inversion. Define the complete light palette on `:root`; redefine tokens only
  inside `@media (prefers-color-scheme: dark)` guarded as `:root:not([data-theme="light"])`,
  and again under `:root[data-theme="dark"]` so the toggle wins in both directions. Never
  give a colour its only definition inside a media or `[data-theme]` block.
- **Density tokens.** Two modes from one component set: `comfortable` (default — customer
  and pharmacy surfaces, 44 px minimum touch targets) and `compact` (admin and vendor
  tables). Density drives spacing, row height and control size. **Never fork a component
  for admin.**
- Spacing on a 4pt scale with semantic names (`--space-sm`, not `--space-8`):
  4, 8, 12, 16, 24, 32, 48, 64, 96.

Contrast: WCAG AA minimum on body text, AAA on data and numbers, **verified in both
themes**. Report the ratios you measured for the main text/surface pairs.

---

### TASK 3 — Typography

Per `.impeccable.md`:

- **Primary: `Readex Pro`** (Google Fonts) — Arabic and Latin, variable, designed for
  readability. Replaces IBM Plex Sans Arabic.
- **Data: `Spline Sans Mono`** — SKUs, batch numbers, tracking codes, API keys only. Never
  decorative.
- Every money and quantity column gets `font-variant-numeric: tabular-nums`.
- **Fixed `rem` scale for the product UI.** No fluid `clamp()` type in dashboards — no major
  design system does this in product UI. `clamp()` is allowed on public marketing pages only.
- Five steps, minimum 1.25 ratio between them. Not eight steps 1.1 apart.
- **Arabic needs more line-height than Latin at the same size.** Set it per script rather
  than using one global value.
- Preload the primary font and set `font-display: swap`. Declare a real fallback stack.
- Body text capped at 65–75ch.

Verify both scripts at every step of the scale before committing. An Arabic glyph set that
breaks at one size is a bug you will not see in a Latin preview.

---

### TASK 4 — Four breakpoints, not eleven

Current distinct breakpoints: 992, 768, 640, 700, 1100, 980, 1024, 900, 860, 720, 420 —
eleven, because each was tuned per page.

Collapse to four tokens: **640 / 768 / 1024 / 1280**. Everything else becomes fluid via
`clamp()`, `minmax()`, and `repeat(auto-fit, minmax(…, 1fr))`.

Use **container queries** for component-level responsiveness — a card in a sidebar should
adapt to the sidebar, not the viewport. Viewport queries are for page layout only.

Explicit rules the current CSS lacks, all driven by the counter test:

- **44 px minimum touch targets** on every interactive element in `comfortable` density.
- **Tables become card lists below 768 px**, not horizontal scrollers. A pharmacist cannot
  scroll a table sideways one-handed.
- **Modals become full-bleed sheets on mobile.**
- **The sidebar becomes an off-canvas drawer below 1024 px** with a focus trap and Escape to
  close.
- Logical properties **everywhere** — `margin-inline`, `inset-inline-start`, `padding-block`.
  A physical `left` is a latent RTL bug. This is an RTL-first platform.

---

### TASK 5 — Consolidate the icon set

**Decision from the product owner: the icons stay.** They are not a performance problem —
94 inline SVGs across 63,931 template lines, no network requests, no decode cost. Do not
remove them. Consolidate them.

- `icons.templ` generates a **202 KB Go file** compiled into every build. Reduce it.
- Pages import icons individually; the sidebar already uses an `Icon(name, class)`
  dispatcher. Move all callers onto the dispatcher.
- Prune icons nothing references. `make check-unused-components` holds this at zero for
  components — apply the same standard to icons.
- Every icon carries `aria-hidden="true"` when decorative, and a real accessible name when
  it is the only content of a control.

---

### TASK 6 — The component contract

Write one canonical implementation of each, and document what it is:

button, input, select, checkbox/radio, card, table, modal, badge/status pill, empty state,
loading skeleton, error state, pagination, tabs, dropdown, toast.

Each needs: variants, sizes, both density modes, both themes, RTL, keyboard behaviour, and
a focus-visible state. Per `.impeccable.md`, **empty states must teach the interface**, not
just say "nothing here."

**Two modal systems currently coexist** — 35 uses of the native `<dialog>`-based
`components.Modal` and 38 hand-rolled `.modal-overlay` divs driven by `window.openModal`.
The native one is correct: real top layer, focus trap, Escape and backdrop for free. In
this phase, **specify** the single modal contract and migrate the modals only on the Task 7
screens. The remaining migration is Phase 4.

---

### TASK 7 — Prove it on five screens, and only five

Apply the new foundation to exactly these, chosen to cover every shell and both densities:

1. `/admin/dashboard` — admin shell, compact
2. `/admin/organizations` — heaviest admin table, compact, the mobile-table rule
3. Customer catalogue — customer shell, comfortable, the counter test
4. Customer cart — comfortable, form and money display
5. `/vendor/dashboard` — vendor shell

For each: inline styles to classes, ad-hoc markup to the Task 6 components, loading/empty/
error states present, verified at **375 px**, verified keyboard-only, verified in both
themes, verified RTL and LTR.

**Do not touch the other 131 pages.** Report the inline-style count before and after on
these five only.

---

### TASK 8 — New ratchets

Add to the `Makefile` and wire into `make check`:

| Gate | Target |
|---|---|
| `check-important` | ≤ 20 (or the number you defended in Task 1) |
| `check-backdrop-filter` | ≤ 2 |
| `check-transition-all` | 0 |
| `check-breakpoints` | ≤ 4 distinct values |
| `check-physical-properties` | no new `left:`/`right:`/`margin-left:`/`margin-right:` in CSS |

Lower `check-inline-styles` by whatever the five screens removed. **Do not raise any
ceiling.**

---

### PHASE 3 GATE

All ratchets hold, old and new. `go test ./... -count=1` passes. The five screens render
correctly at 375 px and 1440 px, in both themes, in RTL and LTR, and are keyboard
traversable. Lighthouse budget from Phase 2 still passes.

---

## REPORTING — twice, separately

**Each report:**

1. **Per task** — `DONE` / `PARTIAL` / `STOPPED`, files touched, what and why.
2. **Before/after numbers** for every measurable target in that phase. Phase 2 targets are
   bytes and milliseconds; Phase 3 targets are counts. Assertions without numbers are not
   accepted.
3. **Discrepancies** — where the code did not match this prompt's quotes or counts.
4. **Task 8 (Phase 2)** — the full query audit table.
5. **Task 1 (Phase 3)** — the final `!important` count and every one you could not remove,
   with its reason.
6. **Deferred** — noticed and deliberately untouched.
7. **Gate output** — the six commands, each with its real number against its ceiling, and
   `NOT RUN` where applicable.
8. **Diff summary** — `git diff --stat` against the parent branch.

**Stop after Phase 2 and wait for review. Do not begin Phase 4.**
