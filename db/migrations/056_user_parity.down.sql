-- Migration 056 Down

BEGIN;

ALTER TABLE profile.user_profiles
    DROP COLUMN IF EXISTS national_id,
    DROP COLUMN IF EXISTS passport_number,
    DROP COLUMN IF EXISTS latitude,
    DROP COLUMN IF EXISTS longitude,
    DROP COLUMN IF EXISTS radius_meters;

ALTER TABLE identity.users
    DROP COLUMN IF EXISTS first_name,
    DROP COLUMN IF EXISTS last_name,
    DROP COLUMN IF EXISTS referral_code,
    DROP COLUMN IF EXISTS referred_by,
    DROP COLUMN IF EXISTS referral_count;

COMMIT;
