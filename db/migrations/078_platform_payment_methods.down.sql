-- Migration 078 Down: Drop billing.platform_payment_methods
DROP TABLE IF EXISTS billing.platform_payment_methods CASCADE;
