#!/bin/bash
set -euo pipefail

# =============================================================================
# PostgreSQL Initial Database & Role Setup for Dawa24
# =============================================================================
# Executed by the PostgreSQL container on first startup (docker-entrypoint-initdb.d).
#
# Creates the runtime role dawa24_app: LOGIN, not a superuser, no BYPASSRLS,
# cannot create databases or roles, owns nothing. The server and worker connect
# as this role; migrations run as the owner (POSTGRES_USER) and grant it table
# privileges (migrations 222 and 228).
#
# The password is passed to psql as a variable and quoted by psql (:'app_pw'),
# never spliced into the SQL text, so any character is safe in it.
# =============================================================================

if [ -z "${DB_APP_PASSWORD:-}" ] || [ "${#DB_APP_PASSWORD}" -lt 24 ]; then
    echo "!!! DB_APP_PASSWORD must be set and at least 24 characters" >&2
    exit 1
fi

echo ">>> Initializing Dawa24 database and roles..."

psql -v ON_ERROR_STOP=1 \
     -v app_pw="$DB_APP_PASSWORD" \
     --username "$POSTGRES_USER" --dbname "$POSTGRES_DB" <<-'EOSQL'
    CREATE EXTENSION IF NOT EXISTS pg_trgm;
    CREATE EXTENSION IF NOT EXISTS unaccent;
    CREATE EXTENSION IF NOT EXISTS pgcrypto;
    CREATE EXTENSION IF NOT EXISTS citext;

    SELECT format('CREATE ROLE dawa24_app WITH LOGIN PASSWORD %L NOSUPERUSER NOCREATEDB NOCREATEROLE NOINHERIT NOBYPASSRLS NOREPLICATION', :'app_pw')
     WHERE NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'dawa24_app')
    \gexec

    SELECT format('ALTER ROLE dawa24_app WITH LOGIN PASSWORD %L NOSUPERUSER NOCREATEDB NOCREATEROLE NOBYPASSRLS NOREPLICATION', :'app_pw')
    \gexec

    -- The runtime role reaches this database only; PUBLIC loses the default.
    SELECT format('REVOKE ALL ON DATABASE %I FROM PUBLIC', current_database()) \gexec
    SELECT format('GRANT CONNECT, TEMPORARY ON DATABASE %I TO dawa24_app', current_database()) \gexec
    REVOKE CREATE ON SCHEMA public FROM PUBLIC;
EOSQL

echo ">>> Database and role initialization completed successfully."
