# Task prompt: finish moving Arabic UI text into the i18n catalogue

Give this whole file to the model as its prompt. It is one job, done the same
way 231 times already.

---

## What you are doing

Repository: `F:\Dawa 24\dawa24-store` — a Go web application (chi + templ + pgx),
Arabic-first, bilingual Arabic/English.

Hardcoded Arabic strings are still sitting inside Go handler code. Because they
are hardcoded, an English-speaking user is shown Arabic no matter what language
they picked. Your job is to move them into the translation catalogue so both
languages work.

There is a working translation system already. You are not designing anything.
You are doing the same small edit, over and over, and checking it compiles.

**2,275 strings remain across ~235 files. You will not finish them all. That is
fine — every file you finish is a real improvement. Work down the list in order
and stop when you run out of budget.**

---

## The one edit you are making

### Before

```go
h.redirectWithNotice(w, r, "/vendor/team", "error", "لا يمكن حذف مالك المنشأة.")
```

### After — two changes

**1. Add the key** to the matching file in `internal/shared/i18n/`
(`catalog_admin.go`, `catalog_modules.go`, `catalog_commerce_ingest.go`, etc.
— pick the one whose name fits; any of them works):

```go
addKey(e, "vendor.team.cannot_delete_owner", "vendor", "لا يمكن حذف مالك المنشأة.", "The organization owner cannot be deleted.", "Team delete guard")
```

The six arguments are, in order:
`addKey(e, KEY, NAMESPACE, ARABIC_TEXT, ENGLISH_TEXT, SHORT_DESCRIPTION)`

- **KEY**: lowercase dotted, `surface.area.what_it_says` — e.g.
  `vendor.team.cannot_delete_owner`. Must be unique across all catalogue files.
- **NAMESPACE**: `"vendor"`, `"customer"`, `"admin"`, `"compare"`, `"promo"`,
  `"geo"`, `"errors"` — whichever fits.
- **ARABIC_TEXT**: the exact original string, copied character for character.
- **ENGLISH_TEXT**: a real, natural English translation. **Never leave it
  empty and never copy the Arabic into it.** This is the whole point of the task.
- **SHORT_DESCRIPTION**: 2–5 English words saying where it appears.

**2. Replace the string in the handler** with a lookup:

```go
h.redirectWithNotice(w, r, "/vendor/team", "error", i18n.T(lang, "vendor.team.cannot_delete_owner"))
```

That's it. That is the entire task, repeated.

---

## Getting `lang` in scope

`i18n.T` needs the user's language as its first argument. In a handler, one of
these is already available or trivially added:

- If the function has `lang` already (very common) — just use `lang`.
- If it has `r *http.Request` — use `langOf(r)`, e.g.
  `i18n.T(langOf(r), "key")`.
- If it has neither (a helper deep in the call chain) — **skip that string and
  move on.** Threading a new parameter through call sites is a bigger change
  than this task, and getting it wrong breaks the build.

Make sure the file imports:
```go
"github.com/muhiya/dawa24-store/internal/shared/i18n"
```
Running `goimports -w <file>` adds it for you.

---

## Strings with placeholders — important

If the Arabic string is used as a format string with `%d`, `%s` etc., you
**must** write it this way:

```go
// RIGHT
msg := fmt.Sprintf(i18n.T(lang, "vendor.import.rows_done"), count)

// WRONG — go vet fails the build with a printf warning
msg := i18n.T(lang, "vendor.import.rows_done", count)
```

`go vet` treats `i18n.T` as a printf-style function, and the *key* has no `%`
verbs in it, so passing extra arguments to `i18n.T` is reported as an error.
Always wrap with `fmt.Sprintf` instead.

---

## DO NOT TOUCH — read this twice

Converting any of the following will break the application. If a string is in
this list, leave it exactly as it is and move to the next one.

**1. Spreadsheet column-matching data.** These files hold lists of Arabic
column headers used to recognise columns in customer-uploaded Excel/CSV files.
They are data the importer matches against, not text shown to anyone.
Translating them silently breaks every file import.

- `internal/modules/compare/columns_data.go`
- `internal/modules/catalog/import_columns.go`
- `internal/modules/catalog/import_rows.go`
- `internal/modules/catalog/import_reader.go`
- anything containing `HeaderAliases`, `aliases`, or a big `[]string{...}` of
  Arabic words with no sentence in it

**2. Country names with flag emoji.**
- `internal/ui/visitor.go`
- `internal/modules/platform_admin/postgres/visitors.go`

These contain strings like `"مصر 🇪🇬"`, and there is SQL that compares against
that exact string. Change one and rows already in the database stop matching.

**3. Strings that are already bilingual.** If you see
`i18n.Text{"ar": "...", "en": "..."}`, it already holds both languages. Leave it.

**4. Sample data in generated spreadsheets.** Files like
`admin_import_sample.go` and `vendor_ingest_sample_handlers.go` contain real
Arabic pharmaceutical product names used as example rows in a downloadable
template. Those are sample *data*. (The column headers and instructions in the
same file usually *are* UI text and can be converted — use judgement, and when
unsure, skip.)

**5. Anything inside a `//` comment.** Comments are for developers.

**6. Anything in a `_test.go` file.**

### The decision rule, in one line

> Would a user see this string on screen as a sentence or label? Convert it.
> Is it a keyword the code matches against, or example data? Leave it.

When you are not sure: **leave it.** A skipped string costs nothing. A wrongly
converted one breaks imports or database matching.

---

## The order to work in

Do the files in this order. Finish one file completely, verify, then start the
next. Never edit more than one file before verifying.

### Priority 1 — vendor screens (the user asked for these to be strict)

```
internal/ui/vendor_team_page_handlers.go        27 strings
internal/ui/vendor_sponsorship_handlers.go      16
internal/ui/vendor_ingest_review_handlers.go    16
internal/ui/vendor_catalog_handlers.go          16
internal/ui/vendor_order_handlers.go            15
internal/ui/pages/vendor_ingest_models.go       15
internal/ui/vendor_ingest_handlers.go           13
internal/ui/vendor_user_org_handlers.go         11
internal/ui/vendor_ingest_bulk_handlers.go       9
internal/ui/vendor_content_handlers.go           7
internal/ui/vendor_variant_handlers.go           6
internal/ui/vendor_purchase_request_handlers.go  6
internal/ui/vendor_offer_handlers.go             6
internal/ui/vendor_institutional_handlers.go     5
internal/ui/vendor_saving_import_ops.go          4
internal/ui/vendor_handlers.go                   3
internal/ui/vendor_inventory_handlers.go         2
```

### Priority 2 — shared screens both audiences see

```
internal/ui/smart_order_handlers.go             49
internal/ui/notifications_dispatch.go           27
internal/ui/decision_memory_handlers.go         27
internal/ui/compare_upload_handlers.go          26
internal/ui/coverage_write_handlers.go          23
internal/ui/job_form_handlers.go                21
internal/ui/suppliers_handlers.go               19
internal/ui/settings_payment_handlers.go        18
internal/ui/document_serve_handlers.go          28
internal/ui/ai_usage_handlers.go                28
internal/ui/team_import_ops.go                  33
```

### Priority 3 — admin screens (best-effort; lowest value, only staff see them)

```
internal/ui/admin_audit_handlers.go             82
internal/ui/admin_reference_handlers.go         21
internal/ui/admin_roles_handlers.go             19
internal/ui/admin_geography_handlers.go         14
internal/ui/admin_settings_handlers.go          13
internal/ui/admin_finance_handlers.go           13
internal/ui/admin_billing_handlers.go           13
internal/ui/admin_commerce_handlers.go          12
... and the rest of internal/ui/admin_*.go
```

Do **not** start on `internal/modules/**` — those are domain-layer files where
`lang` is usually not in scope, and most of their Arabic is matching data.

---

## Your loop, exactly

For each file, in order:

**Step 1 — see what is in it.** Run in bash:

```bash
cd "F:/Dawa 24/dawa24-store"
grep -n '"[^"]*[؀-ۿ][^"]*"' internal/ui/vendor_team_page_handlers.go
```

**Step 2 — edit.** For each user-facing string in that file: add an `addKey`
line to a catalogue file in `internal/shared/i18n/`, and replace the literal
with `i18n.T(lang, "your.key")`. Skip anything on the DO NOT TOUCH list.

**Step 3 — verify.** Run all four, in this order:

```bash
cd "F:/Dawa 24/dawa24-store"
goimports -w internal/ui/vendor_team_page_handlers.go internal/shared/i18n/
gofmt -l . | grep -v '^tmp'
go build ./...
go vet ./...
```

- `gofmt -l` must print nothing.
- `go build` must print nothing.
- `go vet` must print nothing. If it complains about printf, you passed
  arguments to `i18n.T` — wrap with `fmt.Sprintf` as shown above.

**Step 4 — if anything failed, fix it before moving on.** Do not start the next
file with a broken build.

**Step 5 — every 5 files, run the tests once:**

```bash
go test -short -count=1 ./...
```

All 69 packages must pass. Then check the tree is clean:

```bash
git status --porcelain | grep 'internal/ui/data' | wc -l
```

That must print `0`.

---

## When you stop

**1. Update the ratchet.** Count what is left:

```bash
cd "F:/Dawa 24/dawa24-store"
pat=$(printf '\330')
LC_ALL=C grep -rc "\"[^\"]*$pat[^\"]*\"" --include='*.go' internal/ui internal/modules cmd 2>/dev/null \
  | grep -v "_test\|_templ\|/i18n/" | awk -F: '{s+=$2} END{print s+0}'
```

Take that number and put it into `Makefile` in the `check-hardcoded-arabic`
target, replacing **both** places the old number appears (`-gt 2276` and
`ceiling 2276`). The number must only ever go **down**.

**2. Append a short section to `docs/REFACTOR_2026-08-30.md`** saying: how many
strings you converted, which files you finished, which you skipped and why.

**3. Do not commit anything.** The user reviews the diff themselves.

**4. Report honestly.** Say exactly how many you converted and which files are
still untouched. Do not round up, and do not claim a file is done if you skipped
strings inside it.

---

## Rules you must not break

- Never leave the English translation empty or as a copy of the Arabic.
- Never convert a string on the DO NOT TOUCH list.
- Never commit.
- Never raise a ratchet number.
- Never move on from a file that does not build.
- If you are unsure whether a string is UI text or data, skip it.
