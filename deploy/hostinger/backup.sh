#!/usr/bin/env bash
# =============================================================================
# Dawa24 Store - Automated Daily Backup Script for Hostinger VPS
# =============================================================================
# Backs up PostgreSQL database and user-uploaded media files with 7-day retention
#
# Restore example:
#   docker compose exec -T postgres pg_restore -U postgres -d dawa24_store --clean < backup_db_YYYYMMDD_HHMMSS.dump
# =============================================================================

set -euo pipefail

BACKUP_DIR="/opt/dawa24/backups"
TIMESTAMP="$(date +'%Y%m%d_%H%M%S')"
DB_BACKUP_FILE="${BACKUP_DIR}/dawa24_db_${TIMESTAMP}.dump"
MEDIA_BACKUP_FILE="${BACKUP_DIR}/dawa24_media_${TIMESTAMP}.tar.gz"

mkdir -p "${BACKUP_DIR}"

echo ">>> [$(date)] Starting Dawa24 Backup..."

# 1. Backup PostgreSQL Database
echo ">>> Dumping PostgreSQL database to ${DB_BACKUP_FILE}..."
docker compose -f /opt/dawa24/deploy/hostinger/docker-compose.yml exec -T postgres \
    pg_dump -U postgres -d dawa24_store -Fc > "${DB_BACKUP_FILE}"

echo ">>> Database dump completed. Size: $(du -h "${DB_BACKUP_FILE}" | cut -f1)"

# 2. Backup Uploaded Media & Documents
echo ">>> Archiving uploads volume to ${MEDIA_BACKUP_FILE}..."
docker run --rm \
    -v dawa24-store_uploads:/data:ro \
    -v "${BACKUP_DIR}":/backup \
    alpine tar -czf "/backup/dawa24_media_${TIMESTAMP}.tar.gz" -C /data .

echo ">>> Media archive completed. Size: $(du -h "${MEDIA_BACKUP_FILE}" | cut -f1)"

# 3. Purge backups older than 7 days
echo ">>> Purging backups older than 7 days..."
find "${BACKUP_DIR}" -type f -name "dawa24_*" -mtime +7 -delete

echo ">>> [$(date)] All backups finished successfully."
