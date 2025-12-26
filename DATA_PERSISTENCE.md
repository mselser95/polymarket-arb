# Data Persistence Guide

## TL;DR: Your Data is Safe

**YES**, your data persists across:
- ✅ PC restarts/reboots
- ✅ `docker-compose down` and `docker-compose up`
- ✅ Container restarts
- ✅ Docker daemon restarts
- ✅ System crashes (data committed to disk)

**NO**, data is lost only when:
- ❌ You run `docker-compose down -v` (explicitly deletes volumes)
- ❌ You run `docker volume rm polymarket-arb_postgres_data`
- ❌ You delete `/var/lib/docker/volumes/` manually

---

## How Data Persistence Works

### Docker Named Volumes

The `docker-compose.yml` defines three named volumes:

```yaml
volumes:
  postgres_data:      # Database tables, indexes, all SQL data
  prometheus_data:    # Time-series metrics (30 days retention)
  grafana_data:       # Dashboards, settings, user preferences
```

These volumes are stored on your host machine at:
```bash
# Linux
/var/lib/docker/volumes/polymarket-arb_postgres_data/_data/
/var/lib/docker/volumes/polymarket-arb_prometheus_data/_data/
/var/lib/docker/volumes/polymarket-arb_grafana_data/_data/

# macOS (Docker Desktop)
~/Library/Containers/com.docker.docker/Data/vms/0/data/Docker.raw

# Windows (Docker Desktop)
\\wsl$\docker-desktop-data\version-pack-data\community\docker\volumes\
```

**Key Point**: These volumes exist OUTSIDE the container lifecycle. Destroying a container does NOT destroy its volumes.

---

## Common Scenarios

### Scenario 1: Normal Stop/Start

```bash
# Stop services
docker-compose down

# Your PC restarts or you go to sleep...

# Start services again
docker-compose up -d
```

**Result**: All data intact. PostgreSQL has all your arbitrage opportunities, Prometheus has historical metrics, Grafana has your dashboards.

**Why**: `docker-compose down` only removes containers and networks, NOT volumes (unless you use `-v` flag).

---

### Scenario 2: PC Crash or Force Shutdown

```bash
# System crashes while containers are running
# *power loss, kernel panic, forced reboot*

# After reboot, restart services
docker-compose up -d
```

**Result**: All COMMITTED data is safe. Postgres uses write-ahead logging (WAL), so transactions are durable.

**Potential Loss**: Only uncommitted/in-flight transactions at the moment of crash (typically <1 second of data).

---

### Scenario 3: Rebuild Containers

```bash
# Update code and rebuild
docker-compose build --no-cache

# Recreate containers with new image
docker-compose up -d --force-recreate
```

**Result**: All data intact. Volumes are detached during rebuild and reattached to new containers.

---

### Scenario 4: Delete Volumes (⚠️ DATA LOSS)

```bash
# Stop and DELETE volumes
docker-compose down -v  # ⚠️ DESTROYS ALL DATA
```

**Result**: ALL data lost. Volumes are permanently deleted.

**Use Case**: Clean slate for testing, or removing the entire stack.

---

## Database Initialization

### First Start (Empty Database)

```bash
docker-compose up -d
```

**What Happens**:
1. `postgres` service creates empty database volume
2. `db-init` service runs migrations (`001_initial_schema.up.sql`)
3. Tables created: `arbitrage_opportunities`, indexes, views
4. `wallet-tracker` and `app` services start after migrations complete

**Logs**:
```bash
docker-compose logs db-init
# 🔧 Initializing Polymarket database...
# ✅ PostgreSQL is ready
# 📦 Running migrations...
#   - Applying 001_initial_schema.up.sql...
# ✅ Database initialization complete
```

### Subsequent Starts (Existing Database)

```bash
docker-compose down
docker-compose up -d
```

**What Happens**:
1. `postgres` service reattaches to existing volume
2. `db-init` service runs migrations again (but `CREATE TABLE IF NOT EXISTS` is idempotent)
3. No data loss, all existing rows preserved

**Key Point**: The migrations use `CREATE TABLE IF NOT EXISTS`, so re-running them is safe.

---

## Verify Data Persistence

### Check Volume Status

```bash
# List volumes
docker volume ls | grep polymarket

# Output:
# polymarket-arb_postgres_data
# polymarket-arb_prometheus_data
# polymarket-arb_grafana_data

# Inspect volume (shows mount point)
docker volume inspect polymarket-arb_postgres_data

# Check volume size
docker system df -v | grep polymarket
```

### Query Database Directly

```bash
# Connect to PostgreSQL
docker-compose exec postgres psql -U polymarket -d polymarket_arb

# List tables
\dt

# Check row count
SELECT COUNT(*) FROM arbitrage_opportunities;

# Exit
\q
```

### Check Prometheus Data Retention

```bash
# Open Prometheus UI
open http://localhost:9090

# Query historical data (should show 30 days)
polymarket_wallet_usdc_balance[30d]
```

---

## Backup and Restore

### Backup PostgreSQL

```bash
# SQL dump (human-readable)
docker-compose exec postgres pg_dump -U polymarket polymarket_arb > backup-$(date +%Y%m%d).sql

# Compressed binary dump (faster, smaller)
docker-compose exec postgres pg_dump -U polymarket -Fc polymarket_arb > backup-$(date +%Y%m%d).dump

# Backup volume as tarball
docker run --rm \
  -v polymarket-arb_postgres_data:/data \
  -v $(pwd):/backup \
  alpine tar czf /backup/postgres-volume-$(date +%Y%m%d).tar.gz /data
```

### Restore PostgreSQL

```bash
# From SQL dump
cat backup-20241225.sql | docker-compose exec -T postgres psql -U polymarket -d polymarket_arb

# From binary dump
cat backup-20241225.dump | docker-compose exec -T postgres pg_restore -U polymarket -d polymarket_arb --clean

# From volume tarball (⚠️ overwrites existing data)
docker-compose down
docker volume rm polymarket-arb_postgres_data
docker volume create polymarket-arb_postgres_data
docker run --rm \
  -v polymarket-arb_postgres_data:/data \
  -v $(pwd):/backup \
  alpine tar xzf /backup/postgres-volume-20241225.tar.gz -C /
docker-compose up -d
```

### Backup Prometheus

```bash
# Prometheus data is in TSDB format (not SQL)
docker run --rm \
  -v polymarket-arb_prometheus_data:/data \
  -v $(pwd):/backup \
  alpine tar czf /backup/prometheus-$(date +%Y%m%d).tar.gz /data
```

### Backup Grafana

```bash
# Backup dashboards via API
curl -u admin:admin http://localhost:3000/api/search | \
  jq -r '.[] | .uri' | \
  while read uri; do
    curl -u admin:admin "http://localhost:3000/api/dashboards/$uri" > "dashboard-${uri//\//-}.json"
  done

# Backup entire Grafana volume
docker run --rm \
  -v polymarket-arb_grafana_data:/data \
  -v $(pwd):/backup \
  alpine tar czf /backup/grafana-$(date +%Y%m%d).tar.gz /data
```

---

## Migration Management

### Check Migration Status

```bash
# View migrations applied
docker-compose exec postgres psql -U polymarket -d polymarket_arb -c "\d"

# Check table schema
docker-compose exec postgres psql -U polymarket -d polymarket_arb -c "\d arbitrage_opportunities"
```

### Apply New Migrations

```bash
# 1. Create new migration file
cat > migrations/002_add_index.up.sql << 'EOF'
CREATE INDEX idx_arbitrage_opportunities_profit_bps 
ON arbitrage_opportunities(profit_bps DESC);
