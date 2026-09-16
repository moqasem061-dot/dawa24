-- Migration 212 Down: Re-enable wallet checkout if needed
UPDATE billing.platform_payment_methods
SET is_checkout_enabled = true
WHERE id = 'wallet';
