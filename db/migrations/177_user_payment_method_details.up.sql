-- Migration 177: keep the structured fields behind a saved payment method.
--
-- billing.user_payment_methods stored only a provider and a rendered string —
-- "CIB • أحمد محمد • IBAN: EG38..." — which is enough to display and not enough
-- to edit: there is no way back from that sentence to the bank name, the
-- holder and the IBAN that composed it. The edit route existed and no screen
-- could offer a form for it.
--
-- details holds the fields as submitted. account_identifier stays as the
-- rendered line every existing screen already reads, so nothing has to change
-- to keep working, and a row saved before this migration simply opens its edit
-- form with the sub-fields blank and the rendered line shown.
--
-- The provider CHECK is added at the same time. The column was free text and
-- the handler folds "vodafone_cash" onto "wallet", so a value outside the four
-- the platform renders could only have come from a request nobody validated.
BEGIN;

ALTER TABLE billing.user_payment_methods
  ADD COLUMN IF NOT EXISTS details JSONB NOT NULL DEFAULT '{}'::jsonb;

COMMENT ON COLUMN billing.user_payment_methods.details IS 'حقول وسيلة الدفع كما أدخلها المستخدم، لإعادة فتحها للتعديل';

UPDATE billing.user_payment_methods SET provider = 'wallet'
 WHERE provider IN ('vodafone_cash', 'e_wallet', 'mobile_wallet');
UPDATE billing.user_payment_methods SET provider = 'bank'
 WHERE provider NOT IN ('bank', 'instapay', 'wallet', 'card');

ALTER TABLE billing.user_payment_methods
  DROP CONSTRAINT IF EXISTS user_payment_methods_provider_check;
ALTER TABLE billing.user_payment_methods
  ADD CONSTRAINT user_payment_methods_provider_check
  CHECK (provider IN ('bank', 'instapay', 'wallet', 'card'));

COMMIT;
