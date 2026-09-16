-- Migration 077: Institutional Works Table and Schema
BEGIN;

CREATE TABLE IF NOT EXISTS org.institutional_works (
    id           BIGSERIAL PRIMARY KEY,
    public_id    UUID NOT NULL DEFAULT gen_random_uuid(),
    title        JSONB NOT NULL DEFAULT '{"ar":"","en":""}'::jsonb,
    description  JSONB NOT NULL DEFAULT '{"ar":"","en":""}'::jsonb,
    icon         TEXT NOT NULL DEFAULT 'building',
    pricing_type TEXT NOT NULL DEFAULT 'free',
    is_active    BOOLEAN NOT NULL DEFAULT true,
    view_type    INT NOT NULL DEFAULT 1,
    slug         TEXT NOT NULL DEFAULT '',
    parent_id    BIGINT REFERENCES org.institutional_works(id) ON DELETE SET NULL,
    sort_order   INT NOT NULL DEFAULT 0,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at   TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_institutional_works_parent ON org.institutional_works (parent_id) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_institutional_works_active ON org.institutional_works (is_active) WHERE deleted_at IS NULL;

-- Ensure institutional_work_id column exists on org.branch_institutional_works
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM information_schema.columns 
        WHERE table_schema = 'org' AND table_name = 'branch_institutional_works' AND column_name = 'institutional_work_id'
    ) THEN
        ALTER TABLE org.branch_institutional_works ADD COLUMN institutional_work_id BIGINT REFERENCES org.institutional_works(id) ON DELETE CASCADE;
        CREATE INDEX IF NOT EXISTS idx_branch_inst_works_work_id ON org.branch_institutional_works (institutional_work_id);
    END IF;
END $$;

COMMIT;
