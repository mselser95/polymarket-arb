#!/bin/bash
set -e

echo "🔧 Initializing Polymarket database..."

# Wait for postgres to be ready
until pg_isready -U polymarket; do
  echo "⏳ Waiting for PostgreSQL to be ready..."
  sleep 2
done

echo "✅ PostgreSQL is ready"

# Run migrations
echo "📦 Running migrations..."
for migration in /migrations/*.up.sql; do
  if [ -f "$migration" ]; then
    echo "  - Applying $(basename $migration)..."
    psql -U polymarket -d polymarket_arb -f "$migration"
  fi
done

echo "✅ Database initialization complete"
