-- Migration 056: User, Profile & Referral Parity
-- Aligns identity.users and profile.user_profiles with KYC, referrals, and geo-radius.

BEGIN;

-- 1. Extend identity.users
ALTER TABLE identity.users
    ADD COLUMN IF NOT EXISTS first_name     TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS last_name      TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS referral_code  TEXT NOT NULL DEFAULT ('REF' || upper(substr(replace(gen_random_uuid()::text, '-', ''), 1, 8))),
    ADD COLUMN IF NOT EXISTS referred_by    BIGINT REFERENCES identity.users(id) ON DELETE SET NULL,
    ADD COLUMN IF NOT EXISTS referral_count INT NOT NULL DEFAULT 0;

CREATE INDEX IF NOT EXISTS idx_users_referral_code ON identity.users (referral_code);

-- 2. Extend profile.user_profiles
ALTER TABLE profile.user_profiles
    ADD COLUMN IF NOT EXISTS national_id      TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS passport_number  TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS latitude         NUMERIC(10,8),
    ADD COLUMN IF NOT EXISTS longitude        NUMERIC(11,8),
    ADD COLUMN IF NOT EXISTS radius_meters    INT NOT NULL DEFAULT 10000;

COMMIT;
