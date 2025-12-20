# AKS-operator

Vitistack Kubernetes provider for AKS (Azure Kubernetes Service)

## Prerequisites

- Go 1.25+
- Azure subscription with permissions to create AKS clusters
- kubectl configured for your cluster
- Azure CLI (for obtaining credentials)

## Azure Credentials Setup

The operator requires Azure credentials to manage AKS clusters. Choose your authentication method:

| Method                                                         | Best For          | Admin Required      |
| -------------------------------------------------------------- | ----------------- | ------------------- |
| [User Credentials (Azure CLI)](docs/azure-user-credentials.md) | Local development | No                  |
| [Service Principal](docs/azure-service-principal.md)           | Production, CI/CD | Yes (initial setup) |
| [Workload Identity](docs/azure-workload-identity.md)           | Running in AKS    | Yes                 |

### Quick Start

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
