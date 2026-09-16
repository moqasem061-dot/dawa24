-- Migration 178: admin-reviewed changes to a company's own profile.
--
-- /vendor/organization was one form with one "save everything" button, and the
-- one field it did write — org.organizations.name — is read by nothing. Every
-- screen that displays a company reads trade_name first and falls back to
-- legal_name: the admin list, the supplier profile, the market board, the
-- reviews. So a supplier edited their trade name, the write succeeded, and
-- nothing they could see anywhere changed. Organisation 51 still shows
-- name.ar = 'شركة ويزر فارماالاب' beside trade_name.ar = 'شركة ويزر فارما'.
--
-- The page is now five independent section forms, and the section that carries
-- the company's identity — its legal name, trade name, commercial register and
-- tax number — goes through this table rather than straight onto the row.
-- Those four are what the platform verified when it approved the company; a
-- supplier changing them unilaterally would invalidate that approval silently.
-- The other sections (contact details, description, order limits, logo) apply
-- immediately, because getting a wrong phone number corrected should not need a
-- moderator.
--
-- Modelled on promo.ads.pending_changes (migration 164) so the platform has one
-- shape for "a tenant proposed a change and an administrator decides", not two.

CREATE TABLE IF NOT EXISTS org.profile_change_requests (
    id              BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    public_id       UUID        NOT NULL DEFAULT gen_random_uuid(),
    organization_id BIGINT      NOT NULL REFERENCES org.organizations(id) ON DELETE CASCADE,
    requested_by    BIGINT      NOT NULL REFERENCES identity.users(id)    ON DELETE RESTRICT,
    section         TEXT        NOT NULL,
    proposed        JSONB       NOT NULL DEFAULT '{}'::jsonb,
    previous        JSONB       NOT NULL DEFAULT '{}'::jsonb,
    status          TEXT        NOT NULL DEFAULT 'pending',
    admin_notes     TEXT        NOT NULL DEFAULT '',
    reviewed_by     BIGINT      REFERENCES identity.users(id) ON DELETE SET NULL,
    reviewed_at     TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT profile_change_requests_section_chk
        CHECK (section IN ('identity', 'limits', 'contact', 'description', 'media')),
    CONSTRAINT profile_change_requests_status_chk
        CHECK (status IN ('pending', 'approved', 'rejected', 'withdrawn'))
);

COMMENT ON TABLE  org.profile_change_requests             IS 'طلبات تعديل بيانات المنشأة المعروضة على إدارة المنصة';
COMMENT ON COLUMN org.profile_change_requests.section     IS 'قسم البيانات المطلوب تعديله: الهوية، الحدود، التواصل، الوصف، الوسائط';
COMMENT ON COLUMN org.profile_change_requests.proposed    IS 'القيم المقترحة من المنشأة';
COMMENT ON COLUMN org.profile_change_requests.previous    IS 'القيم قبل التعديل، لعرض المقارنة على المراجع';

CREATE UNIQUE INDEX IF NOT EXISTS profile_change_requests_public_id_key
    ON org.profile_change_requests (public_id);

-- One open request per section per company. A second submission has to replace
-- or wait for the first, rather than queueing two answers to one question.
CREATE UNIQUE INDEX IF NOT EXISTS profile_change_requests_one_pending
    ON org.profile_change_requests (organization_id, section)
    WHERE status = 'pending';

CREATE INDEX IF NOT EXISTS idx_profile_change_requests_pending
    ON org.profile_change_requests (created_at DESC)
    WHERE status = 'pending';

CREATE INDEX IF NOT EXISTS idx_profile_change_requests_org
    ON org.profile_change_requests (organization_id, created_at DESC);

ALTER TABLE org.profile_change_requests ENABLE ROW LEVEL SECURITY;
ALTER TABLE org.profile_change_requests FORCE ROW LEVEL SECURITY;

CREATE POLICY profile_change_requests_tenant_isolation ON org.profile_change_requests
    USING (platform.tenant_visible(organization_id))
    WITH CHECK (platform.tenant_visible(organization_id));

CREATE TRIGGER profile_change_requests_touch BEFORE UPDATE ON org.profile_change_requests
    FOR EACH ROW EXECUTE FUNCTION platform.touch_updated_at();
