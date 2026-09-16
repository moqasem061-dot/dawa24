-- Reverse 173_capsule_interactive.
--
-- Dropping the columns loses stored links and any attachment whose bytes were
-- kept here rather than in object storage. Both are recoverable in the sense
-- that matters: an answer stays readable without its links, and an attachment
-- older than a day was already due to be swept.

BEGIN;

ALTER TABLE assistant.attachments DROP COLUMN IF EXISTS content;
ALTER TABLE assistant.messages DROP COLUMN IF EXISTS entities;
ALTER TABLE assistant.turns DROP COLUMN IF EXISTS entities;

COMMIT;
