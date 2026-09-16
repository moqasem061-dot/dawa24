-- 105_align_org_type_check.up.sql
-- Ensure org.organizations type check constraint accepts all valid domain types
BEGIN;

ALTER TABLE org.organizations DROP CONSTRAINT IF EXISTS organizations_type_check;
ALTER TABLE org.organizations ADD CONSTRAINT organizations_type_check
    CHECK (type IN ('vendor', 'customer', 'supplier', 'pharmacy', 'chain_pharmacy', 'company', 'agency'));

COMMIT;
