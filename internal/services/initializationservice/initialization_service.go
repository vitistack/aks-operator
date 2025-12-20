package initializationservice

import (
	"context"
	"fmt"
	"os"

	"github.com/vitistack/aks-operator/internal/consts"
	"github.com/vitistack/aks-operator/internal/managers"
	"github.com/vitistack/aks-operator/pkg/clients/azure"
	"github.com/vitistack/aks-operator/pkg/interfaces"
	"github.com/vitistack/common/pkg/loggers/vlog"
	"github.com/vitistack/common/pkg/operator/crdcheck"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// AzureServices holds all initialized Azure clients and managers
type AzureServices struct {
	ClientFactory      interfaces.AzureClientFactory
	VMSizeManager      interfaces.VMSizeManager
	AgentPoolManager   interfaces.AgentPoolManager
	AKSClusterManager  interfaces.AKSClusterManager
	MachineClassMapper interfaces.MachineClassMapper
}

// CheckPrerequisites verifies that required CRDs are installed before starting.
// It exits the process with code 1 if mandatory APIs are missing.
func CheckPrerequisites() {
	vlog.Info("Running prerequisite checks...")

	crdcheck.MustEnsureInstalled(context.TODO(),
		// your CRD plural
		crdcheck.Ref{Group: "vitistack.io", Version: "v1alpha1", Resource: "machines"},
		crdcheck.Ref{Group: "vitistack.io", Version: "v1alpha1", Resource: "kubernetesclusters"},
		crdcheck.Ref{Group: "vitistack.io", Version: "v1alpha1", Resource: "kubernetesproviders"},
		crdcheck.Ref{Group: "vitistack.io", Version: "v1alpha1", Resource: "machineproviders"},
		crdcheck.Ref{Group: "vitistack.io", Version: "v1alpha1", Resource: "networknamespaces"},
		crdcheck.Ref{Group: "vitistack.io", Version: "v1alpha1", Resource: "networkconfigurations"},
		crdcheck.Ref{Group: "vitistack.io", Version: "v1alpha1", Resource: "vitistacks"},
		crdcheck.Ref{Group: "vitistack.io", Version: "v1alpha1", Resource: "machineclasses"},
	)

	vlog.Info("✅ Prerequisite checks passed")
}

// CheckAzureEnvironment verifies that required Azure environment variables are set.
// Returns an error if any required variables are missing.
func CheckAzureEnvironment() error {
	vlog.Info("Checking Azure environment variables...")

	required := []string{
		consts.AZURE_SUBSCRIPTION_ID,
	}

	var missing []string
	for _, envVar := range required {
		if os.Getenv(envVar) == "" {
			missing = append(missing, envVar)
		}
	}

	if len(missing) > 0 {
		return fmt.Errorf("missing required Azure environment variables: %v", missing)
	}

	// Check for authentication - either service principal or managed identity
	// Support both AZURE_CLIENT_ID (preferred) and AZURE_OBJECT_ID (deprecated)
	clientID := os.Getenv(consts.AZURE_CLIENT_ID)
	if clientID == "" {
		clientID = os.Getenv(consts.AZURE_OBJECT_ID) // Fallback to deprecated
	}
	clientSecret := os.Getenv(consts.AZURE_CLIENT_SECRET)
	tenantID := os.Getenv(consts.AZURE_TENANT_ID)
	hasClientCredentials := clientID != "" && clientSecret != ""
	hasManagedIdentity := os.Getenv("AZURE_CLIENT_ID") != "" || os.Getenv("AZURE_FEDERATED_TOKEN_FILE") != ""

	if !hasClientCredentials && !hasManagedIdentity {
		vlog.Info("No explicit Azure credentials found, will use DefaultAzureCredential (managed identity, CLI, etc.)")
	} else if hasClientCredentials {
		if tenantID == "" {
			return fmt.Errorf("AZURE_TENANT_ID is required when using service principal authentication (AZURE_CLIENT_ID + AZURE_CLIENT_SECRET)")
		}
		vlog.Info("Using service principal authentication")
	} else {
		vlog.Info("Using managed identity or workload identity authentication")
	}

	vlog.Info("✅ Azure environment check passed",
		"subscriptionId", maskString(os.Getenv(consts.AZURE_SUBSCRIPTION_ID)),
	)
	return nil
}

// InitializeAzureServices creates and validates all Azure clients and managers.
// This should be called at startup to fail fast if Azure connectivity is not available.
func InitializeAzureServices(k8sClient client.Client) (*AzureServices, error) {
	vlog.Info("Initializing Azure services...")

	// Create the client factory
	factory, err := azure.NewClientFactory()
	if err != nil {
		return nil, fmt.Errorf("failed to create Azure client factory: %w", err)
	}

	// Create VM size client and manager
	vmSizeClient, err := factory.NewVMSizeClient()
	if err != nil {
		return nil, fmt.Errorf("failed to create VM size client: %w", err)
	}
	vmSizeManager := managers.NewVMSizeManager(vmSizeClient)

	// Create MachineClass mapper (uses K8s client to read MachineClass CRDs)
	machineClassMapper := managers.NewMachineClassMapper(k8sClient, vmSizeManager)

	// Create agent pool client and manager
	agentPoolClient, err := factory.NewAgentPoolClient()
	if err != nil {
		return nil, fmt.Errorf("failed to create agent pool client: %w", err)
	}
	agentPoolManager := managers.NewAgentPoolManager(agentPoolClient, vmSizeManager, machineClassMapper)

	// Create managed cluster client and manager
	managedClusterClient, err := factory.NewManagedClusterClient()
	if err != nil {
		return nil, fmt.Errorf("failed to create managed cluster client: %w", err)
	}
	aksClusterManager := managers.NewAKSClusterManager(managedClusterClient, agentPoolManager, machineClassMapper)

	vlog.Info("✅ Azure services initialized successfully")

	return &AzureServices{
		ClientFactory:      factory,
		VMSizeManager:      vmSizeManager,
		AgentPoolManager:   agentPoolManager,
		AKSClusterManager:  aksClusterManager,
		MachineClassMapper: machineClassMapper,
	}, nil
}

// ValidateAzureConnectivity performs a lightweight check to verify Azure connectivity.
// This is useful to fail fast at startup if Azure is not reachable.
func ValidateAzureConnectivity(ctx context.Context, services *AzureServices) error {
	vlog.Info("Validating Azure connectivity...")

	// Try to list VM sizes in a common region as a connectivity check
	testLocation := os.Getenv("AZURE_DEFAULT_LOCATION")
	if testLocation == "" {
		testLocation = "westeurope"
	}

	_, err := services.VMSizeManager.ListVMSizes(ctx, testLocation)
	if err != nil {
		return fmt.Errorf("azure connectivity check failed: %w", err)
	}

	vlog.Info("✅ Azure connectivity validated", " testLocation: ", testLocation)
	return nil
}

// maskString masks a string for logging, showing only first 4 and last 4 characters
func maskString(s string) string {
	if len(s) <= 8 {
		return "****"
	}
	return s[:4] + "****" + s[len(s)-4:]
}
