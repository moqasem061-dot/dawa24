BEGIN;

ALTER TABLE platform.import_runs DROP CONSTRAINT import_runs_kind_check;
ALTER TABLE platform.import_runs ADD CONSTRAINT import_runs_kind_check
    CHECK (kind IN ('saving_products', 'team', 'compare', 'temp_warehouse',
                    'catalog', 'catalog_images', 'temp_warehouse_review'));

-- Platform administrators may have no tenant organization.
ALTER TABLE platform.import_runs ALTER COLUMN organization_id DROP NOT NULL;
ALTER TABLE platform.import_runs ADD CONSTRAINT import_runs_review_organization_check
    CHECK (organization_id IS NOT NULL OR kind = 'temp_warehouse_review');

CREATE UNIQUE INDEX import_runs_one_warehouse_review
    ON platform.import_runs (kind)
    WHERE kind = 'temp_warehouse_review' AND state IN ('queued', 'processing', 'ready', 'committing');

COMMIT;
