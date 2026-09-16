-- 060_two_account_types (down)
--
-- The value mapping is intentionally one-way: a 'customer' org could have
-- been 'pharmacy', 'chain_pharmacy' or 'individual' and a 'vendor' org
-- 'supplier', 'company' or 'agency' — the original value is gone. Rolling
-- back restores the old CHECK constraint surface (which the legacy ETL
-- needs) without pretending the data came back.

BEGIN;

ALTER TABLE org.organizations DROP CONSTRAINT IF EXISTS organizations_type_check;
ALTER TABLE org.organizations ADD CONSTRAINT organizations_type_check
    CHECK (type IN ('supplier','pharmacy','chain_pharmacy','company','agency'));

ALTER TABLE org.organizations DROP COLUMN IF EXISTS is_chain;

ALTER TABLE identity.users DROP CONSTRAINT IF EXISTS users_role_check;

COMMIT;