-- Migration 179: where a package's credits went.
--
-- promo.sponsorship_purchases carries credits_total and credits_used, and that
-- is the whole record. A supplier who buys 50 credits and finds 31 left can see
-- that 19 went; nothing anywhere says to what. Six call sites move that counter
-- — an ad going live, an ad being refused, a sponsorship request submitted, a
-- batch of requests created, a moderator rejecting one — and four of them do it
-- with the error discarded (`_ = s.repo.Increment...`), so a refund that failed
-- looks exactly like a refund that worked.
--
-- This table is the ledger those movements were missing. Every change to
-- credits_used writes one row in the same transaction, so the counter and the
-- history cannot disagree, and كشف حساب للباقة becomes a query rather than a
-- reconstruction.
--
-- delta is signed: a consumption is negative, a refund positive, matching the
-- direction of the remaining balance rather than the direction of credits_used.
-- balance_after is stored rather than derived because a statement read at page
-- 3 of 40 should not have to sum the preceding 39.

CREATE TABLE IF NOT EXISTS promo.sponsorship_credit_entries (
    id              BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    public_id       UUID        NOT NULL DEFAULT gen_random_uuid(),
    organization_id BIGINT      NOT NULL REFERENCES org.organizations(id) ON DELETE CASCADE,
    purchase_id     BIGINT      NOT NULL REFERENCES promo.sponsorship_purchases(id) ON DELETE CASCADE,
    delta           INTEGER     NOT NULL,
    balance_after   INTEGER     NOT NULL,
    reason          TEXT        NOT NULL,
    entity_type     TEXT        NOT NULL DEFAULT '',
    entity_id       BIGINT,
    actor_user_id   BIGINT      REFERENCES identity.users(id) ON DELETE SET NULL,
    note            TEXT        NOT NULL DEFAULT '',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT sponsorship_credit_entries_delta_chk CHECK (delta <> 0),
    CONSTRAINT sponsorship_credit_entries_reason_chk CHECK (reason IN (
        'ad_created',
        'ad_refunded',
        'sponsorship_requested',
        'sponsorship_batch',
        'sponsorship_rejected',
        'adjustment'
    ))
);

COMMENT ON TABLE  promo.sponsorship_credit_entries               IS 'سجل استهلاك وإرجاع رصيد باقات الرعاية';
COMMENT ON COLUMN promo.sponsorship_credit_entries.delta         IS 'سالب للاستهلاك، موجب للإرجاع';
COMMENT ON COLUMN promo.sponsorship_credit_entries.balance_after IS 'الرصيد المتبقي بعد هذه الحركة';
COMMENT ON COLUMN promo.sponsorship_credit_entries.entity_type   IS 'نوع الكيان المستهلك: إعلان أو طلب رعاية';

CREATE UNIQUE INDEX IF NOT EXISTS sponsorship_credit_entries_public_id_key
    ON promo.sponsorship_credit_entries (public_id);

-- The statement reads one purchase newest-first; the organization index serves
-- the admin drill-down that spans a company's purchases.
CREATE INDEX IF NOT EXISTS idx_sponsorship_credit_entries_purchase
    ON promo.sponsorship_credit_entries (purchase_id, created_at DESC, id DESC);

CREATE INDEX IF NOT EXISTS idx_sponsorship_credit_entries_org
    ON promo.sponsorship_credit_entries (organization_id, created_at DESC);

ALTER TABLE promo.sponsorship_credit_entries ENABLE ROW LEVEL SECURITY;
ALTER TABLE promo.sponsorship_credit_entries FORCE ROW LEVEL SECURITY;

CREATE POLICY sponsorship_credit_entries_tenant_isolation ON promo.sponsorship_credit_entries
    USING (platform.tenant_visible(organization_id))
    WITH CHECK (platform.tenant_visible(organization_id));

-- Backfill: every purchase that has already spent credits gets one opening
-- entry, so a statement opened the day this ships balances to the counter
-- instead of starting from a total that does not match its own history.
INSERT INTO promo.sponsorship_credit_entries
    (organization_id, purchase_id, delta, balance_after, reason, note, created_at)
SELECT p.organization_id,
       p.id,
       -p.credits_used,
       GREATEST(p.credits_total - p.credits_used, 0),
       'adjustment',
       'رصيد مستهلك قبل بدء تسجيل الحركات',
       p.created_at
FROM promo.sponsorship_purchases p
WHERE p.credits_used > 0
  AND NOT EXISTS (
      SELECT 1 FROM promo.sponsorship_credit_entries e WHERE e.purchase_id = p.id
  );
