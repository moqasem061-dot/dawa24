-- 127_smartorder_files (up)
--
-- The uploaded workbook has to survive the gap between step 1 (upload) and
-- step 2 (confirm the column mapping), because the rows are not staged until
-- the buyer has confirmed how to read them.
--
-- It was originally held in process memory. That works on a single instance and
-- fails in every other case: a redeploy between the two steps loses the file, a
-- second instance never had it, and the buyer is told to upload again for no
-- reason they can see. A pharmacy re-uploading a 9,000-line workbook because a
-- deployment happened is not an acceptable failure.
--
-- Stored as BYTEA on its own table rather than a column on runs: the blob is
-- large and read exactly once, and keeping it off the run row means listing a
-- buyer's history never drags megabytes through the pool. Deleted as soon as the
-- rows are staged.

BEGIN;

CREATE TABLE smartorder.run_files (
    run_id          BIGINT PRIMARY KEY REFERENCES smartorder.runs(id) ON DELETE CASCADE,
    organization_id BIGINT NOT NULL REFERENCES org.organizations(id) ON DELETE CASCADE,
    filename        TEXT NOT NULL DEFAULT '',
    content         BYTEA NOT NULL,
    size_bytes      INT NOT NULL DEFAULT 0,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

COMMENT ON TABLE  smartorder.run_files IS 'الملف المرفوع - يُحفظ بين خطوة الرفع وخطوة تعيين الأعمدة ثم يُحذف';
COMMENT ON COLUMN smartorder.run_files.content IS 'محتوى الملف - يُحذف فور تجهيز الصفوف';

CREATE INDEX run_files_org_idx     ON smartorder.run_files (organization_id);
CREATE INDEX run_files_created_idx ON smartorder.run_files (created_at);

ALTER TABLE smartorder.run_files ENABLE ROW LEVEL SECURITY;
ALTER TABLE smartorder.run_files FORCE ROW LEVEL SECURITY;
CREATE POLICY run_files_tenant_isolation ON smartorder.run_files
    USING (platform.tenant_visible(organization_id))
    WITH CHECK (platform.tenant_visible(organization_id));

COMMIT;
