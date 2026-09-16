# PHASE 4 & 5 — EXECUTION PROMPT (Antigravity / Gemini)

**Prerequisite: Phase 3 is not done.** It was issued in the same document as Phase 2 and you
correctly stopped at the Phase 2 gate. Complete Phase 3 from `docs/PHASE_2_AND_3_PROMPT.md`
first — with the Task 0 addendum in §1 below — and report it. Only then start Phase 4.

Branches: `phase-3-design-system` from `phase-2-performance`, then `phase-4-chrome` from
Phase 3, then `phase-5-conversion` from Phase 4.

**Report three times** — Phase 3, Phase 4, then Phase 5 *per wave*. Do not combine reports.

---

## 0. PHASE 2 REVIEW — ACCEPTED, WITH ONE MISSED GATE

Real wins, verified against the tree:

| Item | Before | After | Verdict |
|---|---|---|---|
| `transition: all` | 32 | **0** | Verified |
| `backdrop-filter` rules | 25 | **2** (both modal overlays) | Verified — target met |
| `app.js` | 59.6 KB | **16.2 KB** | Verified |
| `components.css` | 130 KB | **69.7 KB** | Verified |
| `layout.css` | 41.5 KB | **21.8 KB** | Verified |

The Capsule assistant split is correct and the panel route is properly mounted in
`RegisterApprovedSharedRoutes`, so it inherits `RequireAuth` + `RequireApproved`. You
reasoned about where it should live rather than mounting it in the first group to hand.

The Task 8 query audit is the best artefact of the phase — ten real findings with the
migration files checked, and no SQL changed. That is exactly the scope discipline asked for.
Reporting Lighthouse as **NOT RUN** rather than fabricating a score was right.

### The missed gate

The Phase 2 gate said **CSS ≤ 90 KB on a dashboard route**. Measured now:

```
tokens + base + layout + glass + components + foundations + utilities + app = 154,955 bytes
```

All eight original stylesheets still load on every route in `base.templ`; the surface sheets
are **added on top** (`customer.css` is a further 41 KB). So a customer route now transfers
roughly 196 KB and an admin route roughly 156 KB.

That is still a real improvement on 233 KB, and the split was worth doing. But the gate was
not met, and your report did not say so — it reported "~144 KB uncompressed (~28 KB
compressed)" and moved on. **When a stated gate is missed, the report must say MISSED and by
how much.** Silently reframing to a metric that passes is the same failure as a false green.

### Measurement discipline — this is the thing to fix

Three numbers in your report were better than reality, all in the flattering direction:

| You reported | Actual | Command |
|---|---|---|
| 85 oversized Go files | **101** | the Makefile's own command |
| 3,988 inline styles | **4,001** | the Makefile's own command |
| CSS ≤ budget | **155 KB shell** | sum of files in `base.templ` |

You reported 85 twice, in two consecutive phases. You are almost certainly excluding
`_test.go`, which the gate does not.

**From now on: paste raw command output, not transcribed numbers.** Do not retype a figure
into a table. Copy the terminal output verbatim into the report. A number you typed by hand
is a number I have to re-measure, and I have re-measured three of yours.

---

## 1. PHASE 3 — TASK 0 ADDENDUM

Add these to Phase 3 as it was written. Everything else in
`docs/PHASE_2_AND_3_PROMPT.md` Phase 3 stands unchanged.

**0a. Get the shell under 90 KB.** Phase 3's cascade work is what makes this reachable —
you cannot delete duplicated rules until the layer order tells you which copy wins.
Specifically:

- `glass.css` (8 KB) exists to style a glass aesthetic that Phase 2 largely removed. Check
  whether anything still needs it; if not, delete it and fold the survivors into
  `components`.
- `foundations.css` (22.4 KB, 181 `!important`) and `utilities.css` (14.4 KB, 195
  `!important`) overlap heavily with `components.css`. Once `@layer` declares the order, the
  duplicates can be deleted rather than out-shouted.
- `customer.css` at 41 KB is larger than some shells. Split it per surface (catalogue, cart,
  orders) and load per route.

Report the shell total as raw `wc -c` output, before and after.

**0b. Measurement.** Use the exact Makefile commands. Paste raw output.

---

# PHASE 4 — CHROME, NAVIGATION, MODALS

Read `.impeccable.md` before starting. The counter test governs every decision here: **one
hand, mid-range Android, bright light, interrupted.**

## Why this phase exists

The product owner named the dashboard top navbar as the single worst part of the platform.
It is the clearest instance of the general disease, and this phase fixes the disease.

---

### TASK 1 — One dashboard top bar

**Current state — verify before editing.** A single CSS rule serves two unrelated jobs:

```css
.public-navbar,
.top-navbar { height: var(--header-height, 4.5rem); … }
```

The marketing navbar wants 4.5 rem, a brand lockup and generous rhythm. The dashboard navbar
sits above a data table and wants to be compact and quiet. They currently share height,
padding, sticky positioning and shadow.

Then each shell hand-builds its contents differently:

| Shell | Left | Right | Layout via |
|---|---|---|---|
| Public | Brand + 5 nav links | Auth actions, mobile toggle | `.top-navbar-brand` |
| Admin | Page title | Lang, theme, user chip, logout | `.top-navbar-start/end` |
| Vendor | Page title | Lang, theme, static badge, logout | `.top-navbar-start/end` |
| Pharmacy | Title + branch selector | Lang, theme, cart, badge, logout | **inline `style=`** |

The pharmacy shell uses inline styles for its top-bar layout instead of the classes the other
two use, so it does not even align with them.

**Required:**

1. One `DashboardTopBar` component with a declared contract:
   - a title/breadcrumb slot,
   - a contextual slot (branch selector, org switcher — whatever that shell needs),
   - a fixed action cluster: search, notifications, user menu.
2. `.site-header` becomes a **separate class** for the public navbar, sharing **no** rules
   with the dashboard bar. Delete the combined selector.
3. Four implementations become two. No inline styles in any shell.
4. Density-aware per `.impeccable.md`: `compact` for admin and vendor, `comfortable` for
   pharmacy.

---

### TASK 2 — A real user menu

Phase 0 replaced the hardcoded "Super Admin" string with the actual actor, which was the
minimum. Now build the control properly: name, role, organisation, organisation switcher
where the actor belongs to several, settings, and logout.

Keyboard accessible, Escape to close, focus returns to the trigger on close, and it must not
be a `<div>` pretending to be a menu. On mobile it becomes a sheet, not a dropdown that
overflows the viewport.

`/org/switch/{id}` already exists in Tier B — wire the switcher to it. Do not invent a new
endpoint.

---

### TASK 3 — One modal system

**Two coexist:** 35 uses of the native `<dialog>`-based `components.Modal`, and 38
hand-rolled `.modal-overlay` divs driven by `window.openModal`.

The native one is correct — real top layer, focus trap, Escape and backdrop for free. The
legacy one has none of that, which means roughly half the platform's dialogs are
keyboard-inaccessible and behave differently under the same trigger.

**Required:**

1. Migrate all 38 legacy dialogs to `components.Modal`.
2. Delete `window.openModal` / `window.closeModal` and the `.modal-overlay` CSS.
3. Preserve the historical comment in `app.js` explaining the backdrop-filter cost —
   rewritten to past tense, since Phase 2 removed the cause. Do not delete the reasoning.
4. Those modal rules currently carry `!important` on nearly every declaration
   (`components.css` ~line 222). With Phase 3's `@layer` in place they should not need it —
   remove them as part of this migration.

This is the highest-value consistency win available: it fixes focus management, Escape,
scroll lock, mobile behaviour and visual consistency across 38 sites in one pass.

**Migrate in batches of ~8 and verify each batch before continuing.** A single 38-file
commit is not reviewable.

---

### TASK 4 — The modal contract

Specify once, then hold everywhere:

- Sizes from tokens (`sm` / `md` / `lg` / `xl` / `full`), never ad-hoc widths.
- Body scroll lock on open; restore exact scroll position on close; correct behaviour with
  two modals open (the existing open-count logic in `app.js` already handles this — keep it).
- **Full-bleed sheet below 768 px.** A centred dialog with margins is unusable one-handed.
- Focus moves to the first interactive element on open, returns to the trigger on close.
- Escape and backdrop click close; a form mid-edit warns before discarding.
- **Destructive actions require typed confirmation**, not just a red button. This platform
  moves medicine and money.
- Loading and error states inside the modal body — `components.Modal` already has a `State`
  field; use it rather than inventing per-page spinners.

---

### TASK 5 — The sidebar

Currently a squeezed column at every width. Per the counter test it must become:

- **Off-canvas drawer below 1024 px**, opened by a control in the top bar, with a focus trap,
  Escape to close, backdrop click to dismiss, and focus returned to the trigger.
- **Collapsed icon rail** above 1024 px with accessible tooltips, state persisted.
- Scroll position preserved across navigation — this already works in `admin.templ`'s inline
  script. Move that logic into the shell JS module rather than leaving it inline in a
  template, and keep its behaviour identical.
- The active-link resolution currently in that same inline script must keep working. It is
  load-bearing: it highlights the correct link when the route does not exactly match an href.

The sidebar already renders from the RBAC registry (`layouts/sidebar.templ`). **Do not change
what it renders or how visibility is decided** — that is settled and correct. This task is
presentation only.

---

### TASK 6 — Navigation information architecture

With one top bar and one sidebar, review the grouping itself. For each of the three
dashboards, report:

- Sections and items, in order, with the permission that gates each.
- Any item whose label does not describe what the page does.
- Any item that duplicates another route.
- Any section with one item, or with more than about seven.

**Report only. Change nothing in this task** without flagging it — navigation changes alter
muscle memory for existing users and are the product owner's call, not yours.

---

### TASK 7 — Ratchets

| Gate | Target |
|---|---|
| `check-modal-legacy` | 0 occurrences of `modal-overlay` or `window.openModal` |
| `check-inline-styles` | lower by whatever the shells removed |
| `check-topbar-impls` | 2 (`DashboardTopBar`, `site-header`) |

### PHASE 4 GATE

All ratchets hold. Tests pass. The three dashboards work at 375 px and 1440 px, in both
themes, RTL and LTR, keyboard-only. Zero legacy modals. **Report and stop for review.**

---

# PHASE 5 — PAGE CONVERSION AND ENGLISH

This is the largest phase. It runs in **four waves**, and you **report after each wave and
stop**. Do not attempt all four in one pass.

## The English requirement

The product owner has confirmed **English is a real requirement, not aspirational.**

There are **1,246 Arabic string literals hardcoded in Go**, tracked by
`make check-hardcoded-arabic`. The language toggle appears in every navbar and promises
something the platform cannot deliver: a large share of the interface renders Arabic
regardless of the chosen language.

Every literal must move to `internal/shared/i18n` and be called through `i18n.T(lang, key)`.
This is per-page work and belongs inside each wave's checklist, not as a separate task —
converting a page means converting its strings.

**Key naming:** follow the convention already in `internal/shared/i18n`. Read it before
inventing a scheme. Report the convention you found.

**Do not machine-translate.** Where an English string is not already available, add the key
with the Arabic text present and the English marked as needing translation, and list those
keys in your report. A wrong English string in a medicine platform is worse than an obviously
missing one.

---

### The per-page checklist

Every page in every wave gets all of this. A page is not done until each line is true:

1. Inline `style=` attributes replaced with classes from the Phase 3 component set.
2. Ad-hoc markup replaced with Phase 4 components. No new one-off card/table/modal markup.
3. Loading, empty and error states present. Per `.impeccable.md`, **empty states teach the
   interface** — not "no data".
4. Verified at **375 px**. Tables become card lists below 768 px; they do not scroll
   sideways.
5. Keyboard traversable; visible focus on every interactive element.
6. Both themes checked.
7. RTL and LTR both checked.
8. Arabic literals moved to `i18n`.
9. Touch targets ≥ 44 px in `comfortable` density.
10. No `!important` added. No new breakpoint introduced.

---

### Wave 1 — the daily drivers

`/admin/dashboard`, `/vendor/dashboard`, pharmacy dashboard, customer catalogue, cart,
checkout, customer order detail, settings.

Phase 3 already converted five of these as its proof set. Wave 1 finishes the rest and
verifies the five still hold after Phase 4's chrome changes.

### Wave 2 — the admin heavy screens

Approvals, organisations, finance, catalog inventory, users, roles, cities, developers.

`admin_catalog_inventory.templ` is **2,736 lines** and `admin_organizations.templ` is 1,460.
These must be decomposed into per-tab and per-section components as part of conversion — a
2,700-line template cannot be reviewed or maintained, and converting it in place just moves
the problem.

### Wave 3 — vendor operations

Offers, inventory, ingest, warehouses, team, transfers, coverage, finance.

### Wave 4 — public and the long tail

Marketing pages, auth, onboarding, jobs, compare, and everything remaining. This is the
largest count but the simplest pages.

---

### Ratchets — lower after every wave

At the end of each wave, lower `check-inline-styles` and `check-hardcoded-arabic` to the new
measured values and commit that with the wave. **This is the definition of done for the
wave.** A wave that converts pages without lowering the ceiling has not finished, because the
count can drift back up before the next one starts.

Targets by end of Phase 5: inline styles **≤ 300**, Arabic literals in Go **0**.

If a wave cannot reach its share, report the real number and why. Do not raise a ceiling.

---

## REPORTING

**Six separate reports: Phase 3, Phase 4, then one per Phase 5 wave.**

Each contains:

1. **Per task or per page** — `DONE` / `PARTIAL` / `STOPPED`, files touched, what and why.
2. **Raw command output** for every ratchet. Pasted, not transcribed. This is not optional
   any more.
3. **Every stated gate, explicitly marked MET or MISSED**, with the number. A missed gate
   reported honestly is fine; a missed gate not mentioned is not.
4. **Discrepancies** — where the code did not match this prompt's quotes or counts.
5. **Untranslated keys** (Phase 5) — every key added without a verified English string.
6. **Task 6 IA report** (Phase 4) — findings only, no changes.
7. **Deferred** — noticed and deliberately untouched.
8. **Diff summary** — `git diff --stat` against the parent branch.

**Stop at every gate. Do not run two phases, or two waves, without review in between.**
