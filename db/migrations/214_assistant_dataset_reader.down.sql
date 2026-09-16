-- 214_assistant_dataset_reader.down.sql
-- Roles are cluster-wide; privileges are per database. Remove this database's
-- privileges, then drop the role only if no other database still uses it.
BEGIN;
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'dawa24_assistant_ro') THEN
        EXECUTE 'DROP OWNED BY dawa24_assistant_ro';
        BEGIN
            DROP ROLE dawa24_assistant_ro;
        EXCEPTION WHEN dependent_objects_still_exist THEN
            RAISE NOTICE 'dawa24_assistant_ro still has privileges in another database; kept';
        END;
    END IF;
END $$;
COMMIT;
