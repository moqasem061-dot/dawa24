BEGIN;

DROP TABLE IF EXISTS org.organization_followers CASCADE;
DROP TABLE IF EXISTS org.organization_reviews CASCADE;
DROP TABLE IF EXISTS commerce.wishlists CASCADE;
DROP TABLE IF EXISTS identity.user_addresses CASCADE;

COMMIT;
