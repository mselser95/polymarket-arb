# Quick Reference Card

## Data Persistence

| Action | Data Preserved? | Notes |
|--------|----------------|-------|
| `docker-compose down` | ✅ YES | Only removes containers, volumes remain |
| `docker-compose up -d` | ✅ YES | Reattaches existing volumes |
| `docker-compose restart` | ✅ YES | Containers restart, volumes untouched |
| PC Reboot | ✅ YES | Volumes stored on host filesystem |
| System Crash | ✅ YES | PostgreSQL WAL ensures durability |
| `docker-compose down -v` | ❌ NO | **Explicitly deletes volumes** |
| Container rebuild | ✅ YES | Volumes detached/reattached |

## Database Initialization

### First Start
```bash
docker-compose up -d
# 1. postgres starts → creates empty volume
# 2. db-init runs migrations → creates tables
# 3. wallet-tracker starts → begins tracking
```

### Subsequent Starts
```bash
docker-compose down
docker-compose up -d
# 1. postgres reattaches existing volume
# 2. db-init runs (idempotent, no changes)
# 3. wallet-tracker starts → existing data intact
```

## Quick Commands

### Check if Data Exists
```bash
# List volumes
docker volume ls | grep polymarket

# Check database content
docker-compose exec postgres psql -U polymarket -d polymarket_arb -c "\dt"
docker-compose exec postgres psql -U polymarket -d polymarket_arb -c "SELECT COUNT(*) FROM arbitrage_opportunities;"
```

### View Logs
```bash
# All services
docker-compose logs -f

# Database initialization (one-time)
docker-compose logs db-init

# Wallet tracker
docker-compose logs -f wallet-tracker

# Postgres errors
docker-compose logs postgres | grep ERROR
```

### Backup Data
```bash
# Quick backup (before maintenance)
docker-compose exec postgres pg_dump -U polymarket -Fc polymarket_arb > backup.dump

# Restore if needed
cat backup.dump | docker-compose exec -T postgres pg_restore -U polymarket -d polymarket_arb --clean
```

### Clean Slate (⚠️ Deletes ALL data)
```bash
# Stop and remove everything including volumes
docker-compose down -v

# Start fresh
docker-compose up -d
# (migrations run again, empty database)
```

## Storage Configuration

### Wallet Tracker
- **Current**: In-memory metrics → Prometheus scraping
- **Database**: NOT writing to postgres (metrics only)
- **To enable postgres**: Add `STORAGE_MODE: postgres` in docker-compose.yml

### Arbitrage Bot (when enabled)
- **Current**: Commented out in docker-compose.yml
- **Database**: DOES write to postgres (`STORAGE_MODE: postgres`)
- **Table**: `arbitrage_opportunities`

## Port Map

| Port | Service | URL |
|------|---------|-----|
| 3000 | Grafana | http://localhost:3000 (admin/admin) |
| 5432 | PostgreSQL | postgresql://polymarket:polymarket123@localhost:5432/polymarket_arb |
| 8081 | Wallet Tracker | http://localhost:8081/metrics |
| 9090 | Prometheus | http://localhost:9090 |

## Troubleshooting

### "Database already initialized" warnings
**Normal** - Migrations are idempotent, safe to re-run

### Wallet tracker not starting
```bash
# Check environment variables
docker-compose exec wallet-tracker env | grep POLYMARKET

# Check logs
docker-compose logs wallet-tracker
```

### Prometheus not scraping
```bash
# Check targets
curl http://localhost:9090/api/v1/targets | jq

# Test metrics endpoint
curl http://localhost:8081/metrics
```

### Can't connect to database
```bash
# Check postgres health
docker-compose exec postgres pg_isready -U polymarket

# Try connecting
docker-compose exec postgres psql -U polymarket -d polymarket_arb
```

## Key Files

| File | Purpose |
|------|---------|
| `docker-compose.yml` | Service definitions and dependencies |
| `prometheus.yml` | Metrics scraping configuration |
| `init-db.sh` | Database initialization script |
| `migrations/*.sql` | Database schema migrations |
| `grafana/dashboards/*.json` | Pre-configured dashboards |
| `.env` | Credentials (NEVER commit!) |

## Environment Variables

### Required (in .env)
```bash
POLYMARKET_PRIVATE_KEY=<hex_without_0x>
POLYMARKET_API_KEY=<from_derive_api_creds>
POLYMARKET_SECRET=<api_secret>
POLYMARKET_PASSPHRASE=<passphrase>
```

### Optional
```bash
LOG_LEVEL=info                    # debug, info, warn, error
STORAGE_MODE=postgres             # console or postgres (for arb bot)
```

## Common Workflows

### Daily Monitoring
```bash
# Check wallet balance
curl http://localhost:8081/metrics | grep polymarket_wallet_usdc_balance

# View in Grafana
open http://localhost:3000/d/trading-performance
```

### After Code Changes
```bash
# Rebuild and restart (data preserved)
docker-compose build wallet-tracker
docker-compose up -d --force-recreate wallet-tracker

# Check logs
docker-compose logs -f wallet-tracker
```

### Before Maintenance
```bash
# Backup database
docker-compose exec postgres pg_dump -U polymarket -Fc polymarket_arb > backup-$(date +%Y%m%d).dump

# Stop services
docker-compose down

# ... perform maintenance ...

# Restart services (data intact)
docker-compose up -d
```

### Investigating Issues
```bash
# Check service health
docker-compose ps

# Check resource usage
docker stats

# Check volumes
docker volume ls | grep polymarket
docker system df -v | grep polymarket

# Follow all logs
docker-compose logs -f --tail=100
```

## Reference Documentation

- **[DOCKER.md](DOCKER.md)** - Complete Docker guide
- **[DATA_PERSISTENCE.md](DATA_PERSISTENCE.md)** - Detailed persistence guide
- **[README.md](README.md)** - Project overview
- **[CLAUDE.md](CLAUDE.md)** - Development guide
