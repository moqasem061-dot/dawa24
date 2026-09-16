-- Migration 049 Down

BEGIN;

ALTER TABLE platform_admin.documents
    DROP COLUMN IF EXISTS user_id,
    DROP COLUMN IF EXISTS status,
    DROP COLUMN IF EXISTS review_notes,
    DROP COLUMN IF EXISTS reviewed_by,
    DROP COLUMN IF EXISTS reviewed_at,
    DROP COLUMN IF EXISTS mime_type,
    DROP COLUMN IF EXISTS size_bytes,
    DROP COLUMN IF EXISTS original_name,
    DROP COLUMN IF EXISTS meta,
    DROP COLUMN IF EXISTS deleted_at,
    DROP COLUMN IF EXISTS updated_at;

DROP SCHEMA IF EXISTS chat CASCADE;

COMMIT;
