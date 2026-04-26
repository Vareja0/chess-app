#!/bin/bash
sudo systemctl stop chess-app 2>/dev/null || true
cd /opt/chess-app 2>/dev/null || true
docker compose -f docker-compose.prod.yml down -v 2>/dev/null || true
