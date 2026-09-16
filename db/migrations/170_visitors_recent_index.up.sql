-- 170_visitors_recent_index
--
-- The admin dashboard's "recent visitors" list reads
--
--   SELECT … FROM platform_admin.visitors ORDER BY created_at DESC, id DESC
--   LIMIT $1 OFFSET $2
--
-- and the only index on the table is on visited_at, which that ORDER BY cannot
-- use. Postgres therefore read and sorted every row in the table to hand back
-- ten of them, on a table that gains a row per visitor per day and is the
-- largest one on the platform.
--
-- Combined with the five sequential GROUP BY scans the same handler used to
-- run, this is what made an admin dashboard load hold a pooled connection long
-- enough to starve ordinary traffic and eventually time out into a 502.
--
-- CREATE INDEX is not CONCURRENTLY here because the migration runner wraps each
-- migration in a transaction and CONCURRENTLY cannot run inside one.

BEGIN;

CREATE INDEX IF NOT EXISTS visitors_created_at_idx
    ON platform_admin.visitors (created_at DESC, id DESC);

COMMIT;
