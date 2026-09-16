-- 027_schema_code_reconciliation
--
-- Adds columns that repository SQL already selects but that no migration ever
-- created. Every one of these would fail at runtime with
-- `column "..." does not exist`, taking the whole endpoint with it — the
-- address book, organization reads and branch listing were all dead.
--
-- Found by test/schema_consistency_test.go, which parses the migrations and
-- checks every SELECT column list in internal/**/postgres against them. The
-- migrations have never been run against a real PostgreSQL, so nothing else
-- would have caught this before the first deploy.
--
-- The domain structs are treated as the intent and the schema is corrected to
-- match, not the reverse: org.Organization.Validate() already requires
-- legal_name and commercial_register, which are legal requirements for an
-- Egyptian B2B counterparty, and Egyptian delivery addresses genuinely need
-- building/floor/apartment.

BEGIN;

-- ---------------------------------------------------------------------------
-- identity.user_addresses
-- ---------------------------------------------------------------------------
-- Two columns were named differently from the code that reads them, and four
-- were missing entirely. Renaming rather than adding duplicates keeps one name
-- per concept; the table holds no production data yet.
ALTER TABLE identity.user_addresses RENAME COLUMN address_line TO address;
ALTER TABLE identity.user_addresses RENAME COLUMN phone_number TO phone;

ALTER TABLE identity.user_addresses
    ADD COLUMN IF NOT EXISTS recipient TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS building  TEXT,
    ADD COLUMN IF NOT EXISTS floor     TEXT,
    ADD COLUMN IF NOT EXISTS apartment TEXT;

COMMENT ON COLUMN identity.user_addresses.recipient IS 'اسم المستلم — the person receiving, who is often not the account holder';
COMMENT ON COLUMN identity.user_addresses.building IS 'رقم العمارة';
COMMENT ON COLUMN identity.user_addresses.floor IS 'الدور';
COMMENT ON COLUMN identity.user_addresses.apartment IS 'الشقة';

-- ---------------------------------------------------------------------------
-- org.organizations
-- ---------------------------------------------------------------------------
-- Legal identity and trade terms. Validate() already rejects an organization
-- without legal_name or commercial_register, so the application refused to
-- create rows the table could not have stored anyway.
ALTER TABLE org.organizations
    ADD COLUMN IF NOT EXISTS legal_name          TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS trade_name          JSONB NOT NULL DEFAULT '{"ar":"","en":""}'::JSONB,
    ADD COLUMN IF NOT EXISTS commercial_register TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS credit_limit        NUMERIC(14,2) NOT NULL DEFAULT 0.00,
    ADD COLUMN IF NOT EXISTS payment_terms_days  INT NOT NULL DEFAULT 0;

COMMENT ON COLUMN org.organizations.legal_name IS 'الاسم القانوني — registered legal name, distinct from the trading name';
COMMENT ON COLUMN org.organizations.commercial_register IS 'السجل التجاري — commercial registration number';
COMMENT ON COLUMN org.organizations.credit_limit IS 'الحد الائتماني — how much this counterparty may owe at once';
COMMENT ON COLUMN org.organizations.payment_terms_days IS 'مدة السداد بالأيام — net payment terms';

-- Commercial register numbers identify a company uniquely, so a duplicate is a
-- data-entry error or an impersonation attempt. Partial so the empty default
-- above does not collide across existing rows.
CREATE UNIQUE INDEX IF NOT EXISTS organizations_commercial_register_key
    ON org.organizations (commercial_register)
    WHERE commercial_register <> '' AND deleted_at IS NULL;

ALTER TABLE org.organizations
    ADD CONSTRAINT organizations_credit_limit_non_negative CHECK (credit_limit >= 0),
    ADD CONSTRAINT organizations_payment_terms_non_negative CHECK (payment_terms_days >= 0);

-- ---------------------------------------------------------------------------
-- org.branches
-- ---------------------------------------------------------------------------
ALTER TABLE org.branches
    ADD COLUMN IF NOT EXISTS public_id UUID NOT NULL DEFAULT gen_random_uuid(),
    ADD COLUMN IF NOT EXISTS code      TEXT;

CREATE UNIQUE INDEX IF NOT EXISTS branches_public_id_key ON org.branches (public_id);

-- Branch codes appear on delivery paperwork, so they must be unambiguous within
-- an organization. Scoped per organization rather than globally: two suppliers
-- may both call a branch "CAI-01".
CREATE UNIQUE INDEX IF NOT EXISTS branches_org_code_key
    ON org.branches (organization_id, code)
    WHERE code IS NOT NULL AND deleted_at IS NULL;

-- ---------------------------------------------------------------------------
-- org.members
-- ---------------------------------------------------------------------------
-- role_key already carries the authorization decision; role_id is the numeric
-- handle the repository reads. is_active is the fast membership check, kept
-- alongside the richer status column it summarises.
ALTER TABLE org.members
    ADD COLUMN IF NOT EXISTS role_id   BIGINT,
    ADD COLUMN IF NOT EXISTS is_active BOOLEAN NOT NULL DEFAULT true;

-- Keep is_active consistent with status for rows created before this migration.
UPDATE org.members SET is_active = (status = 'active');

CREATE INDEX IF NOT EXISTS members_active_idx ON org.members (organization_id, is_active);

COMMIT;
