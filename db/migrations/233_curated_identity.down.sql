-- 233_curated_identity.down.sql
--
-- Revert curated identity facts and pack relationships.

BEGIN;

DROP TRIGGER IF EXISTS trg_pack_relationships_revision_bump ON catalog.pack_relationships;
DROP TRIGGER IF EXISTS trg_curated_identity_facts_revision_bump ON catalog.curated_identity_facts;

DROP TABLE IF EXISTS catalog.pack_relationships;
DROP TABLE IF EXISTS catalog.curated_identity_facts;

COMMIT;
