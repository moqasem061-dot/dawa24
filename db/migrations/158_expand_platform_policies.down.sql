-- 158_expand_platform_policies (down)
DELETE FROM platform_admin.policies
WHERE policy_key IN ('shipping_return', 'cookies', 'payment') AND version = '2.0';