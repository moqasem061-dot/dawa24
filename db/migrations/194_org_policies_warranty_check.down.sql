-- Revert org.organization_policies policy_type check constraint to exclude 'warranty'
ALTER TABLE org.organization_policies DROP CONSTRAINT IF EXISTS organization_policies_policy_type_check;
ALTER TABLE org.organization_policies ADD CONSTRAINT organization_policies_policy_type_check
    CHECK (policy_type IN ('terms', 'returns', 'privacy', 'shipping'));
