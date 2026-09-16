BEGIN;
ALTER TABLE org.branches
    ALTER COLUMN address DROP NOT NULL, ALTER COLUMN address DROP DEFAULT,
    ALTER COLUMN phone   DROP NOT NULL, ALTER COLUMN phone   DROP DEFAULT;
ALTER TABLE org.organizations
    ALTER COLUMN coverage_image      DROP NOT NULL, ALTER COLUMN coverage_image      DROP DEFAULT,
    ALTER COLUMN image               DROP NOT NULL, ALTER COLUMN image               DROP DEFAULT,
    ALTER COLUMN tax_number          DROP NOT NULL, ALTER COLUMN tax_number          DROP DEFAULT,
    ALTER COLUMN address             DROP NOT NULL, ALTER COLUMN address             DROP DEFAULT,
    ALTER COLUMN phone               DROP NOT NULL, ALTER COLUMN phone               DROP DEFAULT,
    ALTER COLUMN email               DROP NOT NULL, ALTER COLUMN email               DROP DEFAULT,
    ALTER COLUMN organization_number DROP NOT NULL, ALTER COLUMN organization_number DROP DEFAULT;
COMMIT;
