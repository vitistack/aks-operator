# Azure Workload Identity & Managed Identity

This guide explains how to use Workload Identity or Managed Identity to authenticate the AKS Operator when running in Azure.

## Overview

When running the operator in Azure (on AKS or Azure VMs), you can use managed identities instead of storing credentials in environment variables or secrets.

| Method                | Use Case                                        |
| --------------------- | ----------------------------------------------- |
| **Workload Identity** | Running in AKS (recommended)                    |
| **Managed Identity**  | Running on Azure VMs, Container Instances, etc. |

## Workload Identity (AKS)

Workload Identity allows Kubernetes pods to authenticate to Azure using Azure AD without managing secrets.

### Prerequisites

1. AKS cluster with OIDC issuer and workload identity enabled
2. Azure AD application or managed identity
3. Federated credentials configured

### Step 1: Enable Workload Identity on AKS

```bash
# Enable OIDC issuer
az aks update \
  --resource-group <RESOURCE_GROUP> \
  --name <AKS_CLUSTER> \
  --enable-oidc-issuer \
  --enable-workload-identity
```

### Step 2: Create Managed Identity

```bash
# Create managed identity
az identity create \
  --name aks-operator-identity \
  --resource-group <RESOURCE_GROUP> \
  --location <LOCATION>

# Get the client ID
az identity show \
  --name aks-operator-identity \
  --resource-group <RESOURCE_GROUP> \
  --query clientId -o tsv
```

### Step 3: Assign Permissions

```bash
IDENTITY_CLIENT_ID=$(az identity show --name aks-operator-identity --resource-group <RESOURCE_GROUP> --query clientId -o tsv)

az role assignment create \
  --assignee $IDENTITY_CLIENT_ID \
  --role Contributor \
  --scope /subscriptions/<SUBSCRIPTION_ID>
```

### Step 4: Create Federated Credential

```bash
# Get AKS OIDC issuer URL
AKS_OIDC_ISSUER=$(az aks show --name <AKS_CLUSTER> --resource-group <RESOURCE_GROUP> --query "oidcIssuerProfile.issuerUrl" -o tsv)

# Create federated credential
az identity federated-credential create \
  --name aks-operator-federated \
  --identity-name aks-operator-identity \
  --resource-group <RESOURCE_GROUP> \
  --issuer $AKS_OIDC_ISSUER \
  --subject system:serviceaccount:<NAMESPACE>:aks-operator-controller-manager \
  --audience api://AzureADTokenExchange
```

### Step 5: Configure Service Account

Add annotations to the operator's service account:

```yaml
apiVersion: v1
kind: ServiceAccount
metadata:
  name: aks-operator-controller-manager
  namespace: <NAMESPACE>
  annotations:
    azure.workload.identity/client-id: <MANAGED_IDENTITY_CLIENT_ID>
```

### Step 6: Configure Pod

Add label to pod spec (in deployment):

```yaml
spec:
  template:
    metadata:
      labels:
        azure.workload.identity/use: "true"
```

### Step 7: Set Subscription ID

The operator still needs to know which subscription to use:

```yaml
env:
  - name: AZURE_SUBSCRIPTION_ID
    value: "<SUBSCRIPTION_ID>"
```

Or via ConfigMap/Secret.

---

## Managed Identity (Azure VMs)

When running on Azure VMs, Azure Container Instances, or other Azure compute, you can use the VM's managed identity.

### Step 1: Enable System-Assigned Identity

```bash
# For a VM
az vm identity assign \
  --resource-group <RESOURCE_GROUP> \
  --name <VM_NAME>
```

### Step 2: Assign Permissions

```bash
# Get the principal ID
PRINCIPAL_ID=$(az vm show --resource-group <RESOURCE_GROUP> --name <VM_NAME> --query identity.principalId -o tsv)

# Assign Contributor role
az role assignment create \
  --assignee $PRINCIPAL_ID \
  --role Contributor \
  --scope /subscriptions/<SUBSCRIPTION_ID>
```

### Step 3: Configure Operator

Only set the subscription ID - no client credentials needed:

```dotenv
AZURE_SUBSCRIPTION_ID=<subscription-id>
```

The Azure SDK will automatically use the VM's managed identity.

---

## How DefaultAzureCredential Works

The Azure SDK's `DefaultAzureCredential` tries authentication in this order:

1. **Environment Variables** - `AZURE_CLIENT_ID`, `AZURE_CLIENT_SECRET`, `AZURE_TENANT_ID`
2. **Workload Identity** - When running in AKS with workload identity
3. **Managed Identity** - When running on Azure compute
4. **Azure CLI** - Local development

When using workload identity or managed identity, do NOT set `AZURE_CLIENT_ID`, `AZURE_CLIENT_SECRET`, or `AZURE_TENANT_ID` - this allows the SDK to use the managed identity.

---

## Troubleshooting

### "DefaultAzureCredential failed to retrieve a token"

**Possible causes:**

1. Workload identity not properly configured
2. Missing federated credential
3. Service account not annotated correctly

**Check:**

```bash
# Check if workload identity is enabled
az aks show --name <AKS_CLUSTER> --resource-group <RG> --query "securityProfile.workloadIdentity"

# Check OIDC issuer
az aks show --name <AKS_CLUSTER> --resource-group <RG> --query "oidcIssuerProfile"

# Check federated credentials
az identity federated-credential list --identity-name <IDENTITY> --resource-group <RG>
```

### "AuthorizationFailed"

The managed identity doesn't have the required permissions.

**Solution:**

```bash
az role assignment create \
  --assignee <MANAGED_IDENTITY_CLIENT_ID> \
  --role Contributor \
  --scope /subscriptions/<SUBSCRIPTION_ID>
```

### Works Locally but Not in Cluster

**Cause:** Locally you have Azure CLI credentials, but in-cluster needs workload identity.

**Solution:** Complete the workload identity setup (Steps 1-7 above).

---

## Related Documentation

- [Azure Workload Identity Documentation](https://learn.microsoft.com/en-us/azure/aks/workload-identity-overview)
- [Azure Managed Identity Documentation](https://learn.microsoft.com/en-us/azure/active-directory/managed-identities-azure-resources/overview)
- [Azure Authentication Overview](./azure-authentication.md)
- [Azure Permissions](./azure-permissions.md)
