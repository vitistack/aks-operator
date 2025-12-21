# AKS-operator

Vitistack Kubernetes provider for AKS (Azure Kubernetes Service)

## Prerequisites

- Go 1.25+
- Azure subscription with permissions to create AKS clusters
- kubectl configured for your cluster
- Azure CLI (for obtaining credentials)

## Azure Credentials Setup

The operator requires Azure credentials to manage AKS clusters. Choose your authentication method:

| Method                                                         | Best For              | Admin Required      | Works in Cluster? |
| -------------------------------------------------------------- | --------------------- | ------------------- | ----------------- |
| [User Credentials (Azure CLI)](docs/azure-user-credentials.md) | Local development     | No                  | ❌ No             |
| [Service Principal](docs/azure-service-principal.md)           | Production, CI/CD     | Yes (initial setup) | ✅ Yes            |
| [Workload Identity](docs/azure-workload-identity.md)           | Running in AKS        | Yes                 | ✅ Yes            |
| Managed Identity                                               | Azure VMs/VMSS nodes  | Yes                 | ✅ Yes            |

> **Important:** User credentials (Azure CLI) only work when running the operator locally with `make run`.
> To deploy the operator in a Kubernetes cluster, you **must** use Service Principal, Workload Identity, or Managed Identity.

### Quick Start (Local Development)

**Option 1: User Credentials (Easiest for local development)**

If you have **Contributor** role on the subscription:

```bash
az login
export AZURE_SUBSCRIPTION_ID=$(az account show --query id -o tsv)
make run
```

**Option 2: Service Principal (Recommended for production)**

```bash
export AZURE_SUBSCRIPTION_ID=<subscription-id>
export AZURE_TENANT_ID=<tenant-id>
export AZURE_CLIENT_ID=<client-id>
export AZURE_CLIENT_SECRET=<client-secret>
make run
```

See [Azure Authentication Guide](docs/azure-authentication.md) for detailed setup instructions.

## Installation

### Helm Chart

The operator is available as an OCI Helm chart from GitHub Container Registry.

> **⚠️ Authentication Required:** The operator requires Azure credentials to function.
> User credentials (Azure CLI) do **not** work in a cluster - you must configure one of:
> - Service Principal (recommended)
> - Workload Identity (for AKS clusters)
> - Managed Identity (for Azure VM/VMSS nodes)

#### With Azure Service Principal (Recommended)

```bash
# Login to GitHub Container Registry
helm registry login ghcr.io

# Create a secret with Azure credentials
kubectl create namespace vitistack

kubectl create secret generic azure-credentials \
  --namespace vitistack \
  --from-literal=AZURE_SUBSCRIPTION_ID=<subscription-id> \
  --from-literal=AZURE_TENANT_ID=<tenant-id> \
  --from-literal=AZURE_CLIENT_ID=<client-id> \
  --from-literal=AZURE_CLIENT_SECRET=<client-secret>

# Install the operator referencing the existing secret
helm install aks-operator oci://ghcr.io/vitistack/helm/aks-operator \
  --namespace vitistack \
  --set azure.existingSecret=azure-credentials
```

#### With Workload Identity (AKS)

If your AKS cluster has Workload Identity enabled:

```bash
helm install aks-operator oci://ghcr.io/vitistack/helm/aks-operator \
  --namespace vitistack \
  --create-namespace \
  --set azure.subscriptionId=<subscription-id> \
  --set "serviceAccount.annotations.azure\.workload\.identity/client-id=<managed-identity-client-id>"
```

See [Workload Identity Guide](docs/azure-workload-identity.md) for setup instructions.

#### With Azure Credentials in Values

```bash
# Install with credentials directly (not recommended for production)
helm install aks-operator oci://ghcr.io/vitistack/helm/aks-operator \
  --namespace vitistack \
  --create-namespace \
  --set azure.subscriptionId=<subscription-id> \
  --set azure.tenantId=<tenant-id> \
  --set azure.clientId=<client-id> \
  --set azure.clientSecret=<client-secret>
```

#### From Local Chart

```bash
helm install aks-operator ./charts/aks-operator \
  --namespace vitistack \
  --create-namespace
```

#### Configuration

| Parameter | Description | Default |
|-----------|-------------|---------|
| `replicaCount` | Number of replicas | `1` |
| `image.repository` | Image repository | `ghcr.io/vitistack/viti-aks-operator` |
| `image.tag` | Image tag | `""` (uses chart appVersion) |
| `image.pullPolicy` | Image pull policy | `IfNotPresent` |
| `serviceAccount.create` | Create service account | `true` |
| `rbac.create` | Create RBAC resources | `true` |
| `leaderElection.enabled` | Enable leader election | `false` |
| `azure.existingSecret` | Name of existing secret with Azure credentials | `""` |
| `azure.subscriptionId` | Azure Subscription ID | `""` |
| `azure.tenantId` | Azure Tenant ID | `""` |
| `azure.clientId` | Azure Client ID (Service Principal) | `""` |
| `azure.clientSecret` | Azure Client Secret | `""` |
| `env` | Additional environment variables | `[]` |
| `envFrom` | Additional envFrom sources | `[]` |
| `resources.limits.cpu` | CPU limit | `100m` |
| `resources.limits.memory` | Memory limit | `128Mi` |

See [values.yaml](charts/aks-operator/values.yaml) for all available options.

## Azure Resource Groups

The `spec.data.project` field maps to the Azure Resource Group name:

```yaml
apiVersion: vitistack.io/v1alpha1
kind: KubernetesCluster
metadata:
  name: my-cluster
spec:
  data:
    project: my-resource-group # Must exist in Azure
    region: norwayeast
```

> **Important:** The resource group must exist before creating a cluster.

```bash
az group create --name my-project --location norwayeast
```

See [Azure Resource Groups Guide](docs/azure-resource-groups.md) for details.

## Documentation

- [Azure Authentication Overview](docs/azure-authentication.md)
- [Service Principal Setup](docs/azure-service-principal.md) - For admins and production
- [User Credentials Setup](docs/azure-user-credentials.md) - For local development
- [Azure Permissions](docs/azure-permissions.md) - Required roles and permissions
- [Resource Groups](docs/azure-resource-groups.md) - How projects map to resource groups
- [Workload Identity](docs/azure-workload-identity.md) - For running in AKS
- [Troubleshooting](docs/troubleshooting.md) - Common issues and solutions

## Development

```bash
# Build
make build

# Run tests
make test

# Run linter
make lint

# Run security scanner
make gosec

# Run vulnerability check
make govulncheck

# Run locally
make run
```

## License

Apache License 2.0
