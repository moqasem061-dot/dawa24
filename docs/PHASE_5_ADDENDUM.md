# PHASE 5 — ADDENDUM (paste alongside `docs/REMAINING_WORK_PROMPT.md`)

Phase 4 is accepted. Start Phase 5 as written in `REMAINING_WORK_PROMPT.md` §"PHASE 5",
with the three amendments below folded in.

**Start from:** `phase-4-modals` at `f4967e5`. Branch `phase-5-conversion` from it.

---

## A. What Phase 4 achieved, and the one thing it did not

Verified against the branch ref (not the working tree):

| Target | Result |
|---|---|
| `modal-overlay` occurrences | **0** (was 38) |
| `window.openModal` / `openModal(` | **0** (was 26) |
| `class="top-navbar"` in templates | **0** |
| `!important` | **3** — held |
| inline styles | **3374** (was 3616) |
| sidebar `id="app-sidebar"`, inline script moved out | done |
| `go build`, `go vet`, `go test ./...` | pass |

The accessibility win is real: every dialog is now a native `<dialog>`, so top layer, focus
trap, Escape and backdrop work everywhere. That was the biggest single correctness gain
available and it landed.

**What did not happen: consolidation.** `components.Modal` is still used 16 times — unchanged
— while pages now contain **41 hand-written `<dialog>` blocks**. Each one restates the
component's own markup:

```templ
<dialog id="brand-create-modal" class="modal" aria-labelledby="brand-create-title">
    <div class="modal-box modal-md">
        <div class="modal-header">
        ...
```

That is exactly what `components.Modal(ModalProps{...})` renders. So 38 hand-written overlays
became 41 hand-written dialogs. The ratchet passes because it counts the old class name, but
the duplication the task existed to remove is still there — and Task 4's instruction to
"specify once in `modal.templ`, then hold everywhere" cannot hold while 41 pages each own a
copy of the markup.

**This is not a redo.** Phase 5 already opens every page and its checklist already says
"ad-hoc markup replaced with components." The dialog consolidation folds into that work at
almost no extra cost. Do not do it as a separate pass.

---

## B. Amendments to the Phase 5 per-page checklist

Add three items. A page is not done until these are also true:

**11. Every `<dialog>` on the page goes through `components.Modal`.** Replace hand-written
`<dialog class="modal"><div class="modal-box">…` with `@components.Modal(components.ModalProps{
ID: …, Title: …, Size: …})` and the body as children. If a dialog needs something `ModalProps`
cannot express, **extend `ModalProps` once** and use it everywhere — do not leave that one
hand-written.

**12. Destructive actions inside a modal require typed confirmation.** Task 4 specified this
and it was not implemented — `modal.templ` has `ConfirmModalProps` with a `Danger` flag but no
typed-confirmation path. Add it to `ConfirmModalProps` once (the operator types the record's
name or a fixed word before the confirm button enables), then use it for every delete,
suspend, reject and refund. This platform moves medicine and money; a red button is not
enough.

**13. The `tr(lang, ar, en)` helper is gone from this page's path.** It lives in
`internal/ui/components/dashboard_topbar.templ` and was a deliberate stopgap so two shell
strings did not invent a third i18n convention. Fold its strings into the catalogue during
Wave 1 and delete the function.

---

## C. Two new ratchets, installed in Wave 1

```
check-modal-handwritten   raw `<dialog` in internal/ui/pages/*.templ  →  target 0
check-modal-legacy        already installed in Phase 4                →  keep at 0
```

Set `check-modal-handwritten` to the measured count at the start of Wave 1 (expected 41) and
lower it with every wave, exactly like `check-inline-styles`. **Do not set it to 0 before the
count is 0** — Phase 3 shipped a failing build that way.

---

## D. Reporting

As specified in `REMAINING_WORK_PROMPT.md` §"REPORTING", plus per wave:

- raw `<dialog` count in pages, before and after
- shell CSS bytes (still ~150 KB against the 90 KB target; see §"Shell CSS" in the main prompt)
- `check-modal-handwritten` ceiling, and proof it matches the measured value

**One process note.** Three commits on `phase-4-modals` are named `afasf`, `gdfgdf` and `gs`.
Two of them contain real work — `264b7f2 "gdfgdf"` is the entire sidebar task. A commit message
is how the next person finds out why something changed; `gdfgdf` costs nothing today and costs
an hour in six months. Name them for what they do, as `f4967e5` already does.
