#!/bin/bash
# before_install.sh
set -e
systemctl stop chess-app 2>/dev/null || true
rm -f /opt/chess-app/docker-compose.prod.yml
mkdir -p /opt/chess-app /var/log/chess-app
chown -R ec2-user:ec2-user /opt/chess-app /var/log/chess-app
