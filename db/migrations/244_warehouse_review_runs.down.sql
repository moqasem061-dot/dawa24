BEGIN;

DROP INDEX platform.import_runs_one_warehouse_review;
-- Remove only this feature's progress records when removing the feature.
DELETE FROM platform.import_runs WHERE kind = 'temp_warehouse_review';
ALTER TABLE platform.import_runs DROP CONSTRAINT import_runs_review_organization_check;
ALTER TABLE platform.import_runs ALTER COLUMN organization_id SET NOT NULL;
ALTER TABLE platform.import_runs DROP CONSTRAINT import_runs_kind_check;
ALTER TABLE platform.import_runs ADD CONSTRAINT import_runs_kind_check
    CHECK (kind IN ('saving_products', 'team', 'compare', 'temp_warehouse', 'catalog', 'catalog_images'));

COMMIT;
