#!/bin/bash
# validate_service.sh
set -e
for i in $(seq 1 15); do
  CODE=$(curl http://localhost:3000/ || true)
  [[ "$CODE" =~ ^(200|301|302)$ ]] && { echo "OK (HTTP $CODE)"; exit 0; }
  echo "Tentativa $i/15 — HTTP $CODE. Aguardando 3s..."
  sleep 3
done
echo "FALHA: app não respondeu"
journalctl -u chess-app -n 30 --no-pager
exit 1
