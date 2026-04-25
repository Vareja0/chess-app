#!/bin/bash
# application_start.sh
set -e
cd /opt/chess-app
echo "=== Pull da imagem e start dos containers ==="
docker compose -f docker-compose.prod.yml pull app
systemctl start chess-app
sleep 5
systemctl is-active chess-app || { journalctl -u chess-app -n 20 --no-pager; exit 1; }
