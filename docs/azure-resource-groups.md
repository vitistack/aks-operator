# Azure Resource Groups

This guide explains how Azure Resource Groups work with the AKS Operator.

## How Project Maps to Resource Group

When creating a `KubernetesCluster` resource, the `spec.data.project` field specifies the **Azure Resource Group name**. The AKS cluster will be created inside this resource group.

```yaml
apiVersion: vitistack.io/v1alpha1
kind: KubernetesCluster
metadata:
  name: my-cluster
  namespace: my-namespace
spec:
  data:
    project: my-resource-group # ← Azure Resource Group name
    region: norwayeast # ← Should match RG location
    # ... other fields
```

> **Important:** The resource group must exist in Azure before applying the `KubernetesCluster` manifest. The operator does **not** create resource groups automatically.

## Creating a Resource Group

### Using Azure CLI

```bash
az group create --name <project-name> --location <azure-region>

# Example
az group create --name my-project --location norwayeast
```

### Using Azure Portal

1. Go to **Resource groups**
2. Click **Create**
3. Enter the name and select a region
4. Click **Review + create**

## Common Azure Regions

| Region       | Location Code |
| ------------ | ------------- |
| Norway East  | `norwayeast`  |
| Norway West  | `norwaywest`  |
| West Europe  | `westeurope`  |
| North Europe | `northeurope` |
| UK South     | `uksouth`     |
| East US      | `eastus`      |
| West US 2    | `westus2`     |
| Central US   | `centralus`   |

List all available regions:

```bash
az account list-locations -o table
```

## Verifying Resource Group

Before applying a cluster manifest:

```bash
# Check if exists
az group show --name <project-name> --query name -o tsv

# List all resource groups
az group list -o table
```

## Permissions on Resource Groups

The identity (service principal or user) needs **Contributor** role on the resource group:

### Check Permissions

```bash
az role assignment list \
  --assignee <SERVICE_PRINCIPAL_OR_USER_ID> \
  --scope /subscriptions/<SUBSCRIPTION_ID>/resourceGroups/<RESOURCE_GROUP> \
  -o table
```

### Grant Permissions

```bash
az role assignment create \
  --assignee <SERVICE_PRINCIPAL_OR_USER_ID> \
  --role Contributor \
  --scope /subscriptions/<SUBSCRIPTION_ID>/resourceGroups/<RESOURCE_GROUP>
```

## Troubleshooting

### ResourceGroupNotFound

```
ERROR CODE: ResourceGroupNotFound
Resource group 'my-project' could not be found.
```

**Causes:**

1. Resource group doesn't exist
2. Service principal doesn't have access

**Solution:**

```bash
# Create if missing
az group create --name my-project --location norwayeast

# Or verify it exists
az group show --name my-project

# Check permissions
az role assignment list --assignee <CLIENT_ID> --scope /subscriptions/<SUB_ID>/resourceGroups/my-project -o table
```

## Related Documentation

- [Azure Permissions](./azure-permissions.md)
- [Troubleshooting](./troubleshooting.md)
