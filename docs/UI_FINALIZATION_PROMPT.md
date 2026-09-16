# UI FINALIZATION — EXECUTION PROMPT (Antigravity / Gemini)

Finish the Dawa24 platform UI. This supersedes every earlier prompt.

**Start from:** `main` at `fb62b72`. Verify with `git log --oneline -1`.
Branches, in order: `ui-1-pages` → `ui-2-chrome` → `ui-3-reorg`.

**Three gates. Stop and report at each.**

---

## 0. READ THIS FIRST — THE ONE BUG BEHIND MOST OF THE LIST

Phase 5 removed 2,089 inline `style` attributes across 136 templates and replaced them with
class names. **A large number of those class names were never written into any stylesheet,
and in many places the class was not replaced at all** — the element was simply left bare.

Every ratchet reports these pages as converted. `check-inline-styles` counts occurrences of
`style="`; it cannot tell a conversion from a deletion. This is recorded as **ADR 0017** and
it is the single most important thing to understand before touching anything:

> A ratchet measures what it counts, not what you care about.

Measured now, classless structural tags per reported page:

```
wallet                       95      admin_settings               71
mfa_settings                 65      admin_cities                 58
vendor_jobs                  54      auth                         55
admin_match_decisions        47      tenant_sessions              43
organization_documents       42      compare_tool                 37
admin_policies               37      vendor_user_organizations    33
customer_invoices            32      customer_user_organizations  29
```

Those are `<div>`, `<h1>`, `<p>`, `<table>`, `<form>`, `<label>` with **no class**. That is
why these pages look unstyled. It is not a missing stylesheet link and not a broken import —
do not go looking for one.

### Three separate text-corruption bugs — do not confuse them

| Symptom | Cause | Recoverable? |
|---|---|---|
| `????????` | Arabic destroyed to question marks when the file was written | **No** — every character collapsed to the same `?`. 206 lines were recovered from git ancestors; ~119 remain and must be written by hand. |
| `ØªØ±Ø§Ùƒ` | UTF-8 read as CP1252, re-encoded | **Yes, exactly** — already fixed on both dashboards. |
| SQL `i18n.TDefault(...)` in JSON | The i18n sweep rewrote Arabic inside raw SQL strings | Fixed — 14 occurrences. |

If you find more of type 2, reverse it with CP1252 (bytes `0x81 0x8d 0x8f 0x90 0x9d` need
mapping through by ordinal), and **only accept a conversion whose result contains Arabic**.

If you find more of type 1, **do not** match greedily on `?`-count and sequence. That was
tried; it recovered a hundred extra strings and put some in the wrong place — it labelled a
`closed` option with المتقدم. On a platform that moves medicine, a plausible wrong Arabic
label is worse than a visible `????`, because nobody reports it. Match only when the
surrounding markup is identical, or write the string by hand.

---

## 1. NON-NEGOTIABLE RULES

1. **Every style goes in a `.css` file, inside the right `@layer`.** Never an inline `style`
   attribute, never a `<style>` block in a template. Inline styles are at **81** and must not
   rise.
2. **`!important` stays at 3.** The three survivors are hide/show primitives in `app.css`,
   which is deliberately outside `@layer` (ADR 0014). Do not add a fourth.
3. **Breakpoints stay 640 / 768 / 1024.** No new values.
4. **Logical properties only** — `margin-inline`, `inset-inline-start`, `padding-block`. A
   physical `left` is a latent RTL bug. This platform is Arabic-first.
5. **Never `letter-spacing` on Arabic.** It breaks the cursive joins. If a treatment is wanted
   for English, scope it: `:root[lang="en"] .x`.
6. **Reuse before you write.** Check `components.css` and the component set first. A second
   `.card` is a defect, not a feature.
7. **THE STOP RULE.** Where this prompt quotes code or a number, verify it first. If it does
   not match, stop that item, report it, move on.
8. **Paste raw command output. Never transcribe a number.**
9. **Mark every stated gate MET or MISSED. A skipped task is a discrepancy.**
10. **No new `go.mod` dependencies. No database. No SQL migrations.**
11. **Do not touch authorization.** Report bugs; do not fix them here.
12. **Name commits for what they do.** Many commits here are named `a`, `fasf`, `gdfgdf`.
13. Read `.impeccable.md` before any design decision. The **counter test** governs: one hand,
    mid-range Android, bright light, interrupted.

### Gate commands — `make` is unavailable; run these and paste output

```bash
gofmt -l ./cmd ./internal
go vet ./...
go test ./... -count=1
grep -oh 'style="' internal/ui/pages/*.templ internal/ui/layouts/*.templ | wc -l
grep -oh '!important' internal/ui/static/css/*.css | wc -l
```

`templ` is at `/c/Users/mydwa/go/bin/templ`. Regenerate only what you edit; commit the
generated `*_templ.go` — they are deliberately tracked.

**Verify visually.** `test/visual_regression_test.go` captures the component gallery in 8
combinations using headless Chrome. Light and dark now genuinely differ (~178 KB vs ~175 KB);
if they collapse back to within a few bytes, something re-broke the theme and you must stop.

---

# GATE 1 — THE PAGES

Work each page to the same standard. **A page is done when every line below is true**, and
you report per-page, per-item.

### The per-page checklist

1. **No classless structural tag.** Every `<div>`, `<h1>`, `<p>`, `<table>`, `<form>`,
   `<label>` carries a class that resolves to a real rule.
2. Layout uses grid/flex with `gap`, from tokens. No magic pixel values.
3. Verified at **375px**. Tables become card lists below 768px — never sideways scrollers.
4. Both themes.
5. RTL **and** LTR.
6. Keyboard traversable, visible focus.
7. Loading, empty and error states present. Per `.impeccable.md` an empty state teaches the
   interface — never "no data".
8. Touch targets ≥ 44px in `comfortable` density.
9. Every number `tabular-nums`.
10. No `????` and no mojibake left on the page.

### The pages, in priority order

**Group A — admin**
- `/admin/settings` (868 lines, 71 bare). **The tab navigation is the main defect**: fix the
  tab switching and every tab's contents, not just the shell. Use the existing tab component.
- `/admin/cities` (838 lines, 58 bare) — layout, styling and function.
- `/admin/match-decisions` (363 lines, 47 bare) — has essentially no styling.
- `/admin/dashboard` — the mojibake is fixed; audit what remains against `.impeccable.md`.

**Group B — auth and shared**
- `/auth/login` (457 lines, 55 bare) — not wired to the design system at all.
- `/invoices` (206 lines, 32 bare)
- `/customer/user-organization` (426 lines, 29 bare)

**Group C — vendor**
- `/vendor/dashboard` — mojibake fixed; audit the rest.
- `/vendor/wallet` (755 lines, 95 bare) — the worst on the list.
- `/vendor/jobs` (905 lines, 54 bare). **The add-job modal is the specific complaint.** Its
  form posts to `POST /vendor/jobs`, which is registered at `vendor_routes.go:341` and handled
  by `VendorJobCreateSubmit` — so the route exists. Verify the field names the form submits
  match what the handler reads, fix the modal's text (some is still `????`), and confirm a
  created job actually persists and appears in the list.
- `/vendor/documents` — **modals only.**
- `/vendor/policies`, `/vendor/user-organization`, `/vendor/mfa`, `/vendor/sessions`
- `/compare/tool` (863 lines, 37 bare) — component and section layout is wrong.

**Group D — customer**
- `/customer/jobs`, `/customer/sessions`, `/customer/mfa`

Commit per group, not per page, and not all four in one commit.

---

# GATE 2 — CHROME AND CROSS-CUTTING

### 2.1 Dropdown and select widths

Filter and modal dropdowns stretch to their widest option and break the layout.
`select.form-input` is defined at `components.css:1508` with no width ceiling.

Give selects and dropdown menus a sane `inline-size` and `max-inline-size`, with
`text-overflow: ellipsis` on long options. **Do not fix this per page** — one rule in
`components.css`, then verify the pages that were breaking.

### 2.2 The footer

`.footer-logo` is `height: 36px` in `public.css:802`, which does not look oversized on its own
— so **something else is winning, or the class is not applied where you think**. Find out
which before changing the number; do not simply shrink it until it looks right.

Then fix the footer's spacing, alignment and column rhythm, and check it at 375px.

### 2.3 One avatar

The public header renders `.account-avatar` (32px, `components.css:569`); the dashboard
account menu renders `.user-menu-avatar`. Two components for the same thing.

Consolidate on one. Keep `.account-avatar` — it is the older name and used in more places —
and make `components.UserMenu` use it. Delete whichever rule ends up unreferenced.

### 2.4 Finish the `????` recovery

~119 lines still carry `????`, concentrated in `vendor_jobs` (28), `admin_warehouses` (32),
`vendor_coverage_table` (14) and `admin_saving_products` (7).

These have no matching ancestor line, so they must be written by hand from what the
surrounding markup is plainly doing. **Report every string you write**, so the wording can be
checked. Where you genuinely cannot tell what a label meant, say so and leave it — an honest
gap beats an invented label.

### 2.5 The visual gallery covers what you changed

`internal/ui/pages/component_gallery.templ` is the visual regression fixture. If you add or
change a component in Gate 1 or 2, add it there so the suite can see it.

---

# GATE 3 — THE REORG

**This has been skipped three times and reported as done each time.** `internal/ui` holds
**248 flat `.go` files** beside `pages/`, `components/`, `layouts/`, `static/`.

The filenames already carry the grouping: 62 `admin_`, 40 `vendor_`, 25 `customer_`.

```
internal/ui/admin/  vendor/  customer/  shared/  public/  view/
```

`pages/`, `components/`, `layouts/`, `static/` do not move. `view/` takes the domain→template
mapping currently spread across `*_models.go`.

**Rules for this specific task:**

- **One audience per commit.** Six commits, not one. A 248-file move in one commit cannot be
  reviewed or bisected.
- **No behaviour change in a move commit.** Move, fix imports, run tests, commit. Anything
  else is a separate commit.
- Test files move with the code they test.
- `UIHandler` methods sit on one receiver across all these files. **Keep the single receiver** —
  splitting the type is out of scope.
- **`go test ./... -count=1` after every commit, not at the end.**

**If Go's package rules make a clean six-way split impossible, stop and report the shape that
is actually achievable.** A partial reorganisation reported honestly is a good outcome. A
forced one that fragments the handler type is not, and silently skipping it a fourth time is
the worst of the three.

---

## REPORTING — three times

Each report contains:

1. **Per page, per checklist item: MET or MISSED.** Not "checklist applied" — that was
   reported once while item 8 was untouched on every page.
2. Raw pasted output for every gate command.
3. **Bare-tag count per page, before and after.** This is the number that actually tracks the
   defect; inline-style count does not.
4. Every string you wrote by hand for 2.4.
5. Discrepancies — including anything you did not do, and anything where a diagnosis above did
   not match what you found. **If I am wrong about a cause, say so.** These were read out of
   source, not observed in a running browser.
6. `git diff --stat` against the parent branch.
7. `git branch --show-current` and `git log --oneline -1` after committing — another session
   has moved HEAD in this repository between tool calls before.

**Stop at every gate.**
