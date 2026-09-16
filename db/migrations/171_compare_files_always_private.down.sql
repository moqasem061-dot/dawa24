-- 171_compare_files_always_private (down)
--
-- Which files a vendor had published is not recorded anywhere else, so the
-- normalised rows cannot be restored. Only the column comment is reverted.

BEGIN;

COMMENT ON COLUMN compare.files.visibility IS
    'مستوى ظهور الملف: private (خاص بالمورد) أو public (ظاهر في خصومات السوق العامة).';

COMMIT;
