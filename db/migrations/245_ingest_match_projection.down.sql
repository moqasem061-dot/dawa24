BEGIN;
-- Data repair is retained on rollback: both server versions can read the
-- corrected product links. Reintroducing stale links would corrupt imports.
COMMIT;
