# Installation

```bash
# Create namespace
kubectl create namespace kelon

# Load configmaps
kubectl apply -f ./manifests/configmaps.yml

# Start MongoDB
helm install -n kelon -f ./manifests/mongo-values.yml mongo stable/mongodb

# Start PostgreSQL
helm install -n kelon -f ./manifests/postgres-values.yml postgres stable/postgresql

# Start MySQL
helm install -n kelon -f ./manifests/mysql-values.yml mysql stable/mysql

# Start Kelon
kubectl apply -f ./manifests/kube-mgmt-deployment.yml
```
