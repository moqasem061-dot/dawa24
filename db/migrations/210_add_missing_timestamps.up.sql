BEGIN;

-- Add created_at to billing.invoice_lines
ALTER TABLE billing.invoice_lines
  ADD COLUMN IF NOT EXISTS created_at TIMESTAMPTZ NOT NULL DEFAULT now();

-- Add created_at and updated_at to identity.user_security
ALTER TABLE identity.user_security
  ADD COLUMN IF NOT EXISTS created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  ADD COLUMN IF NOT EXISTS updated_at TIMESTAMPTZ NOT NULL DEFAULT now();

CREATE TRIGGER user_security_touch
  BEFORE UPDATE ON identity.user_security
  FOR EACH ROW EXECUTE FUNCTION platform.touch_updated_at();

-- Add created_at and updated_at to identity.user_mfa
ALTER TABLE identity.user_mfa
  ADD COLUMN IF NOT EXISTS created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  ADD COLUMN IF NOT EXISTS updated_at TIMESTAMPTZ NOT NULL DEFAULT now();

CREATE TRIGGER user_mfa_touch
  BEFORE UPDATE ON identity.user_mfa
  FOR EACH ROW EXECUTE FUNCTION platform.touch_updated_at();

-- Add created_at to org.role_permissions
ALTER TABLE org.role_permissions
  ADD COLUMN IF NOT EXISTS created_at TIMESTAMPTZ NOT NULL DEFAULT now();

-- Add created_at to identity.role_permissions
ALTER TABLE identity.role_permissions
  ADD COLUMN IF NOT EXISTS created_at TIMESTAMPTZ NOT NULL DEFAULT now();

-- Add created_at to identity.user_address_histories
ALTER TABLE identity.user_address_histories
  ADD COLUMN IF NOT EXISTS created_at TIMESTAMPTZ NOT NULL DEFAULT now();

COMMIT;
