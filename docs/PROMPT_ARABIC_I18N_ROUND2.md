# Task prompt: finish the Arabic i18n migration (round 2)

Give this whole file to the model as its prompt.

---

## Read this first — the job is not finished

A previous run reported this task 100% complete. It was not. The measured count
of hardcoded Arabic string literals in Go is **1,246**, not zero.

That previous run did good work — around 1,030 strings were converted with real
English translations, and the vendor and customer handler files are genuinely
done. The reporting was wrong, not the code. Your job is to finish it, and to
report only what you actually did.

**Never claim a percentage you have not measured.** The exact command that
measures it is at the bottom of this file. Run it, and report the number it
prints.

---

## Before you start: check the build

```bash
cd "F:/Dawa 24/dawa24-store"
go build ./...
```

If this prints errors, **stop and tell the user**. At the time this prompt was
written the build was broken by an unrelated in-progress change in
`internal/modules/identity/service_mfa.go` (a `*int` field being assigned an
`int`). Do not start editing on top of a broken build, and do not try to fix
someone else's half-finished work.

Only continue when `go build ./...` prints nothing.

---

## What you are doing

Repository: `F:\Dawa 24\dawa24-store` — a Go web application (chi + templ +
pgx), Arabic-first, bilingual Arabic/English.

Hardcoded Arabic strings inside Go code mean an English-speaking user is shown
Arabic no matter what language they chose. You move those strings into the
translation catalogue so both languages work.

You are not designing anything. You are repeating one small edit and checking
that it compiles.

---

## The one edit

### Before

```go
h.redirectWithNotice(w, r, "/vendor/team", "error", "لا يمكن حذف مالك المنشأة.")
```

### After — two changes

**1. Add the key** to a file in `internal/shared/i18n/` (`catalog_admin.go`,
`catalog_modules.go`, `catalog_commerce_ingest.go`, … — pick the one whose name
fits; any of them works):

```go
addKey(e, "vendor.team.cannot_delete_owner", "vendor", "لا يمكن حذف مالك المنشأة.", "The organization owner cannot be deleted.", "Team delete guard")
```

Arguments in order:
`addKey(e, KEY, NAMESPACE, ARABIC_TEXT, ENGLISH_TEXT, SHORT_DESCRIPTION)`

- **KEY** — lowercase dotted, `surface.area.what_it_says`. Must be unique.
- **NAMESPACE** — `"vendor"`, `"customer"`, `"admin"`, `"compare"`, `"promo"`,
  `"geo"`, `"errors"`.
- **ARABIC_TEXT** — the original string, copied exactly.
- **ENGLISH_TEXT** — a real, natural English translation. **Never empty, never
  a copy of the Arabic.** That is the entire point of the task.
- **SHORT_DESCRIPTION** — 2–5 English words saying where it appears.

**2. Replace the literal** in the Go file:

```go
h.redirectWithNotice(w, r, "/vendor/team", "error", i18n.T(lang, "vendor.team.cannot_delete_owner"))
```

That is the whole task, repeated.

---

## Getting `lang` in scope

`i18n.T` needs the language as its first argument.

- The function already has `lang` — use it. (Most common.)
- The function has `r *http.Request` — use `langOf(r)`.
- **It has neither — skip that string and move on.** Threading a new parameter
  through call sites is a bigger change than this task and breaks builds.

This last case matters in the `internal/ui/pages/*_models.go` files listed
below: many of their functions are plain constructors with no request and no
language. Check first; if there is no `lang`, skip the file entirely rather
than restructuring it.

Run `goimports -w <file>` after editing — it adds
`"github.com/muhiya/dawa24-store/internal/shared/i18n"` for you.

---

## Format strings — the one trap that fails the build

If the string has `%d`, `%s`, `%v` in it:

```go
// RIGHT
msg := fmt.Sprintf(i18n.T(lang, "vendor.import.rows_done"), count)

// WRONG — go vet fails: the key has no % verbs in it
msg := i18n.T(lang, "vendor.import.rows_done", count)
```

Always wrap with `fmt.Sprintf`. Never pass extra arguments to `i18n.T`.

---

## DO NOT TOUCH — read twice

Converting anything below breaks the application. Leave it exactly as it is.

**1. Spreadsheet column-matching data.** Lists of Arabic column headers the
importer matches against in customer-uploaded Excel/CSV files. Not text anyone
reads. Translating them breaks every file import.

- `internal/modules/compare/columns_data.go` (41)
- `internal/modules/catalog/import_columns.go` (24)
- `internal/modules/catalog/import_rows.go` (55)
- `internal/modules/catalog/import_reader.go` (20)
- anything with `HeaderAliases`, `aliases`, or a long `[]string{…}` of Arabic
  words with no sentence among them

**2. Country names with flag emoji.**
- `internal/ui/visitor.go` (63)
- `internal/modules/platform_admin/postgres/visitors.go`

There is SQL comparing against those exact strings. Change one and rows already
in the database stop matching.

**3. `Icon string` struct fields holding an emoji.**
- `internal/ui/pages/wizard.go` (16)
- `internal/ui/pages/admin_import_wizard.go` (28)

**4. Sample data in generated spreadsheets.** Real Arabic pharmaceutical product
names used as example rows in downloadable templates.
- `internal/ui/admin_import_sample.go` (30)
- `internal/ui/vendor_ingest_sample_handlers.go` (35)

Column *headings* and *instructions* in those files may be UI text and can be
converted. Sample product rows are data. When unsure, skip.

**5. Already-bilingual values.** `i18n.Text{"ar": "...", "en": "..."}` holds
both languages already.

**6. Comments, and anything in a `_test.go` file.**

### The decision rule

> Would a user read this on screen as a sentence or a label? Convert it.
> Is it a keyword the code matches against, or example data? Leave it.

**When unsure, skip.** A skipped string costs nothing. A wrongly converted one
breaks file imports or database matching.

---

## Work in this order

Finish one file completely, verify, then start the next. Never edit two files
before verifying.

### Priority 1 — `internal/ui`, the genuinely convertible ones

```
internal/ui/team_import_ops.go                33
internal/ui/document_serve_handlers.go        16
internal/ui/compare_upload_handlers.go        12
internal/ui/saving_products_sessions_ops.go   10
internal/ui/admin_image_import_sessions.go    10
internal/ui/pages/admin_import_mapping.go      9
```

Then the remaining `internal/ui/*.go` files with counts under 10 — find them
with the command in "Measuring" below.

### Priority 2 — view models, but check for `lang` first

```
internal/ui/pages/dashboard_models.go   33
internal/ui/pages/documents_models.go   30
internal/ui/pages/account_models.go     17
```

If a function here has no `lang` parameter and no request, skip the whole file
and say so in your report.

### Priority 3 — service layer, user-facing messages only

These return error and status messages that reach the screen. Many of their
functions take no language — convert only where `lang` is genuinely available,
skip the rest.

```
internal/modules/ingest/domain.go            46
internal/modules/smartorder/pipeline/query.go 45
internal/modules/compare/comparison.go        36
internal/modules/org/postgres/institutional.go 24
internal/modules/commerce/availability.go     17
internal/modules/attachments/service.go       16
```

Do not start Priority 3 until Priorities 1 and 2 are done.

---

## Your loop, exactly

**Step 1 — see what is in the file:**

```bash
cd "F:/Dawa 24/dawa24-store"
grep -n '"[^"]*[؀-ۿ][^"]*"' internal/ui/team_import_ops.go
```

**Step 2 — edit.** Add `addKey` lines; replace literals with `i18n.T(lang, …)`.
Skip anything on the DO NOT TOUCH list.

**Step 3 — verify. All four, in order:**

```bash
cd "F:/Dawa 24/dawa24-store"
goimports -w internal/ui/team_import_ops.go internal/shared/i18n/
gofmt -l . | grep -v '^tmp'
go build ./...
go vet ./...
```

`gofmt -l` must print nothing. `go build` must print nothing. `go vet` must
print nothing — if it reports printf, you passed arguments to `i18n.T`; wrap
with `fmt.Sprintf`.

**Step 4 — if anything failed, fix it before the next file.**

**Step 5 — every 5 files:**

```bash
go test -short -count=1 ./...
git status --porcelain | grep 'internal/ui/data' | wc -l
```

All 69 packages must pass, and that count must be `0`.

---

## Measuring — run this before you report anything

```bash
cd "F:/Dawa 24/dawa24-store"
pat=$(printf '\330')

# total remaining
LC_ALL=C grep -rc "\"[^\"]*$pat[^\"]*\"" --include='*.go' internal/ui internal/modules cmd 2>/dev/null \
  | grep -v "_test\|_templ\|/i18n/" | awk -F: '{s+=$2} END{print s+0}'

# per file, biggest first
LC_ALL=C grep -rc "\"[^\"]*$pat[^\"]*\"" --include='*.go' internal/ui internal/modules cmd 2>/dev/null \
  | grep -v "_test\|_templ\|/i18n/" | grep -v ':0$' | sort -t: -k2 -rn | head -30
```

The starting number for this round is **1,246**.

---

## When you stop

1. **Update the ratchet.** Run the total command above and put the number into
   `Makefile`, in the `check-hardcoded-arabic` target, replacing **both** places
   the old number appears (`-gt 1246` and `ceiling 1246`). It may only go down.

2. **Append to `docs/REFACTOR_2026-08-30.md`**: how many strings you converted,
   which files you finished, which you skipped and the reason.

3. **Do not commit.** The user reviews the diff.

4. **Report the measured number, not an estimate.** State how many remain and
   which files still have strings. If you skipped a file, name it and say why.
   Do not say "100%" unless the total command prints `0` — and it will not,
   because the DO NOT TOUCH list alone accounts for roughly 300 strings that
   must stay.

---

## Rules you must not break

- Never leave the English translation empty or as a copy of the Arabic.
- Never convert anything on the DO NOT TOUCH list.
- Never pass extra arguments to `i18n.T` — use `fmt.Sprintf`.
- Never commit.
- Never raise a ratchet number.
- Never move past a file that does not build.
- Never report a completion percentage you did not measure.
- When unsure whether a string is UI text or data, skip it.
