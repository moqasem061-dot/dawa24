-- Migration 182 down: No-op as restoring arbitrary inactive flags is not reversible safely
BEGIN;
COMMIT;