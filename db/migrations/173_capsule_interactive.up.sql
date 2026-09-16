-- 173_capsule_interactive
--
-- Two changes, both about the assistant being usable rather than merely
-- correct.
--
-- 1. Answers become clickable. A turn now records which of the caller's own
--    records it named — an order number, a product, a supplier — together with
--    the dashboard link for each. The drawer renders those as inline links and
--    as a reference strip under the answer, so "طلب رقم PO-1042" is one click
--    away from the order and its فاتورة instead of being a number to go and
--    look up. It is stored rather than recomputed because reopening a
--    conversation from history must not degrade to plain text, and because the
--    polling fallback reads the turn row and never sees the stream.
--
-- 2. Attachments stop depending on object storage being reachable. The bytes
--    still go to S3/MinIO when there is one; when there is not — no bucket
--    configured, a credential expired, the endpoint down — they are kept here
--    instead, and the upload succeeds. Before this, an unconfigured bucket made
--    every attachment fail with a generic error, which is exactly what a
--    photographed prescription looked like from the user's side: broken.
--
--    This is deliberately bounded: assistant attachments are capped at 10 MB
--    each, are swept after 24 hours when no message references them, and go
--    with their conversation after six months. It is a fallback for small
--    short-lived blobs, not a file store.

BEGIN;

-- ---------------------------------------------------------------------------
-- 1. Interactive references
-- ---------------------------------------------------------------------------

ALTER TABLE assistant.turns
    ADD COLUMN IF NOT EXISTS entities JSONB NOT NULL DEFAULT '[]'::jsonb;

ALTER TABLE assistant.messages
    ADD COLUMN IF NOT EXISTS entities JSONB NOT NULL DEFAULT '[]'::jsonb;

COMMENT ON COLUMN assistant.turns.entities IS
    'السجلات التي ذكرتها الإجابة مع رابط كل سجل داخل لوحة التحكم';
COMMENT ON COLUMN assistant.messages.entities IS
    'السجلات المرتبطة بالرسالة، تُحفظ لتبقى الروابط تعمل عند فتح المحادثة لاحقاً';

-- ---------------------------------------------------------------------------
-- 2. Attachment bytes, when object storage is not available
-- ---------------------------------------------------------------------------

ALTER TABLE assistant.attachments
    ADD COLUMN IF NOT EXISTS content BYTEA;

COMMENT ON COLUMN assistant.attachments.content IS
    'محتوى الملف عند تعذّر التخزين السحابي؛ فارغ عندما يكون storage_key مضبوطاً';

COMMIT;
