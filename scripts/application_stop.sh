#!/bin/bash
# application_stop.sh
cd /opt/chess-app 2>/dev/null || true
docker compose -f docker-compose.prod.yml down -v 2>/dev/null || true
systemctl stop chess-app 2>/dev/null || true
cd /opt/chess-app 2>/dev/null && docker compose -f docker-compose.prod.yml down 2>/dev/null || true
