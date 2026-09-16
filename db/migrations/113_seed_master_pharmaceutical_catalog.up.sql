-- 113_seed_master_pharmaceutical_catalog (up)
-- Seed standard master pharmaceutical catalog products for multi-stage matching.

BEGIN;

DO $$
DECLARE
    v_org_id BIGINT;
BEGIN
    SELECT id INTO v_org_id FROM org.organizations ORDER BY id LIMIT 1;
    IF v_org_id IS NOT NULL THEN
        INSERT INTO catalog.products (
            organization_id, name, sku, barcode, price, dosage_form,
            scientific_name, concentration, unit, manufacturing_companies, status
        ) VALUES
        (v_org_id, '{"ar":"بانادول إكسترا 500 مجم أقراص","en":"Panadol Extra 500mg Tablets"}'::jsonb, 'PAN-EXT-500', '6221234567890', 55.00, 'أقراص', 'Paracetamol + Caffeine', '500mg', 'علبة', 'GSK', 'active'),
        (v_org_id, '{"ar":"كونجستال أقراص للبرد والاحتقان","en":"Congestal Tablets"}'::jsonb, 'CONG-TAB-650', '6229876543210', 35.00, 'أقراص', 'Paracetamol + Pseudoephedrine', '650mg', 'علبة', 'Sigma', 'active'),
        (v_org_id, '{"ar":"أوجمنتين 1 جم أقراص","en":"Augmentin 1g Tablets"}'::jsonb, 'AUG-1G', '6223334445556', 115.00, 'أقراص', 'Amoxicillin + Clavulanic Acid', '1g', 'علبة', 'GlaxoSmithKline', 'active'),
        (v_org_id, '{"ar":"كتافلام 50 مجم أقراص","en":"Cataflam 50mg Tablets"}'::jsonb, 'CATAF-50', '6227778889990', 42.00, 'أقراص', 'Diclofenac Potassium', '50mg', 'شريط', 'Novartis', 'active'),
        (v_org_id, '{"ar":"بروفين 400 مجم أقراص","en":"Brufen 400mg Tablets"}'::jsonb, 'BRUF-400', '6221112223334', 48.00, 'أقراص', 'Ibuprofen', '400mg', 'علبة', 'Abbott', 'active'),
        (v_org_id, '{"ar":"أنتينال 220 مجم شراب","en":"Antinal 220mg Suspension"}'::jsonb, 'ANTIN-SYR', '6224445556667', 28.00, 'شراب', 'Nifuroxazide', '220mg/5ml', 'زجاجة', 'Amoun', 'active'),
        (v_org_id, '{"ar":"أوتريفين 0.1% بخاخ للأنف للكبار","en":"Otrivin 0.1% Adult Nasal Spray"}'::jsonb, 'OTRIV-SPR-AD', '6228889990001', 24.00, 'بخاخ', 'Xylometazoline', '0.1%', 'بخاخ', 'GSK Consumer', 'active'),
        (v_org_id, '{"ar":"كيتوفان 50 مجم كبسولات","en":"Ketofan 50mg Capsules"}'::jsonb, 'KETO-50-CAP', '6222223334445', 32.00, 'كبسولات', 'Ketoprofen', '50mg', 'شريط', 'Amoun', 'active'),
        (v_org_id, '{"ar":"فلاجيل 500 مجم أقراص","en":"Flagyl 500mg Tablets"}'::jsonb, 'FLAG-500', '6226667778889', 30.00, 'أقراص', 'Metronidazole', '500mg', 'علبة', 'Sanofi', 'active'),
        (v_org_id, '{"ar":"بانادول نايت أقراص","en":"Panadol Night Tablets"}'::jsonb, 'PAN-NGHT', '6225556667778', 65.00, 'أقراص', 'Paracetamol + Diphenhydramine', '500mg', 'علبة', 'GSK', 'active');
    END IF;
END $$;

COMMIT;
