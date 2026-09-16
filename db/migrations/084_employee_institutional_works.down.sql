BEGIN;

DROP INDEX IF EXISTS catalog.products_inst_work_ids_gin;
ALTER TABLE catalog.products DROP COLUMN IF EXISTS institutional_work_ids;

DROP TABLE IF EXISTS org.employee_institutional_works CASCADE;

COMMIT;
