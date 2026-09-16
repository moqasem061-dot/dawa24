-- Migration 172 down: intentionally a no-op.
--
-- The ensure migration only creates tables that should already exist; rolling
-- it back must not drop live offer data. Environments that need the 058 shape
-- removed should roll back 058 itself (down to 057) instead.

BEGIN;

-- no-op

COMMIT;
