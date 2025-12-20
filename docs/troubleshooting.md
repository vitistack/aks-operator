# Troubleshooting

Common issues and solutions for the AKS Operator.

## Authentication Errors

### AuthorizationFailed / 403 Forbidden

```
The client 'xxx' with object id 'xxx' does not have authorization to perform action 'Microsoft.ContainerService/managedClusters/write' over scope '...'
```

**Cause:** The identity (service principal or user) doesn't have the required permissions.

**Solution:**

1. Check current role assignments:

   ```bash
   # For service principal
   az role assignment list --assignee <CLIENT_ID> -o table

   # For user (Azure CLI auth)
   az role assignment list --assignee $(az ad signed-in-user show --query id -o tsv) -o table
   ```

2. If missing, assign Contributor role (requires admin):
   ```bash
   az role assignment create \
     --assignee <CLIENT_ID_OR_USER_ID> \
     --role Contributor \
     --scope /subscriptions/<SUBSCRIPTION_ID>
   ```

---

### ServicePrincipalNotFound

```
ServicePrincipalNotFound: Service principal 'xxx' not found in Azure AD
```

**Cause:** Using the **Object ID** instead of the **Client ID (Application ID)**.

**Solution:**

Use the Application (client) ID from:

- Azure CLI: `az ad sp list --display-name "aks-operator-sp" --query "[0].appId" -o tsv`
- Azure Portal: Microsoft Entra ID → App registrations → Application (client) ID

**Common mistake:**

- ❌ Object ID: `a90a2452-ff95-4ce7-b5f0-d017d639e463`
- ✅ Client ID (appId): `8eecf6da-e926-49a5-a47b-ff05722cb862`

---

### Operator Hangs During Initialization

**Cause:** Missing `AZURE_TENANT_ID` when using service principal authentication.

Without the tenant ID, the Azure SDK attempts managed identity authentication, which times out when running locally.

**Solution:**

Ensure `AZURE_TENANT_ID` is set:

```bash
# Get tenant ID
az account show --query tenantId -o tsv

# Add to .env
AZURE_TENANT_ID=<tenant-id>
```

**Alternative:** If using Azure CLI credentials (not service principal), don't set `AZURE_CLIENT_ID` or `AZURE_CLIENT_SECRET` at all.

---

### Token Expired / Invalid Credentials

```
AADSTS7000215: Invalid client secret provided
```

**Cause:** The client secret has expired or is incorrect.

**Solution:**

Create a new client secret:

```bash
az ad sp credential reset \
  --id <CLIENT_ID> \
  --append \
  --display-name "new-secret" \
  --query password -o tsv
```

Update `AZURE_CLIENT_SECRET` in your `.env` file.

---

## Resource Errors

### ResourceGroupNotFound

```
ERROR CODE: ResourceGroupNotFound
Resource group 'my-project' could not be found.
```

**Cause:** The resource group specified in `spec.data.project` doesn't exist.

**Solution:**

Create the resource group:

```bash
az group create --name my-project --location norwayeast
```

Or verify it exists:

```bash
az group show --name my-project
```

---

### ResourceNotFound (Cluster)

```
ERROR CODE: ResourceNotFound
The resource 'Microsoft.ContainerService/managedClusters/my-cluster' was not found.
```

**Cause:** The AKS cluster doesn't exist in Azure (possibly deleted manually).

**Solution:**

If the Kubernetes CRD object exists but the Azure cluster doesn't:

1. The operator will recreate it on next reconciliation, OR
2. Delete the CRD object to clean up

---

## Cluster Lifecycle Issues

### Cluster Deleted in Kubernetes but Still Exists in Azure

**Cause:** The delete operation failed (usually due to permissions) and the finalizer is blocking Kubernetes from removing the CRD object.

**Solution:**

1. Fix the permissions issue (see AuthorizationFailed above)
2. The operator will retry and complete deletion

**OR** if you want to delete manually:

1. Delete the AKS cluster in Azure Portal
2. Remove the finalizer from the CRD:
   ```bash
   kubectl patch kubernetescluster <NAME> -n <NAMESPACE> --type=json \
     -p='[{"op": "remove", "path": "/metadata/finalizers"}]'
   ```

---

### CRD Object Stuck in Terminating State

**Cause:** The finalizer is waiting for Azure deletion to complete, but deletion is failing.

**Solution:**

1. Check operator logs:

   ```bash
   kubectl logs -n <OPERATOR_NAMESPACE> deployment/aks-operator-controller-manager
   ```

2. Look for the actual error (usually permissions)

3. Fix the underlying issue

4. **OR** force removal:
   ```bash
   kubectl patch kubernetescluster <NAME> -n <NAMESPACE> --type=json \
     -p='[{"op": "remove", "path": "/metadata/finalizers"}]'
   ```

---

### Cluster Creation Taking Too Long

AKS cluster creation typically takes 5-15 minutes. The operator logs progress every 60 seconds.

**Check progress:**

```bash
# Operator logs
kubectl logs -n <NAMESPACE> deployment/aks-operator-controller-manager -f

# Azure CLI
az aks show --name <CLUSTER_NAME> --resource-group <RESOURCE_GROUP> --query provisioningState
```

---

## State Secret Issues

### Inconsistent State in Secret

Example: `aks_provision_state: Failed` but `cluster_provisioning: true`

**Cause:** Older versions didn't properly reset state on failure.

**Solution:** The latest code fixes this. After updating the operator, delete the secret to reset state:

```bash
kubectl delete secret <CLUSTER_ID> -n <NAMESPACE>
```

The operator will recreate it with correct state.

---

### Secret Not Updating

**Cause:** Secret update failed or operator crashed during update.

**Solution:**

1. Check operator logs for errors
2. Trigger reconciliation by updating the CRD:
   ```bash
   kubectl annotate kubernetescluster <NAME> -n <NAMESPACE> reconcile=$(date +%s)
   ```

---

## Connectivity Issues

### Cannot Connect to Azure

```
dial tcp: lookup management.azure.com: no such host
```

**Cause:** DNS resolution or network connectivity issue.

**Solution:**

1. Check DNS resolution:

   ```bash
   nslookup management.azure.com
   ```

2. Check network connectivity:

   ```bash
   curl -I https://management.azure.com
   ```

3. If behind a proxy, configure proxy settings:
   ```bash
   export HTTP_PROXY=http://proxy:port
   export HTTPS_PROXY=http://proxy:port
   ```

---

### Timeouts

```
context deadline exceeded
```

**Cause:** Network issues or Azure API throttling.

**Solution:**

1. Check Azure service health: https://status.azure.com
2. Retry after a few minutes
3. Check if you're hitting API rate limits

---

## Verification Commands

### Test Azure Credentials

```bash
# Service principal
az login --service-principal \
  --username $AZURE_CLIENT_ID \
  --password $AZURE_CLIENT_SECRET \
  --tenant $AZURE_TENANT_ID

# User credentials
az login

# Test AKS access
az aks list --subscription $AZURE_SUBSCRIPTION_ID -o table
```

### Check Role Assignments

```bash
# Service principal
az role assignment list --assignee $AZURE_CLIENT_ID -o table

# User
az role assignment list --assignee $(az ad signed-in-user show --query id -o tsv) -o table
```

### Check Operator Health

```bash
# Logs
kubectl logs -n <NAMESPACE> deployment/aks-operator-controller-manager

# Events
kubectl get events -n <NAMESPACE> --sort-by='.lastTimestamp'

# CRD status
kubectl get kubernetesclusters -A
kubectl describe kubernetescluster <NAME> -n <NAMESPACE>
```

---

## Related Documentation

- [Azure Authentication Overview](./azure-authentication.md)
- [Azure Permissions](./azure-permissions.md)
- [Service Principal Setup](./azure-service-principal.md)
- [User Credentials Setup](./azure-user-credentials.md)
