-- Irreversible by design: the previous NULL carried no information, so there is
-- nothing to restore. Clearing branch_id again would orphan rows that are now
-- correctly linked.
SELECT 1;
