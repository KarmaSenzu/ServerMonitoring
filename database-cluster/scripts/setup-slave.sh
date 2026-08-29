#!/bin/bash
# ===============================================================================
# PostgreSQL Slave/Replica Setup Script
# 
# Script ini dijalankan otomatis saat slave container pertama kali start.
# Melakukan pg_basebackup dari master untuk inisialisasi replication.
# ===============================================================================

set -e  # Exit on error

echo "=================================="
echo "PostgreSQL Slave Setup Starting..."
echo "=================================="

# ===============================================================================
# ENVIRONMENT VARIABLES (dari docker-compose.yml)
# ===============================================================================
MASTER_HOST="${POSTGRES_MASTER_HOST:-db-master}"
MASTER_PORT="${POSTGRES_MASTER_PORT:-5432}"
REPLICATION_USER="${POSTGRES_REPLICATION_USER:-replicator}"
REPLICATION_PASSWORD="${POSTGRES_REPLICATION_PASSWORD}"
PGDATA="${PGDATA:-/var/lib/postgresql/data}"

# ===============================================================================
# VALIDATION
# ===============================================================================
if [ -z "$REPLICATION_PASSWORD" ]; then
    echo "ERROR: POSTGRES_REPLICATION_PASSWORD is not set"
    exit 1
fi

# ===============================================================================
# CHECK IF ALREADY INITIALIZED
# ===============================================================================
if [ -f "$PGDATA/standby.signal" ]; then
    echo "✓ Slave already initialized (standby.signal exists)"
    echo "✓ Skipping pg_basebackup"
    exit 0
fi

if [ -f "$PGDATA/PG_VERSION" ] && [ ! -f "$PGDATA/standby.signal" ]; then
    echo "✓ Data directory exists but not in standby mode"
    echo "✓ This might be a converted master or existing database"
    exit 0
fi

# ===============================================================================
# WAIT FOR MASTER TO BE READY
# ===============================================================================
echo ""
echo "Waiting for master database ($MASTER_HOST:$MASTER_PORT) to be ready..."
until PGPASSWORD="$REPLICATION_PASSWORD" pg_isready -h "$MASTER_HOST" -p "$MASTER_PORT" -U "$REPLICATION_USER" -t 1 > /dev/null 2>&1; do
    echo "  Master not ready yet, retrying in 5 seconds..."
    sleep 5
done
echo "✓ Master database is ready"

# ===============================================================================
# BACKUP EXISTING DATA (if any)
# ===============================================================================
if [ -d "$PGDATA" ] && [ "$(ls -A $PGDATA)" ]; then
    echo ""
    echo "Backing up existing data directory..."
    BACKUP_DIR="${PGDATA}_backup_$(date +%Y%m%d_%H%M%S)"
    mv "$PGDATA" "$BACKUP_DIR"
    echo "✓ Backup created at: $BACKUP_DIR"
fi

# ===============================================================================
# CREATE FRESH DATA DIRECTORY
# ===============================================================================
mkdir -p "$PGDATA"
chmod 700 "$PGDATA"
chown -R postgres:postgres "$PGDATA"

# ===============================================================================
# RUN PG_BASEBACKUP (Clone dari Master)
# ===============================================================================
echo ""
echo "Running pg_basebackup from master..."
echo "  Source: $MASTER_HOST:$MASTER_PORT"
echo "  Target: $PGDATA"
echo ""

PGPASSWORD="$REPLICATION_PASSWORD" pg_basebackup \
    -h "$MASTER_HOST" \
    -p "$MASTER_PORT" \
    -U "$REPLICATION_USER" \
    -D "$PGDATA" \
    -Fp \
    -Xs \
    -P \
    -R

if [ $? -ne 0 ]; then
    echo "ERROR: pg_basebackup failed"
    exit 1
fi

echo ""
echo "✓ pg_basebackup completed successfully"

# ===============================================================================
# CONFIGURE REPLICATION (primary_conninfo)
# ===============================================================================
echo ""
echo "Configuring replication settings..."

# pg_basebackup dengan flag -R sudah membuat standby.signal dan primary_conninfo
# Tapi kita override untuk memastikan settings benar

cat >> "$PGDATA/postgresql.auto.conf" <<EOF

# Replication settings (auto-configured by setup-slave.sh)
primary_conninfo = 'host=$MASTER_HOST port=$MASTER_PORT user=$REPLICATION_USER password=$REPLICATION_PASSWORD application_name=$(hostname)'
primary_slot_name = 'replication_slot_$(hostname)'
recovery_target_timeline = 'latest'
EOF

# Ensure standby.signal exists (ini yang bikin PostgreSQL jalan sebagai replica)
touch "$PGDATA/standby.signal"

echo "✓ Replication configuration completed"

# ===============================================================================
# SET PERMISSIONS
# ===============================================================================
chown -R postgres:postgres "$PGDATA"
chmod 700 "$PGDATA"

# ===============================================================================
# SUMMARY
# ===============================================================================
echo ""
echo "=================================="
echo "✓ Slave setup completed successfully!"
echo "=================================="
echo ""
echo "Configuration:"
echo "  Master: $MASTER_HOST:$MASTER_PORT"
echo "  Replica: $(hostname)"
echo "  Data: $PGDATA"
echo ""
echo "Next steps:"
echo "  1. PostgreSQL will start automatically in standby mode"
echo "  2. Verify replication on master: SELECT * FROM pg_stat_replication;"
echo "  3. Check slave status: SELECT pg_is_in_recovery();"
echo ""
