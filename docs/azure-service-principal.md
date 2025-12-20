# Azure Service Principal Setup

This guide explains how to create and configure an Azure Service Principal for the AKS Operator. This is the **recommended** method for production, CI/CD, and shared environments.

## Overview

| Scenario                                                 | Jump To                                 |
| -------------------------------------------------------- | --------------------------------------- |
| I'm an admin and need to create a service principal      | [Admin Setup](#for-administrators)      |
| I'm a user and need to use an existing service principal | [Non-Admin Setup](#for-non-admin-users) |
| I need to troubleshoot service principal issues          | [Troubleshooting](#troubleshooting)     |

## Prerequisites

- [Azure CLI](https://docs.microsoft.com/en-us/cli/azure/install-azure-cli) installed
- Logged in to Azure: `az login`

---

## For Administrators

This section is for Azure administrators who can create service principals and assign roles.

### Quick Setup

```bash
# Get subscription ID
SUBSCRIPTION_ID=$(az account show --query id -o tsv)

# Create service principal with Contributor role
az ad sp create-for-rbac \
  --name "aks-operator-sp" \
  --role Contributor \
  --scopes /subscriptions/$SUBSCRIPTION_ID
```

Output:

```json
{
  "appId": "xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx",
  "displayName": "aks-operator-sp",
  "password": "xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx",
  "tenant": "xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx"
}
```

**Share these with the user:**
| Output Field | Environment Variable | Description |
|--------------|---------------------|-------------|
| `appId` | `AZURE_CLIENT_ID` | Application (client) ID |
| `password` | `AZURE_CLIENT_SECRET` | Client secret (shown only once!) |
| `tenant` | `AZURE_TENANT_ID` | Azure AD tenant ID |
| (from account) | `AZURE_SUBSCRIPTION_ID` | Subscription ID |

### Detailed Admin Setup

#### Step 1: Get IDs

```bash
# Get subscription ID
az account show --query id -o tsv

# Get tenant ID
az account show --query tenantId -o tsv
```

#### Step 2: Create Service Principal

**Option A: With Contributor Role (Recommended)**

```bash
az ad sp create-for-rbac \
  --name "aks-operator-sp" \
  --role Contributor \
  --scopes /subscriptions/<SUBSCRIPTION_ID>
```

**Option B: With Least-Privilege Role**

For production, use the more restrictive AKS Contributor role:

```bash
az ad sp create-for-rbac \
  --name "aks-operator-sp" \
  --role "Azure Kubernetes Service Contributor" \
  --scopes /subscriptions/<SUBSCRIPTION_ID>
```

**Option C: Scoped to Resource Group**

For even more restrictive access:

```bash
az ad sp create-for-rbac \
  --name "aks-operator-sp" \
  --role Contributor \
  --scopes /subscriptions/<SUBSCRIPTION_ID>/resourceGroups/<RESOURCE_GROUP>
```

#### Step 3: Share Credentials Securely

Share the credentials with the user via secure channel (password manager, secrets vault):

- `appId` - This is the **Client ID** (`AZURE_CLIENT_ID`)
- `password` - This is the **Client Secret** (`AZURE_CLIENT_SECRET`)
- `tenant` - This is the **Tenant ID** (`AZURE_TENANT_ID`)
- Subscription ID - `AZURE_SUBSCRIPTION_ID`

### Assigning Role to Existing Service Principal

If a service principal already exists:

```bash
# Get the appId of existing service principal
az ad sp list --display-name "aks-operator-sp" --query "[0].appId" -o tsv

# Assign Contributor role on subscription
az role assignment create \
  --assignee <appId> \
  --role Contributor \
  --scope /subscriptions/<SUBSCRIPTION_ID>

# Or assign on specific resource group
az role assignment create \
  --assignee <appId> \
  --role Contributor \
  --scope /subscriptions/<SUBSCRIPTION_ID>/resourceGroups/<RESOURCE_GROUP>
```

### Creating New Secret for Existing Service Principal

```bash
# Create new secret (append to existing)
az ad sp credential reset \
  --id <appId> \
  --append \
  --display-name "aks-operator-secret" \
  --query password -o tsv
```

> **Warning:** Without `--append`, all existing secrets will be invalidated.

---

## For Non-Admin Users

This section is for users who need to use an existing service principal.

### If You Have Service Principal Credentials

Your administrator should have provided:

- `appId` (Client ID) - Use as `AZURE_CLIENT_ID`
- `password` (Client Secret) - Use as `AZURE_CLIENT_SECRET`
- `tenant` (Tenant ID) - Use as `AZURE_TENANT_ID`
- Subscription ID - Use as `AZURE_SUBSCRIPTION_ID`

Configure your `.env` file:

```dotenv
AZURE_SUBSCRIPTION_ID=<subscription-id>
AZURE_TENANT_ID=<tenant-id>
AZURE_CLIENT_ID=<client-id>
AZURE_CLIENT_SECRET=<client-secret>
```

Or export as environment variables:

```bash
export AZURE_SUBSCRIPTION_ID=<subscription-id>
export AZURE_TENANT_ID=<tenant-id>
export AZURE_CLIENT_ID=<client-id>
export AZURE_CLIENT_SECRET=<client-secret>
```

### If You Need to Request a Service Principal

Send this to your Azure administrator:

> **Request:** Please create a service principal for the AKS Operator
>
> **Required role:** Contributor (or Azure Kubernetes Service Contributor)
> **Scope:** Subscription `<your-subscription-id>` (or specific resource group)
>
> **Command:**
>
> ```bash
> az ad sp create-for-rbac \
>   --name "aks-operator-sp" \
>   --role Contributor \
>   --scopes /subscriptions/<SUBSCRIPTION_ID>
> ```

### If Service Principal Exists But You Need Credentials

#### Get Client ID

If you know the service principal name:

```bash
az ad sp list --display-name "aks-operator-sp" --query "[0].appId" -o tsv
```

Or in Azure Portal:

1. Go to **Microsoft Entra ID** > **App registrations**
2. Search for the service principal
3. Copy the **Application (client) ID**

#### Get Client Secret

You **cannot** retrieve an existing secret from Azure. Ask your administrator to:

1. Create a new secret for you, OR
2. Check if it's stored in a secrets vault

#### Request Role Assignment

If the service principal doesn't have Contributor role:

```bash
# Ask admin to run this
az role assignment create \
  --assignee <appId> \
  --role Contributor \
  --scope /subscriptions/<SUBSCRIPTION_ID>
```

### Alternative: Use User Credentials

If getting a service principal is difficult, you can use your own Azure CLI credentials for local development. See [User Credentials Guide](./azure-user-credentials.md).

---

## Verify Setup

Test that your service principal works:

```bash
# Set environment variables
export AZURE_SUBSCRIPTION_ID=<subscription-id>
export AZURE_TENANT_ID=<tenant-id>
export AZURE_CLIENT_ID=<client-id>
export AZURE_CLIENT_SECRET=<client-secret>

# Test login
az login --service-principal \
  --username $AZURE_CLIENT_ID \
  --password $AZURE_CLIENT_SECRET \
  --tenant $AZURE_TENANT_ID

# Test AKS access
az aks list --subscription $AZURE_SUBSCRIPTION_ID -o table

# Check role assignments
az role assignment list --assignee $AZURE_CLIENT_ID -o table
```

---

## Troubleshooting

### "ServicePrincipalNotFound"

**Cause:** Using Object ID instead of Client ID.

**Solution:** Use the `appId` (Application/Client ID), not the Object ID:

```bash
az ad sp list --display-name "aks-operator-sp" --query "[0].appId" -o tsv
```

### "AuthorizationFailed" / 403

**Cause:** Missing Contributor role.

**Solution:** Check role assignments:

```bash
az role assignment list --assignee <appId> -o table
```

If empty, ask admin to assign Contributor role.

### Operator Hangs on Startup

**Cause:** Missing `AZURE_TENANT_ID`.

**Solution:** Ensure all four environment variables are set:

```bash
env | grep AZURE_
```

### Secret Expired

**Cause:** Client secrets have expiration dates.

**Solution:** Ask admin to create a new secret:

```bash
az ad sp credential reset --id <appId> --append --query password -o tsv
```

---

## Security Best Practices

1. **Use short-lived secrets** - Set expiration to 6-12 months
2. **Store secrets securely** - Use Azure Key Vault or similar
3. **Rotate secrets regularly** - Replace before expiration
4. **Use least-privilege** - Prefer `Azure Kubernetes Service Contributor` over `Contributor`
5. **Scope to resource group** - Limit to specific resource groups when possible
6. **Monitor usage** - Enable Azure AD sign-in logs

## Next Steps

- [Azure Permissions](./azure-permissions.md) - Detailed permission requirements
- [User Credentials](./azure-user-credentials.md) - Alternative for local development
- [Troubleshooting](./troubleshooting.md) - More troubleshooting tips
