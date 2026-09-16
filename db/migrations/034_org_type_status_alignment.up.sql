-- org.organizations.type and .status rejected values the application produces.
--
-- The CHECK constraints were written from the legacy schema
-- (enum('supplier','company','agency')), while internal/modules/org/domain.go
-- defines supplier / pharmacy / chain_pharmacy. Nothing reconciled the two, so
-- registering a pharmacy - the primary customer type on a pharmaceutical
-- marketplace - failed with a constraint violation surfaced as a 500. The same
-- applies to status: the domain has StatusSuspended and the constraint did not,
-- so suspending an organisation was impossible.
--
-- Both sets are permitted here rather than one replacing the other. The legacy
-- values are still needed: the ETL loads existing organisations verbatim and
-- preserves their primary keys, so 'company' and 'agency' must remain insertable
-- until that data is mapped.
--
-- OPEN DECISION, for the owner: how legacy 'company' and 'agency' map onto
-- supplier / pharmacy / chain_pharmacy. Until that is answered the ETL cannot
-- translate them and they load as-is. This migration deliberately does not guess.

BEGIN;

ALTER TABLE org.organizations DROP CONSTRAINT IF EXISTS organizations_type_check;
ALTER TABLE org.organizations ADD CONSTRAINT organizations_type_check
    CHECK (type IN ('supplier','pharmacy','chain_pharmacy','company','agency'));

ALTER TABLE org.organizations DROP CONSTRAINT IF EXISTS organizations_status_check;
ALTER TABLE org.organizations ADD CONSTRAINT organizations_status_check
    CHECK (status IN ('pending','approved','rejected','suspended'));

COMMIT;
