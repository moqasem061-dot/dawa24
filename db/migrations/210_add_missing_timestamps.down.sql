BEGIN;

-- Remove triggers
DROP TRIGGER IF EXISTS user_security_touch ON identity.user_security;
DROP TRIGGER IF EXISTS user_mfa_touch ON identity.user_mfa;

-- Remove columns
ALTER TABLE identity.user_address_histories DROP COLUMN IF EXISTS created_at;
ALTER TABLE identity.role_permissions DROP COLUMN IF EXISTS created_at;
ALTER TABLE org.role_permissions DROP COLUMN IF EXISTS created_at;

ALTER TABLE identity.user_mfa 
  DROP COLUMN IF EXISTS updated_at,
  DROP COLUMN IF EXISTS created_at;

ALTER TABLE identity.user_security 
  DROP COLUMN IF EXISTS updated_at,
  DROP COLUMN IF EXISTS created_at;

ALTER TABLE billing.invoice_lines DROP COLUMN IF EXISTS created_at;

COMMIT;
