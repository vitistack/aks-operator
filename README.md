# AKS-operator

Vitistack Kubernetes provider for AKS (Azure Kubernetes Service)

## Prerequisites

- Go 1.25+
- Azure subscription with permissions to create AKS clusters
- kubectl configured for your cluster
- Azure CLI (for obtaining credentials)

## Azure Credentials Setup

The operator requires Azure credentials to manage AKS clusters. You'll need to create an Azure Service Principal and configure the following environment variables:

| Variable                | Description                            |
| ----------------------- | -------------------------------------- |
| `AZURE_SUBSCRIPTION_ID` | Your Azure subscription ID             |
| `AZURE_OBJECT_ID`       | The Object ID of the service principal |
| `AZURE_CLIENT_SECRET`   | The client secret for authentication   |

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

> **Note:** If you don't have permission to create service principals (requires Azure AD admin), see [Alternative: Using an Existing Service Principal](#alternative-using-an-existing-service-principal) below.

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

### Alternative: Using an Existing Service Principal

If you don't have Azure AD admin permissions to create a service principal, an administrator can create one for you and grant you the Contributor role assignment.

#### For Administrators

Create a service principal and share the credentials with the user:

```bash
# Create the service principal
az ad sp create-for-rbac \
  --name "aks-operator-sp" \
  --role Contributor \
  --scopes /subscriptions/<SUBSCRIPTION_ID>

# Get the Object ID to share
az ad sp show --id <appId> --query id -o tsv
```

Share the `appId`, `password`, and Object ID with the user.

#### For Non-Admin Users

If an existing service principal already exists and you need to use it:

1. **Get the credentials** from your administrator:

   - `appId` (Client ID)
   - `password` (Client Secret)
   - Object ID

2. **Request Contributor role assignment** on the subscription or resource group. Ask your administrator to run:

   ```bash
   # Assign Contributor role to the service principal on a subscription
   az role assignment create \
     --assignee <appId> \
     --role Contributor \
     --scope /subscriptions/<SUBSCRIPTION_ID>
   ```

   Or for a specific resource group (more restrictive):

   ```bash
   # Assign Contributor role to the service principal on a resource group
   az role assignment create \
     --assignee <appId> \
     --role Contributor \
     --scope /subscriptions/<SUBSCRIPTION_ID>/resourceGroups/<RESOURCE_GROUP_NAME>
   ```

3. **Verify the role assignment**:

   ```bash
   az role assignment list \
     --assignee <appId> \
     --scope /subscriptions/<SUBSCRIPTION_ID> \
     -o table
   ```

4. **Configure your environment variables** as described in [Step 5](#step-5-configure-environment-variables).

#### Required Permissions

The service principal needs at minimum the following permissions:

| Permission                             | Scope              | Purpose                                        |
| -------------------------------------- | ------------------ | ---------------------------------------------- |
| `Contributor`                          | Subscription or RG | Create/manage AKS clusters                     |
| `Azure Kubernetes Service Contributor` | Subscription or RG | (Alternative) More restrictive AKS-only access |

For production environments, consider using the more restrictive `Azure Kubernetes Service Contributor` role:

```bash
az role assignment create \
  --assignee <appId> \
  --role "Azure Kubernetes Service Contributor" \
  --scope /subscriptions/<SUBSCRIPTION_ID>
```

### Using Workload Identity (Alternative)

If running in Azure (AKS), you can use [Workload Identity](https://learn.microsoft.com/en-us/azure/aks/workload-identity-overview) instead of a client secret. The operator will automatically use `DefaultAzureCredential` which supports:

- Environment variables
- Workload Identity
- Managed Identity
- Azure CLI credentials

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

# Build the operator
make build

# Run locally
make run
```

## License

Apache License 2.0
