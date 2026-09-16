-- 104_supplier_featured_sections.down.sql

BEGIN;

ALTER TABLE promo.highlight_sections
    DROP COLUMN IF EXISTS description,
    DROP COLUMN IF EXISTS section_type,
    DROP COLUMN IF EXISTS color,
    DROP COLUMN IF EXISTS show_in_header;

COMMIT;
