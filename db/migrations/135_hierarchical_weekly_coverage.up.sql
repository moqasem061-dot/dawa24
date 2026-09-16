-- 135_hierarchical_weekly_coverage.up.sql
-- Add governorate_id to workflow.weekly_coverages and optimize spatial coverage lookup

-- 1. Add governorate_id column
ALTER TABLE workflow.weekly_coverages
    ADD COLUMN IF NOT EXISTS governorate_id BIGINT REFERENCES platform_admin.governorates(id) ON DELETE SET NULL;

-- 2. Populate governorate_id from cities if city_id exists
UPDATE workflow.weekly_coverages wc
SET governorate_id = c.governorate_id
FROM platform_admin.cities c
WHERE wc.city_id = c.id AND wc.governorate_id IS NULL AND c.governorate_id IS NOT NULL;

-- 3. Update coordinates from cities where latitude/longitude is null but city_id is set
UPDATE workflow.weekly_coverages wc
SET latitude = c.latitude, longitude = c.longitude
FROM platform_admin.cities c
WHERE wc.city_id = c.id AND (wc.latitude IS NULL OR wc.longitude IS NULL);

-- 4. Create performance indexes for spatial coverage matching & organization queries
CREATE INDEX IF NOT EXISTS idx_weekly_coverages_org_day_active
    ON workflow.weekly_coverages (organization_id, day_of_week, is_active);

CREATE INDEX IF NOT EXISTS idx_weekly_coverages_gov_city
    ON workflow.weekly_coverages (governorate_id, city_id);

CREATE INDEX IF NOT EXISTS idx_weekly_coverages_branch_day
    ON workflow.weekly_coverages (branch_id, day_of_week);
