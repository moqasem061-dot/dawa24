-- 106_drop_workflow_services.down.sql
CREATE TABLE IF NOT EXISTS workflow.services (
    id BIGSERIAL PRIMARY KEY,
    public_id UUID NOT NULL DEFAULT gen_random_uuid(),
    title JSONB NOT NULL,
    description JSONB NOT NULL,
    icon TEXT,
    pricing_type TEXT NOT NULL DEFAULT 'free',
    parent_id BIGINT REFERENCES workflow.services(id) ON DELETE SET NULL,
    is_active BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
