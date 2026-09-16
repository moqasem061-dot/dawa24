DROP TRIGGER IF EXISTS trg_cascade_variant_soft_delete ON catalog.product_variants;
DROP FUNCTION IF EXISTS catalog.cascade_variant_soft_delete();

DROP TRIGGER IF EXISTS trg_cascade_product_soft_delete ON catalog.products;
DROP FUNCTION IF EXISTS catalog.cascade_product_soft_delete();
