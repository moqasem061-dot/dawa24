#!/usr/bin/env bash
# =============================================================================
# Dawa24 Store - Hostinger KVM VPS Initial Setup & Hardening Script
# =============================================================================
# Run this once as root on a fresh Ubuntu 22.04 / 24.04 Hostinger VPS:
#   curl -sSL https://raw.githubusercontent.com/<user>/dawa24-store/main/deploy/hostinger/setup.sh | sudo bash
# =============================================================================

set -euo pipefail

if [ "$EUID" -ne 0 ]; then
    echo "[-] Error: Please run this script as root (sudo bash setup.sh)."
    exit 1
fi

echo "================================================================="
echo ">>> Step 1/5: Updating System Packages..."
echo "================================================================="
export DEBIAN_FRONTEND=noninteractive
apt-get update -y
apt-get upgrade -y
apt-get install -y curl git ufw ca-certificates gnupg lsb-release htop fail2ban

echo "================================================================="
echo ">>> Step 2/5: Installing Docker Engine & Docker Compose Plugin..."
echo "================================================================="
install -m 0755 -d /etc/apt/keyrings
if [ ! -f /etc/apt/keyrings/docker.gpg ]; then
    curl -fsSL https://download.docker.com/linux/ubuntu/gpg | gpg --dearmor -o /etc/apt/keyrings/docker.gpg
    chmod a+r /etc/apt/keyrings/docker.gpg
fi

echo \
  "deb [arch=$(dpkg --print-architecture) signed-by=/etc/apt/keyrings/docker.gpg] https://download.docker.com/linux/ubuntu \
  $(lsb_release -cs) stable" | tee /etc/apt/sources.list.d/docker.list > /dev/null

apt-get update -y
apt-get install -y docker-ce docker-ce-cli containerd.io docker-buildx-plugin docker-compose-plugin
systemctl enable --now docker

echo "================================================================="
echo ">>> Step 3/5: Configuring UFW Firewall..."
echo "================================================================="
ufw default deny incoming
ufw default allow outgoing
ufw allow 22/tcp comment 'SSH'
ufw allow 80/tcp comment 'HTTP'
ufw allow 443/tcp comment 'HTTPS'
ufw allow 443/udp comment 'HTTP3-QUIC'
echo "y" | ufw enable
ufw status verbose

echo "================================================================="
echo ">>> Step 4/5: Applying High-Concurrency Linux Kernel Tuning..."
echo "================================================================="
cat << 'EOF' > /etc/sysctl.d/99-dawa24.conf
# Virtual Memory: prevent swap thrashing while ensuring OOM safety
vm.swappiness = 10
vm.overcommit_memory = 1
vm.dirty_ratio = 15
vm.dirty_background_ratio = 5

# Socket Backlogs: handle massive bursts of simultaneous TCP handshakes
net.core.somaxconn = 4096
net.ipv4.tcp_max_syn_backlog = 4096
net.core.netdev_max_backlog = 4096

# Fast Socket Recycling & Timeouts
net.ipv4.tcp_tw_reuse = 1
net.ipv4.tcp_fin_timeout = 15

# Ephemeral Port Range Expansion
net.ipv4.ip_local_port_range = 10240 65535

# TCP Buffer Autosizing
net.core.rmem_max = 16777216
net.core.wmem_max = 16777216
net.ipv4.tcp_rmem = 4096 87380 16777216
net.ipv4.tcp_wmem = 4096 65536 16777216
EOF

sysctl --system

echo "================================================================="
echo ">>> Step 5/5: Preparing Application Directory & Automated Backups..."
echo "================================================================="
mkdir -p /opt/dawa24/backups
chmod -R 750 /opt/dawa24

# Setup daily backup cron job at 03:00 AM Cairo time (01:00 UTC)
cat << 'EOF' > /etc/cron.d/dawa24-backup
0 1 * * * root /opt/dawa24/deploy/hostinger/backup.sh >> /var/log/dawa24-backup.log 2>&1
EOF
chmod 644 /etc/cron.d/dawa24-backup

echo "================================================================="
echo ">>> Setup completed successfully!"
echo "Next Steps:"
echo " 1. Clone your repository into /opt/dawa24:"
echo "    git clone <your-repo-url> /opt/dawa24"
echo " 2. Create /opt/dawa24/.env from template:"
echo "    cp /opt/dawa24/deploy/hostinger/.env.production.example /opt/dawa24/.env"
echo "    nano /opt/dawa24/.env"
echo " 3. Run deploy script:"
echo "    bash /opt/dawa24/deploy/hostinger/deploy.sh"
echo "================================================================="
