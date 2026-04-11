#!/usr/bin/env bash
set -euo pipefail

# ── Prerequisites ─────────────────────────────────────────────────────────────


# ── Configuration ────────────────────────────────────────────────────────────
PROJECT_ID="${PROJECT_ID:?Set PROJECT_ID}"
REGION="${REGION:-us-central1}"
ZONE="${ZONE:-us-central1-a}"
CLUSTER_NAME="${CLUSTER_NAME:-chess.k8s.local}"

CHART_DIR="./chess-app"
KOPS_DIR="./kops"
TF_DIR="./terraform"
# ─────────────────────────────────────────────────────────────────────────────

echo "==> [1/5] Terraform: provision VPC, static IP, GCS bucket"
cd "$TF_DIR"
terraform init -input=false
terraform apply -input=false -auto-approve \
  -var="project_id=$PROJECT_ID" \
  -var="region=$REGION" \
  -var="zone=$ZONE" \
  -var="cluster_name=$CLUSTER_NAME"

STATIC_IP=$(terraform output -raw static_ip_address)
STATE_BUCKET=$(terraform output -raw kops_state_bucket)
VPC_NAME=$(terraform output -raw vpc_name)
cd ..

echo "  Static IP : $STATIC_IP"
echo "  State     : $STATE_BUCKET"
echo "  VPC       : $VPC_NAME"

echo "==> [2/5] kops: create Kubernetes cluster"
export KOPS_STATE_STORE="$STATE_BUCKET"

# Substitute placeholders in cluster.yaml and pipe to kops
sed \
  -e "s|REPLACE_PROJECT_ID|$PROJECT_ID|g" \
  -e "s|REPLACE_CLUSTER_NAME|$CLUSTER_NAME|g" \
  -e "s|REPLACE_STATE_BUCKET|$STATE_BUCKET|g" \
  -e "s|REPLACE_REGION|$REGION|g" \
  -e "s|REPLACE_ZONE|$ZONE|g" \
  "$KOPS_DIR/cluster.yaml" | /usr/local/bin/kops/kops replace -f - --force

/usr/local/bin/kops/kops update cluster "$CLUSTER_NAME" --yes --admin

echo "  Waiting for cluster to become ready (this takes ~5 min)..."
/usr/local/bin/kops/kops validate cluster "$CLUSTER_NAME" --wait 10m

echo "==> [3/5] Helm: add Bitnami repo and update dependencies"
helm repo add bitnami https://charts.bitnami.com/bitnami
helm repo update
helm dependency update "$CHART_DIR"

echo "==> [4/5] Helm: install nginx-ingress controller with static IP"
helm repo add ingress-nginx https://kubernetes.github.io/ingress-nginx
helm repo update
helm upgrade --install ingress-nginx ingress-nginx/ingress-nginx \
  --namespace ingress-nginx \
  --create-namespace \
  --set controller.service.loadBalancerIP="$STATIC_IP" \
  --set controller.service.annotations."cloud\.google\.com/load-balancer-type"=External \
  --wait

echo "==> [5/5] Helm: install chess-app"
helm upgrade --install chess-app "$CHART_DIR" \
  -f "$CHART_DIR/values-local.yaml" \
  --set ingress.host="" \
  --wait

echo ""
echo "Done! Application is available at: http://$STATIC_IP"
