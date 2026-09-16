-- Clean up any existing orphaned stocks where the parent variant or product is soft-deleted
UPDATE inventory.stocks
SET deleted_at = now()
WHERE deleted_at IS NULL
  AND (
      product_variant_id IN (SELECT id FROM catalog.product_variants WHERE deleted_at IS NOT NULL)
      OR product_id IN (SELECT id FROM catalog.products WHERE deleted_at IS NOT NULL)
  );

-- Create trigger function to cascade variant soft-deletes to stocks
CREATE OR REPLACE FUNCTION catalog.cascade_variant_soft_delete()
RETURNS TRIGGER AS $$
BEGIN
    IF NEW.deleted_at IS NOT NULL AND (OLD.deleted_at IS NULL) THEN
        UPDATE inventory.stocks
        SET deleted_at = NEW.deleted_at
        WHERE product_variant_id = NEW.id
          AND deleted_at IS NULL;
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS trg_cascade_variant_soft_delete ON catalog.product_variants;
CREATE TRIGGER trg_cascade_variant_soft_delete
AFTER UPDATE OF deleted_at ON catalog.product_variants
FOR EACH ROW EXECUTE FUNCTION catalog.cascade_variant_soft_delete();

-- Create trigger function to cascade product soft-deletes to variants and stocks
CREATE OR REPLACE FUNCTION catalog.cascade_product_soft_delete()
RETURNS TRIGGER AS $$
BEGIN
    IF NEW.deleted_at IS NOT NULL AND (OLD.deleted_at IS NULL) THEN
        UPDATE catalog.product_variants
        SET deleted_at = NEW.deleted_at
        WHERE product_id = NEW.id
          AND deleted_at IS NULL;

        UPDATE inventory.stocks
        SET deleted_at = NEW.deleted_at
        WHERE product_id = NEW.id
          AND deleted_at IS NULL;
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS trg_cascade_product_soft_delete ON catalog.products;
CREATE TRIGGER trg_cascade_product_soft_delete
AFTER UPDATE OF deleted_at ON catalog.products
FOR EACH ROW EXECUTE FUNCTION catalog.cascade_product_soft_delete();
