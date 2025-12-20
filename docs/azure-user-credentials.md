# Azure User Credentials (Azure CLI)

This guide explains how to use your personal Azure credentials for local development. This is the **easiest** method if you already have **Contributor** access to the subscription.

## Overview

| Requirement      | Details                                        |
| ---------------- | ---------------------------------------------- |
| Admin Required   | **No** - uses your existing Azure permissions  |
| Best For         | Local development, personal testing, debugging |
| Not Suitable For | Production, CI/CD, shared environments         |

## Prerequisites

- [Azure CLI](https://docs.microsoft.com/en-us/cli/azure/install-azure-cli) installed
- **Contributor** role on the Azure subscription (or resource group)

## Quick Setup

```bash
# Step 1: Login to Azure
az login

# Step 2: Set subscription ID
export AZURE_SUBSCRIPTION_ID=$(az account show --query id -o tsv)

# Step 3: Run the operator
make run
```

That's it! The operator will use your Azure CLI credentials automatically.

---

## Detailed Setup

### Step 1: Login to Azure

```bash
az login
```

This opens a browser for authentication. After successful login, you'll see your subscriptions listed.

### Step 2: Select the Correct Subscription

If you have multiple subscriptions:

```bash
# List all subscriptions
az account list -o table

# Set the active subscription
az account set --subscription "<subscription-name-or-id>"

# Verify
az account show --query "{name:name, id:id}" -o table
```

### Step 3: Configure Environment

You only need `AZURE_SUBSCRIPTION_ID`. **Do NOT set** the service principal variables.

**Option A: Environment Variable**

```bash
export AZURE_SUBSCRIPTION_ID=$(az account show --query id -o tsv)
```

**Option B: .env File**

```dotenv
AZURE_SUBSCRIPTION_ID=<your-subscription-id>

# Leave these empty or don't set them at all
# AZURE_TENANT_ID=
# AZURE_CLIENT_ID=
# AZURE_CLIENT_SECRET=
```

### Step 4: Run the Operator

```bash
make run
```

---

## Checking Your Permissions

### Do I Have Contributor Access?

#### Via Azure CLI

```bash
# Get your user ID
USER_ID=$(az ad signed-in-user show --query id -o tsv)

# List your role assignments
az role assignment list --assignee $USER_ID -o table
```

Look for **Contributor** or **Owner** in the `RoleDefinitionName` column.

#### Via Azure Portal

1. Go to **Subscriptions** > Select your subscription
2. Click **Access control (IAM)**
3. Click **Check access**
4. Search for your name
5. View your role assignments

You should see **Contributor** or **Owner** role.

### What If I Don't Have Contributor Access?

Options:

1. **Request Contributor role** - Ask your Azure administrator
2. **Use a Service Principal** - Ask admin to create one for you. See [Service Principal Guide](./azure-service-principal.md)

---

## How It Works

The Azure SDK's `DefaultAzureCredential` checks for authentication methods in order:

1. Environment variables (service principal) - **skipped if not set**
2. Workload Identity
3. Managed Identity
4. **Azure CLI credentials** ← Used when no other method is configured

When you don't set `AZURE_CLIENT_ID`/`AZURE_CLIENT_SECRET`/`AZURE_TENANT_ID`, the SDK automatically falls back to your `az login` session.

---

## Common Issues

### "AZURE_TENANT_ID not set" or Similar Errors

**Cause:** Partial environment variables are set. The SDK thinks you want service principal auth but it's incomplete.

**Solution:** Clear all service principal variables:

```bash
unset AZURE_CLIENT_ID
unset AZURE_CLIENT_SECRET
unset AZURE_TENANT_ID

# Only set subscription ID
export AZURE_SUBSCRIPTION_ID=$(az account show --query id -o tsv)
```

Or in your `.env` file, remove or comment out the service principal variables.

### Token Expired

**Cause:** Azure CLI tokens expire after some time.

**Solution:** Re-authenticate:

```bash
az login
```

### Wrong Subscription

**Cause:** Your CLI is pointing to a different subscription.

**Solution:**

```bash
# Check current subscription
az account show --query "{name:name, id:id}" -o table

# Change if needed
az account set --subscription "<correct-subscription>"
```

### "AuthorizationFailed" / 403

**Cause:** Your user doesn't have Contributor role.

**Solution:**

1. Check your role assignments (see above)
2. Request Contributor role from your Azure administrator
3. Or use a Service Principal instead

### Resource Group Not Found

**Cause:** The resource group in `spec.data.project` doesn't exist.

**Solution:** Create the resource group first:

```bash
az group create --name <project-name> --location <region>
```

---

## Limitations

| Good For             | Not Good For           |
| -------------------- | ---------------------- |
| Local development    | Production deployments |
| Quick testing        | CI/CD pipelines        |
| Personal projects    | Shared environments    |
| Debugging            | Running in containers  |
| Learning/exploration | Automated processes    |

## When to Switch to Service Principal

Switch to a [Service Principal](./azure-service-principal.md) when:

- Deploying to production
- Setting up CI/CD pipelines
- Running in containers or Kubernetes
- Sharing credentials with a team
- Need audit trail of automated actions

---

## Next Steps

- [Service Principal Setup](./azure-service-principal.md) - For production/CI/CD
- [Azure Permissions](./azure-permissions.md) - Understand required permissions
- [Troubleshooting](./troubleshooting.md) - More troubleshooting tips
