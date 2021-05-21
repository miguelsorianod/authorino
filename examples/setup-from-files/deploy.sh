#!/bin/bash

set -euo pipefail

export AUTHORINO_NAMESPACE="authorino"

echo "Creating namespace"
kubectl create namespace "${AUTHORINO_NAMESPACE}"

echo "Deploying Envoy"
kubectl -n "${AUTHORINO_NAMESPACE}" apply -f examples/envoy/envoy-deploy.yaml

echo "Deploying Talker API"
kubectl -n "${AUTHORINO_NAMESPACE}" apply -f examples/talker-api/talker-api-deploy.yaml

echo "Deploying Authorino"
kustomize build examples/setup-from-files | kubectl -n "${AUTHORINO_NAMESPACE}" apply -f -

echo "Wait for all deployments to be up"
kubectl -n "${AUTHORINO_NAMESPACE}" wait --timeout=300s --for=condition=Available deployments --all
