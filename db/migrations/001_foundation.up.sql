-- 001_foundation
--
-- Extensions, schemas, and the row-level security mechanism that every
-- tenant-owned table depends on.
--
-- Schemas mirror the Go module boundaries in internal/modules/. A table's schema
-- tells you which module owns it, and cross-schema foreign keys are the explicit
-- points where two bounded contexts touch.

BEGIN;

-- pg_trgm powers Arabic fuzzy product matching without an external search
-- service. unaccent normalises Latin transliterations of drug names.
-- pgcrypto encrypts TOTP secrets at rest and generates UUIDs.
CREATE EXTENSION IF NOT EXISTS pg_trgm;
CREATE EXTENSION IF NOT EXISTS unaccent;
CREATE EXTENSION IF NOT EXISTS pgcrypto;
CREATE EXTENSION IF NOT EXISTS citext;

CREATE SCHEMA IF NOT EXISTS identity;
CREATE SCHEMA IF NOT EXISTS profile;
CREATE SCHEMA IF NOT EXISTS org;
CREATE SCHEMA IF NOT EXISTS catalog;
CREATE SCHEMA IF NOT EXISTS inventory;
CREATE SCHEMA IF NOT EXISTS commerce;
CREATE SCHEMA IF NOT EXISTS promo;
CREATE SCHEMA IF NOT EXISTS billing;
CREATE SCHEMA IF NOT EXISTS ingest;
CREATE SCHEMA IF NOT EXISTS workflow;
CREATE SCHEMA IF NOT EXISTS hr;
CREATE SCHEMA IF NOT EXISTS platform;
CREATE SCHEMA IF NOT EXISTS ai;

COMMENT ON SCHEMA identity  IS 'Authentication, roles, permissions, sessions';
COMMENT ON SCHEMA org       IS 'Organizations (vendors/suppliers), branches, membership';
COMMENT ON SCHEMA catalog   IS 'Products, variants, categories, brands';
COMMENT ON SCHEMA inventory IS 'Warehouses, stock, movements, transfers';
COMMENT ON SCHEMA commerce  IS 'Carts, orders, shipments, order lines';
COMMENT ON SCHEMA promo     IS 'Offers, ads, sponsorships, placements';
COMMENT ON SCHEMA billing   IS 'Wallets, payments, plans, subscriptions, entitlements';
COMMENT ON SCHEMA ingest    IS 'Bulk import pipeline';
COMMENT ON SCHEMA ai        IS 'AI capability call metadata only. Never provider keys.';

-- ---------------------------------------------------------------------------
-- Tenant isolation
-- ---------------------------------------------------------------------------
-- The application sets these two GUCs with SET LOCAL inside every transaction
-- (see internal/platform/database). Policies read them through the helpers below.
--
-- current_setting(..., true) returns NULL rather than erroring when the setting
-- is absent, which is what makes an unset tenant resolve to "match nothing"
-- instead of raising and turning a leak into a 500.

CREATE OR REPLACE FUNCTION platform.current_org_id() RETURNS BIGINT
LANGUAGE sql STABLE PARALLEL SAFE AS $$
    SELECT NULLIF(current_setting('app.current_org_id', true), '')::BIGINT;
$$;

COMMENT ON FUNCTION platform.current_org_id() IS
    'Organization bound to the current transaction, or NULL. Read by every RLS policy.';

CREATE OR REPLACE FUNCTION platform.is_system() RETURNS BOOLEAN
LANGUAGE sql STABLE PARALLEL SAFE AS $$
    SELECT COALESCE(current_setting('app.is_system', true) = 'on', false);
$$;

COMMENT ON FUNCTION platform.is_system() IS
    'True when the transaction was opened via database.AsSystem(). Grants cross-tenant reads to platform admin and background jobs.';

-- platform.tenant_visible is applied verbatim by every tenant policy, so the
-- isolation rule is defined once. Changing the rule is a single migration
-- rather than an audit of forty policies.
CREATE OR REPLACE FUNCTION platform.tenant_visible(row_org_id BIGINT) RETURNS BOOLEAN
LANGUAGE sql STABLE PARALLEL SAFE AS $$
    SELECT platform.is_system()
        OR (platform.current_org_id() IS NOT NULL AND row_org_id = platform.current_org_id());
$$;

-- ---------------------------------------------------------------------------
-- Shared conventions
-- ---------------------------------------------------------------------------

-- Every mutable table gets this trigger so updated_at is maintained by the
-- database. Application-maintained timestamps drift the moment one code path
-- forgets, and bulk imports are exactly the code path that forgets.
CREATE OR REPLACE FUNCTION platform.touch_updated_at() RETURNS TRIGGER
LANGUAGE plpgsql AS $$
BEGIN
    NEW.updated_at := now();
    RETURN NEW;
END;
$$;

-- Arabic normalisation, mirroring internal/shared/arabic.Normalize.
--
-- It exists in SQL as well as Go so that trigram indexes are built over the
-- normalised form. Matching "بانادول" against "بَانادُول" has to happen inside
-- the index, not after the rows come back.
--
-- Any change here must be made in both places and covered by the parity suite.
CREATE OR REPLACE FUNCTION platform.normalize_arabic(input TEXT) RETURNS TEXT
LANGUAGE sql IMMUTABLE PARALLEL SAFE AS $$
    SELECT trim(regexp_replace(
        lower(
            translate(
                regexp_replace(
                    regexp_replace(COALESCE(input, ''), '[ً-ٟـ]', '', 'g'),
                    '[%\-_/\\()\[\]{}.,]', ' ', 'g'
                ),
                'أإآٱةى٠١٢٣٤٥٦٧٨٩',
                'ااااهي0123456789'
            )
        ),
        '\s+', ' ', 'g'
    ));
$$;

COMMENT ON FUNCTION platform.normalize_arabic(TEXT) IS
    'Mirror of internal/shared/arabic.Normalize. Keep both implementations in sync; parity suite enforces it.';

-- ---------------------------------------------------------------------------
-- Audit trail
-- ---------------------------------------------------------------------------
-- This one genuinely belongs in PostgreSQL: it is a compliance record for a
-- regulated (pharmaceutical) marketplace, written in the same transaction as
-- the change it describes. Operational logs go to stdout instead.

CREATE TABLE platform.audit_log (
    id              BIGSERIAL PRIMARY KEY,
    organization_id BIGINT,
    actor_user_id   BIGINT,
    action          TEXT NOT NULL,
    entity_type     TEXT NOT NULL,
    entity_id       TEXT NOT NULL,
    before          JSONB,
    after           JSONB,
    ip              INET,
    request_id      TEXT,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX audit_log_entity_idx  ON platform.audit_log (entity_type, entity_id, created_at DESC);
CREATE INDEX audit_log_actor_idx   ON platform.audit_log (actor_user_id, created_at DESC);
CREATE INDEX audit_log_org_idx     ON platform.audit_log (organization_id, created_at DESC);

COMMENT ON TABLE platform.audit_log IS
    'Append-only business audit trail. UPDATE and DELETE are revoked from the application role in 002.';

-- ---------------------------------------------------------------------------
-- Settings
-- ---------------------------------------------------------------------------
-- Replaces the legacy 85-column single-row full_settings table, where adding a
-- setting meant a schema migration and credentials lived in table columns.
-- Secrets are not permitted here; they belong in environment variables.

CREATE TABLE platform.settings (
    key         TEXT PRIMARY KEY,
    value       JSONB NOT NULL,
    value_type  TEXT NOT NULL CHECK (value_type IN ('string','integer','decimal','boolean','json','i18n')),
    description TEXT,
    is_public   BOOLEAN NOT NULL DEFAULT false,
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_by  BIGINT
);

COMMENT ON COLUMN platform.settings.is_public IS
    'True when the value may be exposed to unauthenticated clients. Defaults false so a new setting is private by mistake, not public by mistake.';

COMMIT;
