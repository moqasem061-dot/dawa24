# PHASE 5 — Design system, navigation, and the final walk

**Depends on:** Phases 1–4. Do this **last** — it touches every surviving page,
so it must run once, on the final set.

---

## The measurement

**4,938 inline `style="` attributes** across 97 templates, against two CSS files.
That single number explains why the design looks random: every page invents its
own spacing, radius and colour, so a fix on one page never generalises.

Worst offenders:
`settings_unified.templ` 245 · `admin_developers.templ` 230 ·
`admin_settings.templ` 215 · `settings.templ` 149 · `admin_institutional.templ` 131

Target: **≤1,000**, with the remainder being genuinely one-off positioning.

---

## TASK 5.1 — Establish design tokens

Read `internal/ui/static/css/app.css` and `components.css` first — tokens may
already exist (`var(--radius-md)`, `var(--surface-sunken)` appear in templates,
so at least some do). **Extend what is there; do not start a third system.**

Ensure the token set covers, with light and dark values:

| Group | Tokens |
|---|---|
| spacing | `--space-1` … `--space-8` on a consistent scale |
| radius | `--radius-sm/md/lg/xl` |
| colour | `--surface`, `--surface-sunken`, `--surface-raised`, `--border`, `--text`, `--text-secondary`, `--accent`, `--accent-subtle`, plus success/warning/danger/info |
| typography | `--font-size-xs` … `--font-size-2xl`, `--font-weight-*`, `--line-height-*` |
| elevation | `--shadow-sm/md/lg` |
| layout | `--sidebar-w`, `--content-max-w`, `--header-h` |

Every token gets a dark-mode value in the same file, defined the way the existing
theme toggle expects.

---

## TASK 5.2 — Replace inline styles with classes

Work **file by file**, largest first. For each:

1. Read the inline styles and group them into recurring patterns.
2. Add a class to `components.css` for each pattern (`.stat-grid`,
   `.page-header`, `.form-row`, `.card-toolbar`, …).
3. Replace the inline styles with the class.
4. Anything genuinely one-off — a specific grid column count, an exact pixel
   offset — may stay inline, but must use tokens (`padding: var(--space-4)`),
   never literals (`padding: 1.25rem`).

Commit per file. Record the running total in `PROGRESS.md`.

**Add a CI gate** once the count is under target:
```makefile
check-inline-styles:
	@n=$$(grep -oh 'style="' internal/ui/pages/*.templ internal/ui/layouts/*.templ | wc -l); \
	if [ "$$n" -gt 1000 ]; then echo "FAIL: $$n inline styles (max 1000)"; exit 1; fi; \
	echo "OK: $$n inline styles"
```
Wire it into `make check`. Prove it fails by temporarily lowering the threshold.

---

## TASK 5.3 — Make pages use the component library

34 components exist and pages hand-roll their own instead. For every surviving
page:

| If the page has | It must use |
|---|---|
| a table | `components.DataTable` |
| an empty list | `components.EmptyState` |
| a failed load | `components.ErrorState` |
| a metric | `components.StatCard` |
| a dialog | `components.Modal` |
| a paged list | `components.Pagination` |
| tabs | `components.Tabs` — **one tab implementation platform-wide** |
| a form field | `components.Forms` helpers |
| a money value | `components.MoneyDisplay` |
| a status | `components.Badges` |

Audit which components are used where:
```bash
for f in internal/ui/components/*.templ; do
  for fn in $(grep -oE '^templ [A-Za-z0-9_]+' "$f" | awk '{print $2}'); do
    echo "$(grep -rl "components\.$fn" internal/ui/pages | wc -l)  $fn"
  done
done | sort -n
```
A component used zero times is either dead (delete it — Phase 3 rule) or should
be adopted.

**Reduce card nesting.** The user reported excessive cards. A card inside a card
inside a card is the default output of unguided generation. One card per logical
group; sections within it use spacing, not more borders.

---

## TASK 5.4 — Fix the footer

**Audit PART 3.** The footer is defined in `customer.templ:238` with no height
constraint and renders on authenticated pages where it wastes vertical space.

1. **Authenticated dashboard shells** (customer, vendor, admin) get **no full
   footer** — at most a single-line bar with copyright and a couple of links, or
   nothing.
2. **Public/marketing pages** keep the full footer, but collapsed: on mobile the
   link groups become accordions.
3. Cap its height and let content own the viewport:
   `.site-footer { padding: var(--space-4) 0; }` and no fixed positioning.
4. Verify at 375px, 768px, 1280px that the footer occupies a reasonable share of
   the first viewport on both a marketing page and a dashboard page.

---

## TASK 5.5 — Final navigation architecture

By now the surface is minimal. Build the sidebars **once**, properly.

### Rules
1. Every top-level route appears in exactly one sidebar entry, or is deleted.
2. Detail routes (`/{id}`, `/{id}/edit`) are reached from their list.
3. Every entry is permission-aware — a user never sees a link that 404s for them.
4. Groups are collapsible; the group containing the active route starts open.
5. **Maximum depth: two.** Group → item. No third level.
6. Related items sit together; no orphan entries at the bottom.

### The three sidebars

**Admin** — Laravel's 10 groups (`PLAN_V5/06_PHASE_5_ADMIN.md` §5.2 has the
authoritative Arabic labels), minus everything Phase 3 deleted.

**Vendor** — currently 32 flat entries. Group them:
المنشأة والفروع · الكتالوج والمخزون · العروض والتسويق · الطلبات والمالية ·
المحتوى والسياسات · الأدوات

**Customer (pharmacy)** — verify against the Laravel customer layout. Remove the
cPanel entry (Phase 3). Point settings at `/settings?tab=…`.

### Produce `docs/PLAN_V7/NAVIGATION.md`

| Route | Sidebar group | Entry label | Permission | Reachable |
|---|---|---|---|---|

**Every route needs a non-empty "Reachable".** Then assert it:
```bash
# every registered top-level route appears in NAVIGATION.md
# and the dead-target scan returns 0
```

---

## TASK 5.6 — The full product walk

For **customer**, **vendor**, **staff**, on a seeded database:

### 5.6.1 Navigation
Click every sidebar entry. Target: zero non-200 for links visible to that actor.

### 5.6.2 Every interactive element
Every button, form, filter, search box, pagination control, modal. For each write
action, **check the database afterwards**. Record per page: elements found,
elements exercised, elements that did nothing.

### 5.6.3 The five states
For each list screen force and confirm: populated · empty (`EmptyState`) ·
error (`ErrorState`, **not** an empty list) · loading (`Skeleton`) · partial.

The empty-vs-error distinction is the most important check here.

### 5.6.4 Consistency
Two screens doing similar jobs must look and behave alike: same table style, same
filter placement, same button order, same modal shape, same empty-state voice.
List every inconsistency found and fix it.

### 5.6.5 Localization and RTL
Every page in `ar` and `en`. No untranslated string, no raw `{"ar":…}` JSON
leaking. RTL: direction, icon mirroring, number and date formatting, label
alignment. The language switch preserves the current page.

### 5.6.6 Responsive
375px, 768px, 1280px. Sidebar collapse, tables scroll or stack, touch targets
≥44px, no horizontal body scroll, footer behaves per Task 5.4.

Record everything in `docs/PLAN_V7/WALKTHROUGH.md`.

---

## PHASE 5 GATE — and the final gate for PLAN_V7

```bash
make check                    # includes check-inline-styles
DATABASE_URL="..." go test ./... -race
DATABASE_URL="..." go run ./cmd/migratecheck -from 1 -roundtrip
```

### The metrics — every one must have moved

| | Baseline | Target | Actual |
|---|---:|---:|---:|
| Routes | 447 | ≤ 330 | |
| Pages | 97 | ≤ 75 | |
| Tables | 161 | ≤ 140 | |
| Inline styles | 4,938 | ≤ 1,000 | |
| Data-less pages | 31 | 0 | |
| Dead tables | — | 0 | |
| Duplicate clusters | 9 | 0 | |
| Admin routes not in navigation | 162 | 0 | |

### The functional gate

- [ ] A vendor creates weekly coverage; a customer in range sees the offer
- [ ] The cart refuses out-of-stock, out-of-coverage and ineligible items, each with its own Arabic reason
- [ ] Checkout re-validates inside the order transaction; the race test passes
- [ ] One settings page, one branch management system, one policy editor, one user list, one category screen
- [ ] Category → brand filtering works and is enforced server-side
- [ ] Translations, cPanel and every dead file are gone
- [ ] Every sidebar entry resolves for the actor who can see it
- [ ] Every button either does something or is gone
- [ ] Empty and error states are visibly different on every list screen
- [ ] Zero test skips with `DATABASE_URL` set

---

## Closing note

The last two plans were executed as surface because their criteria could be
satisfied by a page that renders. This one is measured by **subtraction**: if
routes, pages, tables and inline styles have not gone down, the work did not
happen, regardless of what the commits say.

The goal is not a platform with more screens. It is a platform where every screen
has one job, one implementation, and a working connection to the database.
