#!/bin/bash
set -e

echo "🚀 Starting Polymarket Monitoring Stack..."
echo ""

# Check if .env exists
if [ ! -f .env ]; then
    echo "❌ Error: .env file not found"
    echo ""
    echo "Please create .env file with your credentials:"
    echo "  cp .env.example .env"
    echo "  # Edit .env with your POLYMARKET_PRIVATE_KEY and API credentials"
    exit 1
fi

# Check if Docker is running
if ! docker info > /dev/null 2>&1; then
    echo "❌ Error: Docker is not running"
    echo "Please start Docker Desktop or Docker daemon"
    exit 1
fi

# Build and start services
echo "📦 Building Docker images..."
docker-compose build

echo ""
echo "🎬 Starting services..."
docker-compose up -d

echo ""
echo "⏳ Waiting for database initialization..."
sleep 3

# Show db-init logs
echo ""
echo "📊 Database Initialization:"
docker-compose logs db-init

echo ""
echo "⏳ Waiting for services to be healthy..."
sleep 5

# Check service health
echo ""
echo "🔍 Service Status:"
docker-compose ps

echo ""
echo "✅ Monitoring stack is running!"
echo ""
echo "📊 Access the services:"
echo "  • Grafana:        http://localhost:3000 (admin/admin)"
echo "  • Prometheus:     http://localhost:9090"
echo "  • Wallet Metrics: http://localhost:8081/metrics"
echo "  • Health Check:   http://localhost:8081/health"
echo ""
echo "📝 View logs:"
echo "  docker-compose logs -f"
echo ""
echo "🛑 Stop services:"
echo "  docker-compose down"
echo ""
