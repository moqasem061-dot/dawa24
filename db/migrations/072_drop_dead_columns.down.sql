-- 072_drop_dead_columns (down)
--
-- Restore the column. Individual rank values were dropped with it, so
-- restored rows carry the legacy default (97) — acceptable for a column no
-- query has ever read.

BEGIN;

ALTER TABLE org.organizations
    ADD COLUMN IF NOT EXISTS rank INT NOT NULL DEFAULT 97;

COMMIT;
