# PostgreSQL Database Cluster (Master-Slave)

**Centralized Database Hub dengan TimescaleDB untuk Multiple Systems**

PostgreSQL 16 + TimescaleDB cluster dengan master-slave replication, optimized untuk time-series data dan general-purpose workloads.

## 🎯 Features

- **Master-Slave Replication**: 1 master (write) + 2 slaves (read) dengan automatic failover support
- **TimescaleDB Extension**: Auto-partitioning, compression, dan retention policies untuk time-series data
- **High Performance**: Optimized untuk monitoring metrics, logs, dan transactional workloads
- **pgAdmin 4**: Web-based database management interface
- **Automated Backups**: Daily backups dengan retention policy (7 days / 4 weeks / 6 months)
- **Read Scaling**: Distribute read queries across multiple replicas
- **Docker-based**: Easy deployment, reproducible environments

## 🏗️ Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                     Applications                            │
│  (VPS Dashboard, Web Apps, Analytics, etc.)                │
└───────────────┬─────────────────────┬───────────────────────┘
                │                     │
                │ WRITE              │ READ
                ▼                     ▼
    ┌───────────────────┐   ┌──────────────┐  ┌──────────────┐
    │   DB MASTER       │──▶│  DB SLAVE 1  │  │  DB SLAVE 2  │
    │   (Primary)       │   │  (Replica)   │  │  (Replica)   │
    │   Port: 5432      │   │  Port: 5433  │  │  Port: 5434  │
    │   READ + WRITE    │   │  READ ONLY   │  │  READ ONLY   │
    └───────────────────┘   └──────────────┘  └──────────────┘
            │                      ▲                  ▲
            │                      │                  │
            └──────────────────────┴──────────────────┘
                   Streaming Replication (WAL)
```

## 📋 Prerequisites

- Docker 20.10+
- Docker Compose 2.0+
- Minimal 2GB RAM available untuk database cluster
- Minimal 10GB disk space untuk data + backups

## 🚀 Quick Start

### 1. Setup Environment

```bash
cd database-cluster

# Copy environment template
cp .env.example .env

# Edit .env dengan credentials Anda
nano .env  # atau text editor favorit
```

**IMPORTANT**: Set passwords yang kuat untuk:
- `POSTGRES_PASSWORD` (superuser password)
- `REPLICATION_PASSWORD` (replication user password)
- `PGADMIN_PASSWORD` (pgAdmin login password)

### 2. Deploy Cluster

```bash
# Build dan start semua containers
docker compose up -d

# Check status
docker compose ps

# View logs
docker compose logs -f
```

### 3. Verify Replication

Connect ke master dan check replication status:

```bash
# Connect ke master container
docker exec -it db-cluster-master psql -U dbadmin -d postgres

# Check connected slaves
SELECT * FROM pg_stat_replication;

# Expected output: 2 rows (slave-1 dan slave-2)
```

### 4. Access pgAdmin

1. Open browser: `http://localhost:5050`
2. Login dengan credentials dari `.env`:
   - Email: `PGADMIN_EMAIL`
   - Password: `PGADMIN_PASSWORD`
3. Servers sudah pre-configured (lihat `pgadmin/servers.json`)

## ⚙️ Configuration

### Environment Variables

Lihat `.env.example` untuk daftar lengkap. Key variables:

| Variable | Default | Description |
|----------|---------|-------------|
| `POSTGRES_USER` | `dbadmin` | PostgreSQL superuser |
| `POSTGRES_PASSWORD` | *required* | Superuser password |
| `REPLICATION_PASSWORD` | *required* | Replication password |
| `DB_MASTER_PORT` | `5432` | Master host port |
| `DB_SLAVE1_PORT` | `5433` | Slave 1 host port |
| `DB_SLAVE2_PORT` | `5434` | Slave 2 host port |
| `PGADMIN_PORT` | `5050` | pgAdmin web interface |
| `BACKUP_SCHEDULE` | `@daily` | Backup cron schedule |
| `BACKUP_KEEP_DAYS` | `7` | Daily backup retention |

### Performance Tuning

Adjust berdasarkan RAM server Anda di `.env`:

```bash
# Rule of thumb:
# shared_buffers = 25% of RAM
# effective_cache_size = 50-75% of RAM

SHARED_BUFFERS=256MB           # 25% dari 1GB RAM
EFFECTIVE_CACHE_SIZE=1GB       # 50-75% dari 1GB RAM
WORK_MEM=16MB                  # Per query operation
MAINTENANCE_WORK_MEM=128MB     # For VACUUM, CREATE INDEX
```

### Creating Databases

Edit `init-scripts/01-init-master.sql` dan uncomment section untuk create databases:

```sql
-- Example: Create database untuk monitoring server
CREATE DATABASE monitoring_db OWNER dbadmin;

-- Example: Create database untuk web app
CREATE DATABASE webapp_db OWNER dbadmin;
```

Restart master untuk apply changes:

```bash
docker compose restart db-master
```

## 📊 Usage

### Connection Strings

**Master (Write Operations):**
```
postgresql://dbadmin:PASSWORD@localhost:5432/postgres
```

**Slave 1 (Read Operations):**
```
postgresql://dbadmin:PASSWORD@localhost:5433/postgres
```

**Slave 2 (Read Operations):**
```
postgresql://dbadmin:PASSWORD@localhost:5434/postgres
```

**From Inside Docker Network:**
```
Master:  postgresql://dbadmin:PASSWORD@db-master:5432/postgres
Slave 1: postgresql://dbadmin:PASSWORD@db-slave-1:5432/postgres
Slave 2: postgresql://dbadmin:PASSWORD@db-slave-2:5432/postgres
```

### Sample Time-Series Schema

Uncomment section di `init-scripts/01-init-master.sql` untuk create sample monitoring schema dengan:
- TimescaleDB hypertables (auto-partitioning)
- Retention policies (auto-delete old data)
- Compression policies (auto-compress old data)
- Continuous aggregates (pre-computed views)

### Application Integration

**Go Example:**
```go
// Master connection (write)
masterDB, _ := sql.Open("postgres", "postgresql://dbadmin:pass@localhost:5432/mydb")

// Slave connection (read)
slaveDB, _ := sql.Open("postgres", "postgresql://dbadmin:pass@localhost:5433/mydb")
```

**Node.js Example:**
```javascript
// Master (write)
const masterPool = new Pool({
  host: 'localhost',
  port: 5432,
  database: 'mydb',
  user: 'dbadmin',
  password: 'pass'
});

// Slave (read)
const slavePool = new Pool({
  host: 'localhost',
  port: 5433,
  database: 'mydb',
  user: 'dbadmin',
  password: 'pass'
});
```

## 🔍 Verification & Monitoring

### Check Cluster Health

```bash
# Check all containers running
docker compose ps

# Check master health
docker exec db-cluster-master pg_isready -U dbadmin

# Check replication lag
docker exec -it db-cluster-master psql -U dbadmin -c "SELECT * FROM pg_stat_replication;"

# Check if slave is in recovery mode (should return 't')
docker exec -it db-cluster-slave-1 psql -U dbadmin -c "SELECT pg_is_in_recovery();"
```

### View Backups

```bash
# List backup files
docker exec db-cluster-backup ls -lh /backups

# Restore from backup (example)
docker exec -i db-cluster-master psql -U dbadmin -d postgres < backup.sql
```

### Monitor Logs

```bash
# All services
docker compose logs -f

# Specific service
docker compose logs -f db-master
docker compose logs -f db-slave-1
```

## 🛠️ Troubleshooting

### Slave Not Replicating

```bash
# Check master replication slots
docker exec -it db-cluster-master psql -U dbadmin -c "SELECT * FROM pg_replication_slots;"

# Check slave logs
docker compose logs db-slave-1

# Rebuild slave from master
docker compose stop db-slave-1
docker volume rm db-cluster-slave1-data
docker compose up -d db-slave-1
```

### Connection Refused

Check if containers are running and ports are not blocked:

```bash
docker compose ps
netstat -tuln | grep -E '5432|5433|5434|5050'
```

### High Replication Lag

Check network between master-slave and disk I/O:

```bash
# Check replication lag in bytes
docker exec -it db-cluster-master psql -U dbadmin -c "SELECT client_addr, state, sent_lsn, write_lsn, flush_lsn, replay_lsn, sync_state FROM pg_stat_replication;"
```

Consider increasing WAL retention:

```conf
# In config/master-postgresql.conf
wal_keep_size = 2GB  # Increase from 1GB
```

### Disk Space Full

```bash
# Check disk usage
docker system df

# Clean up old backups manually
docker exec db-cluster-backup rm /backups/old-backup.sql.gz

# Adjust retention in .env
BACKUP_KEEP_DAYS=3  # Reduce from 7
```

## 📚 Additional Resources

- [PostgreSQL Documentation](https://www.postgresql.org/docs/)
- [TimescaleDB Docs](https://docs.timescale.com/)
- [Replication Tutorial](https://www.postgresql.org/docs/current/warm-standby.html)

---

**Created**: 2026-07-13  
**Author**: Damar - Database Cluster untuk Monitoring Server
