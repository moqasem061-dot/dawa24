-- Migration 054 Down

BEGIN;

DROP TABLE IF EXISTS platform_admin.feature_flags;

COMMIT;
