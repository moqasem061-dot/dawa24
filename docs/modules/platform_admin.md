# Module: Platform Admin

## Overview

The `platform_admin` bounded context provides global administrative configurations, geographical reference data (countries, cities), and public tenant settings.

## Schema Mapping

- **PostgreSQL Schemas:** `platform_admin`
- **Migrations:** `012_platform_admin.up.sql`, `082_developer_tables.up.sql`
- **Tables Owned:**
  - `platform_admin.system_settings` — Key-value system configurations with public visibility flags.
  - `platform_admin.countries` & `platform_admin.cities` — Normalized geography master data.
  - `platform_admin.sql_logs` — Audit log of all queries executed through the Developer SQL Console.
  - `platform_admin.error_logs` — System diagnostic error tracking and resolution workflow.

## Invariants & Rules

1. **Global Configuration:** System settings can be marked `is_public: true` to be safely exposed to frontend clients without authentication.
2. **Normalized Geography:** Countries and cities are structured with standardized ISO-3166 alpha codes, international dialing prefixes, and local currencies.
3. **Hardened SQL Console (Phase 0 Task 0.3):**
   - **Read-only enforcement:** Enforced at PostgreSQL engine level via `SET LOCAL transaction_read_only = on` in addition to query prefix pre-filtering. Any write (DML/DDL/write CTEs) is rejected.
   - **Statement timeout:** Strict 10-second timeout via `SET LOCAL statement_timeout = '10s'`.
   - **Row cap:** Maximum 1,000 rows returned per execution with `Truncated: true` indicator.
   - **Always rolled back:** Diagnostic transactions never commit state.
   - **Full audit trail:** Every execution is asynchronously recorded in `platform_admin.sql_logs`.
4. **Error Log Porting Decision:**
   - Laravel's `full_error_logs` table had 69 columns. The Go version retains all operational diagnostic fields (`error_level`, `error_message`, `exception_class`, `stack_trace`, `file_path`, `line_number`, `http_method`, `url_path`, `ip_address`, `user_agent`, `request_payload`, `status`, `developer_notes`, `fixed_by`, `fixed_at`), while dropping PHP/Laravel framework-specific columns (`php_version`, `laravel_version`, `composer_lock_hash`, etc.).

