BEGIN;

-- Repair staged engine decisions only; never rewrite committed or human-reviewed rows.
WITH decisions AS (
    SELECT id, CASE WHEN match_decision->>'state' IN ('accepted', 'confirmed')
                    THEN (match_decision->>'product_id')::bigint END AS chosen_id
    FROM ingest.catalog_import_rows
    WHERE outcome = 'staged' AND NOT human_locked AND NOT is_manually_matched
      AND match_decision IS NOT NULL
      AND match_decision->>'state' = match_state
)
UPDATE ingest.catalog_import_rows r
SET product_id = d.chosen_id,
    variant_id = CASE WHEN r.product_id IS NOT DISTINCT FROM d.chosen_id THEN r.variant_id END
FROM decisions d
WHERE r.id = d.id
  AND r.product_id IS DISTINCT FROM d.chosen_id;

COMMIT;
