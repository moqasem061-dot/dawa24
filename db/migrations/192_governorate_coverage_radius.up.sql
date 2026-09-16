-- Add coverage_radius_meters to platform_admin.governorates to align with cities
ALTER TABLE platform_admin.governorates
ADD COLUMN IF NOT EXISTS coverage_radius_meters integer NOT NULL DEFAULT 25000;

COMMENT ON COLUMN platform_admin.governorates.coverage_radius_meters IS 'Standard spatial coverage radius in meters for the whole governorate (default 25km)';
