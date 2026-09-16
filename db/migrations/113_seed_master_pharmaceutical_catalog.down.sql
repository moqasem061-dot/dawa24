-- 113_seed_master_pharmaceutical_catalog (down)
BEGIN;
DELETE FROM catalog.products WHERE sku IN (
    'PAN-EXT-500', 'CONG-TAB-650', 'AUG-1G', 'CATAF-50', 'BRUF-400',
    'ANTIN-SYR', 'OTRIV-SPR-AD', 'KETO-50-CAP', 'FLAG-500', 'PAN-NGHT'
);
COMMIT;
