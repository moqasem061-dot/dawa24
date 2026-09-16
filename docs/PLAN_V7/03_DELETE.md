# PHASE 3 — Delete

**Depends on:** Phase 2.
**Pure removal.** This phase adds nothing. It is the fastest reduction in
complexity available.

Every removal gets a row in `DELETED.md`: what, kind, reason, and — if it may
return — the condition under which it would.

---

## TASK 3.1 — Remove the translation system

**Audit §1.6.** `platform_admin.translations` is written by exactly one screen
and read by nothing. Templates use inline `if lang == "ar"`. An administrator can
edit translations that never appear anywhere.

### Confirm before deleting

```bash
# who reads the table?
grep -rn "platform_admin.translations" internal/ --include=*.go
# does any render path consume a translation lookup?
grep -rn "Translate\|translate_inc\|i18n\." internal/ui/pages/*.templ | head
```
If the second command shows a real lookup used at render time, **stop** and
report it — the system is live and this task changes.

### Remove

| Layer | What |
|---|---|
| Route | `GET /admin/translations`, `POST /admin/translations` |
| Handler | `AdminTranslationsPage`, `AdminTranslationsSubmit` |
| Template | `internal/ui/pages/admin_translations.templ` |
| Sidebar | the الترجمات entry |
| Service | `ListTranslations`, `UpsertTranslation` and their repository methods |
| Domain | `platform_admin.Translation` |
| Migration | `db/migrations/NNN_drop_translations.up.sql` dropping `platform_admin.translations`; the `.down.sql` recreates the table (data is not recoverable — say so in the comment) |
| Permission | remove the translations permission key and its grants |

**Do not remove** `internal/shared/i18n` or the `lang`/`dir` plumbing — the
platform is still bilingual through the templates. Only the *editable overrides*
go.

### Tests
- `/admin/translations` returns 404
- `make check` green
- `migratecheck -roundtrip` green

---

## TASK 3.2 — Remove the pharmacy cPanel

**Audit §1.8.** `/customer/cpanel` is a link hub to pages the sidebar already
exposes. Laravel has one, but Laravel parity is not a reason to keep a screen
whose only function is duplicating navigation.

Remove: the route, `CustomerCPanelPage`, its template section, and any link to
it. Anything on it that is **not** reachable elsewhere moves onto
`/customer/dashboard` first — check that before deleting.

```bash
grep -rn "cpanel\|CPanel" internal/ui/ --include=*.go --include=*.templ
```

- [ ] Nothing unique was lost; anything unique moved to the dashboard
- [ ] `/customer/cpanel` returns 404 or 301 to `/customer/dashboard`

---

## TASK 3.3 — Delete dead files

### Confirmed dead
`internal/ui/layouts/pharmacy.templ` — 203 lines, **zero** references to
`PharmacyShell`. Delete it and its generated `_templ.go`.

### Find the rest

```bash
# templates whose templ funcs are never called
for f in internal/ui/pages/*.templ; do
  base=$(basename "$f" .templ)
  for fn in $(grep -oE '^templ [A-Za-z0-9_]+' "$f" | awk '{print $2}'); do
    n=$(grep -rl "pages\.$fn" internal/ui/*.go | wc -l)
    [ "$n" -eq 0 ] && echo "UNUSED: $base :: $fn"
  done
done

# handlers never registered
grep -ohE "^func \(h \*UIHandler\) ([A-Za-z0-9_]+)\(w http" internal/ui/*.go \
  | sed -E 's/func \(h \*UIHandler\) //;s/\(w http//' | sort -u > /tmp/h.txt
grep -ohE "h\.[A-Za-z0-9_]+\)" internal/ui/handlers.go internal/ui/admin_routes_*.go \
  | sed 's/h\.//;s/)//' | sort -u > /tmp/r.txt
comm -23 /tmp/h.txt /tmp/r.txt

# components never used
for f in internal/ui/components/*.templ; do
  for fn in $(grep -oE '^templ [A-Za-z0-9_]+' "$f" | awk '{print $2}'); do
    n=$(grep -rl "components\.$fn" internal/ui/pages internal/ui/layouts | wc -l)
    [ "$n" -eq 0 ] && echo "UNUSED COMPONENT: $fn"
  done
done
```

Delete everything the scans find, unless it is a lowercase helper. Record each.

---

## TASK 3.4 — Resolve the 31 data-less pages

```bash
grep -rhoE 'pages\.[A-Za-z0-9_]+\(lang, dir\)' internal/ui/*.go | sort -u
```

For each, in this order:

1. **Did Phase 2 already delete it?** (settings sub-pages, catalog duplicates) → done.
2. **Does Laravel have this screen?**
   ```bash
   grep -ril "<concept>" "F:/Dawa 24/Laravel/app/Livewire/"
   ```
   **No → delete it.** It was invented. This is the majority case for the
   thirteen Phase-8 monetisation placeholders.
3. **Yes, and it is in scope → connect it** (the six-link chain; PLAN_V6 §A.5).
   Only do this for screens that appear in the final sidebar.
4. **Yes, but out of scope → delete it and record the decision.** A placeholder
   is worse than nothing; it looks finished.

**Expected outcome: most of the 31 are deleted, not connected.** The platform
does not need thirteen monetisation screens before it has one working order flow.

Record the disposition of all 31 in `DELETED.md` or `PROGRESS.md`.

---

## TASK 3.5 — Drop the dead tables

```bash
grep -ohiE "CREATE TABLE (IF NOT EXISTS )?[a-z_]+\.[a-z_0-9]+" db/migrations/*.up.sql \
 | sed -E "s/CREATE TABLE (IF NOT EXISTS )?//I" | sort -u \
 | while read t; do
     [ "$(grep -rl "$t" internal/ --include=*.go | wc -l)" -eq 0 ] && echo "DEAD: $t"
   done
```

For each: **connect** (only if its screen survives), or **drop** via a migration
whose `.down.sql` recreates it. Deferring is allowed for at most three, each with
a written justification naming the future task.

Special cases:
- `catalog.product_infos` — a 5-column key/value bag whose name collides with
  `catalog.product_index`. If nothing uses it, **drop it**; the collision alone is
  a hazard.
- Tables backing screens Task 3.4 deletes → drop them in the same migration.

---

## TASK 3.6 — Dead code sweep

```bash
go vet ./...
golangci-lint run --enable unused,deadcode 2>/dev/null || golangci-lint run
```

Plus:
- unused service methods: for each exported method on a service, is it called?
- unused repository methods and their interface declarations
- duplicate API endpoints: `grep -rhoE '"/api/v1/[^"]*"' internal/modules/*/http/*.go | sort | uniq -d`
- unused imports, unused struct fields, orphan test helpers

---

## PHASE 3 GATE

```bash
make check && DATABASE_URL="..." go test ./... -race
DATABASE_URL="..." go run ./cmd/migratecheck -from 1 -roundtrip
```

- [ ] Routes, pages, tables and total lines **all decreased** — record before/after
- [ ] Data-less pages: **0**
- [ ] Dead tables: **0**, or ≤3 with written justifications
- [ ] Zero unused templates, handlers or components
- [ ] Translations gone; cPanel gone; `pharmacy.templ` gone
- [ ] `DELETED.md` accounts for every removal
