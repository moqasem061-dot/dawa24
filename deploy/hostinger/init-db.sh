#!/bin/bash
set -e

# =============================================================================
# PostgreSQL Initial Database & Role Setup for Dawa24
# =============================================================================
# Automatically executed by PostgreSQL container on first startup
# =============================================================================

echo ">>> Initializing Dawa24 Database and Roles..."

psql -v ON_ERROR_STOP=1 --username "$POSTGRES_USER" --dbname "$POSTGRES_DB" <<-EOSQL
    -- Enable required extensions
    CREATE EXTENSION IF NOT EXISTS pg_trgm;
    CREATE EXTENSION IF NOT EXISTS unaccent;
    CREATE EXTENSION IF NOT EXISTS pgcrypto;
    CREATE EXTENSION IF NOT EXISTS citext;

    -- Create non-superuser application role with NOBYPASSRLS for tenant isolation
    DO \$\$
    BEGIN
        IF NOT EXISTS (SELECT FROM pg_catalog.pg_roles WHERE rolname = 'dawa24_app') THEN
            CREATE ROLE dawa24_app WITH LOGIN PASSWORD '${DB_APP_PASSWORD}';
        ELSE
            ALTER ROLE dawa24_app WITH LOGIN PASSWORD '${DB_APP_PASSWORD}';
        END IF;
    END
    \$\$;

    ALTER ROLE dawa24_app NOBYPASSRLS;
    GRANT CONNECT ON DATABASE dawa24_store TO dawa24_app;
EOSQL

echo ">>> Database and role initialization completed successfully."
