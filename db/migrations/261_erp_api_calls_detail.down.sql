BEGIN;
-- Reverses 261. The index goes first; the columns are dropped with their data.
DROP INDEX IF EXISTS erp.api_calls_org_id_idx;
ALTER TABLE erp.api_calls
  DROP COLUMN IF EXISTS error_detail,
  DROP COLUMN IF EXISTS auth_reason,
  DROP COLUMN IF EXISTS ref,
  DROP COLUMN IF EXISTS bytes_out,
  DROP COLUMN IF EXISTS user_agent,
  DROP COLUMN IF EXISTS query;
COMMIT;
