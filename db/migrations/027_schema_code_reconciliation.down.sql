BEGIN;

DROP INDEX IF EXISTS org.members_active_idx;
ALTER TABLE org.members DROP COLUMN IF EXISTS is_active, DROP COLUMN IF EXISTS role_id;

DROP INDEX IF EXISTS org.branches_org_code_key;
DROP INDEX IF EXISTS org.branches_public_id_key;
ALTER TABLE org.branches DROP COLUMN IF EXISTS code, DROP COLUMN IF EXISTS public_id;

ALTER TABLE org.organizations
    DROP CONSTRAINT IF EXISTS organizations_payment_terms_non_negative,
    DROP CONSTRAINT IF EXISTS organizations_credit_limit_non_negative;
DROP INDEX IF EXISTS org.organizations_commercial_register_key;
ALTER TABLE org.organizations
    DROP COLUMN IF EXISTS payment_terms_days,
    DROP COLUMN IF EXISTS credit_limit,
    DROP COLUMN IF EXISTS commercial_register,
    DROP COLUMN IF EXISTS trade_name,
    DROP COLUMN IF EXISTS legal_name;

ALTER TABLE identity.user_addresses
    DROP COLUMN IF EXISTS apartment,
    DROP COLUMN IF EXISTS floor,
    DROP COLUMN IF EXISTS building,
    DROP COLUMN IF EXISTS recipient;
ALTER TABLE identity.user_addresses RENAME COLUMN phone TO phone_number;
ALTER TABLE identity.user_addresses RENAME COLUMN address TO address_line;

COMMIT;
