#!/usr/bin/env bash
# =============================================================================
# Dawa24 Store - Automated Daily Backup Script for Hostinger VPS
# =============================================================================
# Backs up PostgreSQL database and user-uploaded media files with 7-day retention
#
# Restore example:
#   docker compose -f /opt/dawa24/deploy/hostinger/docker-compose.yml exec -T postgres \
#     sh -c 'pg_restore -U "$POSTGRES_USER" -d "$POSTGRES_DB" --clean' < dawa24_db_YYYYMMDD_HHMMSS.dump
# =============================================================================

set -euo pipefail

BACKUP_DIR="/opt/dawa24/backups"
COMPOSE_FILE="/opt/dawa24/deploy/hostinger/docker-compose.yml"
TIMESTAMP="$(date +'%Y%m%d_%H%M%S')"
DB_BACKUP_FILE="${BACKUP_DIR}/dawa24_db_${TIMESTAMP}.dump"
MEDIA_BACKUP_FILE="${BACKUP_DIR}/dawa24_media_${TIMESTAMP}.tar.gz"

mkdir -p "${BACKUP_DIR}"

echo ">>> [$(date)] Starting Dawa24 Backup..."

# 1. Backup PostgreSQL Database
echo ">>> Dumping PostgreSQL database to ${DB_BACKUP_FILE}..."
# User and database come from the container's own environment, so the dump
# follows POSTGRES_USER / POSTGRES_DB instead of a name hardcoded here.
docker compose -f "${COMPOSE_FILE}" exec -T postgres \
    sh -c 'pg_dump -U "$POSTGRES_USER" -d "$POSTGRES_DB" -Fc' > "${DB_BACKUP_FILE}"

echo ">>> Database dump completed. Size: $(du -h "${DB_BACKUP_FILE}" | cut -f1)"

# 2. Backup Uploaded Media & Documents
echo ">>> Archiving uploads volume to ${MEDIA_BACKUP_FILE}..."
# Mount the uploads through the running server container. Naming the volume
# here is what broke this before: Compose prefixes volume names with the
# project name, and `docker run -v <name>:...` silently creates an EMPTY volume
# when the name is wrong, so every media backup was an empty archive.
SERVER_CONTAINER="$(docker compose -f "${COMPOSE_FILE}" ps -q server)"
if [ -z "${SERVER_CONTAINER}" ]; then
    echo "!!! server container is not running; cannot locate the uploads volume" >&2
    exit 1
fi
docker run --rm \
    --volumes-from "${SERVER_CONTAINER}:ro" \
    -v "${BACKUP_DIR}":/backup \
    alpine tar -czf "/backup/dawa24_media_${TIMESTAMP}.tar.gz" -C /app/data .

echo ">>> Media archive completed. Size: $(du -h "${MEDIA_BACKUP_FILE}" | cut -f1)"

# 3. Purge backups older than 7 days
echo ">>> Purging backups older than 7 days..."
find "${BACKUP_DIR}" -type f -name "dawa24_*" -mtime +7 -delete

echo ">>> [$(date)] All backups finished successfully."
