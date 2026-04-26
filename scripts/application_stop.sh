#!/bin/bash
cd /opt/chess-app 2>/dev/null || true
docker compose -f docker-compose.prod.yml down 2>/dev/null || true
docker volume rm chess-app_pgdata chess-app_redis-data 2>/dev/null || true
