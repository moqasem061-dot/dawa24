-- 174_drop_superseded_registration_branches
--
-- Until now, registration auto-inserted a hidden "main branch" named after the
-- organization. The owner would then create their first real branch from the
-- Branches page and mark it main — leaving the auto row behind as a second,
-- unexplained branch (is_main flipped to false by that promotion). It showed up
-- in the branch count and, through a manager's stale branch_id, in the
-- Employees screen as a location nobody had created.
--
-- The registration insert is now gone (see identity/postgres/register_org.go),
-- and org.service.CreateBranch promotes the first real branch to main. This
-- migration clears the rows the old behaviour already left behind — but only
-- the ones that are provably the untouched auto row AND have been superseded by
-- a real branch, so an org that still has only its registration branch keeps
-- operating with it.

BEGIN;

WITH superseded AS (
    SELECT b.id, b.organization_id
    FROM org.branches b
    JOIN org.organizations o ON o.id = b.organization_id
    WHERE b.deleted_at IS NULL
      AND b.is_main = false
      AND COALESCE(b.code, '') = ''
      AND b.manager_id IS NULL
      AND (
            b.name->>'ar' = o.legal_name
         OR b.name->>'ar' = o.trade_name->>'ar'
         OR b.name->>'en' = o.trade_name->>'en'
      )
      -- another active branch exists for the same org
      AND EXISTS (
            SELECT 1 FROM org.branches x
            WHERE x.organization_id = b.organization_id
              AND x.id <> b.id
              AND x.deleted_at IS NULL
      )
)
, cleared_members AS (
    UPDATE org.members m
    SET branch_id = NULL, updated_at = now()
    FROM superseded s
    WHERE m.branch_id = s.id
    RETURNING m.id
)
UPDATE org.branches b
SET deleted_at = now(), status = 'inactive', updated_at = now()
FROM superseded s
WHERE b.id = s.id;

-- Keep the denormalised counter honest wherever it now disagrees with reality.
UPDATE org.organizations o
SET branch_count = c.n
FROM (
    SELECT o2.id, COUNT(b.id) AS n
    FROM org.organizations o2
    LEFT JOIN org.branches b ON b.organization_id = o2.id AND b.deleted_at IS NULL
    GROUP BY o2.id
) c
WHERE c.id = o.id AND o.branch_count IS DISTINCT FROM c.n;

COMMIT;
