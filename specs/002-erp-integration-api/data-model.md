# Data Model: ERP Integration API

Migrations (next free number is 255; the last applied is `254_saving_guc_policies`). Each is `NNN_name.up.sql` plus `.down.sql`, wrapped in `BEGIN; ... COMMIT;`. Slices ship in this order.

| No. | Name | Slice | Contents |
|---|---|---|---|
| 255 | `255_erp_schema` | S1 | schema `erp`, all nine tables below, grants |
| 256 | `256_audit_actor_kind` | S1 | two columns on `platform.audit_log` |
| 257 | `257_user_org_erp_code_index` | S5 | lookup index on `org.user_organizations` |
| 258 | `258_order_number_sequence` | S7 | `commerce.order_number_seq` |

Rules for every `CREATE TABLE` in these files (they are parsed by `test/schema_consistency_test.go`): the closing `);` sits alone on its own line, and every table gets `COMMENT ON TABLE` and one `COMMENT ON COLUMN` per column in Arabic (below).

Standard comments for any column with a blank Arabic cell: `id` = المعرف الداخلي; `public_id` = المعرف العام; `organization_id` = المنشأة المالكة للسجل; `created_at` = تاريخ الإنشاء; `updated_at` = تاريخ آخر تعديل; `position` and other unlisted columns: use the cell text.

Every table in 255 has `organization_id BIGINT NOT NULL REFERENCES org.organizations(id) ON DELETE CASCADE`, then:
```
ALTER TABLE erp.<t> ENABLE ROW LEVEL SECURITY;
ALTER TABLE erp.<t> FORCE ROW LEVEL SECURITY;
CREATE POLICY <t>_tenant_isolation ON erp.<t>
    USING (platform.tenant_visible(organization_id))
    WITH CHECK (platform.tenant_visible(organization_id));
```
Touch trigger on tables with `updated_at`: `CREATE TRIGGER <t>_touch BEFORE UPDATE ON erp.<t> FOR EACH ROW EXECUTE FUNCTION platform.touch_updated_at();`

Migration 255 ends with the app-role grant, because `222_dawa24_app_role.up.sql` lists schemas by name and does not know `erp`:
```
DO $$ BEGIN IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname='dawa24_app') THEN
  GRANT USAGE ON SCHEMA erp TO dawa24_app;
  GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA erp TO dawa24_app;
  GRANT USAGE, SELECT, UPDATE ON ALL SEQUENCES IN SCHEMA erp TO dawa24_app;
  ALTER DEFAULT PRIVILEGES IN SCHEMA erp GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES TO dawa24_app;
  ALTER DEFAULT PRIVILEGES IN SCHEMA erp GRANT USAGE, SELECT, UPDATE ON SEQUENCES TO dawa24_app;
END IF; END $$;
```
`255.down`: `DROP SCHEMA IF EXISTS erp CASCADE;`

## erp.integrations
One row per organization and provider. Unique: `(organization_id, provider) WHERE status <> 'closed'`; unique `public_id`.

| Column | Type | Constraint | Arabic comment |
|---|---|---|---|
| id | BIGINT identity | PK | المعرف الداخلي للربط |
| public_id | UUID | NOT NULL DEFAULT gen_random_uuid() | المعرف العام للربط |
| organization_id | BIGINT | see above | المنشأة المالكة للربط |
| provider | TEXT | NOT NULL, CHECK `provider ~ '^[a-z][a-z0-9_]{2,30}$'` | اسم مزود نظام الـ ERP مثل orgasoft |
| status | TEXT | NOT NULL DEFAULT 'active', CHECK in (active, paused, closed) | حالة الربط: نشط أو موقوف أو مغلق |
| created_by | BIGINT | FK identity.users ON DELETE SET NULL | المستخدم الذي أنشأ الربط |
| created_at / updated_at | TIMESTAMPTZ | NOT NULL DEFAULT now() | تاريخ الإنشاء / آخر تعديل |

States: `active` to `paused` and back; either to `closed` (final). A paused or closed integration refuses every key (`ERP_INTEGRATION_PAUSED`).

## erp.api_keys
Unique `key_prefix`; unique `public_id`. Index `(organization_id, status)`; index `(integration_id)`.

| Column | Type | Constraint | Arabic comment |
|---|---|---|---|
| id, public_id, organization_id | | as above | المعرف الداخلي / العام / المنشأة |
| integration_id | BIGINT | NOT NULL FK erp.integrations ON DELETE CASCADE | الربط الذي ينتمي إليه المفتاح |
| name | TEXT | NOT NULL, CHECK length 1 to 80 | اسم وصفي للمفتاح يختاره المالك |
| key_prefix | TEXT | NOT NULL UNIQUE, CHECK `~ '^[a-z0-9]{8}$'` | بادئة المفتاح الظاهرة للعرض والبحث |
| key_hash | TEXT | NOT NULL, CHECK length = 64 | بصمة HMAC-SHA256 للجزء السري ولا يمكن استرجاعه |
| scopes | TEXT[] | NOT NULL, CHECK cardinality >= 1 | الصلاحيات الممنوحة للمفتاح |
| warehouse_id | BIGINT | NOT NULL FK inventory.warehouses | المستودع الذي تكتب إليه المزامنة |
| branch_id | BIGINT | NOT NULL FK org.branches | الفرع المرتبط بالمفتاح |
| buying_user_id | BIGINT | FK identity.users ON DELETE SET NULL | المستخدم الذي تُسجَّل باسمه أوامر الشراء |
| rate_limit_per_minute | INT | NOT NULL DEFAULT 600, CHECK 10 to 6000 | حد الطلبات في الدقيقة لهذا المفتاح |
| max_batch_items | INT | NOT NULL DEFAULT 50000, CHECK 1 to 50000 | أقصى عدد أصناف في دفعة مزامنة واحدة |
| status | TEXT | NOT NULL DEFAULT 'active', CHECK in (active, revoked) | حالة المفتاح |
| expires_at | TIMESTAMPTZ | | وقت انتهاء صلاحية المفتاح إن وُجد |
| last_used_at | TIMESTAMPTZ | | آخر استخدام ناجح للمفتاح |
| last_used_ip | INET | | عنوان آخر استخدام |
| revoked_at | TIMESTAMPTZ | | وقت الإلغاء |
| revoked_by | BIGINT | FK identity.users ON DELETE SET NULL | من ألغى المفتاح |
| revoke_reason | TEXT | NOT NULL DEFAULT '' | سبب الإلغاء |
| rotated_from_id | BIGINT | FK erp.api_keys(id) ON DELETE SET NULL | المفتاح القديم الذي حل هذا محله |
| created_by | BIGINT | FK identity.users ON DELETE SET NULL | من أنشأ المفتاح |
| created_at / updated_at | TIMESTAMPTZ | NOT NULL DEFAULT now() | |

States: `active` to `revoked` (final). Rotation creates a new `active` key (`rotated_from_id` set) that copies scopes, warehouse, branch, buying user, rate limit and max batch items from the old key, and sets `expires_at = now() + grace` on the old key (grace default 24 h, 0 to 72 h). The owner can change `rate_limit_per_minute` later (range 10 to 6000) with `UpdateKeyLimits`; every change writes an audit row. A key is usable when `status='active'` and (`expires_at IS NULL` or in the future). `last_used_at` is updated at most once per minute per key.

## erp.product_links
Unique `(integration_id, external_code)`; unique `(integration_id, variant_id)`; index `(organization_id)`.

| Column | Type | Constraint | Arabic comment |
|---|---|---|---|
| id, organization_id | | | |
| integration_id | BIGINT | NOT NULL FK erp.integrations ON DELETE CASCADE | الربط صاحب الكود |
| external_code | TEXT | NOT NULL, CHECK length 1 to 64 | كود الصنف في نظام الـ ERP |
| variant_id | BIGINT | NOT NULL FK catalog.product_variants ON DELETE CASCADE | صنف المورد في دواء 24 المرتبط بهذا الكود |
| product_id | BIGINT | FK catalog.products ON DELETE SET NULL | المنتج في الكتالوج الرئيسي |
| tax_percent | NUMERIC(5,2) | CHECK null or 0 to 100 | نسبة الضريبة المرسلة من النظام وتُخزَّن فقط ولا تُطبَّق |
| link_source | TEXT | NOT NULL, CHECK in (sku_match, barcode_match, memory, engine, manual_review) | طريقة الربط الأول |
| last_hash | TEXT | NOT NULL DEFAULT '' | بصمة آخر بيانات مزامنة لاكتشاف عدم التغير |
| last_synced_at | TIMESTAMPTZ | | آخر مزامنة ناجحة |
| created_at / updated_at | TIMESTAMPTZ | NOT NULL DEFAULT now() | |

## erp.sync_batches
Unique `(integration_id, idempotency_key) WHERE idempotency_key IS NOT NULL`. Index `(integration_id, id) WHERE state IN ('queued','processing')`. Index `(organization_id, accepted_at DESC)`.

| Column | Type | Constraint | Arabic comment |
|---|---|---|---|
| id | BIGINT identity | PK | المعرف الداخلي للدفعة |
| public_id | UUID | NOT NULL DEFAULT gen_random_uuid() | المعرف العام للدفعة وهو syncId |
| organization_id | BIGINT | as above | المنشأة المالكة للسجل |
| integration_id | BIGINT | NOT NULL FK ON DELETE CASCADE | الربط |
| api_key_id | BIGINT | FK erp.api_keys ON DELETE SET NULL | المفتاح الذي أرسل الدفعة |
| warehouse_id, branch_id | BIGINT | NOT NULL | المستودع والفرع وقت الإرسال |
| client_batch_id | TEXT | CHECK null or length <= 100 | معرف الدفعة من جهة الـ ERP |
| idempotency_key | TEXT | CHECK null or length <= 100 | مفتاح منع التكرار |
| body_sha256 | TEXT | NOT NULL | بصمة محتوى الدفعة للكشف عن تعارض مفتاح التكرار |
| state | TEXT | NOT NULL DEFAULT 'queued', CHECK in (queued, processing, completed, failed) | حالة الدفعة |
| item_count, chunk_count, chunks_done | INT | NOT NULL DEFAULT 0 | عدد الأصناف والأجزاء والمنجز منها |
| created_count, updated_count, unchanged_count, needs_review_count, failed_count | INT | NOT NULL DEFAULT 0 | عدادات النتائج |
| error_code | TEXT | | رمز الفشل الكلي إن وُجد |
| accepted_at | TIMESTAMPTZ | NOT NULL DEFAULT now() | وقت قبول الدفعة |
| started_at, finished_at | TIMESTAMPTZ | | وقت بدء ونهاية المعالجة |

States: `queued` to `processing` (first chunk starts) to `completed` (last chunk done; item failures do not fail the batch) or `failed` (a chunk exhausts its 3 attempts; its pending items are set `failed` with `ERP_ITEM_WRITE_FAILED`).

## erp.sync_items
Unique `(batch_id, position)`. Index `(batch_id, chunk_no) WHERE state = 'pending'`. Index `(batch_id, id)`.

| Column | Type | Constraint | Arabic comment |
|---|---|---|---|
| id | BIGINT identity | PK | المعرف الداخلي |
| organization_id | BIGINT | as above | المنشأة المالكة للسجل |
| batch_id | BIGINT | NOT NULL FK erp.sync_batches ON DELETE CASCADE | الدفعة |
| position | INT | NOT NULL | ترتيب الصنف داخل الدفعة بدءاً من الصفر |
| chunk_no | INT | NOT NULL | رقم الجزء الذي يعالج الصنف |
| external_code | TEXT | NOT NULL DEFAULT '' | كود الصنف كما أُرسل |
| payload | JSONB | NOT NULL DEFAULT '{}' | بيانات الصنف بعد التوحيد وتُفرَّغ بعد المعالجة |
| state | TEXT | NOT NULL DEFAULT 'pending', CHECK in (pending, created, updated, unchanged, needs_review, failed) | نتيجة الصنف |
| code | TEXT | | رمز الخطأ أو الملاحظة |
| message | JSONB | | الرسالة بالعربية والإنجليزية |
| variant_id | BIGINT | | صنف المورد الناتج |
| processed_at | TIMESTAMPTZ | | وقت المعالجة |

State: `pending` to exactly one terminal state. A retried chunk skips rows that are not `pending`.

## erp.review_items
Unique `(integration_id, external_code) WHERE state = 'open'`. Index `(organization_id, state)`.

| Column | Type | Constraint | Arabic comment |
|---|---|---|---|
| id, organization_id | | | |
| integration_id | BIGINT | NOT NULL FK ON DELETE CASCADE | الربط |
| external_code | TEXT | NOT NULL | كود الصنف |
| payload | JSONB | NOT NULL | آخر بيانات وصلت للصنف |
| candidates | JSONB | NOT NULL DEFAULT '[]' | المنتجات المقترحة من المطابقة |
| reason_code | TEXT | NOT NULL | سبب الحاجة للمراجعة |
| batch_id | BIGINT | FK erp.sync_batches ON DELETE SET NULL | آخر دفعة أرسلته |
| state | TEXT | NOT NULL DEFAULT 'open', CHECK in (open, resolved, dismissed) | حالة المراجعة |
| resolved_product_id, resolved_variant_id | BIGINT | | المنتج والصنف بعد الحل |
| resolved_by | BIGINT | FK identity.users ON DELETE SET NULL | من حل المراجعة |
| resolved_at | TIMESTAMPTZ | | وقت الحل |
| created_at / updated_at | TIMESTAMPTZ | NOT NULL DEFAULT now() | |

State: `open` to `resolved` or `dismissed` (final). A later sync for the same code finds the open row and updates `payload`, `candidates`, `batch_id`.

## erp.order_imports
Unique `(integration_id, direction, order_id)`. Index `(integration_id, direction, order_number)`.

| Column | Type | Constraint | Arabic comment |
|---|---|---|---|
| id, organization_id | | | |
| integration_id | BIGINT | NOT NULL FK ON DELETE CASCADE | الربط |
| direction | TEXT | NOT NULL, CHECK in (sales, purchase) | اتجاه الأمر: مبيعات أو شراء |
| order_id | BIGINT | NOT NULL | معرف الطلب في commerce.orders (دون مفتاح أجنبي عمداً) |
| shipment_id | BIGINT | | معرف شحنة المورد في أوامر المبيعات |
| order_number | TEXT | NOT NULL | رقم الطلب في دواء 24 |
| erp_reference | TEXT | NOT NULL, CHECK length 1 to 100 | مرجع الأمر في نظام الـ ERP |
| acked_status | TEXT | NOT NULL | حالة الأمر وقت الاستلام لكشف التغيير بعده |
| api_key_id | BIGINT | FK erp.api_keys ON DELETE SET NULL | المفتاح الذي أكد الاستلام |
| ack_count | INT | NOT NULL DEFAULT 1 | عدد مرات تأكيد الاستلام |
| first_acked_at, acked_at | TIMESTAMPTZ | NOT NULL DEFAULT now() | أول وآخر تأكيد |

## erp.idempotency_records
Unique `(integration_id, scope, external_reference)`.

| Column | Type | Constraint | Arabic comment |
|---|---|---|---|
| id, organization_id | | | |
| integration_id | BIGINT | NOT NULL FK ON DELETE CASCADE | الربط |
| scope | TEXT | NOT NULL, CHECK in ('purchase_order') | نوع العملية |
| external_reference | TEXT | NOT NULL, CHECK length 1 to 100 | المرجع الفريد من نظام الـ ERP |
| request_sha256 | TEXT | NOT NULL | بصمة الطلب الأصلي |
| state | TEXT | NOT NULL DEFAULT 'in_progress', CHECK in (in_progress, done) | حالة المعالجة |
| order_id | BIGINT | | الطلب الناتج |
| order_number | TEXT | | رقم الطلب الناتج |
| response | JSONB | | الرد المخزن لإعادته عند التكرار |
| created_at / updated_at | TIMESTAMPTZ | NOT NULL DEFAULT now() | |

State: insert `in_progress`; on success `done` with `response`; on a refusal the row is deleted so the ERP can retry. An `in_progress` row older than 2 minutes is repaired by looking up the order note marker.

## erp.api_calls
No updates. `organization_id` is nullable (failed authentication has no organization); the standard policy then shows such rows only to `AsSystem`. Index `(organization_id, created_at DESC)`, `(api_key_id, created_at DESC)`, `(created_at)`.

| Column | Type | Arabic comment |
|---|---|---|
| id BIGINT identity PK | | |
| organization_id BIGINT NULL FK ON DELETE CASCADE | المنشأة صاحبة المفتاح إن عُرفت |
| api_key_id BIGINT NULL FK erp.api_keys ON DELETE SET NULL | المفتاح المستخدم |
| key_prefix TEXT NOT NULL DEFAULT '' | بادئة المفتاح فقط |
| request_id TEXT NOT NULL | مرجع الطلب |
| method TEXT NOT NULL | نوع الطلب |
| route TEXT NOT NULL | نمط المسار بدون قيم |
| status INT NOT NULL | رمز الاستجابة |
| error_code TEXT NOT NULL DEFAULT '' | رمز الخطأ المستقر |
| duration_ms INT NOT NULL | زمن المعالجة بالمللي ثانية |
| bytes_in INT NOT NULL DEFAULT 0 | حجم الطلب |
| ip INET | عنوان المرسل |
| sensitive_read BOOLEAN NOT NULL DEFAULT false | هل قرأ الطلب بيانات حساسة |
| created_at TIMESTAMPTZ NOT NULL DEFAULT now() | وقت الطلب |

## Migration 256: audit actor kind
```
ALTER TABLE platform.audit_log
  ADD COLUMN IF NOT EXISTS actor_kind TEXT NOT NULL DEFAULT 'user',
  ADD COLUMN IF NOT EXISTS actor_key_id BIGINT;
ALTER TABLE platform.audit_log ADD CONSTRAINT audit_log_actor_kind_chk
  CHECK (actor_kind IN ('user','api_key','system'));
CREATE INDEX IF NOT EXISTS audit_log_actor_key_idx ON platform.audit_log (actor_key_id, created_at DESC) WHERE actor_key_id IS NOT NULL;
```
Comments: `actor_kind` = نوع الفاعل: مستخدم أو مفتاح API أو النظام; `actor_key_id` = معرف مفتاح API عندما يكون الفاعل جهازاً. Expand-only and backward compatible: the old image inserts without the new columns and gets the defaults. Down: drop the index, constraint and both columns. The existing append-only revoke (`228`) is unchanged; a new column keeps the table's grants.

## Migration 257: customer code lookup
```
CREATE INDEX IF NOT EXISTS idx_user_orgs_vendor_number
  ON org.user_organizations (vendor_org_id, lower(organization_number))
  WHERE deleted_at IS NULL AND organization_number <> '';
```
Not unique on purpose (existing duplicates). Down: `DROP INDEX`.

## Migration 258: order numbers
```
CREATE SEQUENCE IF NOT EXISTS commerce.order_number_seq;
SELECT setval('commerce.order_number_seq',
  GREATEST((SELECT COALESCE(MAX(order_number::bigint),0) FROM commerce.orders
            WHERE order_number ~ '^[0-9]{1,8}$'), 0) + 1000, false);
COMMENT ON SEQUENCE ... IS 'متسلسلة أرقام الطلبات: تضمن عدم تكرار الرقم بين النسخ المتعددة للخادم';
GRANT USAGE, SELECT ON SEQUENCE commerce.order_number_seq TO dawa24_app;  -- inside the IF EXISTS role DO block
```
Order numbers of 9 digits or more (date format) never collide with it. Down: `DROP SEQUENCE`. Rollback to the old image is safe: the old code ignores the sequence.

## Relationships
`organizations` 1-n `integrations` 1-n `api_keys`; `integrations` 1-n `product_links` (n-1 `product_variants`), `sync_batches` 1-n `sync_items`, `review_items`, `order_imports`, `idempotency_records`. No table in `erp` is read by another module.
