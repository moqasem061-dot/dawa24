-- Migration 059 Down
BEGIN;

DELETE FROM identity.roles WHERE key = 'org_pharmacist';

COMMIT;
