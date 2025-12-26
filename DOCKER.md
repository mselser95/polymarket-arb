# Docker Compose Monitoring Stack

Complete observability stack for Polymarket arbitrage bot with PostgreSQL, Prometheus, Grafana, and wallet tracking.

## Services

| Service | Port | Description |
|---------|------|-------------|
| **postgres** | 5432 | PostgreSQL database for persistent storage |
| **db-init** | - | One-time database initialization (runs migrations) |
| **prometheus** | 9090 | Metrics collection and time-series database |
| **grafana** | 3000 | Visualization dashboards (user: admin / pass: admin) |
| **wallet-tracker** | 8081 | Real-time wallet balance and P&L monitoring |

## Data Persistence 💾

**Your data is SAFE across restarts!** Docker named volumes ensure:
- ✅ Survives `docker-compose down` / `docker-compose up`
- ✅ Survives PC reboots and crashes
- ✅ Survives container rebuilds
- ❌ Only deleted with `docker-compose down -v` (explicit flag)

All data stored in persistent volumes:
- `postgres_data` - Database tables, arbitrage opportunities
- `prometheus_data` - 30 days of time-series metrics
- `grafana_data` - Dashboard configurations

See **[DATA_PERSISTENCE.md](DATA_PERSISTENCE.md)** for complete guide on backups, migrations, and data safety.

## Quick Start

### 1. Prerequisites

```bash
# Docker & Docker Compose installed
docker --version
docker-compose --version

# Environment file configured
cp .env.example .env
# Edit .env with your POLYMARKET_PRIVATE_KEY and credentials
```

### 2. Start the Stack

```bash
# Build and start all services
docker-compose up -d

# View logs
docker-compose logs -f

# View specific service logs
docker-compose logs -f wallet-tracker
docker-compose logs -f prometheus
docker-compose logs -f grafana
```

### 3. Access the Services

- **Grafana Dashboards**: http://localhost:3000
  - Username: `admin`
  - Password: `admin`
  - Dashboards are pre-loaded from `grafana/dashboards/`

- **Prometheus Metrics**: http://localhost:9090
  - Query metrics directly
  - View targets: http://localhost:9090/targets
  - Check scrape health

- **Wallet Tracker Metrics**: http://localhost:8081/metrics
  - Raw Prometheus metrics
  - Health check: http://localhost:8081/health

- **PostgreSQL**: localhost:5432
  - Database: `polymarket_arb`
  - User: `polymarket`
  - Password: `polymarket123`

## Configuration

### Environment Variables

The wallet tracker uses your `.env` file for credentials:

```bash
# Required for wallet tracker
POLYMARKET_PRIVATE_KEY=<your_private_key_hex>
POLYMARKET_API_KEY=<from_derive_api_creds>
POLYMARKET_SECRET=<from_derive_api_creds>
POLYMARKET_PASSPHRASE=<from_derive_api_creds>
```

### Prometheus Scrape Config

Edit `prometheus.yml` to customize:
- Scrape intervals (default: 30s for wallet, 15s for bot)
- Add additional targets
- Configure alerting rules

### Grafana Dashboards

Pre-configured dashboards in `grafana/dashboards/`:
- **01-trading-performance.json**: Wallet balance, P&L, position tracking
- **02-system-health.json**: Service health, error rates, uptime
- **03-market-activity.json**: Orderbook updates, arbitrage opportunities
- **04-latency-performance.json**: Response times, processing latency

Grafana auto-loads dashboards from provisioning config:
- `grafana/provisioning/dashboards.yml`
- `grafana/provisioning/datasources.yml`

## Common Operations

### View Service Status

```bash
# List running containers
docker-compose ps

# Check health status
docker-compose ps wallet-tracker
docker-compose ps prometheus
docker-compose ps grafana
```

### Restart Services

```bash
# Restart all services
docker-compose restart

# Restart specific service
docker-compose restart wallet-tracker
docker-compose restart prometheus
```

### View Real-Time Logs

```bash
# All services
docker-compose logs -f

# Specific service with timestamp
docker-compose logs -f --timestamps wallet-tracker

# Last 100 lines
docker-compose logs --tail=100 wallet-tracker
```

### Stop and Clean Up

```bash
# Stop services (keep data)
docker-compose stop

# Stop and remove containers (keep volumes)
docker-compose down

# Remove everything including volumes (⚠️ deletes all data)
docker-compose down -v

# Rebuild images
docker-compose build --no-cache
docker-compose up -d
```

### Execute Commands in Containers

```bash
# Open shell in wallet-tracker container
docker-compose exec wallet-tracker sh

# Run balance check
docker-compose exec wallet-tracker ./polymarket-arb balance

# Check metrics endpoint
docker-compose exec wallet-tracker wget -qO- http://localhost:8080/metrics
```

## Metrics Exposed

### Wallet Tracker Metrics (Port 8081)

```promql
# Current MATIC balance (for gas)
polymarket_wallet_matic_balance

# Current USDC balance (for trading)
polymarket_wallet_usdc_balance

# USDC approved to CTF Exchange
polymarket_wallet_usdc_allowance

# Number of active positions
polymarket_wallet_active_positions

# Total position value (market value)
polymarket_wallet_total_position_value

# Total cost basis of positions
polymarket_wallet_total_position_cost

# Unrealized P&L (USD)
polymarket_wallet_unrealized_pnl

# Unrealized P&L (percentage)
polymarket_wallet_unrealized_pnl_percent

# Total portfolio value (USDC + positions)
polymarket_wallet_portfolio_value

# Update errors (failures to fetch data)
polymarket_wallet_update_errors_total

# Update duration (latency)
polymarket_wallet_update_duration_seconds

# Last successful update timestamp
polymarket_wallet_last_update_timestamp
```

### Example Prometheus Queries

```promql
# Current portfolio value
polymarket_wallet_portfolio_value

# P&L over last hour
delta(polymarket_wallet_unrealized_pnl[1h])

# Average update latency (5m window)
rate(polymarket_wallet_update_duration_seconds_sum[5m]) / 
rate(polymarket_wallet_update_duration_seconds_count[5m])

# Error rate (errors per minute)
rate(polymarket_wallet_update_errors_total[1m]) * 60

# Position count trend
increase(polymarket_wallet_active_positions[24h])
```

## Troubleshooting

### Wallet Tracker Not Starting

```bash
# Check logs for errors
docker-compose logs wallet-tracker

# Common issues:
# 1. Missing .env file
ls -la .env

# 2. Invalid credentials
docker-compose exec wallet-tracker env | grep POLYMARKET

# 3. Build failed
docker-compose build wallet-tracker
```

### Prometheus Not Scraping

```bash
# Check Prometheus targets
curl http://localhost:9090/api/v1/targets

# Verify wallet-tracker is reachable
docker-compose exec prometheus wget -qO- http://wallet-tracker:8080/metrics

# Check prometheus.yml syntax
docker-compose exec prometheus promtool check config /etc/prometheus/prometheus.yml
```

### Grafana Dashboards Not Loading

```bash
# Check Grafana logs
docker-compose logs grafana

# Verify provisioning files
ls -la grafana/provisioning/
ls -la grafana/dashboards/

# Restart Grafana
docker-compose restart grafana

# Check datasource connection
curl -u admin:admin http://localhost:3000/api/datasources
```

### PostgreSQL Connection Issues

```bash
# Check if PostgreSQL is ready
docker-compose exec postgres pg_isready -U polymarket

# Connect to database
docker-compose exec postgres psql -U polymarket -d polymarket_arb

# View tables
docker-compose exec postgres psql -U polymarket -d polymarket_arb -c '\dt'
```

## Advanced Configuration

### Custom Update Interval

Edit `docker-compose.yml` to change wallet tracker frequency:

```yaml
wallet-tracker:
  command: ["track-balance", "--interval", "30s", "--port", "8080"]
```

### Enable Arbitrage Bot

Uncomment the `app` service in `docker-compose.yml`:

```yaml
app:
  build:
    context: .
    dockerfile: Dockerfile
  container_name: polymarket-arb-bot
  # ... rest of configuration
```

Then update `prometheus.yml` to scrape bot metrics:

```yaml
- job_name: 'polymarket-arb-bot'
  static_configs:
    - targets: ['app:8082']
```

### Persistent Data

All data is stored in Docker volumes:

```bash
# List volumes
docker volume ls | grep polymarket

# Inspect volume
docker volume inspect polymarket-arb_prometheus_data

# Backup volume
docker run --rm -v polymarket-arb_prometheus_data:/data -v $(pwd):/backup \
  alpine tar czf /backup/prometheus-backup.tar.gz /data

# Restore volume
docker run --rm -v polymarket-arb_prometheus_data:/data -v $(pwd):/backup \
  alpine tar xzf /backup/prometheus-backup.tar.gz -C /
```

### Resource Limits

Add resource constraints in `docker-compose.yml`:

```yaml
wallet-tracker:
  deploy:
    resources:
      limits:
        cpus: '0.5'
        memory: 512M
      reservations:
        cpus: '0.25'
        memory: 256M
```

## Security Considerations

### Production Deployment

For production use:

1. **Change default passwords**:
   ```yaml
   grafana:
     environment:
       - GF_SECURITY_ADMIN_PASSWORD=${GRAFANA_ADMIN_PASSWORD}
   
   postgres:
     environment:
       POSTGRES_PASSWORD: ${POSTGRES_PASSWORD}
   ```

2. **Use secrets management**:
   ```bash
   # Use Docker secrets instead of .env
   echo "my_secret_key" | docker secret create polymarket_key -
   ```

3. **Enable TLS**:
   - Configure HTTPS for Grafana
   - Use SSL for PostgreSQL connections
   - Enable authentication on Prometheus

4. **Network isolation**:
   ```yaml
   networks:
     polymarket-network:
       driver: bridge
       internal: true  # No external access
   ```

5. **Regular backups**:
   ```bash
   # Automated backup script
   docker-compose exec postgres pg_dump -U polymarket polymarket_arb > backup.sql
   ```

## Maintenance

### Log Rotation

```bash
# Configure Docker log rotation in /etc/docker/daemon.json
{
  "log-driver": "json-file",
  "log-opts": {
    "max-size": "10m",
    "max-file": "3"
  }
}
```

### Update Services

```bash
# Pull latest images
docker-compose pull

# Rebuild and restart
docker-compose up -d --build

# Prune unused resources
docker system prune -a
```

### Monitor Resource Usage

```bash
# Container stats
docker stats

# Disk usage
docker system df

# Volume sizes
docker system df -v
```

## References

- [Prometheus Documentation](https://prometheus.io/docs/)
- [Grafana Documentation](https://grafana.com/docs/)
- [Docker Compose Reference](https://docs.docker.com/compose/)
- [PostgreSQL Docker Hub](https://hub.docker.com/_/postgres)

## Support

For issues or questions:
- Check logs: `docker-compose logs <service>`
- Verify configuration: `docker-compose config`
- Restart services: `docker-compose restart`
- Report bugs in project repository
