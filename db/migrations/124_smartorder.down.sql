-- 124_smartorder (down)
--
-- Drops the smart ordering schema. fuzzystrmatch is deliberately left installed:
-- an extension is cheap, and dropping one that another migration may later come
-- to rely on is a worse failure than leaving it.

BEGIN;

DROP TABLE IF EXISTS smartorder.criteria_profiles CASCADE;
DROP TABLE IF EXISTS smartorder.run_events       CASCADE;
DROP TABLE IF EXISTS smartorder.line_selections  CASCADE;
DROP TABLE IF EXISTS smartorder.line_candidates  CASCADE;
DROP TABLE IF EXISTS smartorder.run_lines        CASCADE;
DROP TABLE IF EXISTS smartorder.column_mappings  CASCADE;
DROP TABLE IF EXISTS smartorder.run_config       CASCADE;
DROP TABLE IF EXISTS smartorder.runs             CASCADE;

DROP SCHEMA IF EXISTS smartorder CASCADE;

COMMIT;
