# Azure Authentication

The AKS Operator uses the Azure SDK's `DefaultAzureCredential` which supports multiple authentication methods. Choose the method that fits your environment:

## Authentication Methods

| Method                                            | Best For                               | Admin Required      | Documentation                              |
| ------------------------------------------------- | -------------------------------------- | ------------------- | ------------------------------------------ |
| [User Credentials (Azure CLI)](#user-credentials) | Local development, personal testing    | No                  | [Full Guide](./azure-user-credentials.md)  |
| [Service Principal](#service-principal)           | Production, CI/CD, shared environments | Yes (initial setup) | [Full Guide](./azure-service-principal.md) |
| [Workload Identity](#workload-identity)           | Running in AKS                         | Yes                 | [Full Guide](./azure-workload-identity.md) |

## Quick Comparison

### User Credentials (Azure CLI)

**Best for:** Local development when you already have Contributor access.

```bash
# Login to Azure
az login

# Set only the subscription ID
export AZURE_SUBSCRIPTION_ID=$(az account show --query id -o tsv)

# Run the operator
make run
```

**Pros:**

- No admin setup required
- Uses your existing Azure permissions
- Easiest to get started

**Cons:**

- Not suitable for production
- Tokens expire and need re-authentication
- Tied to individual user

See [User Credentials Guide](./azure-user-credentials.md) for details.

---

### Service Principal

**Best for:** Production, CI/CD pipelines, shared environments.

```bash
export AZURE_SUBSCRIPTION_ID=<subscription-id>
export AZURE_TENANT_ID=<tenant-id>
export AZURE_CLIENT_ID=<client-id>
export AZURE_CLIENT_SECRET=<client-secret>
```

**Pros:**

- Recommended for production
- Can be scoped to specific permissions
- Works in CI/CD and containers

**Cons:**

- Requires admin to create (or existing SP)
- Secrets need rotation

See [Service Principal Guide](./azure-service-principal.md) for setup instructions.

---

### Workload Identity

**Best for:** Running the operator inside AKS.

Uses Kubernetes service account federation with Azure AD. No secrets to manage.

See [Workload Identity Guide](./azure-workload-identity.md) for setup.

---

## Required Permissions

Regardless of authentication method, the identity needs **Contributor** role (or equivalent) on the subscription or resource group.

| Azure Operation            | Required Permission                                                            |
| -------------------------- | ------------------------------------------------------------------------------ |
| Create/Update AKS clusters | `Microsoft.ContainerService/managedClusters/write`                             |
| Delete AKS clusters        | `Microsoft.ContainerService/managedClusters/delete`                            |
| Get cluster credentials    | `Microsoft.ContainerService/managedClusters/listClusterAdminCredential/action` |
| Manage agent pools         | `Microsoft.ContainerService/managedClusters/agentPools/*`                      |
| List VM sizes              | `Microsoft.Compute/locations/vmSizes/read`                                     |

The **Contributor** built-in role includes all of these. For least-privilege, use **Azure Kubernetes Service Contributor**.

See [Azure Permissions Guide](./azure-permissions.md) for detailed permission requirements.

## Authentication Priority

The Azure SDK tries authentication methods in this order:

1. **Environment Variables** - `AZURE_CLIENT_ID`, `AZURE_CLIENT_SECRET`, `AZURE_TENANT_ID`
2. **Workload Identity** - When running in AKS with workload identity enabled
3. **Managed Identity** - When running on Azure VMs or other Azure services
4. **Azure CLI** - Uses credentials from `az login`

> **Tip:** For local development, if you don't set `AZURE_CLIENT_ID`/`AZURE_CLIENT_SECRET`, the SDK will automatically fall back to your Azure CLI credentials.

## Environment Variables Reference

| Variable                | Required | Description                                         |
| ----------------------- | -------- | --------------------------------------------------- |
| `AZURE_SUBSCRIPTION_ID` | **Yes**  | Your Azure subscription ID                          |
| `AZURE_TENANT_ID`       | For SP   | Azure AD tenant ID (required for service principal) |
| `AZURE_CLIENT_ID`       | For SP   | Service principal application ID                    |
| `AZURE_CLIENT_SECRET`   | For SP   | Service principal secret                            |

## Decision Tree

```
Do you have Contributor role on the subscription?
├─ Yes → Are you doing local development only?
│        ├─ Yes → Use User Credentials (Azure CLI)
│        └─ No  → Use Service Principal
└─ No  → Ask admin for:
         ├─ Service Principal credentials, OR
         └─ Contributor role assignment for your user
```

## Next Steps

- [User Credentials Setup](./azure-user-credentials.md) - For local development (no admin needed)
- [Service Principal Setup](./azure-service-principal.md) - For production/shared environments
- [Azure Permissions](./azure-permissions.md) - Detailed permission requirements
- [Troubleshooting](./troubleshooting.md) - Common issues and solutions
