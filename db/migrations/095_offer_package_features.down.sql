-- Migration 095: Drop Offer Package Features
-- Schema: promo

DROP TABLE IF EXISTS promo.offer_package_features CASCADE;
DROP INDEX IF EXISTS promo.idx_offer_views_created_at;
DROP INDEX IF EXISTS promo.idx_offer_clicks_created_at;
