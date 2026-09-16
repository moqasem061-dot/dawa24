-- Reverses migration 179.
--
-- The counter on promo.sponsorship_purchases is unaffected: this table records
-- movements, it does not own the balance, so dropping it loses the history and
-- nothing else.

DROP TABLE IF EXISTS promo.sponsorship_credit_entries;
