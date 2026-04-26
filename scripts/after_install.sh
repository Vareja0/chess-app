#!/bin/bash
# after_install.sh
set -euo pipefail

REGION="us-east-1"
ENV="prod"
APP_DIR="/opt/chess-app"

# ── Buscar todos os secrets de uma vez ───────────────────────
echo "=== Buscando secrets ==="
SECRET=$(aws secretsmanager get-secret-value \
  --region "$REGION" \
  --secret-id "${ENV}/chess-app" \
  --query SecretString \
  --output text)

get_secret() {
  local key="$1"
  local val
  val=$(echo "$SECRET" | jq -r ".[\"$key\"]")
  if [ -z "$val" ] || [ "$val" = "null" ]; then
    echo "ERRO: secret '$key' não encontrado ou nulo" >&2
    exit 1
  fi
  echo "$val"
}

DB_PASSWORD=$(get_secret "db-password")
REFRESH_SECRET_KEY=$(get_secret "refresh-secret-key")
SECRET_KEY=$(get_secret "secret-key")
DOMAIN=$(get_secret "domain")
DOCKERHUB_PASSWORD=$(get_secret "dockerhub-password")
DOCKERHUB_USERNAME=$(get_secret "chess-app/dockerhub-username")

# ── Login no DockerHub ────────────────────────────────────────
echo "=== Login no DockerHub ==="
echo "$DOCKERHUB_PASSWORD" | docker login -u "$DOCKERHUB_USERNAME" --password-stdin

# ── Escrever .env lido pelo Docker Compose ───────────────────
echo "=== Escrevendo .env ==="
cat > "${APP_DIR}/.env" <<EOF
PORT=3000
GIN_MODE=release
DB_HOST=db
DB_PORT=5432
DB_USER=chess_user
DB_PASSWORD=${DB_PASSWORD}
DB_NAME=chess
REDIS_HOST=redis:6379
SECRET_KEY=${SECRET_KEY}
REFRESH_SECRET_KEY=${REFRESH_SECRET_KEY}
POSTGRES_USER=chess_user
POSTGRES_PASSWORD=${DB_PASSWORD}
POSTGRES_DB=chess
DOCKERHUB_USERNAME=${DOCKERHUB_USERNAME}
IMAGE_TAG=latest
EOF

chmod 600 "${APP_DIR}/.env"
chown ec2-user:ec2-user "${APP_DIR}/.env"

# ── Pull das imagens mais recentes ───────────────────────────
echo "=== Atualizando imagens Docker ==="
docker compose -f "${APP_DIR}/docker-compose.prod.yml" pull

# ── Systemd gerencia o docker compose ────────────────────────
echo "=== Configurando systemd ==="
cat > /etc/systemd/system/chess-app.service <<'EOF'
[Unit]
Description=Chess App (Docker Compose)
After=docker.service
Requires=docker.service

[Service]
Type=simple
User=ec2-user
WorkingDirectory=/opt/chess-app
EnvironmentFile=/opt/chess-app/.env
ExecStart=docker compose -f docker-compose.prod.yml up
ExecStop=docker compose -f docker-compose.prod.yml down
Restart=on-failure
RestartSec=10

[Install]
WantedBy=multi-user.target
EOF

systemctl daemon-reload
systemctl enable chess-app
systemctl start chess-app

# ── nginx reverse proxy ───────────────────────────────────────
echo "=== Configurando nginx ==="
cat > /etc/nginx/conf.d/chess-app.conf <<EOF
server {
    listen 80;
    server_name ${DOMAIN};

    location /.well-known/acme-challenge/ {
        root /var/www/html;
    }

    location / {
        proxy_pass         http://127.0.0.1:3000;
        proxy_http_version 1.1;
        proxy_set_header   Upgrade \$http_upgrade;
        proxy_set_header   Connection "upgrade";
        proxy_set_header   Host \$host;
        proxy_set_header   X-Real-IP \$remote_addr;
        proxy_set_header   X-Forwarded-For \$proxy_add_x_forwarded_for;
        proxy_set_header   X-Forwarded-Proto \$scheme;
        proxy_read_timeout 86400;
    }
}
EOF

nginx -t && systemctl restart nginx

# ── Certificado Let's Encrypt ─────────────────────────────────
if [ ! -f "/etc/letsencrypt/live/${DOMAIN}/fullchain.pem" ]; then
  echo "=== Obtendo certificado Let's Encrypt ==="
  certbot --nginx --non-interactive --agree-tos \
    --email "admin@${DOMAIN}" \
    --domains "${DOMAIN}" \
    --redirect
else
  echo "=== Renovando certificado existente ==="
  certbot renew --quiet --nginx
fi

# ── Cron de renovação automática ─────────────────────────────
( crontab -l 2>/dev/null | grep -v "certbot renew"; echo "0 3 * * * certbot renew --quiet --nginx" ) | crontab -

echo "=== AfterInstall concluído com sucesso ==="
