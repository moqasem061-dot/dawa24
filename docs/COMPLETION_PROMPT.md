# COMPLETION PROMPT — FINISH THE PLATFORM (Antigravity / Gemini)

Every remaining item on the Dawa24 rebuild, diagnosed and specified. This supersedes all
earlier prompts including `CLOSEOUT_PROMPT.md`.

**Start from:** `phase-7-regression` at `b0d9230`. Verify with `git log --oneline -1`.

Branches, in order: `fix-1-visual-net` → `fix-2-ui-defects` → `fix-3-wiring-audit` →
`fix-4-reorg` → `fix-5-closeout`

**Five gates. Stop and report at each.** Do not run two without review between.

---

## 0. STATE — VERIFY BEFORE STARTING

```
inline styles (pages+layouts)      81
raw <dialog> in pages               1     (component_gallery only)
elements with exact class "modal"   1     (the <dialog> in components/modal.templ)
components.Modal uses              57
!important                          3
undefined CSS classes             235
user-facing Arabic in Go            0
oversized Go files                  0
templates over 1000 lines           0
deadcode findings                 211
shell CSS bytes               167,151
flat .go files in internal/ui     246
ADRs                               18
build / vet / test (70 pkgs)     pass
```

If a number differs, **stop and report it** — the tree moved.

### Settled — do not redo, do not "improve"

Authorization (Phases 0–1.5). The account lifecycle matrix
(`internal/ui/account_lifecycle_matrix_test.go`). The i18n exclusion list (ADR 0018). The
`@layer` order (ADR 0014). The top bar contract (ADR 0016). Report bugs in these; do not
change them.

---

## 1. RULES

1. **THE STOP RULE.** Where this prompt quotes code or a number, verify before acting. If it
   does not match, stop that item, record it under Discrepancies, move on.
2. **Paste raw command output. Never transcribe a number by hand.**
3. **Mark every gate MET or MISSED with its number. A skipped task is a discrepancy.**
4. **Never lower a ceiling below the measured value.**
5. **No new `go.mod` dependencies. No database. No SQL. No migrations.**
6. **Do not delete or rewrite existing comments.** Past tense when a cause is removed.
7. **`!important` stays at 3. Breakpoints stay 640 / 768 / 1024.**
8. **Every new style goes in a `.css` file inside the right `@layer`.** Never inline, never a
   `<style>` block in a template.
9. **Name commits for what they do.** Several commits here are named `a`, `fasf`, `gdfgdf`.
10. Read `.impeccable.md` before UI work. The **counter test** governs: one hand, mid-range
    Android, bright light, interrupted.

### Gate commands — `make` is unavailable; run these and paste output

```bash
gofmt -l ./cmd ./internal
go vet ./...
go test ./... -count=1
grep -oh 'style="' internal/ui/pages/*.templ internal/ui/layouts/*.templ | wc -l
grep -oh '!important' internal/ui/static/css/*.css | wc -l
cat internal/ui/static/css/{tokens,base,layout,components,foundations,utilities,app}.css | wc -c
find ./cmd ./internal -name '*.go' -not -name '*_templ.go' -exec wc -l {} + | grep -v " total$" | awk '$1>400' | wc -l
```

`templ` is at `/c/Users/mydwa/go/bin/templ`. Regenerate only files you edit; commit the
generated `*_templ.go`.

---

# GATE 1 — REPAIR THE VISUAL SAFETY NET

Everything after this gate depends on being able to see what a CSS change did. The suite that
exists cannot see it yet.

### 1.1 The theme axis does nothing

`test/visual_baselines/gallery_comfortable_light_rtl.png` and
`gallery_comfortable_dark_rtl.png` are visually identical. Both render **dark**. All eight
baselines sit within 500 bytes of each other, which is the tell.

Cause: in `internal/ui/pages/component_gallery.templ`, `props.Theme` is consumed in exactly
one place —

```templ
<span class="badge badge-slate">{ fmt.Sprintf("Theme: %s", props.Theme) }</span>
```

— a debug label. It never sets `data-theme` on the document, so `base.templ`'s inline
theme script falls through to the OS preference and headless Chrome renders dark every time.
**Four of the eight baselines are duplicates of the other four.**

**Fix:** make the gallery set `data-theme` on `<html>` from `props.Theme`. The cleanest route
is a `Theme` parameter threaded into the layout the gallery renders inside; if that would
change `Base` in a way that affects real pages, instead have the harness set it before
capture (Chrome's `--headless` supports evaluating script before screenshot, or serve the
gallery through a wrapper that stamps the attribute).

**Proof required:** paste `md5sum` of all eight baselines and the byte sizes. Light and dark
must differ by more than a rounding error. If they still match, the fix did not work — say
so rather than re-committing.

### 1.2 Arabic renders as `????????`

Every Arabic string in the baselines is tofu. The page heading captures as
`Phase) ???????? ????`. The headless browser has no Arabic font, so **the suite cannot detect
any Arabic rendering regression** — on a platform whose primary users read Arabic.

**Fix:** make an Arabic-capable font available to the capture environment. Readex Pro is
already the platform font and loads from Google Fonts; headless Chrome may be blocked from
fetching it. Options, in order of preference: ensure network font loading works in the
capture; or self-host the font file under `internal/ui/static/` and reference it locally.

**If neither is possible on this machine, stop and report it.** Do not proceed to Gate 2 with
an Arabic-blind baseline and do not delete the suite. Write the limitation into the test file
as a comment so the next person does not trust it further than it goes.

### 1.3 The permission matrix was not written

`CLOSEOUT_PROMPT.md` Gate 2 Task 2 required it. No such test exists — verified by grep.

Write it: every role × every route, as a declared table with the expected status code stated
per row, following the shape of `internal/ui/account_lifecycle_matrix_test.go`.

**Verify it can fail:** remove one gate, confirm red, restore, and paste the failure output.

### 1.4 Accessibility beyond five aria-labels

Five `aria-label`s were added to the theme and language toggles. That is the start of the
task, not the task.

Do: keyboard-only traversal of all three dashboards (every interactive element reachable,
visible focus, no trap outside a modal, Escape closes what it should); screen reader on the
sidebar, top bar and modals; contrast measured in both themes, AA on body and AAA on numbers.

Fix anything that is a one-line attribute change. Report anything structural.

**GATE 1: report and stop.**

---

# GATE 2 — THE UI DEFECTS

Each item below was diagnosed against the source. Verify the diagnosis before fixing it.

### 2.1 The "قيد المراجعة" page — `internal/ui/pages/onboarding_pending.templ`

This is 54 lines and is the clearest instance of the ADR 0017 failure. It reports **0 inline
styles**, so every ratchet calls it converted, and it has almost no styling at all:

```templ
<div>                          <!-- no class -->
  <div class="card hover-lift">
    <a href="/"><img src="/static/img/logo.png" alt="DAWA24" /></a>   <!-- unsized -->
    <div>@components.IconShield("icon-lg")</div>                      <!-- no class -->
    <h1>حسابك قيد المراجعة</h1>                                       <!-- no class -->
    <p>…</p>                                                          <!-- no class -->
    <div>ماذا يحدث بعد ذلك؟</div>                                     <!-- no class -->
    <ul><li>…</li></ul>                                               <!-- no class -->
```

Bare `<div>`, `<h1>`, `<p>`, `<ul>`, `<li>` and an unsized `<img>`. There is no layout, no
type scale, no spacing, no centring.

**Three defects, not one:**

**(a) Styling.** Build a proper centred status page: constrained measure, the logo at a fixed
size, a status icon treatment, a heading on the type scale, body text at a readable measure,
and the "what happens next" list as a real list component. All of it in `.css` under
`@layer components` — a `.status-page` block or similar. Nothing inline.

**(b) It lies to suspended organizations.** `authctx.RequireApproved` redirects suspended orgs
to `/onboarding/pending?state=suspended`. The handler
(`internal/ui/subscription_handlers.go`, `OnboardingPendingPage`) reads only:

```go
rejected := r.URL.Query().Get("rejected") == "1" || r.URL.Query().Get("state") == "rejected"
```

So a **suspended** organization is shown "حسابك قيد المراجعة" — *your account is under
review*. It is not; it was suspended. A `pending` and an `under_review` org also get identical
copy though they are different states.

Replace the `rejected bool` with a state value covering `pending`, `under_review`, `rejected`
and `suspended`, and give each its own heading, explanation and next action. Suspended must
say it was suspended and who to contact.

**(c) It is a dead end.** The page tells the caller to wait and offers no way to act. Tier A
deliberately keeps `/documents` reachable while an organization is unapproved, precisely so it
can supply what is missing. Link to it, for every state where supplying documents helps.

Add a test asserting all four states render distinct copy.

### 2.2 Sidebar section titles — `internal/ui/static/css/layout.css:208`

```css
.sidebar-section-header {
  font-size: 0.70rem;
  font-weight: 800;
  text-transform: uppercase;
  letter-spacing: 0.05em;
}
```

Three problems, and the middle one is the visible defect:

1. `font-size: 0.70rem` is a hardcoded literal. Use the type scale token.
2. **`letter-spacing: 0.05em` on Arabic breaks the script.** Arabic is cursive; letter-spacing
   inserts gaps between letters that are supposed to join, so the words visually fall apart.
   This is the reported "font styling issue".
3. `text-transform: uppercase` is a no-op on Arabic, so the rule was written for Latin and
   never checked in the language the platform actually ships in.

**Fix:** drop `letter-spacing` and `text-transform` for Arabic. If the uppercase treatment is
wanted for the English interface, scope it — `:root[lang="en"] .sidebar-section-header` — and
say so in a comment. Use the token for size. Then check the sidebar renders correctly in both
languages before committing.

**Sweep for the same bug:** grep every `letter-spacing` in the stylesheets and check each one
is not applied to Arabic text. This is a category, not a single rule.

### 2.3 Modals

The system is structurally sound and better than the earlier reports suggested — verify this
before changing anything:

- exactly **one** element carries the class token `modal`: the `<dialog>` in
  `internal/ui/components/modal.templ`
- **57** call sites go through `components.Modal`
- `initModalManager` in `app.js:352` delegates `[data-modal-open]`, `[data-dialog-target]`,
  `[data-open-modal]` and calls `showModal()`

So a modal that "opens after the table instead of as a modal" is **not** a missing handler.
It is a dialog that was given the `open` attribute without entering the top layer. There are
exactly two ways that happens here, and both are real:

**(a) The `showModal()` fallback.** `components/modal.templ` `openDialog`:

```js
try { el.showModal(); } catch (_) { el.setAttribute('open', ''); }
```

`setAttribute('open')` shows the dialog **in normal document flow** — no top layer, no
backdrop, no focus trap. It renders exactly where it sits in the DOM, which on these pages is
after the table. `showModal()` throws `InvalidStateError` when the dialog is already open, so
a double-trigger silently downgrades the modal into an inline block permanently.

**Fix:** never fall back to `setAttribute('open')`. If `showModal()` throws because the dialog
is already open, that is a no-op, not a reason to degrade. Handle it explicitly.

**(b) Ancestor containment.** A `position: fixed` element is trapped by any ancestor with
`transform`, `filter`, `contain` or `will-change`. `dialog.modal` sets `position: fixed`
(`components.css:228`), and these ancestors exist:

```
layout.css:59       .sidebar        contain: layout style; transform: translateZ(0)
components.css:2425 (card)          contain: content; transform: translateZ(0)
customer.css:1688   (card)          contain: content; transform: translateZ(0)
```

A dialog opened via `showModal()` is in the top layer and escapes this. A dialog with only
`open` does not, and is clipped or positioned inside the card. Fixing (a) resolves most of it;
audit any dialog rendered inside one of those containers and move it to the end of the page
body if so.

**Do this per page, not globally.** Open every one of the 57 modals in a browser, confirm it
overlays, is centred, traps focus, closes on Escape and on backdrop click, and restores scroll.
Report the list with pass/fail per modal — that list is the deliverable.

### 2.4 The admin dashboard — `internal/ui/pages/admin_dashboard.templ`

538 lines, 0 inline styles. Full review against `.impeccable.md`:

- every stat, table and control has a real CSS rule (it may be an ADR 0017 casualty like 2.1 —
  check before assuming it is fine);
- the summary reads before the detail; state is encoded in form as well as number;
- 375px behaviour: tables become card lists below 768px, not sideways scrollers;
- both themes, RTL and LTR;
- empty states teach rather than saying "no data";
- every number `tabular-nums`;
- no decorative sparklines, no hero metric block, no coloured side-stripes (`.impeccable.md`
  anti-references).

Report a defect list before fixing, then fix.

### 2.5 The undefined-class sweep, applied properly

235 class names still have no CSS rule. Most are Alpine state names inside `:class`
bindings — `activeTab` (29), `activeCategory`, `filterTab`, `policyFilter`, `siteSubTab` —
which are never selectors.

1. Teach `check-undefined-classes` to skip tokens appearing only inside `:class` /
   `x-bind:class` attributes.
2. Re-measure. **Every name that survives is a real unstyled element** — define it in the
   right `.css` file and `@layer`, using tokens.
3. Lower the ceiling to the survivor count. Report both numbers.

**GATE 2: report and stop.**

---

# GATE 3 — THE WIRING AUDIT

Styling is only half of "not finished". This gate answers: does every control do something?

### 3.1 Every form posts somewhere real

For every `<form>` in `internal/ui/pages/`, confirm the `action` resolves to a registered
route and the handler is not a stub. Produce a table: page, form, action, handler, status —
`WIRED` / `STUB` / `NO ROUTE`.

Fix `NO ROUTE`. For `STUB`, report it; do not invent business logic.

### 3.2 Every link goes somewhere

Every `href` that is not `#` or external must resolve to a registered route. The account menu
already shipped a dead link once (`/vendor/user-organizations` — the real path is singular),
so this is a demonstrated failure mode, not a hypothetical.

Report dead links with the page and the intended destination.

### 3.3 Every button does something

A `<button>` with no `type="submit"`, no `@click`, no `hx-*` and no `data-*` handler is
decoration. List them. Wire the ones with an obvious destination; report the rest.

### 3.4 Every HTMX target exists

For every `hx-target`, `hx-get`, `hx-post`: confirm the target selector exists in the rendered
page and the endpoint is registered. A missing target fails silently, which is why this needs
checking rather than testing by hand.

### 3.5 The orphaned smartorder API

`internal/modules/smartorder/http` is imported by nothing. `RegisterRoutes`, `Handler`,
`Reviewer`, the events endpoint and the review endpoints are unreachable. The smartorder
*service* is wired — `cmd/server/smartorder.go` composes it and the UI drives it — but its
JSON API was never mounted.

**Do not delete it and do not wire it.** Report what it exposes, what mounting would require,
and what removing it would cost. Both are product decisions.

### 3.6 Empty, loading and error states

Every list, table and async region needs all three. Report which surfaces lack them and add
them using the existing components. Per `.impeccable.md`, an empty state teaches the
interface.

**GATE 3: report and stop.**

---

# GATE 4 — STRUCTURE

### 4.1 Reorganise `internal/ui` by audience

**This was skipped in the previous round and reported as done.** 246 flat `.go` files still
sit beside `pages/`, `components/`, `layouts/`, `static/`. The filenames already carry the
grouping: 62 `admin_`, 40 `vendor_`, 25 `customer_`.

```
internal/ui/admin/  vendor/  customer/  shared/  public/  view/
```

`pages/`, `components/`, `layouts/`, `static/` do not move.

**One audience per commit. No behaviour change in a move commit. `go test ./... -count=1` after
every commit, not at the end.**

`UIHandler` methods sit on one receiver across all these files. Keep the single receiver —
splitting the type is out of scope. **If Go's package rules make a clean split impossible,
stop and report the shape that is actually achievable.** A partial reorganisation reported
honestly beats a forced one that fragments the handler type.

### 4.2 `deadcode`

211 findings, nothing deleted, deliberately — `deadcode` does not parse `.templ`, so a symbol
it calls unreachable may be reached from a template. Go through the list; establish reachability
before deleting; anything ambiguous goes in the report, not a delete commit. Lower the ceiling
to what survives.

**GATE 4: report and stop.**

---

# GATE 5 — CLOSEOUT

### 5.1 Shell CSS

167,151 bytes. The old 90 KB target was set against a stylesheet missing 468 classes' worth of
rules, so part of the apparent progress toward it was styling that had gone absent (ADR 0017).

1. **State a target you can defend** against the current, complete stylesheet, with reasoning.
2. Meet it. The weight is `components.css`, `foundations.css` and `utilities.css`, which
   overlap heavily; `@layer` now makes the duplicates safely deletable.
3. Move single-surface rules into `admin.css` / `vendor.css` / `customer.css` / `public.css`.
4. **Run the visual regression suite after every deletion batch.** That is what Gate 1 was for.

### 5.2 Real devices — the counter test

Mid-range Android and iOS Safari, RTL and LTR, at 375px.

Complete a full order **one-handed**: find a product, compare suppliers, add to cart, check
out. Then vendor-side: receive the order, mark it shipped.

**Report where it is awkward, not only where it breaks.** This is the acceptance test for the
whole rebuild. If no device is available, say so — do not simulate it and call it done.

### 5.3 Final residual report

Every gate, current value against ceiling, raw output. Then the residual list: **every target
not reached, its number, and why.** That list is the handover document and is worth more than
a clean summary that hides gaps.

**GATE 5: all green, or every gap named with a number.**

---

## REPORTING — five times

Each report:

1. Per task: `DONE` / `PARTIAL` / `STOPPED`, files touched, what changed and why.
2. Raw pasted output for every gate command.
3. Every stated gate marked **MET** or **MISSED**, with its number.
4. Discrepancies, including anything you did not do and anything where my diagnosis above did
   not match what you found. **If I am wrong about a cause, say so** — the diagnoses in Gate 2
   were made by reading source, not by running the app.
5. `git diff --stat` against the parent branch.
6. `git branch --show-current` and `git log --oneline -1` after committing — another session
   has moved HEAD in this repository between tool calls before.

Gate-specific: Gate 1 the eight baseline hashes · Gate 2 the 57-modal pass/fail list and the
admin dashboard defect list · Gate 3 the four wiring tables · Gate 4 any file that could not
move · Gate 5 the residual list.

**Stop at every gate.**
