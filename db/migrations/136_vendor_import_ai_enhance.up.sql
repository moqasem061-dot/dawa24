-- 136_vendor_import_ai_enhance (up)
--
-- The vendor catalogue import's AI tier stopped being a per-batch adjudicator
-- and became the same enhancement stage the smart order runs — one pass over
-- the whole file's residue, against a shared catalogue window, through the
-- shared decision cache in catalog.match_decisions.
--
-- The record of what it did has to say so. One JSONB column rather than six
-- integers, because the shape of that record is the stage's own business and
-- has already changed once; a document keeps the next change out of a
-- migration. Nothing queries inside it — the review screen reads the whole
-- session row anyway — so there is no index to justify.
--
-- Nothing else changes. The decision cache and the alias ledger the stage reads
-- and writes already exist, created for the smart order, and sharing them is
-- the point: an answer bought by a pharmacy's order is free to the vendor whose
-- price list asks the same question.

BEGIN;

ALTER TABLE ingest.catalog_imports
    ADD COLUMN IF NOT EXISTS ai_stats JSONB NOT NULL DEFAULT '{}'::JSONB;

COMMENT ON COLUMN ingest.catalog_imports.ai_stats IS
    'حصيلة مرحلة المطابقة الذكية: كم صنفاً روجع، وكم أُجيب من الذاكرة، وكم تحسّن فعلياً';

COMMIT;
