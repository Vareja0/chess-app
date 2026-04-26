#!/bin/bash
# after_install.sh
set -e

# Obtém o token do IMDSv2 (válido por 6 horas)
TOKEN=$(curl -s -X PUT "http://169.254.169.254/latest/api/token" -H "X-aws-ec2-metadata-token-ttl-seconds: 21600")

# Usa o token para buscar a região
REGION=$(curl -s -H "X-aws-ec2-metadata-token: $TOKEN" http://169.254.169.254/latest/meta-data/placement/region)

# Opcional: Adiciona uma validação para parar o script caso a região falhe
if [ -z "$REGION" ]; then
  echo "Erro: Não foi possível obter a AWS Region do EC2 metadata." >&2
  exit 1
fi
ENV="${ENVIRONMENT:-prod}"

APP_DIR="/opt/chess-app"
 
SECRET=$(aws secretsmanager get-secret-value \
  --region "$REGION" \
  --secret-id "${ENV}/chess-app" \
  --query SecretString \
  --output text)

get_secret() {
  echo "$SECRET" | jq -r ".[\"$1\"]"
}
echo "=== Buscando secrets ==="
DB_PASSWORD=$(get_secret "chess-app/production/db-password")
REFRESH_SECRET_KEY=$(get_secret "chess-app/production/refresh-secret-key")
SECRET_KEY=$(get_secret "chess-app/production/secret-key")
DOMAIN=$(get_secret "chess-app/production/domain")
DOCKERHUB_PASSWORD=$(get_secret "chess-app/dockerhub-password")
REDIS_PASSWORD=(get_secret "chess-app/dockerhub-redis-password")

# ── .env lido pelo Docker Compose ────────────────────────────
echo "=== Escrevendo .env ==="
DOCKERHUB_USERNAME=$(get_secret "chess-app/dockerhub-username")

echo "=== Debug DockerHub ==="
echo "Usuario capturado: '${DOCKERHUB_USERNAME}'"
# Não imprima a senha, mas verifique se ela está vazia/null
if [ "$DOCKERHUB_PASSWORD" == "null" ] || [ -z "$DOCKERHUB_PASSWORD" ]; then
    echo "ALERTA: A senha veio vazia ou nula!"
else
    echo "A senha foi capturada e tem tamanho: ${#DOCKERHUB_PASSWORD} caracteres."
fi

# Login no DockerHub para poder fazer pull de imagens privadas (se necessário)
echo "$DOCKERHUB_PASSWORD" | docker login -u "$DOCKERHUB_USERNAME" --password-stdin

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
REDIS_PASSWORD=${REDIS_PASSWORD}
IMAGE_TAG=latest
EOF
chmod 600 "${APP_DIR}/.env"
chown ec2-user:ec2-user "${APP_DIR}/.env"

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

# ── nginx reverse proxy ───────────────────────────────────────
echo "=== Configurando nginx ==="
DOMAIN_VAL="$DOMAIN"
cat > /etc/nginx/conf.d/chess-app.conf <<EOF
server {
    listen 80;
    server_name ${DOMAIN_VAL};

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

if [ ! -f "/etc/letsencrypt/live/${DOMAIN_VAL}/fullchain.pem" ]; then
  echo "=== Obtendo certificado Let's Encrypt ==="
  certbot --nginx --non-interactive --agree-tos \
    --email "admin@${DOMAIN_VAL}" \
    --domains "${DOMAIN_VAL}" \
    --redirect
else
  certbot renew --quiet --nginx
fi

( crontab -l 2>/dev/null | grep -v "certbot renew"; echo "0 3 * * * certbot renew --quiet --nginx" ) | crontab -

echo "=== AfterInstall concluído ==="
