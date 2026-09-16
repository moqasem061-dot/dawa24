-- 171_compare_files_always_private
--
-- خصومات السوق العامة is fed by temporary warehouses only. The per-file
-- "publish to the public market" toggle that used to set
-- compare.files.visibility to 'public' has been removed from the Compare Tool,
-- and no query reads the column any more (migration 157 added it). Normalise
-- the rows that toggle left behind so nothing in the table still claims a
-- Compare Tool upload is public.

BEGIN;

UPDATE compare.files
SET visibility = 'private', updated_at = now()
WHERE visibility IS DISTINCT FROM 'private'
  AND is_temp_warehouse = FALSE;

COMMENT ON COLUMN compare.files.visibility IS
    'دائماً private. ملفات أداة المقارنة لا تظهر في خصومات السوق العامة إطلاقاً؛ الصفحة تُغذَّى من المستودعات المؤقتة فقط.';

COMMIT;
