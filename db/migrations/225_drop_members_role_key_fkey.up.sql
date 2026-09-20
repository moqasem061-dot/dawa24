BEGIN;

-- Drop obsolete foreign key constraint from org.members to identity.roles(key).
-- Custom roles created in org.roles have keys not present in identity.roles,
-- and org.members.org_role_id (added in 052/145) handles the role link.
ALTER TABLE org.members DROP CONSTRAINT IF EXISTS members_role_key_fkey;

COMMIT;
