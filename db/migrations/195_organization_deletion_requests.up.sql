-- Migration 195: Organization Deletion Requests Queue & Account Deletion Alignment
--
-- Allows organization owners to request permanent deletion/suspension of their
-- organization, branches, and tenant resources with full review and audit trail
-- in the admin console. Also brings identity.account_deletion_requests into
-- strict alignment with the application code.

CREATE TABLE IF NOT EXISTS org.organization_deletion_requests (
    id              BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    public_id       UUID        NOT NULL DEFAULT gen_random_uuid(),
    organization_id BIGINT      NOT NULL REFERENCES org.organizations(id) ON DELETE CASCADE,
    requested_by    BIGINT      NOT NULL REFERENCES identity.users(id)    ON DELETE RESTRICT,
    reason          TEXT        NOT NULL DEFAULT '',
    status          TEXT        NOT NULL DEFAULT 'pending',
    admin_notes     TEXT        NOT NULL DEFAULT '',
    reviewed_by     BIGINT      REFERENCES identity.users(id) ON DELETE SET NULL,
    reviewed_at     TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT org_deletion_requests_status_chk
        CHECK (status IN ('pending', 'approved', 'rejected', 'cancelled'))
);

COMMENT ON TABLE  org.organization_deletion_requests             IS 'طلبات حذف المنشآت المقدمة من الملاك لمراجعة إدارة المنصة';
COMMENT ON COLUMN org.organization_deletion_requests.reason     IS 'سبب طلب الحذف المكتوب من مالك المنشأة';
COMMENT ON COLUMN org.organization_deletion_requests.admin_notes IS 'ملاحظات أو مبررات الإدارة عند القبول أو الرفض';

CREATE UNIQUE INDEX IF NOT EXISTS org_deletion_requests_public_id_key
    ON org.organization_deletion_requests (public_id);

-- One pending deletion request per organization at a time
CREATE UNIQUE INDEX IF NOT EXISTS org_deletion_requests_one_pending
    ON org.organization_deletion_requests (organization_id)
    WHERE status = 'pending';

CREATE INDEX IF NOT EXISTS idx_org_deletion_requests_status
    ON org.organization_deletion_requests (status, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_org_deletion_requests_org
    ON org.organization_deletion_requests (organization_id, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_org_deletion_requests_user
    ON org.organization_deletion_requests (requested_by);

ALTER TABLE org.organization_deletion_requests ENABLE ROW LEVEL SECURITY;
ALTER TABLE org.organization_deletion_requests FORCE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS org_deletion_requests_tenant_isolation ON org.organization_deletion_requests;
CREATE POLICY org_deletion_requests_tenant_isolation ON org.organization_deletion_requests
    USING (platform.tenant_visible(organization_id))
    WITH CHECK (platform.tenant_visible(organization_id));

DROP TRIGGER IF EXISTS org_deletion_requests_touch ON org.organization_deletion_requests;
CREATE TRIGGER org_deletion_requests_touch BEFORE UPDATE ON org.organization_deletion_requests
    FOR EACH ROW EXECUTE FUNCTION platform.touch_updated_at();

-- Schema alignment for identity.account_deletion_requests
ALTER TABLE identity.account_deletion_requests ADD COLUMN IF NOT EXISTS organization_id BIGINT REFERENCES org.organizations(id) ON DELETE SET NULL;
ALTER TABLE identity.account_deletion_requests ADD COLUMN IF NOT EXISTS admin_notes TEXT NOT NULL DEFAULT '';
ALTER TABLE identity.account_deletion_requests ADD COLUMN IF NOT EXISTS created_at TIMESTAMPTZ NOT NULL DEFAULT now();
ALTER TABLE identity.account_deletion_requests ADD COLUMN IF NOT EXISTS updated_at TIMESTAMPTZ NOT NULL DEFAULT now();

CREATE INDEX IF NOT EXISTS idx_account_deletion_requests_org ON identity.account_deletion_requests(organization_id);
