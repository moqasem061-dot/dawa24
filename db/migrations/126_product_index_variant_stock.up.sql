-- 126_product_index_variant_stock (up)
--
-- Indexes for the repaired product_index (see internal/modules/catalog/jobs/reindex_sql.go).
--
-- Until migration 126 the read model held only parent rows, all with variant_id
-- NULL and stock_quantity 0, so none of these access paths existed to need
-- indexing. With variant rows present the model is asked two new questions:
-- "which variants belong to this product" (offer enumeration) and "what is
-- actually obtainable" (availability filtering).

BEGIN;

-- Offer enumeration: every variant of a matched product.
CREATE INDEX IF NOT EXISTS product_index_product_type_idx
    ON catalog.product_index (product_id, product_type);

-- Availability: partial index, because the rows that matter are the buyable
-- ones and they are a minority of the table.
CREATE INDEX IF NOT EXISTS product_index_in_stock_idx
    ON catalog.product_index (product_id)
    WHERE stock_quantity > 0;

-- Variant lookup by id, for correcting or re-selecting a single offer.
CREATE INDEX IF NOT EXISTS product_index_variant_idx
    ON catalog.product_index (variant_id)
    WHERE variant_id IS NOT NULL;

-- Corporate Operations filtering intersects this array per query.
CREATE INDEX IF NOT EXISTS product_index_institutional_idx
    ON catalog.product_index USING GIN (institutional_work_ids);

COMMIT;
