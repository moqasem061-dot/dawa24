BEGIN;

-- Restoring the foreign key constraint requires all role_key values to exist in identity.roles.
-- Custom roles in org.roles do not exist in identity.roles, so the constraint is not restored unconditionally.

COMMIT;
