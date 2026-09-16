-- 060_two_account_types
--
-- Rebuild V2 §1.1: exactly two account types exist on the marketplace —
-- customer (صيدلية) and vendor (مورّد). Platform admin is staff, not an
-- account type. Every other value the legacy system produced (pharmacy,
-- chain_pharmacy, supplier, company, agency, individual, job_seeker) is
-- mapped onto these two or removed from the type column.
--
-- Customer or vendor is a property of the ORGANIZATION, never of the user.
-- identity.users.role becomes platform-level only: it says whether the
-- person is staff ('support','admin','super_admin','developer') or an
-- ordinary 'user'; capability comes from org membership.
--
-- A chain is a customer with several branches, not a third type; the
-- is_chain flag records it. branch_count is the registration-time snapshot
-- (migration 035); the live branches table stays the source of truth for
-- daily operations.

BEGIN;

-- ---------------------------------------------------------------------------
-- Organizations: two types
-- ---------------------------------------------------------------------------
ALTER TABLE org.organizations DROP CONSTRAINT IF EXISTS organizations_type_check;

UPDATE org.organizations SET type = 'vendor'
  WHERE type IN ('supplier','company','agency');

UPDATE org.organizations SET type = 'customer'
  WHERE type IN ('pharmacy','chain_pharmacy','individual');

UPDATE org.organizations SET type = 'customer'
  WHERE type NOT IN ('customer','vendor') OR type IS NULL;

ALTER TABLE org.organizations ALTER COLUMN type SET NOT NULL;
ALTER TABLE org.organizations ADD CONSTRAINT organizations_type_check
  CHECK (type IN ('customer','vendor'));

COMMENT ON COLUMN org.organizations.type IS
  'نوع الحساب — customer (صيدلية) أو vendor (مورّد). منصة الإدارة ليست نوع حساب.';

-- A chain is a customer with several branches, not a third type.
ALTER TABLE org.organizations
  ADD COLUMN IF NOT EXISTS is_chain BOOLEAN NOT NULL DEFAULT false;
UPDATE org.organizations SET is_chain = true WHERE branch_count > 1;


COMMENT ON COLUMN org.organizations.is_chain IS
  'سلسلة صيدليات — chain flag: customer with several branches, not a third type';

-- ---------------------------------------------------------------------------
-- identity.users.role becomes platform-level only
-- ---------------------------------------------------------------------------
UPDATE identity.users SET role = 'user'
  WHERE role NOT IN ('super_admin','admin','support','developer');

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conname = 'users_role_check' AND conrelid = 'identity.users'::regclass
    ) THEN
        ALTER TABLE identity.users ADD CONSTRAINT users_role_check
            CHECK (role IN ('user','support','admin','super_admin','developer'));
    END IF;
END $$;

COMMENT ON COLUMN identity.users.role IS
  'دور المنصة فقط — user/support/admin/super_admin/developer. نوع الحساب (customer/vendor) يأتي من org.organizations.type عبر العضوية.';

COMMIT;