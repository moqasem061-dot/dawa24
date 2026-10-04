BEGIN;

CREATE INDEX IF NOT EXISTS compare_file_rows_market_name_trgm
    ON compare.file_rows USING gin (lower(platform.normalize_arabic(raw_name)) gin_trgm_ops);
CREATE INDEX IF NOT EXISTS compare_file_rows_matched_product
    ON compare.file_rows (matched_product_id) WHERE matched_product_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS products_market_names_trgm
    ON catalog.products USING gin (
        lower(platform.normalize_arabic(COALESCE(name->>'ar', '') || ' ' || COALESCE(name->>'en', ''))) gin_trgm_ops)
    WHERE deleted_at IS NULL;

COMMIT;
