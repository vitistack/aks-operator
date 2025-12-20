# Azure Permissions

This guide details the Azure permissions required by the AKS Operator.

## Required Permissions

The AKS Operator performs the following Azure operations:

| Operation          | Azure API Action                                                               | Purpose                     |
| ------------------ | ------------------------------------------------------------------------------ | --------------------------- |
| Create AKS cluster | `Microsoft.ContainerService/managedClusters/write`                             | Provision new clusters      |
| Update AKS cluster | `Microsoft.ContainerService/managedClusters/write`                             | Modify cluster config       |
| Delete AKS cluster | `Microsoft.ContainerService/managedClusters/delete`                            | Remove clusters             |
| Get AKS cluster    | `Microsoft.ContainerService/managedClusters/read`                              | Check cluster status        |
| Get kubeconfig     | `Microsoft.ContainerService/managedClusters/listClusterAdminCredential/action` | Retrieve access credentials |
| Create agent pool  | `Microsoft.ContainerService/managedClusters/agentPools/write`                  | Add node pools              |
| Delete agent pool  | `Microsoft.ContainerService/managedClusters/agentPools/delete`                 | Remove node pools           |
| List agent pools   | `Microsoft.ContainerService/managedClusters/agentPools/read`                   | Check node pool status      |
| List VM sizes      | `Microsoft.Compute/locations/vmSizes/read`                                     | Validate VM size selection  |

## Built-in Roles

### Contributor (Recommended for Development)

The **Contributor** role includes all required permissions and is the simplest option:

```bash
az role assignment create \
  --assignee <SERVICE_PRINCIPAL_OR_USER_ID> \
  --role Contributor \
  --scope /subscriptions/<SUBSCRIPTION_ID>
```

### Azure Kubernetes Service Contributor (Recommended for Production)

For least-privilege access, use the **Azure Kubernetes Service Contributor** role:

```bash
az role assignment create \
  --assignee <SERVICE_PRINCIPAL_OR_USER_ID> \
  --role "Azure Kubernetes Service Contributor" \
  --scope /subscriptions/<SUBSCRIPTION_ID>
```

This role includes:

- All AKS management permissions
- Does NOT include permissions for other Azure resources

**Note:** You may also need **Reader** role for VM sizes:

```bash
az role assignment create \
  --assignee <SERVICE_PRINCIPAL_OR_USER_ID> \
  --role Reader \
  --scope /subscriptions/<SUBSCRIPTION_ID>
```

## Scope Options

Permissions can be assigned at different scopes:

### Subscription Level (Broader)

```bash
--scope /subscriptions/<SUBSCRIPTION_ID>
```

✅ Allows managing clusters in any resource group
✅ Simpler setup
⚠️ Broader access than necessary

### Resource Group Level (Recommended)

```bash
--scope /subscriptions/<SUBSCRIPTION_ID>/resourceGroups/<RESOURCE_GROUP>
```

✅ Limited to specific resource group
✅ Better security posture
⚠️ Need to assign per resource group

### Multiple Resource Groups

Assign role to each resource group:

```bash
for rg in rg1 rg2 rg3; do
  az role assignment create \
    --assignee <SERVICE_PRINCIPAL_ID> \
    --role Contributor \
    --scope /subscriptions/<SUBSCRIPTION_ID>/resourceGroups/$rg
done
```

## Checking Permissions

### For Service Principal

```bash
# List role assignments
az role assignment list --assignee <CLIENT_ID> -o table

# Check specific scope
az role assignment list \
  --assignee <CLIENT_ID> \
  --scope /subscriptions/<SUBSCRIPTION_ID> \
  -o table
```

### For User (Azure CLI credentials)

```bash
# Get your user ID
az ad signed-in-user show --query id -o tsv

# List your role assignments
az role assignment list \
  --assignee $(az ad signed-in-user show --query id -o tsv) \
  -o table
```

### Via Azure Portal

1. Go to **Subscriptions** → Select subscription
2. Click **Access control (IAM)**
3. Click **Check access**
4. Enter the service principal name or your email
5. View role assignments

## Creating Custom Role (Advanced)

For minimal permissions, create a custom role:

```json
{
  "Name": "AKS Operator Custom",
  "IsCustom": true,
  "Description": "Minimal permissions for AKS Operator",
  "Actions": [
    "Microsoft.ContainerService/managedClusters/read",
    "Microsoft.ContainerService/managedClusters/write",
    "Microsoft.ContainerService/managedClusters/delete",
    "Microsoft.ContainerService/managedClusters/listClusterAdminCredential/action",
    "Microsoft.ContainerService/managedClusters/listClusterUserCredential/action",
    "Microsoft.ContainerService/managedClusters/agentPools/read",
    "Microsoft.ContainerService/managedClusters/agentPools/write",
    "Microsoft.ContainerService/managedClusters/agentPools/delete",
    "Microsoft.Compute/locations/vmSizes/read",
    "Microsoft.Resources/subscriptions/resourceGroups/read"
  ],
  "NotActions": [],
  "AssignableScopes": ["/subscriptions/<SUBSCRIPTION_ID>"]
}
```

Create the role:

```bash
az role definition create --role-definition custom-role.json
```

Assign it:

```bash
az role assignment create \
  --assignee <SERVICE_PRINCIPAL_ID> \
  --role "AKS Operator Custom" \
  --scope /subscriptions/<SUBSCRIPTION_ID>
```

## For Administrators

### Granting Permissions

**Subscription-level Contributor:**

```bash
az role assignment create \
  --assignee <SERVICE_PRINCIPAL_ID> \
  --role Contributor \
  --scope /subscriptions/<SUBSCRIPTION_ID>
```

**Resource group-level Contributor:**

```bash
az role assignment create \
  --assignee <SERVICE_PRINCIPAL_ID> \
  --role Contributor \
  --scope /subscriptions/<SUBSCRIPTION_ID>/resourceGroups/<RESOURCE_GROUP>
```

**AKS-specific role (least privilege):**

```bash
az role assignment create \
  --assignee <SERVICE_PRINCIPAL_ID> \
  --role "Azure Kubernetes Service Contributor" \
  --scope /subscriptions/<SUBSCRIPTION_ID>
```

### Revoking Permissions

```bash
az role assignment delete \
  --assignee <SERVICE_PRINCIPAL_ID> \
  --role Contributor \
  --scope /subscriptions/<SUBSCRIPTION_ID>
```

### Listing All Assignments

```bash
# All assignments for a service principal
az role assignment list --assignee <SERVICE_PRINCIPAL_ID> --all -o table

# All assignments in a subscription
az role assignment list --scope /subscriptions/<SUBSCRIPTION_ID> -o table
```

## Troubleshooting

### AuthorizationFailed / 403 Forbidden

```
The client 'xxx' does not have authorization to perform action 'Microsoft.ContainerService/managedClusters/write'
```

**Cause:** Missing required role assignment.

**Solution:**

1. Check current assignments:
   ```bash
   az role assignment list --assignee <CLIENT_ID> -o table
   ```
2. Assign Contributor role (or request from admin):
   ```bash
   az role assignment create \
     --assignee <CLIENT_ID> \
     --role Contributor \
     --scope /subscriptions/<SUBSCRIPTION_ID>
   ```

### Cannot Assign Roles

```
The client 'xxx' does not have authorization to perform action 'Microsoft.Authorization/roleAssignments/write'
```

**Cause:** You don't have **Owner** or **User Access Administrator** role.

**Solution:** Ask an Azure administrator to assign the role for you.

### Role Assignment Not Taking Effect

Azure role assignments can take up to 5 minutes to propagate.

**Solution:**

1. Wait a few minutes
2. Sign out and back in:
   ```bash
   az logout
   az login
   ```
3. For service principals, restart the operator

## Related Documentation

- [Azure Authentication Overview](./azure-authentication.md)
- [Service Principal Setup](./azure-service-principal.md)
- [User Credentials Setup](./azure-user-credentials.md)
- [Troubleshooting](./troubleshooting.md)
