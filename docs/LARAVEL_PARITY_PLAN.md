# Dawa24 — Laravel Parity Plan

**Measured:** 2026-08-17, against the live database (48 migrations) and the
141-table legacy schema. Every gap below was confirmed with a query, not assumed.

This plan closes the distance between the Go system and the Laravel system in
**structure, naming, depth of logic, and the specific features you named**:
attachments, geography-based price offers, company/branch/employee hierarchy,
multi-category ratings, the jobs toggle, registration parity, and number
formatting.

---

# PART 0 — What is actually missing

The breadth is there — 15 modules, 48 migrations, ~140 page routes. What is
missing is **depth**: the tables exist but carry a fraction of the columns their
Laravel counterparts do, so the logic that depends on those columns cannot exist.

| Area | Laravel | Go now | Consequence |
|---|---|---|---|
| `cities` | `latitude`, `longitude`, `main_city_id`, `population`, `area`, `time_zone`, `is_capital`, `region` | `id, country_id, name, is_active` | **Distance-based pricing is impossible** |
| `weekly_coverages` | `latitude`, `longitude`, `city_id`, `distance` | `distance_meters` only, **no coordinates** | Coverage has a radius but no centre |
| `organization_reviews` | `product_id`, `title`, `comment`, `response`, `response_date`, `is_verified`, `is_public`, `images`, `status`, `helpful_votes` | `rating`, `review_text`, `is_approved` | No categories, no reply, no moderation |
| `documents` | `user_id`, `status`, `meta`, `deleted_at` | no `user_id`, no `status` | **Cannot attach to a user; admin cannot verify** |
| `users` | ~130 columns | ~15 | KYC, referrals, consent, HR fields absent |
| `organization_users` | `branch_id`, `supplier_role_id` | `branch_id` ✔, global `role_key` | No per-organization custom roles |
| `supplier_roles` / `supplier_permissions` | per-org roles | absent | Every org shares one role vocabulary |

## Three live defects to fix before anything else

**1. The chat schema was dropped while the module stayed mounted.**
`045_drop_chat.up.sql` runs `DROP SCHEMA IF EXISTS chat CASCADE`, but
`cmd/server/routes.go` still constructs `chatPostgres.NewRepository(db)` and
`/messages`, `/messages/{id}`, `/messages/{id}/send` and `/admin/messages` are
still registered. The pages return 303 to login when signed out, which masks it —
**signed in, they will 500** on `relation "chat.conversations" does not exist`.
Decide: restore the schema (migration 049) or remove the module and its four
routes. Do not leave it half-mounted.

**2. There is no attachment upload path except spreadsheet import.**
`storage.PresignPut` has exactly one caller — `ingest.PresignUpload`. There is no
endpoint for organization licences, user KYC, avatars, product images, review
images or CVs. `organizations.license_document_url` is a bare `TEXT` column added
in migration 047, and `/admin/approvals` renders a link to it — **but nothing in
the product can populate it.** The admin reviewer sees "غير مرفق" forever.

**3. Money renders without separators.** `money.Amount.String()` returns
`1234567.89`. Every price, total and balance on the platform reads as an
undifferentiated digit run.

---

# PART 1 — Rules

Unchanged from `BUILD_PHASES.md` Part 1, plus:

12. **Coordinates are `NUMERIC(10,8)` / `NUMERIC(11,8)`**, matching Laravel
    exactly. Never `float64` in Go — use a fixed-point type or `decimal`.
13. **Distances are integer metres.** No floats, no kilometres.
14. **Every attachment is a row in `platform_admin.documents`**, never a bare URL
    column. A URL column cannot be verified, revoked or audited.
15. **Arabic naming in the UI matches Laravel's wording**, listed per section
    below. Do not invent new terms for concepts that already have one.

---

# PHASE A — Attachments and MinIO

**Largest functional gap. Registration cannot attach anything today.**

## A.1 — Rebuild `platform_admin.documents` to Laravel's shape

**Migration `049_documents_parity.up.sql`**

```sql
ALTER TABLE platform_admin.documents
    ADD COLUMN IF NOT EXISTS user_id        BIGINT REFERENCES identity.users(id) ON DELETE CASCADE,
    ADD COLUMN IF NOT EXISTS status         TEXT NOT NULL DEFAULT 'pending'
        CHECK (status IN ('pending','verified','rejected')),
    ADD COLUMN IF NOT EXISTS review_notes   TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS reviewed_by    BIGINT REFERENCES identity.users(id) ON DELETE SET NULL,
    ADD COLUMN IF NOT EXISTS reviewed_at    TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS mime_type      TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS size_bytes     BIGINT NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS original_name  TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS meta           JSONB NOT NULL DEFAULT '{}'::JSONB,
    ADD COLUMN IF NOT EXISTS deleted_at     TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS updated_at     TIMESTAMPTZ NOT NULL DEFAULT now();

-- At least one owner. A document belonging to neither a user nor an
-- organization cannot be found again, reviewed, or revoked.
ALTER TABLE platform_admin.documents
    ADD CONSTRAINT documents_has_owner
    CHECK (user_id IS NOT NULL OR organization_id IS NOT NULL);

CREATE INDEX documents_org_type_idx  ON platform_admin.documents (organization_id, document_type, status);
CREATE INDEX documents_user_type_idx ON platform_admin.documents (user_id, document_type, status);
```

`document_type` vocabulary, matching Laravel's usage:
`commercial_register` · `tax_card` · `pharmacist_license` · `pharmacy_license` ·
`national_id` · `passport` · `bank_certificate` · `authorization_letter` ·
`avatar` · `organization_logo` · `product_image` · `review_image` · `cv` ·
`import_file` · `other`

## A.2 — A generic upload module

New: `internal/modules/attachments/{domain.go, repository.go, service.go, postgres/, http/}`.

The ingest module owns a private copy of the presign logic. Lift it out so every
feature uses one path.

**Endpoints:**

| Method | Path | Purpose |
|---|---|---|
| `POST` | `/api/v1/attachments/presign` | Issue a presigned PUT and a pending row |
| `POST` | `/api/v1/attachments/{id}/confirm` | Mark uploaded after the PUT succeeds |
| `GET` | `/api/v1/attachments/{id}` | Presigned GET, short-lived |
| `DELETE` | `/api/v1/attachments/{id}` | Soft delete; object stays for audit |
| `GET` | `/api/v1/admin/attachments` | Cross-tenant list with filters |
| `POST` | `/api/v1/admin/attachments/{id}/verify` | Verify or reject with notes |

**Presign request:** `document_type`, `original_name`, `mime_type`,
`size_bytes`, optional `organization_id`.

**Server-side validation before issuing a URL** — a presigned PUT is a blank
cheque otherwise:

- **MIME allowlist per `document_type`.** Licences: `application/pdf`,
  `image/jpeg`, `image/png`. Images: `image/*` only. Never `text/html` or
  `application/octet-stream`.
- **Size ceiling per type** — 10 MB for documents, 5 MB for images, 50 MB for
  import files. Enforced again on confirm against the object's real size from
  `HeadObject`; the client-declared size is a hint, not a fact.
- **Key layout:** `orgs/{orgID}/{document_type}/{uuid}{ext}` or
  `users/{userID}/{document_type}/{uuid}{ext}`. Never the user's filename in the
  key — `../` and unicode tricks live there.
- **Ownership:** the actor must own the target user or belong to the target
  organization. Check with `authctx`, not the request body.

**Confirm** calls `HeadObject`; if the object is absent or exceeds the ceiling,
the row is deleted and the caller gets a validation error. **A row that says
"uploaded" while MinIO has nothing is worse than no row.**

## A.3 — The upload component

Generalise `components/filedropzone.templ` into `AttachmentUpload`, taking
`document_type`, accept list, max size and a target. Behaviour: drag-drop and
click, **real progress from the XHR upload event** (not a timer), thumbnail for
images and a file chip for PDFs, remove before submit, retry on failure, and an
Arabic error for each rejection reason.

## A.4 — Wire it everywhere

| Screen | Attachments |
|---|---|
| Registration step 3 | commercial register, tax card, pharmacist licence, pharmacy licence |
| `/settings/profile` | avatar, national ID |
| `/settings/organization` | logo, cover, bank certificate, authorization letter |
| Product editor | product images, multiple, ordered |
| Review form | up to 4 images |
| `/jobs/{id}/apply` | CV |
| `/requests` | request attachments |

## A.5 — Admin document management — `/admin/documents`

Filters by organization, user, type and status. Row shows a thumbnail or file
icon, owner, type, size, upload date, status. Preview opens a presigned GET in a
modal (PDF inline, images inline). Verify/reject writes `status`,
`review_notes`, `reviewed_by`, `reviewed_at` **and an audit row in the same
transaction**. Bulk verify for a whole organization.

`/admin/approvals` gains a documents panel per organization — the reviewer sees
every uploaded licence in place, rather than a link to a column nothing fills.

## A.6 — MinIO operational

`STORAGE_*` is read and the client is constructed, but nothing else is verified.
Create the bucket on boot if absent. Set a lifecycle rule expiring
`tmp/` after 24 h. Confirm CORS allows `PUT` from the app origin — **a presigned
PUT from the browser fails on CORS before it ever reaches MinIO**, and the error
surfaces as a generic network failure. Add a `storage` check to `/healthz`.

**Done when:** a pharmacy registers with four documents attached, an admin sees
all four on the approvals screen, previews each, verifies three and rejects one
with a note, and the rejected one shows the note back to the pharmacy.

---

# PHASE B — Geography and عرض الأسعار

The distance-based price offer cannot be built today: cities have no
coordinates and coverage areas have no centre point.

## B.1 — Coordinates on cities

**Migration `050_geography.up.sql`**

```sql
ALTER TABLE platform_admin.cities
    ADD COLUMN IF NOT EXISTS latitude     NUMERIC(10,8),
    ADD COLUMN IF NOT EXISTS longitude    NUMERIC(11,8),
    ADD COLUMN IF NOT EXISTS main_city_id BIGINT REFERENCES platform_admin.cities(id) ON DELETE SET NULL,
    ADD COLUMN IF NOT EXISTS region       JSONB NOT NULL DEFAULT '{"ar":"","en":""}'::JSONB,
    ADD COLUMN IF NOT EXISTS time_zone    TEXT NOT NULL DEFAULT 'Africa/Cairo',
    ADD COLUMN IF NOT EXISTS is_capital   BOOLEAN NOT NULL DEFAULT false,
    ADD COLUMN IF NOT EXISTS population   INT,
    ADD COLUMN IF NOT EXISTS area_km2     NUMERIC(12,3);

ALTER TABLE workflow.weekly_coverages
    ADD COLUMN IF NOT EXISTS city_id   BIGINT REFERENCES platform_admin.cities(id) ON DELETE SET NULL,
    ADD COLUMN IF NOT EXISTS latitude  NUMERIC(10,8),
    ADD COLUMN IF NOT EXISTS longitude NUMERIC(11,8);

CREATE EXTENSION IF NOT EXISTS cube;
CREATE EXTENSION IF NOT EXISTS earthdistance;

-- Metres between two points, as an integer. The unit is metres everywhere in
-- this system; kilometres and floats do not appear.
CREATE OR REPLACE FUNCTION platform.distance_meters(
    lat1 NUMERIC, lon1 NUMERIC, lat2 NUMERIC, lon2 NUMERIC
) RETURNS INT LANGUAGE SQL IMMUTABLE AS $$
    SELECT CASE
        WHEN lat1 IS NULL OR lon1 IS NULL OR lat2 IS NULL OR lon2 IS NULL THEN NULL
        ELSE (earth_distance(
            ll_to_earth(lat1::float8, lon1::float8),
            ll_to_earth(lat2::float8, lon2::float8)
        ))::int
    END;
$$;

CREATE INDEX cities_earth_idx ON platform_admin.cities
    USING gist (ll_to_earth(latitude::float8, longitude::float8))
    WHERE latitude IS NOT NULL;
```

Backfill Egyptian governorate coordinates in the same migration.

## B.2 — The coverage model

A supplier declares, per branch and per weekday, a **centre point** and a
**radius in metres**. A pharmacy is served on that day when the distance from the
coverage centre to the pharmacy's branch is within the radius.

`internal/modules/workflow/coverage_service.go`:

```go
// ServesPoint reports whether a coverage area reaches a point on a given day.
// Distance is metres; the radius is metres. Nothing here is kilometres.
func (s *Service) ServesPoint(ctx context.Context, orgID int64, day time.Weekday, lat, lon Coord) (bool, int, error)
```

Query pattern — filter with the GiST index first, then compute exactly:

```sql
SELECT wc.id, wc.branch_id, wc.distance_meters,
       platform.distance_meters(wc.latitude, wc.longitude, $2, $3) AS metres
FROM workflow.weekly_coverages wc
WHERE wc.organization_id = $1
  AND wc.day_of_week = $4
  AND wc.is_active
  AND wc.latitude IS NOT NULL
  AND earth_box(ll_to_earth(wc.latitude::float8, wc.longitude::float8), wc.distance_meters)
      @> ll_to_earth($2::float8, $3::float8)
  AND platform.distance_meters(wc.latitude, wc.longitude, $2, $3) <= wc.distance_meters
ORDER BY metres ASC;
```

## B.3 — عرض الأسعار — the quotation flow

Laravel's `request_offers` is an empty stub, so the behaviour comes from `offers`
+ `weekly_coverages`. Build it properly on `commerce.quote_requests`.

**Migration `051_quotations.up.sql`** — extend `commerce.quote_requests`:

`delivery_lat`, `delivery_lon`, `delivery_city_id`, `delivery_branch_id`,
`requested_for_date`, `distance_meters` (computed and stored at quote time),
`delivery_fee` (money), `expires_at`, `rejection_reason`.

**Flow:**

1. A pharmacy requests a quote for products, from a branch with coordinates.
2. The system finds suppliers whose coverage reaches that point on the requested
   day, ordered by distance.
3. Each supplier sees the request with **the distance in metres** and the
   pharmacy's city.
4. The supplier quotes a unit price and a delivery fee. **The delivery fee may be
   banded by distance** — configure bands per organization
   (`org.delivery_bands`: `from_meters`, `to_meters`, `fee`).
5. The pharmacy compares quotes side by side: price, delivery fee, total,
   distance, expected day, supplier rating.
6. Accept converts the quote into an order.

`/vendor/coverage` manages the weekly grid: seven rows, each with a map pin, a
radius in metres, hours, and a live preview of how many pharmacies fall inside.

## B.4 — Maps

Currently broken because there is nothing to render. Requirements:

- **One map component**, `components/map_picker.templ`, taking lat/lon/radius and
  emitting changes. Used by branch editing, coverage editing, address picking and
  the supplier profile.
- **A single API key** in `MAPS_API_KEY`, injected server-side into the page, never
  committed. If absent, the component degrades to two numeric inputs and a note —
  it must not render a broken grey box.
- **Reverse geocode on pin drop** to fill the address field, with the raw response
  stored in `meta` so it can be re-parsed later.
- **Radius circle drawn in metres**, matching the stored value exactly.
- Arabic map labels where the provider supports it.

**Done when:** a supplier draws a 5 km Sunday coverage around Nasr City, a
pharmacy 3 km away sees them in quotes and one 8 km away does not, and the
quote shows "٣٫٢ كم" computed from stored coordinates.

---

# PHASE C — Company, branches and employees

Laravel's shape: an organization has branches; `organization_users` links a user
to an organization **and optionally to one branch**, with a `supplier_role_id`
that is defined **per organization**.

`org.members` already has `branch_id`. What is missing is per-organization roles.

## C.1 — Per-organization roles

**Migration `052_org_roles.up.sql`** — schema `org`:

```
roles         id, organization_id, key, name JSONB, description, is_system, created_at
role_permissions  role_id, permission_key
```

Seeded per organization on registration from a template: `org_owner`,
`org_manager`, `org_pharmacist`, `org_accountant`, `org_warehouse`,
`org_sales_rep`, `org_delivery`. An owner may then add their own —
"مسؤول المشتريات" — without affecting any other tenant.

`org.members.role_key` becomes `org.members.org_role_id` referencing
`org.roles(id)`, with the old column kept for one release and backfilled.

**Permission resolution** (`GetPermissionsForUser`) becomes:
platform role → organization role → branch scope. A member with `branch_id` set
sees **only that branch's** stock, orders and transfers.

## C.2 — Branch scoping through the stack

Every tenant-scoped query gains an optional branch predicate. `authctx.Actor`
gains `BranchID *int64`, set at login from the membership. Where it is non-nil:

- `inventory.stocks` filtered to the branch's warehouses
- `commerce.order_shipments` filtered to the branch
- `catalog.product_variants` filtered by `branch_id`
- `workflow.weekly_coverages` limited to the branch

## C.3 — Employee management — `/settings/employees`

List with branch, role, status, last activity. Invite by email or phone with a
role and branch. Employee detail: `employee_code`, `job_title`, `base_salary`,
`variable_salary` (Laravel has all four on `users`; put them on `org.members`
where they belong). Suspend, reassign branch, change role — each writing audit.

`hr.employees` already exists; reconcile it with `org.members` rather than
keeping two employee concepts. **One of them must win — `org.members` is the
membership, `hr.employees` is the HR record; link them by `member_id`.**

**Done when:** a chain pharmacy with three branches has a manager per branch who
sees only their own branch's orders and stock, and the owner sees all three.

---

# PHASE D — Multi-category ratings

Laravel has a flat 1–5 with a comment. You want categories per context. Build the
richer model.

## D.1 — Schema

**Migration `053_reviews.up.sql`** — extend `org.organization_reviews` to
Laravel's columns and add categories:

```sql
ALTER TABLE org.organization_reviews
    ADD COLUMN IF NOT EXISTS order_id      BIGINT REFERENCES commerce.orders(id) ON DELETE SET NULL,
    ADD COLUMN IF NOT EXISTS product_id    BIGINT REFERENCES catalog.products(id) ON DELETE SET NULL,
    ADD COLUMN IF NOT EXISTS title         TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS response      TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS response_at   TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS responded_by  BIGINT REFERENCES identity.users(id) ON DELETE SET NULL,
    ADD COLUMN IF NOT EXISTS is_verified   BOOLEAN NOT NULL DEFAULT false,
    ADD COLUMN IF NOT EXISTS is_public     BOOLEAN NOT NULL DEFAULT true,
    ADD COLUMN IF NOT EXISTS status        TEXT NOT NULL DEFAULT 'pending'
        CHECK (status IN ('pending','approved','rejected')),
    ADD COLUMN IF NOT EXISTS helpful_count INT NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS context       TEXT NOT NULL DEFAULT 'supplier'
        CHECK (context IN ('supplier','pharmacy','product','delivery'));

-- One review per buyer per order. Without this a single purchase can be rated
-- repeatedly and the average is worthless.
CREATE UNIQUE INDEX reviews_one_per_order
    ON org.organization_reviews (user_id, order_id)
    WHERE order_id IS NOT NULL AND deleted_at IS NULL;

CREATE TABLE org.review_criteria (
    key         TEXT PRIMARY KEY,
    name        JSONB NOT NULL,
    context     TEXT NOT NULL,
    weight      NUMERIC(4,3) NOT NULL DEFAULT 1.000,
    sort_order  INT NOT NULL DEFAULT 0,
    is_active   BOOLEAN NOT NULL DEFAULT true
);

CREATE TABLE org.review_ratings (
    review_id    BIGINT NOT NULL REFERENCES org.organization_reviews(id) ON DELETE CASCADE,
    criterion    TEXT NOT NULL REFERENCES org.review_criteria(key),
    score        SMALLINT NOT NULL CHECK (score BETWEEN 1 AND 5),
    PRIMARY KEY (review_id, criterion)
);
```

**Seeded criteria:**

| Context | key | Arabic |
|---|---|---|
| supplier | `delivery_speed` | سرعة التوصيل |
| supplier | `product_quality` | جودة المنتجات |
| supplier | `rep_service` | تعامل المندوب |
| supplier | `price_fairness` | مناسبة الأسعار |
| supplier | `order_accuracy` | دقة تنفيذ الطلب |
| supplier | `packaging` | جودة التغليف |
| pharmacy | `payment_commitment` | الالتزام بالسداد |
| pharmacy | `communication` | سهولة التواصل |
| pharmacy | `receiving_speed` | سرعة الاستلام |
| product | `effectiveness` | فعالية المنتج |
| product | `matches_description` | مطابقة الوصف |
| product | `expiry_period` | فترة الصلاحية |

The overall `rating` becomes the weighted mean of the criteria, computed on
write. Never store an average you cannot recompute.

## D.2 — Eligibility

A review is only accepted when the reviewer **actually bought**: there is an
order between the reviewer's organization and the reviewed organization, in a
terminal status (`delivered` / `completed`), within the last 90 days, and not yet
reviewed. Enforce in the service, not only in the UI.

**Both directions.** A supplier rates the pharmacy that bought from them
(`context = 'pharmacy'`, criteria about payment and communication). This is the
half Laravel does not have and is worth adding.

## D.3 — Surfaces

- **Prompt after delivery** — a card on the order detail and a notification.
- **Review modal**: five stars per criterion with Arabic labels, a title, a
  comment, up to four images (Phase A), and a public/anonymous choice.
- **Supplier profile**: overall average large, a bar per criterion, distribution
  histogram, filterable list, and the supplier's replies inline.
- **Reply** from `/vendor/reviews`, once per review, editable for 24 h.
- **Moderation** at `/admin/reviews` — approve, reject with reason, hide.
- **Aggregates** maintained on `org.organizations`: `rating_average`,
  `rating_count`, and a JSONB `rating_breakdown` per criterion, recomputed in the
  same transaction as the review.

**Done when:** a pharmacy that received an order rates six criteria, the supplier
profile shows six bars and a weighted average, the supplier replies, and a second
review attempt on the same order is refused.

---

# PHASE E — Jobs toggle and باحث عن عمل

## E.1 — A real feature-flag system

`platform_admin.system_settings` exists but nothing reads it as a flag.

**Migration `054_feature_flags.up.sql`**

```sql
CREATE TABLE platform_admin.feature_flags (
    key           TEXT PRIMARY KEY,
    name          JSONB NOT NULL,
    description   JSONB NOT NULL DEFAULT '{"ar":"","en":""}'::JSONB,
    is_enabled    BOOLEAN NOT NULL DEFAULT true,
    updated_by    BIGINT REFERENCES identity.users(id) ON DELETE SET NULL,
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

INSERT INTO platform_admin.feature_flags (key, name, description, is_enabled) VALUES
  ('jobs.enabled', '{"ar":"وظائف","en":"Jobs"}',
   '{"ar":"تفعيل قسم الوظائف وإعلانات التوظيف","en":"Job board and postings"}', true),
  ('jobs.seeker_accounts', '{"ar":"حسابات باحث عن عمل","en":"Job seeker accounts"}',
   '{"ar":"السماح بالتسجيل كباحث عن عمل","en":"Allow job seeker registration"}', true),
  ('reviews.enabled',  '{"ar":"التقييمات","en":"Reviews"}', '{"ar":"","en":""}', true),
  ('offers.enabled',   '{"ar":"العروض","en":"Offers"}',     '{"ar":"","en":""}', true),
  ('finder.enabled',   '{"ar":"دليل الأدوية","en":"Product finder"}', '{"ar":"","en":""}', true),
  ('services.enabled', '{"ar":"الخدمات المؤسسية","en":"Institutional services"}', '{"ar":"","en":""}', true),
  ('compare.enabled',  '{"ar":"مقارنة العروض","en":"Compare plans"}', '{"ar":"","en":""}', true);
```

## E.2 — Enforcement, not decoration

A toggle that only hides a menu item is not a toggle. Three layers:

1. **`internal/platform/features`** — loads all flags into memory at boot,
   refreshes every 60 s and on write, and exposes
   `features.Enabled(ctx, "jobs.enabled") bool`. In-memory because it is read on
   every request; a database round trip per nav render is not acceptable.
2. **Route middleware** `features.Require("jobs.enabled")` wrapping the jobs
   route group. Disabled ⇒ **404**, not 403 — a disabled feature should not
   advertise that it exists.
3. **Navigation and registration** consult the same function, so the menu item,
   the account-type card and the route agree.

## E.3 — The باحث عن عمل account type

A fourth registration type, gated on `jobs.seeker_accounts`:

- Registers a **user with no organization** — `role = 'job_seeker'`.
- Registration collects: name, email, phone, city, specialisation
  (صيدلي / مساعد صيدلي / مندوب مبيعات / محاسب / أمين مخزن), years of experience,
  and a **CV attachment** (Phase A).
- Lands on `/jobs`, not a dashboard.
- Profile at `/settings/profile` gains CV, specialisation, experience and an
  "open to work" switch.
- `/my-applications` lists their applications with status.

**Migration `055_job_seekers.up.sql`** — `hr.job_seeker_profiles`: `user_id`,
`specialisation`, `years_experience`, `cv_document_id`, `is_open_to_work`,
`expected_salary`, `preferred_city_id`, `bio`.

When `jobs.enabled` is off: `/jobs*` 404s, the nav item disappears, the account
type is not offered, **and existing job-seeker accounts can still sign in and
reach their profile** — disabling a feature must not lock people out of their
own account.

**Done when:** the admin turns jobs off in `/admin/settings`, and within 60
seconds `/jobs` returns 404, the nav item is gone, the registration card is gone,
and a job seeker can still sign in.

---

# PHASE F — Registration parity

Laravel's `users` table has ~130 columns against ~15 here. Not all are worth
carrying — `telescope_*`, `facebook_id`, `api_token` are noise. These are not:

## F.1 — Fields to add

**Migration `056_user_parity.up.sql`** — `identity.users` and
`profile.user_profiles`:

| Field | Where | Why |
|---|---|---|
| `first_name`, `last_name` | users | Laravel splits; the UI needs both |
| `date_of_birth`, `gender`, `nationality` | profile | KYC |
| `national_id`, `passport_number` | profile | KYC, encrypted at rest |
| `secondary_phone`, `whatsapp` | profile | Primary contact channel in Egypt |
| `latitude`, `longitude`, `radius` | profile | Laravel's per-user service radius |
| `employee_code`, `job_title`, `base_salary`, `variable_salary` | `org.members` | HR — belongs to membership |
| `kyc_status`, `kyc_notes`, `identity_verified_at` | users | `identity.kyc_records` exists; wire it |
| `terms_accepted_at`, `terms_version`, `privacy_accepted_at` | users | Legal record of consent |
| `marketing_consent`, `newsletter` | preferences | Consent |
| `referral_code`, `referred_by`, `referral_count` | users | Referral programme |
| `total_orders`, `total_spent`, `last_purchase_at` | **derived, not stored** | Laravel stores them and they drift |

## F.2 — Registration by type, matching Laravel

| Step | supplier | pharmacy | chain | job seeker |
|---|---|---|---|---|
| 1 Account type | ✔ | ✔ | ✔ | ✔ (if enabled) |
| 2 Personal | name, email, phone, password | same | same | + specialisation, experience |
| 3 Organization | legal name, trade name ar/en, commercial register, tax number, address, city, **map pin** | + pharmacist licence, pharmacy licence number | + branch count | — |
| 4 Documents | commercial register, tax card | + pharmacist licence, pharmacy licence | same | CV |
| 5 Terms | version recorded | ✔ | ✔ | ✔ |

Each step validates before advancing and **preserves entries on failure**.
Progress is saved so a refresh does not lose everything. `organization_number` is
generated on approval, matching Laravel's format.

---

# PHASE G — Naming and structural parity

## G.1 — Arabic terminology

Use Laravel's wording exactly. Where the current UI has invented a term, change it.

| Concept | Use | Not |
|---|---|---|
| Supplier organization | مورّد | بائع |
| Pharmacy | صيدلية | عميل |
| Branch | فرع | مقر |
| Employee | موظف | عضو |
| Quote request | طلب عرض سعر | استفسار |
| Quotation | عرض سعر | تسعيرة |
| Coverage area | نطاق التغطية | منطقة الخدمة |
| Order | طلب | أوردر |
| Shipment | شحنة | إرسالية |
| Wallet | المحفظة | الرصيد |
| Invoice | فاتورة | إيصال |
| Commercial register | السجل التجاري | التسجيل |
| Tax card | البطاقة الضريبية | الرقم الضريبي |
| Job seeker | باحث عن عمل | متقدم |

## G.2 — Organization columns Laravel has and we do not

`organization_number`, `description`, `image`, `coverage_image`, `settings`
(JSONB), `min_order_price`, `max_order_price`, `is_sponsored` +
`sponsored_start_at`/`_end_at`, `rank`, `main`, `first_time_upload_file`.

`min_order_price` matters: it gates checkout per supplier and the cart must warn
when a supplier's subtotal is below it.

---

# PHASE H — Number and date formatting

## H.1 — One formatting package

`internal/shared/format`:

```go
// Money renders an amount for display: grouped thousands, two decimals, and
// the currency symbol on the correct side for the language.
//   money.FromMinor(123456789) => "١٢٣٤٥٦٧٫٨٩ ج.م"  (ar)
//                              => "1,234,567.89 EGP" (en)
func Money(a money.Amount, lang string) string

// Integer groups thousands: 1234567 => "1,234,567" / "١٬٢٣٤٬٥٦٧"
func Integer(n int64, lang string) string

// Distance renders metres as metres under 1 km and kilometres above,
// one decimal: 3200 => "٣٫٢ كم"
func Distance(metres int, lang string) string

// RelativeTime: "منذ ٥ دقائق", "منذ ٣ أيام"
func RelativeTime(t time.Time, lang string) string

// Date, DateTime: Gregorian with Arabic month names in ar.
func Date(t time.Time, lang string) string
```

**Decision to make and apply consistently:** Arabic-Indic digits (٠١٢٣) or
Western (0123) in the Arabic UI. Laravel uses Western digits with Arabic labels;
recommend matching that and keeping `tabular-nums` for alignment. Whichever you
pick, one function decides it.

`components.MoneyDisplay` calls `format.Money`. Every `.String()` on money in a
template is replaced. Every raw `fmt.Sprintf("%d")` on a count is replaced with
`format.Integer`.

---

# PART 2 — Order of execution

1. **Part 0 defects** — chat decision, then Phase A (attachments), then H
   (formatting). A is the biggest functional hole; H is a day and improves every
   screen.
2. **Phase B** — geography, then quotations. Blocks the marketplace's core value.
3. **Phase C** — company/branch/employee depth.
4. **Phase D** — ratings.
5. **Phase E** — jobs toggle and job seekers.
6. **Phase F** — registration parity.
7. **Phase G** — naming sweep, done continuously as screens are touched.

**Scale:** ~8 migrations, 2 new modules (`attachments`, `features`), ~35
endpoints, ~12 screens, one formatting package applied everywhere.

# PART 3 — Reporting

Per session: what was completed, files changed, **commands run with real
output**, anything contradicting this document with evidence, and what is
blocked. Run the gate before claiming anything done:

```bash
templ generate && go build ./... && go vet ./...
```
