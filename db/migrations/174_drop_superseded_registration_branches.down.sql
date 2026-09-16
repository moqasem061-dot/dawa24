-- 174_drop_superseded_registration_branches (down)
--
-- The soft-deleted rows can be resurrected, but the members whose branch_id was
-- cleared cannot be re-pointed — which branch each one belonged to is not
-- recorded anywhere else. Undelete the branches only.

BEGIN;

UPDATE org.branches b
SET deleted_at = NULL, status = 'active', updated_at = now()
WHERE b.deleted_at IS NOT NULL
  AND b.is_main = false
  AND COALESCE(b.code, '') = ''
  AND b.manager_id IS NULL;

COMMIT;
