-- 239_match_knowledge.down.sql

BEGIN;

DROP POLICY IF EXISTS match_knowledge_tenant_isolation ON catalog.match_knowledge;
DROP TABLE IF EXISTS catalog.match_knowledge;

COMMIT;
