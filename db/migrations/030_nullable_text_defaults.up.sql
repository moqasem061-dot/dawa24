-- 030_nullable_text_defaults
--
-- Several text columns are nullable in the schema while the Go structs model
-- them as plain `string`. pgx cannot scan NULL into a non-pointer string, so
-- GET /api/v1/org/organizations failed with
-- `cannot scan NULL into *string` as soon as one row had no tax number.
--
-- Two ways to fix it: make the Go fields pointers, or make the columns
-- non-nullable with an empty default. The second is chosen because these are
-- fields where "absent" and "empty" carry the same meaning to the business —
-- an organization either has a tax number on file or it does not, and no code
-- distinguishes NULL from ''. Pointer fields would push that non-distinction
-- into every call site and every template.
--
-- Columns where NULL is genuinely meaningful (dates, foreign keys, optional
-- money) are deliberately untouched.

BEGIN;

-- organization_number is deliberately left nullable. Its unique index excludes
-- NULLs, so collapsing them to '' would make every unassigned organization
-- collide with every other. NULL genuinely means "not assigned yet" there,
-- which is the distinction the other columns do not have.
UPDATE org.organizations SET
    email               = COALESCE(email, ''),
    phone               = COALESCE(phone, ''),
    address             = COALESCE(address, ''),
    tax_number          = COALESCE(tax_number, ''),
    image               = COALESCE(image, ''),
    coverage_image      = COALESCE(coverage_image, '');

ALTER TABLE org.organizations
    ALTER COLUMN email          SET DEFAULT '',
    ALTER COLUMN email          SET NOT NULL,
    ALTER COLUMN phone          SET DEFAULT '',
    ALTER COLUMN phone          SET NOT NULL,
    ALTER COLUMN address        SET DEFAULT '',
    ALTER COLUMN address        SET NOT NULL,
    ALTER COLUMN tax_number     SET DEFAULT '',
    ALTER COLUMN tax_number     SET NOT NULL,
    ALTER COLUMN image          SET DEFAULT '',
    ALTER COLUMN image          SET NOT NULL,
    ALTER COLUMN coverage_image SET DEFAULT '',
    ALTER COLUMN coverage_image SET NOT NULL;

UPDATE org.branches SET
    phone   = COALESCE(phone, ''),
    address = COALESCE(address, ''),
    code    = COALESCE(code, '');

ALTER TABLE org.branches
    ALTER COLUMN phone   SET DEFAULT '',
    ALTER COLUMN phone   SET NOT NULL,
    ALTER COLUMN address SET DEFAULT '',
    ALTER COLUMN address SET NOT NULL;

COMMIT;
