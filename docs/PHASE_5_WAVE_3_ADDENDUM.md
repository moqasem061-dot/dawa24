# PHASE 5 WAVE 3 — ADDENDUM

Use with `docs/REMAINING_WORK_PROMPT.md` §"PHASE 5".
**Start from:** `phase-5-conversion` at `daca55c`. Continue on that branch.

---

## A. Wave 2 review — accepted, no corrections

| Target | Before | After | Ceiling | Status |
|---|---|---|---|---|
| inline styles | 3320 | **2657** | 2657 | matches |
| Arabic literals in Go | 1270 | **1223** | 1223 | matches — gate now passes |
| raw `<dialog>` in pages | 37 | **31** | 31 | matches |
| `components.Modal` uses | 19 | **25** | — | |
| `!important` | 3 | **3** | 3 | held |
| templates > 1000 lines | 15 | **14** | — | |
| build / vet / tests | pass | **pass** | | |

**Every ceiling matches its measured value.** That is two waves running, after five phases
where it did not. The `check-hardcoded-arabic` gate that had been failing since Phase 2 now
passes honestly rather than by being ignored.

All three Wave 1 corrections landed: `pharmacy_dashboard` went 69 inline styles → 0, the
Arabic extraction started, and the ceiling was fixed.

All eight Wave 2 pages are at **0 inline styles**.

**The decomposition was done properly and deserves saying so.** The 2,736-line
`admin_catalog_inventory.templ` defined eight page templates. It is now five files organised
by subject, and all eight templates survive:

```
AdminProductDetailPage, AdminProductChildrenPage, AdminApisProductsPage -> admin_product_detail.templ
AdminStocksPage                                                          -> admin_stocks.templ
AdminWarehousesPage, AdminWarehouseDetailPage                            -> admin_warehouses.templ
AdminTempWarehousesPage                                                  -> admin_temp_warehouses.templ
AdminSavingProductsPage                                                  -> admin_saving_products.templ
```

Nothing was dropped in the split. That is the hard part of decomposition and it was done right.

---

## B. A flaw in the plan, and the fix

**This one is mine, not yours.** The Phase 5 instructions said the English work "is per-page
work inside each wave, not a separate sweep — converting a page means converting its strings."

That is wrong for most of what is left. Of the 1,223 remaining Arabic literals:

```
internal/modules   803   (66%)
internal/ui        381   (110 of those in pages/)
cmd                 39
```

**Two thirds live in domain and service code that page waves never open** —
`internal/modules/catalog/import_rows.go` (55), `internal/modules/ingest/domain.go` (46),
`internal/modules/smartorder/pipeline/query.go` (45), `internal/modules/compare/columns_data.go`
(41), and `internal/ui/visitor.go` (63). Waves 3 and 4 cover vendor pages and the public long
tail. You could convert every remaining page perfectly and still finish Phase 5 with ~950
literals and a broken English mode.

The per-page checklist was right for page strings and cannot reach the rest. So the rest needs
its own workstream.

### Wave 3 gets a second track

Run these two in parallel and report them separately:

**Track 1 — pages, as specified.** Wave 3 scope is unchanged: vendor offers, inventory, ingest,
warehouses, team, transfers, coverage, finance. Same ten-item checklist.

**Track 2 — module and service strings.** Work outward from the largest concentrations:

| File | Literals |
|---|---|
| `internal/ui/visitor.go` | 63 |
| `internal/modules/catalog/import_rows.go` | 55 |
| `internal/modules/ingest/domain.go` | 46 |
| `internal/modules/smartorder/pipeline/query.go` | 45 |
| `internal/modules/compare/columns_data.go` | 41 |
| `internal/modules/compare/comparison.go` | 36 |
| `internal/ui/vendor_ingest_sample_handlers.go` | 35 |

**A service-layer string needs care a page string does not.** Many of these are error messages,
validation text and column headers that cross a module boundary. Before moving one, check
whether it reaches a user at all — some are log lines and internal identifiers that must not be
translated, and turning those into i18n keys makes the catalogue worse, not better. **Report
anything you decide is not user-facing and leave it alone.**

Where a module cannot import `i18n` without breaking the `depguard` boundaries in
`.golangci.yml` (`shared/` is a leaf; `platform/` must not import `modules/`), **stop and
report it** rather than weakening the boundary. That is an architectural question, not a
translation one.

Target for Wave 3 Track 2: **at least 250 literals moved.** At Wave 2's rate of 47 per wave the
remaining two waves finish at ~1,130, which is not a working English mode.

---

## C. Modals — on track, no change

The 31 remaining raw dialogs are all reachable by the remaining waves:

```
Wave 3 (vendor):  vendor_jobs 4, vendor_saving 3, vendor_products 3, vendor_coverage 1
Wave 4 (rest):    promo_revenue 3, customer_saving 3, admin_brands 3, suppliers 2,
                  customer_branches 2, admin_products 2, tenant_subscription 1,
                  compare_mapping 1, admin_plans 1, admin_institutional 1, admin_audit 1
```

Keep converting them as the pages come up. Ceiling to the measured value each wave.

---

## D. Also due in Wave 3

**Shell CSS.** Still ~150 KB against the 90 KB target, missed four times now. As vendor pages
convert, move the component rules only the vendor surface uses out of `components.css` into
`vendor.css`. **Report shell bytes with the wave** — if the number does not move again, say so
plainly and we will decide whether the target is realistic rather than carrying it silently to
Phase 7.

---

## E. Reporting

Per `REMAINING_WORK_PROMPT.md` §"REPORTING", plus:

1. **Track 1 and Track 2 reported separately**, each with its own before/after counts.
2. **Per-item MET/MISSED for the ten checklist items on each page** — continue what Wave 2 did.
3. **Track 2: every string you judged not user-facing**, with the file and the reason.
4. **Any `depguard` boundary that blocked an i18n move**, named, not worked around.
5. Raw pasted output for every ceiling against its measured value.
6. Shell CSS bytes.

One process note, repeated: three commits in this wave are named `FGASF`, `KHL;` and `FASFA`.
`daca55c` is named properly and reads well. Do that one every time — the diff is recoverable,
the reasoning is not.
