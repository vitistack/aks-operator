# AKS-operator

Vitistack Kubernetes provider for AKS (Azure Kubernetes Service)

## Prerequisites

- Go 1.25+
- Azure subscription with permissions to create AKS clusters
- kubectl configured for your cluster
- Azure CLI (for obtaining credentials)

## Azure Credentials Setup

The operator requires Azure credentials to manage AKS clusters. You'll need to create an Azure Service Principal and configure the following environment variables:

| Variable | Description |
|----------|-------------|
| `AZURE_SUBSCRIPTION_ID` | Your Azure subscription ID |
| `AZURE_OBJECT_ID` | The Object ID of the service principal |
| `AZURE_CLIENT_SECRET` | The client secret for authentication |

### Step 1: Login to Azure CLI

```bash
az login
```

### Step 2: Get your Subscription ID

```bash
az account show --query id -o tsv
```

This returns your `AZURE_SUBSCRIPTION_ID`.

### Step 3: Create a Service Principal

Create a service principal with Contributor role scoped to your subscription:

```bash
az ad sp create-for-rbac \
  --name "aks-operator-sp" \
  --role Contributor \
  --scopes /subscriptions/<SUBSCRIPTION_ID>
```

This command outputs:

```json
{
  "appId": "xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx",
  "displayName": "aks-operator-sp",
  "password": "xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx",
  "tenant": "xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx"
}
```

- `password` → Use as `AZURE_CLIENT_SECRET`

### Step 4: Get the Object ID

```bash
az ad sp show --id <appId> --query id -o tsv
```

This returns your `AZURE_OBJECT_ID`.

### Step 5: Configure Environment Variables

Copy the `.env.example` file and fill in your values:

```bash
cp .env.example .env
```

Edit `.env`:

```dotenv
AZURE_SUBSCRIPTION_ID=<your-subscription-id>
AZURE_OBJECT_ID=<service-principal-object-id>
AZURE_CLIENT_SECRET=<service-principal-password>
```

### Using Workload Identity (Alternative)

If running in Azure (AKS), you can use [Workload Identity](https://learn.microsoft.com/en-us/azure/aks/workload-identity-overview) instead of a client secret. The operator will automatically use `DefaultAzureCredential` which supports:

- Environment variables
- Workload Identity
- Managed Identity
- Azure CLI credentials

## Development

```bash
# Run tests
make test

# Run linter
make lint

# Run security scanner
make gosec

# Run vulnerability check
make govulncheck

# Build the operator
make build

# Run locally
make run
```

## License

Apache License 2.0
