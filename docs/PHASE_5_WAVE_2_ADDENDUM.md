# PHASE 5 WAVE 2 — ADDENDUM

Use with `docs/REMAINING_WORK_PROMPT.md` §"PHASE 5" and `docs/PHASE_5_ADDENDUM.md`.

**Start from:** `phase-5-conversion` at `2fb3620`. Continue on that branch.

---

## A. Wave 1 review

What landed, verified against the branch ref:

| Target | Before | After |
|---|---|---|
| inline styles | 3374 | **3320** |
| raw `<dialog>` in pages | 41 | **37** |
| `components.Modal` uses | 16 | **19** |
| `tr(lang, ar, en)` helper | present | **removed** |
| typed confirmation in `ConfirmModalProps` | absent | **added** |
| `!important` | 3 | **3** — held |
| `go build` / `go vet` / `go test ./...` | pass | **pass** |

Two ceilings were set correctly this time — `check-inline-styles` at 3320 and
`check-modal-handwritten` at 37 both match the measured values exactly. That is the first
wave where the ratchets were honest on the first attempt. Keep doing that.

### Correction 1 — `make check` fails

```
check-hardcoded-arabic:  measured 1270,  ceiling 1246   ->  FAIL
```

The ceiling has said 1246 since Phase 2 while the real count drifted to 1270. Wave 1 was
required to lower it to the measured value and did not touch it, so the gate has been failing
and the wave was reported complete anyway.

**Fix first, before any Wave 2 work:** measure, set the ceiling to the measured number, and
confirm the gate passes. Then Wave 2's job is to bring the number *down* from there.

### Correction 2 — the English requirement got no work at all

**Arabic literals in Go: 1270 before Wave 1, 1270 after.** `internal/shared/i18n/catalog.go`
changed by 7 lines and `catalog_nav.go` by 1 — those are the two strings from the deleted `tr`
helper and nothing else.

Checklist item 8 — "Arabic literals moved to `i18n`" — was not applied to any of the four
converted pages. This is the item the product owner explicitly confirmed as a real
requirement, not aspirational: the language toggle is in every header and promises an English
mode the platform cannot deliver.

`admin_settings.templ`, `customer_catalog.templ`, `customer_order_detail.templ` and
`settings_unified.templ` are now *styled* correctly but still hardcode Arabic. **Go back and
finish item 8 on those four before starting Wave 2's pages.** A page that has to be reopened
later costs more than one converted properly the first time — that is the whole reason the
checklist is per-page rather than a separate sweep.

### Correction 3 — the pharmacy dashboard was skipped

Wave 1 is "the three dashboards, customer catalogue, cart, checkout, order detail, settings."

- `admin_dashboard`, `vendor_dashboard` — 0 inline styles, converted back in Phase 3. Correctly
  left alone.
- `customer_cart`, `customer_checkout` — 0 inline styles. Verify the other nine checklist items
  hold, then they are done.
- **`pharmacy_dashboard` — 69 inline styles, untouched.** It is one of the three dashboards and
  it is the single most important screen in the platform: it is what a pharmacist opens at the
  counter, on a mid-range Android, one-handed. Convert it.

---

## B. Wave 2 scope

Finish Wave 1's three corrections first, then proceed to Wave 2 as specified: approvals,
organisations, finance, catalog inventory, users, roles, cities, developers.

`admin_catalog_inventory.templ` (2,736 lines) and `admin_organizations.templ` (1,460) must be
**decomposed into per-tab and per-section components as part of conversion**, not converted in
place. Converting a 2,700-line template in place just moves the problem.

---

## C. Ratchets to lower at the end of Wave 2

Measure each, then set the ceiling to the measured value. Never below it.

```
check-inline-styles        currently 3320
check-modal-handwritten    currently 37
check-hardcoded-arabic     fix to measured now, then lower
check-file-size-count      currently 101
```

Modal consolidation moved 41 → 37 in Wave 1, which is four pages' worth. At that rate it does
not reach 0 by Wave 4. Wave 2 touches eight admin screens that carry most of the remaining
dialogs — expect a much larger drop, and report the number either way.

---

## D. Reporting

Per `REMAINING_WORK_PROMPT.md` §"REPORTING", plus:

1. **Raw pasted output of the arabic gate**, before and after the ceiling correction.
2. **Arabic literal count per converted page**, before and after. A page reported DONE with
   its literal count unchanged will be sent back.
3. Raw `<dialog>` count, shell CSS bytes, and every ceiling against its measured value.
4. Explicit MET / MISSED for each of the ten checklist items on each page — not a blanket
   "checklist applied."

The last point exists because Wave 1 reported the checklist applied while item 8 was untouched
on every page. Per-item is harder to report and much harder to get wrong.
