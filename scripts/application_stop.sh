#!/bin/bash
# application_stop.sh
systemctl stop chess-app 2>/dev/null || true
cd /opt/chess-app 2>/dev/null && docker compose -f docker-compose.prod.yml down 2>/dev/null || true
