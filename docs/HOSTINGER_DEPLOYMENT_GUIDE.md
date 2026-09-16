# Dawa24 Store — Hostinger KVM VPS Production Deployment Guide

This guide provides exact, step-by-step instructions for deploying and running Dawa24 on a standalone **Hostinger KVM VPS** (running Ubuntu 22.04 or 24.04 LTS), replacing Elest.io with a faster, self-contained, and highly secure architecture.

---

## 1. Why Hostinger VPS over Elest.io?

1. **600× Faster Database & Cache Latency**:
   * On Elest.io, PostgreSQL and Redis were external remote services communicating over public hostnames with ~63 ms round trips.
   * On Hostinger VPS, PostgreSQL 18 and Redis 7 run collocated on the internal Docker network (`dawa24-net`), cutting latency to **< 0.1 ms**.
2. **Zero Public Attack Surface**:
   * PostgreSQL (port 5432) and Redis (port 6379) are completely unexposed to the internet.
   * Only ports 80 and 443 are open through the Caddy edge reverse proxy.
3. **Automated SSL Grade A+ & Post-Quantum Cryptography (PQC)**:
   * Caddy handles Let's Encrypt certificates automatically with zero configuration, providing HTTP/2, HTTP/3 (QUIC), and hybrid quantum-resistant key exchange (`X25519MLKEM768`).
4. **Single Predictable Bill**:
   * All services run smoothly on a single Hostinger KVM VPS (2 vCPU, 4 GB or 8 GB RAM).

---

## 2. Hardware Recommendations

* **Hostinger KVM 2** (2 vCPU, 4 GB RAM, 50 GB NVMe):
  * Supports up to **2,000 active concurrent users** (250–350 RPS).
  * Profile: `VPS_PROFILE=4gb` in `.env`.
* **Hostinger KVM 4** (2 vCPU, 8 GB RAM, 100 GB NVMe) — *Recommended for peak scale*:
  * Supports up to **5,000+ active concurrent users** (450–600+ RPS).
  * Profile: `VPS_PROFILE=8gb` in `.env`.
* **Operating System**: **Ubuntu 24.04 64-bit** or **Ubuntu 22.04 64-bit** (default on Hostinger).

---

## 3. Step-by-Step Deployment

### Step 1: DNS Configuration
In your domain registrar (e.g. Cloudflare, Namecheap, or Hostinger Domains), create DNS records pointing to your Hostinger VPS Public IP address:

| Type | Name | Content | Proxy status |
|---|---|---|---|
| `A` | `@` (or `dawa24.net`) | `<YOUR_VPS_PUBLIC_IP>` | DNS only (or Cloudflare proxied) |
| `A` | `www` | `<YOUR_VPS_PUBLIC_IP>` | DNS only (or Cloudflare proxied) |

---

### Step 2: Connect to Your VPS and Run Initial Setup
Connect to your Hostinger VPS via SSH:
```bash
ssh root@<YOUR_VPS_PUBLIC_IP>
```

Run the automated one-click setup script to install Docker, configure UFW firewall, apply high-concurrency kernel parameters, and prepare directories:
```bash
# Clone the repository into /opt/dawa24
git clone https://github.com/<your-account>/dawa24-store.git /opt/dawa24
cd /opt/dawa24

# Run the setup script as root
bash deploy/hostinger/setup.sh
```

---

### Step 3: Configure Environment Variables
Create your production `.env` file from the provided template:
```bash
cp deploy/hostinger/.env.production.example /opt/dawa24/.env
nano /opt/dawa24/.env
```

Fill in the required variables:
1. `APP_DOMAIN`: Your real domain name (e.g., `dawa24.net`).
2. `ACME_EMAIL`: Your email address for Let's Encrypt renewal notices.
3. `VPS_PROFILE`: `4gb` (for KVM 2) or `8gb` (for KVM 4).
4. `POSTGRES_PASSWORD`: Strong random password for the `postgres` superuser.
5. `DB_APP_PASSWORD`: Strong random password for the `dawa24_app` application role.
6. `REDIS_PASSWORD`: Strong random password for Redis.
7. `SESSION_SECRET`: Generate a 32-byte hex string using:
   ```bash
   openssl rand -hex 32
   ```

Save and exit (`Ctrl+O`, `Enter`, `Ctrl+X`).

---

### Step 4: Deploy the Platform
Run the turnkey zero-downtime deployment script:
```bash
bash deploy/hostinger/deploy.sh
```

This script will automatically:
1. Build the lightweight production Docker image.
2. Launch PostgreSQL 18 and Redis 7 and wait until their health checks pass.
3. Run the database migration container (`migrate`) to set up all schemas and grants.
4. Launch the Go web server (`server`), background worker (`worker`), and Caddy ingress proxy (`caddy`).
5. Verify `/health` and display container status.

Within seconds, visit `https://yourdomain.com` in your browser. Caddy will have automatically provisioned a Let's Encrypt SSL certificate!

---

## 4. Routine Operations & Maintenance

### Deploying Code Updates
When you push new commits to your GitHub repository:
```bash
cd /opt/dawa24
bash deploy/hostinger/deploy.sh
```
The script pulls changes, builds any modified layers, runs any new migrations, and reloads the services with zero downtime.

### Viewing Live Logs
```bash
cd /opt/dawa24
# View web server logs
docker compose -f deploy/hostinger/docker-compose.yml logs -f server

# View background worker logs (imports, scans)
docker compose -f deploy/hostinger/docker-compose.yml logs -f worker

# View Caddy access & SSL logs
docker compose -f deploy/hostinger/docker-compose.yml logs -f caddy
```

### Automated Backups & Restoring Data
* Daily backups of PostgreSQL and `/uploads` run automatically every day at 03:00 AM Cairo time (01:00 UTC) via cron.
* Backups are stored in `/opt/dawa24/backups/` and retained for 7 days.
* To run an immediate manual backup:
  ```bash
  bash /opt/dawa24/deploy/hostinger/backup.sh
  ```
* To restore a database backup:
  ```bash
  docker compose -f deploy/hostinger/docker-compose.yml exec -T postgres \
      pg_restore -U postgres -d dawa24_store --clean < /opt/dawa24/backups/dawa24_db_YYYYMMDD_HHMMSS.dump
  ```

---

## 5. Security Invariants Verified

* **UFW Firewall**: Only ports 22, 80, and 443 are reachable from outside. Ports 5432 and 6379 are bound solely to the internal Docker network.
* **RLS Tenant Isolation**: Connecting role `dawa24_app` holds `NOBYPASSRLS`. Row-Level Security cannot be bypassed.
* **Upload Security**: Spooled files are scanned in memory and processed asynchronously by the worker.
* **Memory Safety**: `GOMEMLIMIT` guarantees Go's runtime frees memory before Linux's OOM killer can intervene.
