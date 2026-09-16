# PHASE 4 — Product classification

**Depends on:** Phase 3 (the catalog screens are down to two).
**Audit §1.5.**

---

## The two problems

**1. One table looks like two systems.** `catalog.categories` has a
self-referencing `parent_id`. Root rows are "main categories", children are
sub-categories. The admin presents these as separate things, which is why "Main
Categories" appears to belong to nothing.

**2. There is no category→brand relationship.**
```sql
CREATE TABLE catalog.brands (
    id, public_id, name, description, image, status, created_at, updated_at, deleted_at
    -- no category_id
);
```
The behaviour you want — pick a category, then see only that category's brands —
cannot be built without a schema change. `catalog.products` carries both
`category_id` and `brand_id` but nothing constrains them to agree, so a product
can be in "مستحضرات تجميل" with a brand that only makes medical supplies.

---

## TASK 4.1 — Decide the brand↔category cardinality

**This is a product decision. Answer it before writing the migration.**

| Option | Shape | When it is right |
|---|---|---|
| **A — one category per brand** | `catalog.brands.category_id BIGINT REFERENCES catalog.categories(id)` | a manufacturer operates in exactly one segment |
| **B — many-to-many** | `catalog.brand_categories(brand_id, category_id)` | a manufacturer makes both cosmetics and medicines — **common in pharma** |

**Recommended: B.** Your own example (cosmetics, medicines, medical supplies)
describes segments that real manufacturers span. Option A would force
duplicate brand rows.

Check what Laravel does before committing:
```bash
sed -n "/CREATE TABLE \`brands\`/,/ENGINE=/p" "F:/Dawa 24/u924222867_Testv5.sql"
cat "F:/Dawa 24/Laravel/app/Models/Brand.php"
cat "F:/Dawa 24/Laravel/app/Models/Category.php"
grep -rn "brand" "F:/Dawa 24/Laravel/app/Livewire/Employee/VendorProductsImport.php" | head
```
If Laravel has no relationship either, this is new structure — say so in
`DECISIONS.md` and note that it is a deliberate improvement, not a port.

---

## TASK 4.2 — Migration

`db/migrations/NNN_brand_categories.up.sql`, with a working `.down.sql`.

For option B:
```sql
CREATE TABLE catalog.brand_categories (
    brand_id    BIGINT NOT NULL REFERENCES catalog.brands(id)     ON DELETE CASCADE,
    category_id BIGINT NOT NULL REFERENCES catalog.categories(id) ON DELETE CASCADE,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (brand_id, category_id)
);
CREATE INDEX brand_categories_category_idx ON catalog.brand_categories (category_id);

COMMENT ON TABLE catalog.brand_categories IS
  'ربط الشركات المصنعة بالتصنيفات — تحدد أي شركات تظهر عند اختيار تصنيف';
```

Backfill: every existing brand gets linked to the categories of the products that
already reference it, so nothing disappears from existing selectors:
```sql
INSERT INTO catalog.brand_categories (brand_id, category_id)
SELECT DISTINCT p.brand_id, p.category_id
FROM catalog.products p
WHERE p.brand_id IS NOT NULL AND p.category_id IS NOT NULL
ON CONFLICT DO NOTHING;
```

Not a tenant table — no RLS (reference data, like `categories` and `brands`).
Confirm those two do not have RLS today; match them.

Then `migratecheck -roundtrip`.

---

## TASK 4.3 — One category screen, showing the tree

Delete any separate "main categories" view. `/admin/categories` renders the
**whole tree**:

- root categories with expandable children (`@components.Timeline` or a nested list)
- create: name (ar/en), parent (optional), icon, image, sort order, status
- edit, soft-delete, reorder
- **guard against cycles** — a category cannot become its own ancestor. This is a
  domain rule with a unit test.
- show the product count per category (a real `COUNT`, not a literal)

The brands screen (`/admin/brands`) gains a **categories multi-select** writing
`catalog.brand_categories`.

---

## TASK 4.4 — Cascading selector in the product form

Wherever a product is created or edited — admin product form, vendor variant
form, ingest column mapping:

1. **Category first.** Categories load on page render.
2. **Brand second, filtered by category.** On category change, fetch
   `GET /api/v1/catalog/categories/{id}/brands`.
3. Changing the category **clears** a brand that is not valid for the new one,
   and says so.
4. **Server-side validation is the real rule**: on submit, reject a
   `(category_id, brand_id)` pair with no `brand_categories` row. The client
   filter is a convenience; it is not the constraint.

New endpoint:
```go
r.Get("/api/v1/catalog/categories/{id}/brands", h.ListBrandsByCategory)
```
in the catalog module's HTTP handler, gated like its neighbours.

### Tests

| Test | Assertion |
|---|---|
| D1 | the endpoint returns only brands linked to that category |
| D2 | creating a product with a valid pair succeeds |
| D4 | creating a product with a mismatched pair is **rejected server-side**, with an Arabic message |
| D1b | the category tree renders parents and children in one screen |
| unit | a category cannot be made its own ancestor |
| unit | the backfill links every brand that had products |

---

## TASK 4.5 — Seed the real category set

Currently categories may be empty or arbitrary. Seed the segments the platform
actually sells, from Laravel:
```bash
grep -rn "categories" "F:/Dawa 24/cities.sql" "F:/Dawa 24/u924222867_Testv5.sql" | head
```
If Laravel's dump has no category data, seed from the user's own list —
مستحضرات تجميل · أدوية · مستلزمات طبية — plus whatever the product catalogue
implies. Put it in a seed migration, bilingual, with `sort_order`.

---

## PHASE 4 GATE

```bash
make check && DATABASE_URL="..." go test ./internal/modules/catalog/... -race
DATABASE_URL="..." go run ./cmd/migratecheck -from <N> -roundtrip
```

- [ ] One category screen showing the full tree; no separate "main categories"
- [ ] `catalog.brand_categories` exists, backfilled, with a working down migration
- [ ] Selecting a category filters the brand list, everywhere a product is edited
- [ ] A mismatched category/brand pair is rejected **on the server**
- [ ] Cycle guard unit-tested
- [ ] Category product counts are real queries
- [ ] `DECISIONS.md` records the cardinality choice and whether it matches Laravel
