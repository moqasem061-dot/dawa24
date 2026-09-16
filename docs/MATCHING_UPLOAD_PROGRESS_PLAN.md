# Matching, Upload and Progress hardening — plan

Status: EXECUTING. Written 2026-09-05 from a survey of the live code plus the
labelled accuracy corpus in `test/corpus`.

Every item below is a defect that was *reproduced*, not a suspicion. The
reproduction is named beside it.

---

## A. Product matching

The engine (`internal/shared/productmatch`) is good. The wrong matches it does
produce cluster into five causes, and four of them are arithmetic bugs rather
than tuning.

### A1 — A ratio loses every figure but the first  ← the user's "60/100 vs 100/60"

`strengthPattern` is `\d+(?:[./]\d+)?\s*<unit>`, so it captures `10/20mg`
**whole**. `parseStrength` then cuts the number at the first `/` and keeps the
head. `ratioLeads` only recovers a leading figure that sits *outside* the match,
so it recovers nothing here.

Reproduced (`strengthSet`):

| text | parsed | should be |
|---|---|---|
| `alkor plus 10/20mg 14 tab` | `[10 mg]` | `[10, 20] mg` |
| `الكور بلس 10/40 مجم` | `[10 mg]` | `[10, 40] mg` |
| `الكور بلس 20/10 مجم` | `[20 mg]` | `[20, 10] mg` |
| `amlosazide 5/12.5/40 mg` | `[5 mg]` | `[5, 12.5, 40] mg` |

Consequence: the row `10/20` **agrees** with the wrong product `10/40` and
**contradicts** the right one `20/10`. That is exactly four of the twenty-five
wrong applied matches in `accuracy.json`, and the failure the user described.

`parts` is also left at 1, so `isCombination` is false and the strict
combination-vs-combination equality rule never fires.

**Fix:** expand every component of a captured ratio into the dose set, scaled by
the unit that governs it; set `parts` to the true component count. Order is
irrelevant because comparison is set-based — which is what makes `10/20` and
`20/10` the same product and `10/40` a different one.

### A2 — The Latin→Arabic skeleton mis-folds `c` and `ch`

`translit.go` maps `c → k` always and the digraph `ch → s` always. Both are
wrong in pharmaceutical vocabulary.

Reproduced (`Skeleton`):

| word | skeleton | collides with |
|---|---|---|
| `chromax` | `srmks` | `سيرومكس` (`srmks`) — right answer `كروماكس` is `krmks` |
| `cisplatin` | `ksbltn` | `اوكسابلاتين` (`ksbltn`) — right answer `سيسبلاتين` is `sbltn` |

**Fix:** soft `c` before `e/i/y` folds to `s`; `ch` folds to `k` before a
consonant and in the Greek stems that dominate this catalogue (`chrom`, `chlor`,
`chol`, `chem`), `s` otherwise. Both directions are covered by new pinned tests.

### A3 — A skeleton-only match may be applied silently  ← "many letters do not match"

`nameEvidenceOf` lets the cross-script skeleton set the similarity at
`v * 0.86`. With `weight = 0.75` that is `0.645`, comfortably over
`DefaultMinStrong = 0.50`, so it is **applied without asking** even when not one
distinctive word agrees. Ten of the twenty-five recorded wrong applied matches
carry exactly `تشابه الاسم 86%` and nothing else.

**Fix:** a candidate whose only evidence is a wordless channel (`distinctHits ==
0`) is capped below `MinStrong` unless a real attribute corroborates it. It is
still offered, still scored, still one click from acceptance — it is not decided
for the user. This is the change the user asked for in as many words.

### A4 — `20*10` is read as two counts, not a product

`اليرجيل 4 mg 20*10 tabs` yields `counts=[{tablet 10} {tablet 20}]`; the
catalogue says `200 قرص`. **Fix:** recognise `A*B`, `A×B`, `A x B` as a pack
multiplication and record `A*B` as the count (keeping the factors as residual).

### A5 — `50.000 i.u.` parses to nothing

`a-viton 50.000 i.u. 20 caps` → `strengthSet` returns `[]`: `i.u.` is not in
`doseUnits` (only `iu`), and `50.000` is a European thousands separator.
**Fix:** fold `i.u.`/`i.e.`-style dotted unit spellings before parsing, and read
`N.NNN` as thousands when the group is exactly three digits and a unit follows.

### A6 — The three normalisers do not agree, though all three say they must

`sheet.NormalizeName` folds `ئ ؤ ڤ چ گ پ`. `arabic.Normalize` (Go) and
`platform.normalize_arabic` (SQL) do not. All three carry a comment saying the
other two must match them. `ى → ي` *is* handled everywhere, so the user's `بولي`
report is not a folding gap — but this divergence is a real one and produces
exactly its symptom class (a row that matches in the engine and misses in SQL).

**Fix:** one fold table, mirrored in a new migration, plus a parity test that
fails when Go and SQL disagree on a corpus of real names.

### A7 — `compare.MatchLadder` still has two tiers the engine deliberately refuses

`internal/modules/compare/matching.go`:

- the barcode tier compares the row's **barcode against the candidate's SKU**
  (`c.SKU == cleanBarcode`) — `CandidateProduct` has no barcode field at all;
- SKU and barcode both settle at confidence 100 with no name corroboration,
  which is precisely what `MatchOptions.TrustSupplierCode` /
  `CodeIsAuthoritative` exist to prevent everywhere else.

**Fix:** delete the ad-hoc tiers and let `productmatch` decide, with the
identifier options set from what the user actually mapped — the same policy the
other five tools use.

### A8 — Housekeeping

Dead code sweep across `productmatch`, and a full-file read for anything the
deadcode ratchet already flags.

### A9 — Evidence

`TestMatchAccuracy` must show **fewer wrong applied matches** on both label sets.
No change lands without it. `TestFalseConflicts` checks the other direction.

---

## B. Upload latency

### B1 — Multipart uploads are buffered in RAM

`r.ParseMultipartForm(n)` keeps `n` bytes **in memory** per request:

| handler | limit |
|---|---|
| `admin_org_import_handlers.go:190` | 500 MB |
| `admin_temp_warehouse_upload.go:233` | 500 MB |
| `compare_upload_handlers.go:108` | 128 MB |
| eight more | 32 MB |

On a small VPS two concurrent large uploads are enough to push the process into
swap and GC thrash, which is why the slowness is *not* limited to the uploader —
it is the whole site. **Fix:** cap the in-memory part at a few MB everywhere and
let Go spill to disk, which is what the API is designed to do.

### B2 — Files are then read whole into `[]byte` again

`compare_upload_handlers.go` does `io.ReadAll` per file and holds every file of
the batch in memory at once, then runs six parallel workers over them.
**Fix:** stream to a temp file, hand the workers a path.

### B3 — Parse and match run inside the HTTP request

`/compare/upload`, `/vendor/ingest/`, `/admin/products/import/` are exempted
from the 25s request deadline precisely so they can do this. The user waits for
the whole thing with no feedback, and a dropped connection loses the work.

The platform already has the right machinery — `platform/importrun` (durable
runs), `platform/importjobs` (River stage/commit workers), `platform/queue`.
**Fix:** the request stores the file, creates a run, enqueues, and returns the
run id. Everything else moves to the worker.

### B4 — Row writes

Audit for row-at-a-time inserts on the staging path and move them to `COPY`.

### B5 — Pool safety

Bound worker concurrency against `DB_MAX_CONNS` (default 20) so one batch cannot
starve the site.

---

## C. Real-time progress and UX

### C1 — One progress transport

Two SSE endpoints already exist (smart order, ingest wizard) and both **poll the
database once per second per open connection**. That does not scale on the VPS.

**Fix:** `internal/platform/progress` — an in-process hub keyed by run id.
Writers publish; subscribers are fanned out to. One DB read when a subscriber
arrives, a 15s heartbeat, and no per-connection polling. SSE endpoint at
`/imports/{id}/stream`, with the existing JSON poll kept as the fallback for
proxies that break streaming.

### C2 — One progress UI

`import-progress.js` already does the honest-bar arithmetic (never rewinds,
never reaches 100 early, drifts while the server is quiet). Extend it to consume
SSE and fall back to polling, and give it one visual treatment used identically
on a page and inside a modal. Wire: Smart Order, Vendor Import, Admin Catalogue
Import, Savings Products (vendor **and** pharmacy), Compare bulk upload.

### C3 — Compare drag-and-drop progress modal

Opens on drop, shows per-file upload then per-file processing, then the files
appear in the supplier list on the right.

### C4 — Over-limit batches take what fits instead of refusing

`CompareUploadSubmit` currently rejects the whole batch when
`activeCount + len(files) > max`. **Fix:** accept the first `remaining` files,
add them, and tell the user plainly which ones were not taken and why.

### C5 — The review table's own scrollbar

`.review-table-scroll { overflow: auto; max-block-size: min(70vh, 46rem) }`
gives the table a vertical scrollbar even though the page is paginated, and
`.review-table { min-inline-size: 1420px }` forces a horizontal one on any
laptop. **Fix:** drop the vertical scroller, let the page scroll; bring the
required width down so the common case has no horizontal bar either, keeping
`overflow-x` only as the small-screen fallback.

### C6 — The ربط dropdown

`.catalog-dropdown-menu` is `position: absolute` inside a `<td>`, inside a
container with `overflow: auto` — so it is **clipped**, and it nests three
scroll regions (`22rem` menu, `190px` suggestions, `12rem` results) inside
`28rem` of width. **Fix:** promote it to a real centred dialog with one scroll
region and room to show a name, a strength and a form on one line.

### C7 — Queue review

Confirm River timeouts, retry policy and `RecoverStaleRuns` cover every new
job kind, so nothing can wedge a run in `processing` forever.

---

## Execution record — 2026-09-05

Measured against `test/corpus`, which is the only thing in the repo that says
whether a match is CORRECT.

| label set | wrong applied | right applied | precision | recall |
|---|---|---|---|---|
| cross-script (19,996 labels) | 117 → **95** | 16,340 → **16,538** | 99.29% → **99.43%** | 81.72% → **82.71%** |
| siblings (12,183 labels) | 8 → **5** | 11,541 → **11,726** | 99.93% → **99.96%** | 94.73% → **96.25%** |

Precision AND recall improved on both sets: the engine now applies fewer wrong
matches and more right ones. Baseline re-recorded in `test/corpus/accuracy.json`.

### Done

- **A1** ratio expansion (`strength_set.go`, new file; `strengthPattern` numeric
  head repeats). `10/20mg` now reads as `[10, 20]`, order-insensitively.
- **A1b** two units found missing while fixing it: `ملجم` was being read as
  MILLILITRES, and `لتر` was unreadable. Both were in `doseUnits` and absent from
  `strengthPattern`.
- **A2** `latinFold` replaces the context-free digraph table: soft `c` before
  e/i/y, and `ch` as /k/ before a consonant. Breaks the `chromax`/`سيرومكس`,
  `cisplatin`/`اوكسابلاتين` and `cefidime`/`كيفاديم` collisions.
- **A3** `scoredProduct.settleable`: a candidate whose only evidence is a
  wordless channel is offered, never applied — unless the dose agrees or the
  letters are all but identical.
- **A4** `foldPackMultipliers`: `20*10 tabs` is two hundred tablets.
- **A5** `FoldDoseText`: `50.000 i.u.` is fifty thousand international units.
- **A6** `NormalizeText` is now `sheet.NormalizeName`, so the identity key and
  the scorer's tokens cannot disagree; Persian letter forms fold instead of
  being dropped.
- **A7** compare's ad-hoc code/barcode tiers deleted — the barcode tier was
  comparing the row's barcode against the candidate's SKU — and replaced with
  `WithIdentifiers`. `CandidateProduct` now carries a barcode, joined in SQL.
- **A8** ten dead functions removed (`deadcode` 331 → 321).
- **B1** every `ParseMultipartForm` now caps memory at 4 MB instead of passing
  the allowed FILE SIZE (up to 500 MB) as a heap budget. Spreadsheet imports
  additionally bound the request body at 200 MB, which they never did.
- **B3-partial** `httpx` now extends the SOCKET deadlines for uploads and
  long-running routes. This was the root cause of the upload complaint:
  `ReadTimeout` (15s) bounds reading the whole request body, so any upload
  slower than fifteen seconds was cut off mid-transfer. Covered by
  `TestSlowUploadOutlivesTheServerReadTimeout`, which fails without the fix.
- **C1** `internal/platform/progress`: hub + Redis bridge + SSE, wired through
  an `importrun.Repository` decorator so every progress write announces itself.
- **C2-partial** `import-progress.js` gains `follow`/`watch` (SSE with poll
  fallback); both saving-products flows converted from a 500 ms poll to it.
- **C3** compare bulk upload has a real, byte-measured progress modal
  (`components.UploadProgressModal` + `upload-progress.js`), and per-file rows.
- **C4** an over-quota batch takes what fits and names what it skipped, on both
  the server and the client, instead of refusing everything.
- **C5** review table: no vertical scroller (the page is paginated), columns
  re-measured 1420px → 1120px so the horizontal one rarely appears.
- **C6** the ربط dropdown is a real centred dialog, portalled to `<body>` so the
  table's scroll container cannot clip it, with one scroll region instead of
  three nested ones and a header with a close button.

### Not done

- **B3-full** parse-and-match still runs inside the request for compare, vendor
  ingest and admin catalogue import. The durable-run machinery they would move
  to (`importrun` + `importjobs` + River) is wired and in use by the saving
  imports; moving the other three is a per-tool change that wants browser
  verification.
- **B4** row-write batching (`COPY`) on the staging path: not audited.
- **C2-remainder** smart order, vendor ingest and admin catalogue import still
  drive their bars from their own JSON polls at 0.5–1.5s. They work; converting
  them needs a stream endpoint per tool, because their progress is not written
  through `importrun` and so never reaches the hub.
- **C7** River timeout/retry review: not done.
- The SQL `platform.normalize_arabic` still folds fewer letters than
  `sheet.NormalizeName` (`ئ ؤ ڤ چ گ پ`). Aligning it means a new migration and a
  REINDEX of four trigram GIN indexes; recorded here rather than done quietly.

### Gates

`go build`, `go vet` and every package test pass. Pre-existing failures
unchanged and not caused by this work — verified by measuring HEAD in a
detached worktree: `TestDeadcodeRatchet` (331 at HEAD, now 321, ceiling 303),
`TestTenantGatesUseTenantScopedPermissions`, `TestEverySidebarPermissionGatesARoute`,
`check-undefined-classes` (78 at HEAD, 78 now), `check-inline-styles` (17/17),
`check-important` (69/69), `check-modal-*`, `check-transition-all` (7/7).

---

## Second pass — 2026-09-05, after the correction

> "the Compare tool compares files with each other, not with the catalog"

That reframed the first pass. A7 had fixed Compare's *catalogue* ladder, which
is a secondary path; the tool's primary job — grouping supplier file against
supplier file — was answered by a **second matcher living inside
internal/modules/compare** that nothing in the first pass touched.

### The Compare tool's real matcher

`getCoreDrugMatchKey` had its own normaliser, its own noise-word list, its own
strength regex, and a hand-written table of about **sixty** Arabic brand names
mapped to their Latin spellings — against a market of twenty thousand products.
Reproduced failures, all in the tool's core function:

| supplier A | supplier B | was | should be |
|---|---|---|---|
| `الكور بلس 10/20 مجم` | `الكور بلس 20/10 مجم` | two rows | one |
| `سيفيديم 500 مجم فيال` | `cefidime 500mg vial` | two rows | one |
| `زيرتك 10 مجم 20 قرص` | `zyrtec 10mg 20 tabs` | two rows | one |
| `ابيكوبريد 40 مجم` | `ابيكوبرايد 40 مجم` | two rows | one |
| `اتاكاند 16` | `اتاكاند 32` | **ONE row** | two |

The last is the serious one: bare figures of three digits or fewer were dropped
as noise, so two strengths of one blood-pressure medicine were shown as a single
comparison line whose "best price" was the cheaper *drug*.

**Fixed** by `productmatch.ProductKey` — the same consonant skeleton, modifier
vocabulary, identity letters and ratio-aware dose reader the catalogue matcher
uses. `getCoreDrugMatchKey` is now a one-line call to it, so the grouping
contract (a string compared for equality) is unchanged. Pinned by
`product_key_test.go`.

**Also fixed:** `bySKU` merged rows across files on a bare supplier item code
with no name check at all — two suppliers' internal "951" became one product.
It now refuses a code hit whose two rows are demonstrably different products,
and still accepts one when either name is unreadable.

**Also fixed:** the market benchmark grouped offers by EXACT normalised name, so
"how many suppliers carry this" and "what does the market charge" were computed
over a fragment of the market. It uses the same key now.

### River queue review (C7)

- **`smartorder` was not a configured queue.** `queue/jobs.go` inserts
  `SmartOrderRunArgs` into it and `cmd/worker` registers a worker for it, but
  `config.Worker.Queues` listed only imports/ai/notifications/projections/
  maintenance. River polls *only* configured queues, so such a job is written to
  `river_job`, left `available`, and never claimed — silently. The web process
  runs smart orders inline, which is the only reason it had not surfaced.
  Fixed, and guarded by `TestEveryInsertedQueueIsConfigured`, which fails when
  the two lists drift.
- **`statement_timeout` (30s) capped background work.** It is a RuntimeParam on
  every pooled connection and `cmd/worker` opens its pool from the same config,
  so River's 30-*minute* `JobTimeout` was meaningless: one statement inside a
  bulk import was cancelled by Postgres. The worker now has
  `DB_WORKER_STATEMENT_TIMEOUT` (10m), and `db.InLongTx` gives the same relief
  per-transaction in the web process, where the stage/commit workers also run.
  Applied to the bulk staging write.
- Retry policy and stale-run recovery were already correct: `MaxAttempts` 1 for
  stage / 3 for commit, and a 15-minute `RecoverStaleRuns` ticker.

### Progress — the remaining three tools (C2)

All six now stream, each through a decorator on its own write path so no future
progress write can forget to publish:

| tool | was | now |
|---|---|---|
| Savings (pharmacy + vendor) | 500 ms poll | stream |
| Compare bulk upload | nothing at all | measured upload + processing modal |
| Vendor import | 1.5 s poll | stream (`ingest.WithProgressNotifications`) |
| Admin catalogue import | 1.5 s poll | stream (`catalog.WithProgressNotifications`) |
| Smart order | 500 ms poll | stream (`smartorder.WithProgressNotifications`) |

Every one keeps its JSON poll, and the shared bar falls back to it on its own.

### Still not done

- **B3-full**: parse-and-match still runs inside the request for compare, vendor
  ingest and admin catalogue import. The machinery to move them (`importrun` +
  `importjobs` + River) is wired and in use by the saving imports; each tool is
  its own migration and wants browser verification.
- **B4**: `COPY` batching on the staging path — not audited.
- SQL `platform.normalize_arabic` still folds fewer letters than
  `sheet.NormalizeName`; aligning it needs a migration and a GIN REINDEX.

---

## Third pass — the bar that needed a refresh, and Compare off the request thread

### The reported bug: "I need to refresh to see the progress or result"

Reproduced and fixed. `Stream` discards any snapshot older than the one it is
showing — correct for events reordered across a process boundary. But a `Fetch`
has no timestamp to give (a vendor import session carries a phase and a
percentage, nothing that says *when*), so every safety read returned a **zero**
`At`, and a zero time is before everything.

The effect: the moment ONE snapshot had been published, every subsequent safety
read looked stale and was dropped. The single mechanism that recovers a stream
whose publisher has gone quiet was switched off by the first thing the publisher
said — so the bar stopped where the last event left it and the page had to be
reloaded.

`readNow` now stamps a zero `At` with the time of the read.
`TestStreamRecoversWhenThePublisherGoesQuiet` covers it and **fails without the
fix** (verified by reverting).

Two more found while chasing it, both mine, both silent:

- **The Redis handle was resolved at wiring time**, before Redis is dialled, so
  `Publisher` and `Bridge` captured `nil` for the life of the process. Every
  cross-process progress message was dropped, in a deployment that looked
  healthy because the local hub still worked. Both now take a `RedisSource`
  resolver and re-resolve per attempt.
- **The admin catalogue bar would have moved five times per run.** Its session
  row is written once per phase by design; the fine-grained ticks live in an
  in-memory reporter. `catalog.Service.SetProgressNotifier` now publishes every
  tick while the row keeps its five writes.

Also verified: SSE survives the real middleware chain (`RequestID → Recover →
Logger → SecurityHeaders → Locale → RequestTimeout → Compress`) with the exact
headers `EventSource` sends — `TestStreamSurvivesTheServerMiddlewareChain`
asserts the content type, that nothing compressed the stream, and that a
published event arrives.

### Compare moved off the request thread

Vendor ingest (`StageInBackground`) and the admin catalogue import
(`import_prepare.go`) were **already** detached — the survey in the second pass
was wrong about that. Compare was the only one left, and it was the worst case:
`UploadAndProcessCompareFile` parsed the whole workbook and wrote every row of
it inside the POST, for up to ten files, six at a time.

- `Service.RegisterAndStage` records the file and returns; a goroutine with
  `context.WithoutCancel` does the parse, so closing the tab no longer abandons
  a half-staged batch.
- `StageUploadedRows` is the parse, split out of the old method unchanged, so
  both paths read a spreadsheet the same way.
- New `FileProcessing` status. Without it a file that had not been parsed was
  indistinguishable from one parsed and empty, and the tool showed "جاهز" for a
  file still being read.
- `GET /compare/files/staging?ids=` reports batch readiness, ownership-checked
  per file and refused whole if any id is not the caller's.
- The column wizard **waits** on that endpoint behind the shared progress
  dialog, instead of opening a mapping for a file whose columns nobody had read.
- The E2E test now waits for the detached parse and passes under `-race`; the
  mock repository got the mutex it needs now that two goroutines touch it.

The uploaded bytes deliberately travel with the goroutine. Re-reading them from
disk was tried and reverted: `openStoredUpload` searches `data/uploads/compare`
while the writer honours `UPLOAD_DIR`/`DATA_DIR`, so any deployment setting
either would report a good file as unreadable. Peak memory is unchanged from the
synchronous version and is bounded by the request-body cap.

### Still open

- **B4** `COPY` batching on the staging path — not audited.
- SQL `platform.normalize_arabic` — needs a migration and a GIN reindex.
- `cmd/worker` does not run the compare staging; it stays in the web process,
  detached. Moving it to River would need the uploaded file in shared storage
  rather than on one instance's disk.

### Note

`internal/ui/pages/market_discounts.templ` changed under me during this session
(mtime 19:02, three new undefined CSS classes) — another session or tooling is
editing this working tree. Not mine, not touched.

---

## The actual cause of "stuck forever" — a script-ordering bug, not the transport

The report was a vendor import frozen at **1%** on
`.../vendor/ingest/9b47e732-…`. I queried the deployed database rather than
guessing:

```
id  public_id                             phase   pct  rows   updated
89  9b47e732-2b3e-4e0f-9888-e332d5c9d583  review  100  1135   09-05 19:38:02
```

The import had **finished** — 1,135 rows staged, moved to review. The server was
never the problem. The screen simply never learned.

**Why.** `import-progress.js` is loaded with `defer` in the head, so it executes
*after* the document is parsed. The page's inline `<script>` runs *during*
parsing — so at that moment `window.ImportProgress` does not exist, and every
call site guarded itself with:

```js
if (typeof window.ImportProgress !== 'function') return;
```

which did exactly what it says. It returned. Silently, on every page load, with
nothing in the console. The bar then kept the percentage the server had rendered
into the HTML — 1%, written by `StageInBackground` before its goroutine starts —
and nothing ever polled to find out otherwise.

This predates all of this work: the same guard defeated the old `bar.poll` too,
so the bar had never been live on these three screens. It affected the vendor
import, the admin catalogue import and the smart-order ring — every progress bar
started from an immediately-invoked inline block.

**Fixed** by a readiness fence at each site: `DOMContentLoaded` fires only after
every deferred script has executed, and an already-parsed document runs the
initialiser at once. The silent `return` is now a `console.error`, so if it ever
does happen it says so.

**Guarded** by `internal/ui/pages/progress_bootstrap_test.go`, which fails when
an inline script starts a bar without the fence. Its first version matched the
word "DOMContentLoaded" and was satisfied by the *comment* explaining why the
fence is needed — removing the actual listener still passed. It now strips
comments and matches the listener call itself, and was verified to fail on the
real bug.

---

## Fourth pass — the two systems that were still fully synchronous

Reported: the saving-products import spins the browser tab with no progress, and
nothing had changed. Both true, for two separate reasons.

### 1. Nothing was deployed

Every change in this document is **uncommitted, on the local machine**. The
deployed instance has none of it. That should have been the first line of the
previous three reports rather than the last.

### 2. Saving products did all its work inside the POST

`/vendor/saving-products/import/{id}/map` and its pharmacy twin — the mapping
step, the one in the screenshot — did the ENTIRE job synchronously:

- `catSvc.ListMatchProducts` loaded the whole catalogue (up to 150k products)
- built a match index over it
- scored **every row** of the file against that index
- then ran the AI stage over the residue

with the browser holding the connection and nothing on screen. There was no
progress UI on this path at all — the bar existed only on the separate JSON
`/import/start` flow. And the route is not in `httpx.longRunningPrefixes`, so
the request deadline cut it off before it could finish.

Both handlers now call `startSavingImportRun`, which already existed and does
the right thing: a durable `platform.import_runs` row, an in-memory session
mirrored from it, and a goroutine that outlives the request. The wizard
redirects onto the run's id and renders a new `savingProcessingStage` that
streams progress through `/imports/{id}/stream` like every other tool.

### 3. Temp warehouses parsed every file inside the POST

`AdminTempWarehouseUploadSubmit` was written for "60-100+ files in parallel" and
read, parsed and inserted all of them in the request, sixteen at a time, then
waited on the lot. Split into `registerTempWarehouseFile` (in-request: read,
validate, write to disk, create the row as `FileProcessing`) and
`stageTempWarehouseFile` (detached: parse, detect columns, insert rows), with a
**bounded pool of four** — sixteen parsers against a twenty-connection pool is
how a bulk upload takes the rest of the site down. The admin screen polls
`/admin/user/temparte-warehouses/staging` for real per-file progress instead of
animating a fake 35→85→100, and the mapping wizard waits for the columns to
exist before opening. The org-import path had the same defect and now shares the
same code; the old synchronous implementation is deleted.

Both bulk tests (65 and 100 files) now wait for the detached parse and pass.

### Two real bugs found doing it

- **`resolveStoragePath` ignored the configured upload directory.** It searched
  the `UploadBaseDir` constant while `saveUploadedBytes` writes to
  `GetUploadBaseDir()`, which honours `UPLOAD_DIR`/`DATA_DIR`. On any deployment
  setting either, a file that had just been written was reported missing — which
  the detached staging pass depends on. Caught by the 100-file test failing on
  one file; fixed by searching the configured directory first.
- **`temp_warehouses` was not an allowed upload category**, so `sanitizeCategory`
  silently rewrote it to `products` and the warehouse files landed among the
  product images. Nothing broke because the path resolver tried the storage key
  first, so nobody noticed.

### Concurrent edits

`internal/modules/promo`, `internal/modules/commerce` and several
`internal/ui/pages` files changed under me repeatedly during this session, twice
leaving the tree unbuildable for minutes at a time. Another session or watcher
is editing this working tree. Nothing in those files is mine.

