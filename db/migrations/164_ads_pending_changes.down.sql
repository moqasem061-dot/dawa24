-- Migration 164 Down: Drop pending_changes column from promo.ads
ALTER TABLE promo.ads DROP COLUMN IF EXISTS pending_changes;
