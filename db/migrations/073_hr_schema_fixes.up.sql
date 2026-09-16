-- 073_hr_schema_fixes (up)
--
-- Rebuild V2 Phase 6 — 048_seed_realistic_data is removed from the
-- production path. Everything in it that a production install genuinely
-- needs lives here instead:
--
--   1. hr.job_offers / hr.job_applications were created (025) with
--      public_id/status at VARCHAR(32), but the public_id default is
--      'job_'/'app_' + 32 hex chars, which overflows 32. Widening to
--      VARCHAR(64) is the fix 048 carried.
--
--   2. hr.job_categories reference data (idempotent by slug).

BEGIN;

ALTER TABLE hr.job_offers
    ALTER COLUMN public_id TYPE VARCHAR(64),
    ALTER COLUMN status TYPE VARCHAR(64);
ALTER TABLE hr.job_applications
    ALTER COLUMN public_id TYPE VARCHAR(64),
    ALTER COLUMN status TYPE VARCHAR(64);

INSERT INTO hr.job_categories (name, slug, is_active)
VALUES
    ('{"ar":"الصيادلة وإدارة الفروع","en":"Pharmacists & Branch Management"}'::jsonb, 'pharmacists', true),
    ('{"ar":"المندوبون الطبيون والتسويق","en":"Medical Reps & Marketing"}'::jsonb, 'medical-reps', true),
    ('{"ar":"سلسلة الإمداد والمخازن","en":"Supply Chain & Warehouses"}'::jsonb, 'supply-chain', true),
    ('{"ar":"مساعدو الصيدليات","en":"Pharmacy Assistants"}'::jsonb, 'pharmacy-assistants', true)
ON CONFLICT (slug) DO UPDATE SET is_active = true;

COMMIT;