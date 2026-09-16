-- 072_drop_dead_columns (up)
--
-- Rebuild V2 §2.6 — column hygiene. Grep `internal/` proves no query names
-- org.organizations.rank: it is a legacy INT with a 97 default that nothing
-- reads, seeds, or sorts by. (The other candidates from the plan do not
-- exist in this schema: org.organizations has no `main` or
-- `first_time_upload_file`, and identity.users has no social-login or
-- API-token columns — 002 decomposed those into identity.user_identities.)

BEGIN;

ALTER TABLE org.organizations DROP COLUMN IF EXISTS rank;

COMMIT;
