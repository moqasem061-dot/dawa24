-- Migration 053: Multi-Category Ratings & Reviews
-- Extends organization reviews with order and product links, moderation, replies,
-- and criteria-based weighted scores.

BEGIN;

-- 1. Extend Organization Reviews
ALTER TABLE org.organization_reviews
    ADD COLUMN IF NOT EXISTS order_id      BIGINT REFERENCES commerce.orders(id) ON DELETE SET NULL,
    ADD COLUMN IF NOT EXISTS product_id    BIGINT REFERENCES catalog.products(id) ON DELETE SET NULL,
    ADD COLUMN IF NOT EXISTS title         TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS response      TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS response_at   TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS responded_by  BIGINT REFERENCES identity.users(id) ON DELETE SET NULL,
    ADD COLUMN IF NOT EXISTS is_verified   BOOLEAN NOT NULL DEFAULT false,
    ADD COLUMN IF NOT EXISTS is_public     BOOLEAN NOT NULL DEFAULT true,
    ADD COLUMN IF NOT EXISTS status        TEXT NOT NULL DEFAULT 'approved'
        CHECK (status IN ('pending','approved','rejected')),
    ADD COLUMN IF NOT EXISTS helpful_count INT NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS context       TEXT NOT NULL DEFAULT 'supplier'
        CHECK (context IN ('supplier','pharmacy','product','delivery')),
    ADD COLUMN IF NOT EXISTS deleted_at    TIMESTAMPTZ;

CREATE UNIQUE INDEX IF NOT EXISTS reviews_one_per_order
    ON org.organization_reviews (user_id, order_id)
    WHERE order_id IS NOT NULL AND deleted_at IS NULL;

-- 2. Review Criteria
CREATE TABLE IF NOT EXISTS org.review_criteria (
    key         TEXT PRIMARY KEY,
    name        JSONB NOT NULL,
    context     TEXT NOT NULL,
    weight      NUMERIC(4,3) NOT NULL DEFAULT 1.000,
    sort_order  INT NOT NULL DEFAULT 0,
    is_active   BOOLEAN NOT NULL DEFAULT true
);

-- 3. Review Ratings (Per-Criterion Score)
CREATE TABLE IF NOT EXISTS org.review_ratings (
    review_id    BIGINT NOT NULL REFERENCES org.organization_reviews(id) ON DELETE CASCADE,
    criterion    TEXT NOT NULL REFERENCES org.review_criteria(key) ON DELETE CASCADE,
    score        SMALLINT NOT NULL CHECK (score BETWEEN 1 AND 5),
    PRIMARY KEY (review_id, criterion)
);

ALTER TABLE org.review_criteria ENABLE ROW LEVEL SECURITY;
ALTER TABLE org.review_ratings ENABLE ROW LEVEL SECURITY;

DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_policies WHERE policyname = 'review_criteria_policy') THEN
        CREATE POLICY review_criteria_policy ON org.review_criteria
            USING (true)
            WITH CHECK (true);
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_policies WHERE policyname = 'review_ratings_policy') THEN
        CREATE POLICY review_ratings_policy ON org.review_ratings
            USING (true)
            WITH CHECK (true);
    END IF;
END $$;

-- 4. Seed Standard Arabic/English Review Criteria
INSERT INTO org.review_criteria (key, name, context, weight, sort_order) VALUES
    ('delivery_speed', '{"ar":"سرعة التوصيل","en":"Delivery Speed"}'::jsonb, 'supplier', 1.000, 1),
    ('product_quality', '{"ar":"جودة وسلامة المنتجات","en":"Product Quality"}'::jsonb, 'supplier', 1.000, 2),
    ('rep_service', '{"ar":"تعامل المندوب والدعم","en":"Representative Service"}'::jsonb, 'supplier', 0.800, 3),
    ('price_fairness', '{"ar":"مناسبة الأسعار","en":"Price Fairness"}'::jsonb, 'supplier', 0.900, 4),
    ('order_accuracy', '{"ar":"دقة تنفيذ الطلب","en":"Order Accuracy"}'::jsonb, 'supplier', 1.000, 5),
    ('packaging', '{"ar":"جودة التغليف والتبريد","en":"Packaging & Cold Chain"}'::jsonb, 'supplier', 0.800, 6),
    ('payment_commitment', '{"ar":"الالتزام بالسداد","en":"Payment Commitment"}'::jsonb, 'pharmacy', 1.000, 1),
    ('communication', '{"ar":"سهولة التواصل","en":"Communication"}'::jsonb, 'pharmacy', 0.800, 2),
    ('receiving_speed', '{"ar":"سرعة الاستلام","en":"Receiving Speed"}'::jsonb, 'pharmacy', 0.800, 3),
    ('effectiveness', '{"ar":"فعالية المنتج","en":"Effectiveness"}'::jsonb, 'product', 1.000, 1),
    ('matches_description', '{"ar":"مطابقة الوصف","en":"Matches Description"}'::jsonb, 'product', 1.000, 2),
    ('expiry_period', '{"ar":"فترة الصلاحية","en":"Expiry Period"}'::jsonb, 'product', 0.900, 3)
ON CONFLICT (key) DO UPDATE SET is_active = true;

COMMIT;
