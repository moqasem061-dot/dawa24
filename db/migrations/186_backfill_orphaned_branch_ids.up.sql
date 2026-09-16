-- Link orphaned variants, warehouses and offers to their organisation's main branch.
--
-- This ran in cmd/server/main.go until now: a goroutine, started on every boot
-- of every replica, that issued these three unbounded UPDATEs against the live
-- database while the process it lived in was serving requests.
--
-- It is a one-time data repair, not a background task. It did real work the
-- first time it ran and then re-scanned three tables to find nothing on every
-- subsequent deploy and restart — spending CPU and I/O the web process needed
-- for requests, and racing whatever other replicas were starting beside it. A
-- migration runs once, before the server takes traffic, under the advisory lock
-- that already serialises migrations across replicas.
--
-- Each statement is a no-op once applied: every row it targets has branch_id
-- IS NULL, and it sets branch_id to a value that is not null. Re-running it is
-- therefore harmless, which is what makes moving it here safe for a database
-- where the old boot task has already done the work.

UPDATE catalog.product_variants v
SET branch_id = (
    SELECT b.id FROM org.branches b
    WHERE b.organization_id = v.organization_id AND b.deleted_at IS NULL
    ORDER BY b.is_main DESC, b.id ASC LIMIT 1
)
WHERE v.branch_id IS NULL;

UPDATE inventory.warehouses w
SET branch_id = (
    SELECT b.id FROM org.branches b
    WHERE b.organization_id = w.organization_id AND b.deleted_at IS NULL
    ORDER BY b.is_main DESC, b.id ASC LIMIT 1
)
WHERE w.branch_id IS NULL;

UPDATE promo.offers o
SET branch_id = (
    SELECT b.id FROM org.branches b
    WHERE b.organization_id = o.organization_id AND b.deleted_at IS NULL
    ORDER BY b.is_main DESC, b.id ASC LIMIT 1
)
WHERE o.branch_id IS NULL;
