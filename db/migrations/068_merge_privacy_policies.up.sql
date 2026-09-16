-- 068_merge_privacy_policies (up)
--
-- Rebuild V2 §2.4b — platform_admin.privacy_policies (slug + INT version)
-- merges into platform_admin.policies (policy_key + VARCHAR version).
-- Migrated rows are tagged policy_type = 'privacy' so the down migration can
-- restore exactly the rows that came from the legacy table.

BEGIN;

ALTER TABLE platform_admin.policies
    ADD COLUMN IF NOT EXISTS policy_type TEXT NOT NULL DEFAULT 'platform';

COMMENT ON COLUMN platform_admin.policies.policy_type IS
  'نوع السياسة — platform: سياسات المنصة الرئيسية، privacy: واردة من جدول privacy_policies القديم (068)';

INSERT INTO platform_admin.policies (
    policy_key, version, title, content, summary, is_published,
    published_at, created_at, updated_at, policy_type
)
SELECT slug, version::TEXT, title, content, '{"ar":"","en":""}'::jsonb,
       is_published, published_at, created_at, updated_at, 'privacy'
FROM platform_admin.privacy_policies
ON CONFLICT (policy_key, version) DO NOTHING;

DROP TABLE platform_admin.privacy_policies;

COMMIT;
