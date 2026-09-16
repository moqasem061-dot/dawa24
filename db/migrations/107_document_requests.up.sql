-- 107_document_requests.up.sql
-- Administrative document requests issued to organizations with deadlines

CREATE TABLE IF NOT EXISTS platform_admin.document_requests (
    id               BIGSERIAL PRIMARY KEY,
    organization_id  BIGINT NOT NULL REFERENCES org.organizations(id) ON DELETE CASCADE,
    requested_by     BIGINT REFERENCES identity.users(id) ON DELETE SET NULL,
    document_type    TEXT NOT NULL DEFAULT '',
    title            TEXT NOT NULL DEFAULT '',
    description      TEXT NOT NULL DEFAULT '',
    deadline_at      TIMESTAMPTZ NOT NULL,
    status           TEXT NOT NULL DEFAULT 'pending'
        CHECK (status IN ('pending', 'submitted', 'fulfilled', 'cancelled')),
    submitted_doc_id BIGINT REFERENCES platform_admin.documents(id) ON DELETE SET NULL,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS doc_requests_org_status_idx ON platform_admin.document_requests (organization_id, status);

ALTER TABLE platform_admin.document_requests ENABLE ROW LEVEL SECURITY;

DO $$ BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_policies WHERE policyname = 'document_requests_policy') THEN
        CREATE POLICY document_requests_policy ON platform_admin.document_requests
            USING (true)
            WITH CHECK (true);
    END IF;
END $$;
