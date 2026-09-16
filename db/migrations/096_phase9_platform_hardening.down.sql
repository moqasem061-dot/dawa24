-- Migration 096: Rollback Phase 9 Platform Hardening

DROP TABLE IF EXISTS platform_admin.ai_providers CASCADE;
DROP TABLE IF EXISTS identity.session_plan_requests CASCADE;
DROP TABLE IF EXISTS identity.user_session_histories CASCADE;
DROP TABLE IF EXISTS identity.user_sessions CASCADE;
