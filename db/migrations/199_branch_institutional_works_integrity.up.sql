-- 199_branch_institutional_works_integrity.up.sql
-- Repair corporate operations / institutional work integrity on branches (Defect #11 & #16)

-- 1. Backfill institutional_work_id from work_category where missing and a match exists
UPDATE org.branch_institutional_works biw
SET institutional_work_id = iw.id
FROM org.institutional_works iw
WHERE biw.institutional_work_id IS NULL
  AND iw.deleted_at IS NULL
  AND (iw.slug = biw.work_category OR iw.id::text = biw.work_category);

-- 2. Clean up any orphan records where the branch itself was deleted
DELETE FROM org.branch_institutional_works biw
WHERE EXISTS (
    SELECT 1 FROM org.branches b
    WHERE b.id = biw.branch_id AND b.deleted_at IS NOT NULL
);

-- 3. Create index on institutional_work_id if not present
CREATE INDEX IF NOT EXISTS idx_biw_work_id ON org.branch_institutional_works (institutional_work_id);