# GitOps Pipeline Deployment Instructions

## Test Deployment (Single File)
kubectl apply -f gitops-demo-app_2025-09-25_16-56-34/test-deployment.yaml

## Verify Deployment
kubectl get all -n demo-app -l source=backup-restore

## GitOps Production Deployments

### Option 1: ArgoCD
kubectl apply -f gitops-demo-app_2025-09-25_16-56-34/argocd/application.yaml
# ArgoCD will automatically sync and deploy the application

### Option 2: Flux
kubectl apply -f gitops-demo-app_2025-09-25_16-56-34/flux/
# Flux will automatically reconcile and deploy the resources

### Option 3: Manual Kustomize
# Note: Base Kustomization requires fixing of backup resource structure
# Current test uses simplified deployment structure

## Cleanup Test Resources
kubectl delete -f gitops-demo-app_2025-09-25_16-56-34/test-deployment.yaml
