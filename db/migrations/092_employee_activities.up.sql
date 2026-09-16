-- 092_employee_activities.up.sql
-- Table for tracking employee actions and activities across vendor and customer tenants

CREATE TABLE IF NOT EXISTS platform_admin.employee_activities (
    id BIGSERIAL PRIMARY KEY,
    organization_id BIGINT NOT NULL REFERENCES org.organizations(id) ON DELETE CASCADE,
    user_id BIGINT NOT NULL REFERENCES identity.users(id) ON DELETE CASCADE,
    action VARCHAR(100) NOT NULL,
    description TEXT NOT NULL,
    href VARCHAR(255),
    ip VARCHAR(45),
    data JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_employee_activities_org ON platform_admin.employee_activities(organization_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_employee_activities_user ON platform_admin.employee_activities(user_id, created_at DESC);
