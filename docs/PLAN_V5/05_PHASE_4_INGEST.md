# PHASE 4 — Chunked Import Pipeline

**Depends on:** Phase 2 (column detection, matching), Phase 3 (automation upload).
**Blocks:** real-file-size operation of Phases 2, 3, 5, 6.
**Tasks:** 4.

## Why this phase exists

`/what-in` lists as an admin pillar:

> "Bulk Spreadsheet Upload (معالجة آلاف الأصناف بالـ Chunk بدون استهلاك ذاكرة)"

Laravel implements it with `ChunkReadFilter`, `ProcessMainImportChunk`,
`ProcessImportJob`, `ImportMainProductsJob`, `ProcessMatchingJob`,
`ProcessWarehouseBatch`, `ProcessWarehouseFile`, the `import_batches` /
`import_rows` / `import_progress` tables, and `POST /common/upload-chunk`.

**Go has the tables and none of the pipeline.** Uploads are single-shot.

Separately, the vendor ingest wizard is still mostly decorative:
`internal/ui/pages/vendor_ingest.templ` has 2 form/fetch hooks against 10
complete API endpoints, and its step transitions are Alpine
`@click="step = 3"` with no server round-trip.

---

## What already exists (preserve it)

`internal/modules/ingest/` is well built. Ten endpoints, all working:

```
POST /api/v1/ingest/uploads/presign
POST /api/v1/ingest/uploads
POST /api/v1/ingest/sessions
GET  /api/v1/ingest/sessions/{id}
POST /api/v1/ingest/sessions/{id}/mapping
GET  /api/v1/ingest/sessions/{id}/rows
PUT  /api/v1/ingest/sessions/{id}/rows/{rid}
POST /api/v1/ingest/sessions/{id}/commit
POST /api/v1/ingest/sessions/{id}/cancel
GET  /api/v1/ingest/sessions/{id}/events        ← SSE
GET  /api/v1/admin/ingest/sessions
```

**Do not rewrite this module.** Extend it with chunking, and wire the UI to it.

---

## TASK 4.1 — Chunked upload transport

### 4.1.1 Inspect first

```bash
cat F:/Dawa\ 24/Laravel/app/Http/Controllers/Customer/ChunkUploadController.php
cat F:/Dawa\ 24/Laravel/app/Services/ChunkReadFilter.php
cat F:/Dawa\ 24/Laravel/app/Jobs/ProcessMainImportChunk.php
cat F:/Dawa\ 24/Laravel/app/Jobs/ProcessImportJob.php
grep -rn "upload-chunk" F:/Dawa\ 24/Laravel/routes/ F:/Dawa\ 24/Laravel/resources/views/
```

Record in `docs/modules/ingest.md`:
- the chunk size Laravel uses
- how chunks are identified and reassembled
- what happens on a failed/duplicate chunk
- the client-side driver (is it a JS library? which?)
- whether `ChunkReadFilter` chunks the *upload* or the *spreadsheet read* — these
  are different problems and Laravel may do both

### 4.1.2 Two distinct problems — solve both

**(a) Chunked HTTP upload** — the browser sends a large file in pieces.
`internal/modules/ingest/` already has `uploads/presign`. If the storage backend
supports multipart upload (S3/MinIO does), **prefer presigned multipart** over a
custom chunk endpoint: the bytes never touch the app server. Check
`internal/platform/storage` for multipart support before building anything.

If presigned multipart is available:
- extend `presign` to return multipart part URLs
- add `POST /api/v1/ingest/uploads/{id}/complete` to finalise
- the client uploads parts directly to storage

If it is not available, add:
- `POST /api/v1/ingest/uploads/chunk` — accepts `upload_id`, `chunk_index`,
  `total_chunks`, and the bytes
- server-side reassembly with idempotency on `(upload_id, chunk_index)`
- a resume endpoint returning which chunks are already present

**(b) Streaming spreadsheet read** — never load the whole sheet into memory.
The XLSX reader must stream rows. Check which library `go.mod` already has for
`VendorIngestSampleXLSX` and confirm it supports streaming; if it does not,
replace it and record why.

Rows are written to `ingest.import_rows` in batches (e.g. 500) via
`COPY` or a multi-row `INSERT`, inside `db.InTx`.

### 4.1.3 Progress

`ingest.import_progress` exists. Populate it: `total_rows`, `processed_rows`,
`failed_rows`, `stage`, `updated_at`. The existing SSE endpoint
(`sessions/{id}/events`) streams it to the UI.

### 4.1.4 Tests

- T2: a 50,000-row file imports without the process exceeding a memory ceiling
  (assert with `runtime.ReadMemStats` around the import, or run it as a
  long-running integration test with a documented threshold)
- T2b: chunk reassembly is idempotent — re-sending chunk 3 does not corrupt
- T2c: a resumed upload completes correctly after an interrupted one
- T3: cross-tenant — org B cannot read org A's session, rows, or progress
- T6: progress reaches 100% and the SSE stream reflects it

---

## TASK 4.2 — Wire the vendor ingest wizard

### 4.2.1 The current failure

`internal/ui/pages/vendor_ingest.templ` — 324 lines, step transitions are
`@click="step = 3"`. The user advances through a wizard that never talks to the
server. **This is the pattern to eliminate everywhere.**

### 4.2.2 Inspect Laravel's equivalent

```bash
cat F:/Dawa\ 24/Laravel/app/Livewire/Employee/VendorProductsImport.php
cat F:/Dawa\ 24/Laravel/app/Livewire/Employee/VendorProductsList.php
cat F:/Dawa\ 24/Laravel/app/Livewire/Employee/ImportProducts.php
# and the Blade views
```

Record every step, every field, every validation, and the Arabic labels.

### 4.2.3 Rebuild the wizard against the API

Each step is a **server round-trip**:

| Step | Action | Endpoint |
|---|---|---|
| 1 Upload | dropzone → presigned/chunked upload | `uploads/presign` → storage → `uploads` |
| 2 Session | create the import session | `POST sessions` |
| 3 Mapping | show detected headers + preview; user confirms | `POST sessions/{id}/mapping` — **reuse Phase 2's `DetectColumns`** |
| 4 Review | paginated row list with per-row validation errors and inline edit | `GET sessions/{id}/rows`, `PUT sessions/{id}/rows/{rid}` |
| 5 Commit | apply to the catalog | `POST sessions/{id}/commit` |
| — | Cancel at any point | `POST sessions/{id}/cancel` |
| — | Live progress | `GET sessions/{id}/events` (SSE) |

Rules:
- the step number comes from the **session's server-side status**, not Alpine state
- a page reload mid-wizard resumes at the correct step
- navigating away and back resumes
- the commit button is disabled until the session is in a committable state
- all five UI states (§0.7.4), with `@components.Skeleton` during processing

### 4.2.4 Routes

Add to `RegisterVendorRoutes`:
```go
r.Get ("/vendor/ingest",                 h.VendorIngestPage)          // existing
r.Get ("/vendor/ingest/{sessionID}",     h.VendorIngestSessionPage)   // resume
r.Post("/vendor/ingest/upload",          h.VendorIngestUploadSubmit)  // existing
r.Post("/vendor/ingest/{id}/mapping",    h.VendorIngestMappingSubmit)
r.Get ("/vendor/ingest/{id}/rows",       h.VendorIngestRowsPartial)
r.Post("/vendor/ingest/{id}/rows/{rid}", h.VendorIngestRowUpdateSubmit)
r.Post("/vendor/ingest/{id}/commit",     h.VendorIngestCommitSubmit)  // existing
r.Post("/vendor/ingest/{id}/cancel",     h.VendorIngestCancelSubmit)
```

### 4.2.5 Tests

- T6: the full wizard, driven through HTTP, produces catalog rows
- T6b: **reload at each step resumes at that step** — this is the regression test for the Alpine-only bug
- T6c: an invalid row blocks commit and shows an inline Arabic error
- T15: dead-target scan on `vendor_ingest.templ` = 0, and every `@click` that
  changes a step also triggers a server call

---

## TASK 4.3 — Admin bulk import

Laravel: `/admin/import-products`, `/admin/image-products`,
`/admin/orgniazions/products-import/{id}/upload` (per-organization),
`/admin/users/products-import`, `/admin/organizations/products-import`.
Go: one single-shot `POST /admin/products/import`.

### 4.3.1 Inspect

```bash
cat F:/Dawa\ 24/Laravel/app/Livewire/Admin/ImportProducts.php
cat F:/Dawa\ 24/Laravel/app/Livewire/Admin/ImageImportProducts.php
cat F:/Dawa\ 24/Laravel/app/Livewire/Admin/Organizations/VendorProductsImport.php
cat F:/Dawa\ 24/Laravel/app/Livewire/Admin/Organizations/ImportIndex.php
cat F:/Dawa\ 24/Laravel/app/Livewire/Admin/Organizations/ImportShow.php
cat F:/Dawa\ 24/Laravel/app/Livewire/Admin/Users/ImportIndex.php
cat F:/Dawa\ 24/Laravel/app/Jobs/ImportMainProductsJob.php
```

### 4.3.2 Build

Reuse the same wizard component as Task 4.2, parameterised by target
organization. Admin screens (routed in Phase 5):

| Route | Purpose |
|---|---|
| `/admin/products/import` | master catalog import |
| `/admin/products/import/images` | image import (`ImageImportProducts`) |
| `/admin/organizations/imports` | list orgs with import stats (`ImportIndex`) |
| `/admin/organizations/{id}/import` | import on behalf of one org (`VendorProductsImport`) |
| `/admin/organizations/{id}/imports` | that org's imported products, editable (`ImportShow`) |
| `/admin/users/imports` | per-user import index |

**Note `ImportShow`'s deletion behaviour**: the Laravel route comment says the
delete permission is enforced *inside the component* via AJAX and has no route
of its own. In Go it needs a real route with `RequirePagePermission`.

`session_id` sessions carry `organization_id`; an admin importing for org X must
set it explicitly, and the commit must write into org X with `AsSystem` plus a
justifying comment (rule R4).

### 4.3.3 Image import

`ImageScraperService.php` exists in Laravel. Read it: does it scrape images from
URLs in the sheet, or upload a zip? Reproduce the mechanism through
`internal/platform/storage`. If it makes outbound HTTP requests, add a timeout,
a size cap, a content-type allowlist, and SSRF protection (reject private IP
ranges) — this is a new external integration and needs the security treatment.

### 4.3.4 Tests

- T5: each admin import route requires the right permission
- T6: an admin can import for another organization and the rows land under that org
- T3: an org member cannot use the admin import to write into a different org
- T16: image import rejects a private-IP URL (SSRF guard)

---

## TASK 4.4 — Retire the legacy single-shot paths

Once chunked import works:
- keep `POST /admin/products/import` as an alias that creates a session, so
  bookmarks and any scripts keep working
- remove any code path that reads a whole file into memory
- confirm the interim direct-upload note from Phase 2 Task 2.2.3 is resolved and
  update `docs/modules/compare.md`

---

## PHASE 4 COMPLETION GATE

```bash
make check
go test ./internal/modules/ingest/... -race
go test ./test/integration/... -run Ingest
```

- [ ] A 50,000-row file imports without loading it into memory
- [ ] Upload is resumable
- [ ] **The vendor wizard resumes at the correct step after a reload** (T6b)
- [ ] No `@click="step = N"` remains without a server call (T15)
- [ ] Admin can import on behalf of an organization
- [ ] Image import has SSRF protection
- [ ] Phase 2 and Phase 3 uploads route through this pipeline
- [ ] `PROGRESS.md` updated for 4.1–4.4
