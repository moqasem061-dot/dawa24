-- Migration 164: Add pending_changes column to promo.ads for non-disruptive vendor edit moderation
ALTER TABLE promo.ads ADD COLUMN IF NOT EXISTS pending_changes JSONB DEFAULT NULL;
COMMENT ON COLUMN promo.ads.pending_changes IS 'التعديلات المقترحة من المورد قيد مراجعة الإدارة دون إيقاف الإعلان المباشر';
