#!/usr/bin/env bash
# =============================================================================
# Dawa24 Store - Zero-Downtime Deployment Script for Hostinger VPS
# =============================================================================
# Usage:
#   bash /opt/dawa24/deploy/hostinger/deploy.sh
# =============================================================================

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
APP_DIR="$(cd "${SCRIPT_DIR}/../.." && pwd)"

cd "${APP_DIR}"

if [ ! -f ".env" ]; then
    echo "[-] Error: .env file not found in ${APP_DIR}!"
    echo "    Create it by copying deploy/hostinger/.env.production.example"
    exit 1
fi

COMPOSE_FILE="${SCRIPT_DIR}/docker-compose.yml"

echo ">>> [1/5] Pulling latest code changes..."
if [ -d ".git" ]; then
    git fetch origin main
    git reset --hard origin/main
fi

echo ">>> [2/5] Building Docker images..."
docker compose -f "${COMPOSE_FILE}" build

echo ">>> [3/5] Starting database & cache services..."
docker compose -f "${COMPOSE_FILE}" up -d postgres redis

echo ">>> Waiting for PostgreSQL and Redis to report healthy..."
for i in {1..30}; do
    if docker compose -f "${COMPOSE_FILE}" ps postgres | grep -q "healthy" && \
       docker compose -f "${COMPOSE_FILE}" ps redis | grep -q "healthy"; then
        echo ">>> Database and Redis are healthy."
        break
    fi
    sleep 2
done

echo ">>> [4/5] Executing database migrations..."
docker compose -f "${COMPOSE_FILE}" run --rm migrate

echo ">>> [5/5] Deploying Web Server, Worker, and Ingress Proxy..."
docker compose -f "${COMPOSE_FILE}" up -d --remove-orphans server worker caddy

echo ">>> Verifying application readiness..."
sleep 5

for i in {1..15}; do
    if docker compose -f "${COMPOSE_FILE}" exec -T server curl -fsS http://localhost:8070/health >/dev/null 2>&1; then
        echo ">>> Application healthcheck PASSED!"
        echo "================================================================="
        echo ">>> Dawa24 Store deployed successfully on Hostinger VPS!"
        echo "================================================================="
        docker compose -f "${COMPOSE_FILE}" ps
        exit 0
    fi
    echo "    Waiting for server /health (attempt $i/15)..."
    sleep 2
done

echo "[-] Warning: Server failed health check within 30s. Checking logs:"
docker compose -f "${COMPOSE_FILE}" logs --tail=50 server
exit 1
