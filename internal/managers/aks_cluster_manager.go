/*
Copyright 2025.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package managers

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/containerservice/armcontainerservice/v6"
	"github.com/vitistack/aks-operator/internal/consts"
	"github.com/vitistack/aks-operator/pkg/interfaces"
	"github.com/vitistack/common/pkg/loggers/vlog"
	vitistackv1alpha1 "github.com/vitistack/common/pkg/v1alpha1"
)

// AKSClusterManagerImpl implements the AKSClusterManager interface
type AKSClusterManagerImpl struct {
	managedClusterClient interfaces.ManagedClusterClient
	agentPoolManager     interfaces.AgentPoolManager
	machineClassMapper   interfaces.MachineClassMapper
}

// NewAKSClusterManager creates a new AKSClusterManager
func NewAKSClusterManager(managedClusterClient interfaces.ManagedClusterClient, agentPoolManager interfaces.AgentPoolManager, machineClassMapper interfaces.MachineClassMapper) *AKSClusterManagerImpl {
	return &AKSClusterManagerImpl{
		managedClusterClient: managedClusterClient,
		agentPoolManager:     agentPoolManager,
		machineClassMapper:   machineClassMapper,
	}
}

// CreateCluster creates an AKS cluster from KubernetesCluster spec
func (m *AKSClusterManagerImpl) CreateCluster(ctx context.Context, kubernetesCluster *vitistackv1alpha1.KubernetesCluster) (*armcontainerservice.ManagedCluster, error) {
	vlog.Info("Creating AKS cluster", "cluster", kubernetesCluster.Name, "namespace", kubernetesCluster.Namespace, "clusterid", kubernetesCluster.Spec.Cluster.ClusterId)

	resourceGroupName := m.getResourceGroupName(kubernetesCluster)
	clusterName := m.getClusterName(kubernetesCluster)
	location := kubernetesCluster.Spec.Cluster.Region

	managedCluster := m.buildManagedCluster(ctx, kubernetesCluster, location)

	result, err := m.managedClusterClient.CreateOrUpdate(ctx, resourceGroupName, clusterName, managedCluster)
	if err != nil {
		return nil, fmt.Errorf("failed to create AKS cluster: %w", err)
	}

	vlog.Info("AKS cluster created successfully", "cluster", clusterName, "id", *result.ID, "clusterid", kubernetesCluster.Spec.Cluster.ClusterId)
	return result, nil
}

// UpdateCluster updates an existing AKS cluster
func (m *AKSClusterManagerImpl) UpdateCluster(ctx context.Context, kubernetesCluster *vitistackv1alpha1.KubernetesCluster) (*armcontainerservice.ManagedCluster, error) {
	vlog.Info("Updating AKS cluster", "cluster", kubernetesCluster.Name, "namespace", kubernetesCluster.Namespace, "clusterid", kubernetesCluster.Spec.Cluster.ClusterId)

	resourceGroupName := m.getResourceGroupName(kubernetesCluster)
	clusterName := m.getClusterName(kubernetesCluster)
	location := kubernetesCluster.Spec.Cluster.Region

	managedCluster := m.buildManagedCluster(ctx, kubernetesCluster, location)

	result, err := m.managedClusterClient.CreateOrUpdate(ctx, resourceGroupName, clusterName, managedCluster)
	if err != nil {
		return nil, fmt.Errorf("failed to update AKS cluster: %w", err)
	}

	vlog.Info("AKS cluster updated successfully", "cluster", clusterName, "id", *result.ID, "clusterid", kubernetesCluster.Spec.Cluster.ClusterId)
	return result, nil
}

// DeleteCluster deletes an AKS cluster
func (m *AKSClusterManagerImpl) DeleteCluster(ctx context.Context, kubernetesCluster *vitistackv1alpha1.KubernetesCluster) error {
	vlog.Info("Deleting AKS cluster", "cluster", kubernetesCluster.Name, "namespace", kubernetesCluster.Namespace, "clusterid", kubernetesCluster.Spec.Cluster.ClusterId)

	resourceGroupName := m.getResourceGroupName(kubernetesCluster)
	clusterName := m.getClusterName(kubernetesCluster)

	err := m.managedClusterClient.Delete(ctx, resourceGroupName, clusterName)
	if err != nil {
		// Treat "not found" errors as successful deletion - the resource doesn't exist
		// This handles cases where:
		// - The resource group was deleted manually
		// - The AKS cluster was deleted manually
		// - The cluster was never created successfully
		if strings.Contains(err.Error(), "ResourceGroupNotFound") ||
			strings.Contains(err.Error(), "ResourceNotFound") ||
			strings.Contains(err.Error(), "not found") ||
			strings.Contains(err.Error(), "404") {
			vlog.Info("AKS cluster or resource group not found in Azure, treating as already deleted",
				"cluster", clusterName,
				"resourceGroup", resourceGroupName,
				"clusterid", kubernetesCluster.Spec.Cluster.ClusterId)
			return nil
		}
		return fmt.Errorf("failed to delete AKS cluster: %w", err)
	}

	vlog.Info("AKS cluster deleted successfully", "cluster", clusterName, "clusterid", kubernetesCluster.Spec.Cluster.ClusterId)
	return nil
}

// GetCluster retrieves an AKS cluster
func (m *AKSClusterManagerImpl) GetCluster(ctx context.Context, kubernetesCluster *vitistackv1alpha1.KubernetesCluster) (*armcontainerservice.ManagedCluster, error) {
	resourceGroupName := m.getResourceGroupName(kubernetesCluster)
	clusterName := m.getClusterName(kubernetesCluster)

	result, err := m.managedClusterClient.Get(ctx, resourceGroupName, clusterName)
	if err != nil {
		return nil, fmt.Errorf("failed to get AKS cluster: %w", err)
	}

	return result, nil
}

// GetClusterCredentials retrieves the admin credentials for an AKS cluster
func (m *AKSClusterManagerImpl) GetClusterCredentials(ctx context.Context, kubernetesCluster *vitistackv1alpha1.KubernetesCluster) (*armcontainerservice.CredentialResults, error) {
	resourceGroupName := m.getResourceGroupName(kubernetesCluster)
	clusterName := m.getClusterName(kubernetesCluster)

	result, err := m.managedClusterClient.ListClusterAdminCredentials(ctx, resourceGroupName, clusterName)
	if err != nil {
		return nil, fmt.Errorf("failed to get AKS cluster credentials: %w", err)
	}

	return result, nil
}

// ReconcileCluster reconciles the cluster state
func (m *AKSClusterManagerImpl) ReconcileCluster(ctx context.Context, kubernetesCluster *vitistackv1alpha1.KubernetesCluster) (*armcontainerservice.ManagedCluster, error) {
	existingCluster, err := m.GetCluster(ctx, kubernetesCluster)
	if err != nil {
		if strings.Contains(err.Error(), "not found") || strings.Contains(err.Error(), "ResourceNotFound") {
			vlog.Info("🆕 Creating new AKS cluster", "cluster", kubernetesCluster.Name, "clusterid", kubernetesCluster.Spec.Cluster.ClusterId)
			return m.CreateCluster(ctx, kubernetesCluster)
		}
		return nil, err
	}

	// Check if Azure is currently processing an operation on this cluster
	// If so, skip the update and return the current state (no logging - nothing changed)
	if existingCluster.Properties != nil && existingCluster.Properties.ProvisioningState != nil {
		provisioningState := *existingCluster.Properties.ProvisioningState
		if provisioningState == "Creating" || provisioningState == "Updating" || provisioningState == "Deleting" {
			// Only log once when we first detect an operation in progress
			return existingCluster, nil
		}
	}

	// Check if actual changes are needed before triggering an update
	if !m.needsUpdate(ctx, kubernetesCluster, existingCluster) {
		// No logging - nothing changed, silence is golden
		return existingCluster, nil
	}

	vlog.Info("🔄 Updating AKS cluster", "cluster", kubernetesCluster.Name, "clusterid", kubernetesCluster.Spec.Cluster.ClusterId)
	result, err := m.UpdateCluster(ctx, kubernetesCluster)
	if err != nil {
		// Handle 409 Conflict - Azure is processing an operation
		if strings.Contains(err.Error(), "409") ||
			strings.Contains(err.Error(), "Conflict") ||
			strings.Contains(err.Error(), "OperationNotAllowed") ||
			strings.Contains(err.Error(), "in progress") {
			vlog.Info("AKS cluster update conflict (operation in progress), will retry later",
				"cluster", kubernetesCluster.Name,
				"clusterid", kubernetesCluster.Spec.Cluster.ClusterId)
			// Return the existing cluster state - don't treat as error
			return existingCluster, nil
		}
		return nil, err
	}

	if m.agentPoolManager != nil {
		if err := m.agentPoolManager.ReconcileAgentPools(ctx, kubernetesCluster); err != nil {
			vlog.Error(err, "failed to reconcile agent pools", "clusterid", kubernetesCluster.Spec.Cluster.ClusterId)
			return result, fmt.Errorf("failed to reconcile agent pools: %w", err)
		}
	}

	vlog.Info("AKS cluster reconciled successfully", "cluster", kubernetesCluster.Name, "provisioningState", *existingCluster.Properties.ProvisioningState, "clusterid", kubernetesCluster.Spec.Cluster.ClusterId)
	return result, nil
}

func (m *AKSClusterManagerImpl) buildManagedCluster(ctx context.Context, kubernetesCluster *vitistackv1alpha1.KubernetesCluster, location string) armcontainerservice.ManagedCluster {
	kubernetesVersion := m.getKubernetesVersion(kubernetesCluster)
	dnsPrefix := m.getDNSPrefix(kubernetesCluster)
	agentPoolProfiles := m.buildInitialAgentPoolProfiles(ctx, kubernetesCluster)

	managedCluster := armcontainerservice.ManagedCluster{
		Location: to.Ptr(location),
		Tags:     m.buildTags(kubernetesCluster),
		Identity: &armcontainerservice.ManagedClusterIdentity{
			Type: to.Ptr(armcontainerservice.ResourceIdentityTypeSystemAssigned),
		},
		Properties: &armcontainerservice.ManagedClusterProperties{
			DNSPrefix:         to.Ptr(dnsPrefix),
			KubernetesVersion: to.Ptr(kubernetesVersion),
			AgentPoolProfiles: agentPoolProfiles,
			EnableRBAC:        to.Ptr(true),
			NetworkProfile: &armcontainerservice.NetworkProfile{
				NetworkPlugin: to.Ptr(armcontainerservice.NetworkPluginAzure),
				NetworkPolicy: to.Ptr(armcontainerservice.NetworkPolicyAzure),
			},
		},
	}

	// Note: We use SystemAssigned managed identity for the AKS cluster itself.
	// The AZURE_CLIENT_ID/AZURE_CLIENT_SECRET/AZURE_TENANT_ID env vars are used
	// by the operator to authenticate with Azure API (via DefaultAzureCredential),
	// NOT for the AKS cluster's identity.

	return managedCluster
}

func (m *AKSClusterManagerImpl) buildInitialAgentPoolProfiles(
	ctx context.Context,
	kubernetesCluster *vitistackv1alpha1.KubernetesCluster,
) []*armcontainerservice.ManagedClusterAgentPoolProfile {
	nodePools := kubernetesCluster.Spec.Topology.Workers.NodePools
	profiles := make([]*armcontainerservice.ManagedClusterAgentPoolProfile, 0, len(nodePools)+1)
	location := kubernetesCluster.Spec.Cluster.Region

	controlPlane := kubernetesCluster.Spec.Topology.ControlPlane
	vmSize := m.resolveVMSize(ctx, controlPlane.MachineClass, location, interfaces.VMPurposeGeneralPurpose)

	systemPool := &armcontainerservice.ManagedClusterAgentPoolProfile{
		Name:              to.Ptr("system"),
		Count:             to.Ptr(safeIntToInt32(controlPlane.Replicas)),
		VMSize:            to.Ptr(vmSize),
		Mode:              to.Ptr(armcontainerservice.AgentPoolModeSystem),
		OSType:            to.Ptr(armcontainerservice.OSTypeLinux),
		Type:              to.Ptr(armcontainerservice.AgentPoolTypeVirtualMachineScaleSets),
		EnableAutoScaling: to.Ptr(false),
	}
	profiles = append(profiles, systemPool)

	for _, nodePool := range kubernetesCluster.Spec.Topology.Workers.NodePools {
		pool := m.buildAgentPoolProfile(ctx, nodePool, location)
		profiles = append(profiles, pool)
	}

	return profiles
}

func (m *AKSClusterManagerImpl) buildAgentPoolProfile(ctx context.Context, nodePool vitistackv1alpha1.KubernetesClusterNodePool, location string) *armcontainerservice.ManagedClusterAgentPoolProfile {
	// Determine preferred purpose based on node pool hints
	purpose := m.determineNodePoolPurpose(nodePool)
	vmSize := m.resolveVMSize(ctx, nodePool.MachineClass, location, purpose)

	profile := &armcontainerservice.ManagedClusterAgentPoolProfile{
		Name:   to.Ptr(nodePool.Name),
		Count:  to.Ptr(safeIntToInt32(nodePool.Replicas)),
		VMSize: to.Ptr(vmSize),
		Mode:   to.Ptr(armcontainerservice.AgentPoolModeUser),
		OSType: to.Ptr(armcontainerservice.OSTypeLinux),
		Type:   to.Ptr(armcontainerservice.AgentPoolTypeVirtualMachineScaleSets),
	}

	if nodePool.Autoscaling.Enabled {
		profile.EnableAutoScaling = to.Ptr(true)
		profile.MinCount = to.Ptr(safeIntToInt32(nodePool.Autoscaling.MinReplicas))
		profile.MaxCount = to.Ptr(safeIntToInt32(nodePool.Autoscaling.MaxReplicas))
	} else {
		profile.EnableAutoScaling = to.Ptr(false)
	}

	if len(nodePool.Taint) > 0 {
		taints := make([]*string, 0, len(nodePool.Taint))
		for _, taint := range nodePool.Taint {
			taintStr := fmt.Sprintf("%s=%s:%s", taint.Key, taint.Value, taint.Effect)
			taints = append(taints, to.Ptr(taintStr))
		}
		profile.NodeTaints = taints
	}

	if len(nodePool.Metadata.Labels) > 0 {
		profile.NodeLabels = make(map[string]*string)
		for k, v := range nodePool.Metadata.Labels {
			profile.NodeLabels[k] = to.Ptr(v)
		}
	}

	return profile
}

func (m *AKSClusterManagerImpl) buildTags(kubernetesCluster *vitistackv1alpha1.KubernetesCluster) map[string]*string {
	tags := map[string]*string{
		"vitistack-cluster-id": to.Ptr(kubernetesCluster.Spec.Cluster.ClusterId),
		"vitistack-name":       to.Ptr(kubernetesCluster.Name),
		"vitistack-namespace":  to.Ptr(kubernetesCluster.Namespace),
		"environment":          to.Ptr(kubernetesCluster.Spec.Cluster.Environment),
		"managed-by":           to.Ptr("vitistack-aks-operator"),
	}

	if kubernetesCluster.Spec.Cluster.Project != "" {
		tags["project"] = to.Ptr(kubernetesCluster.Spec.Cluster.Project)
	}
	if kubernetesCluster.Spec.Cluster.Workspace != "" {
		tags["workspace"] = to.Ptr(kubernetesCluster.Spec.Cluster.Workspace)
	}

	return tags
}

func (m *AKSClusterManagerImpl) getResourceGroupName(kubernetesCluster *vitistackv1alpha1.KubernetesCluster) string {
	if kubernetesCluster.Spec.Cluster.Project != "" {
		return kubernetesCluster.Spec.Cluster.Project
	}
	return kubernetesCluster.Namespace
}

func (m *AKSClusterManagerImpl) getClusterName(kubernetesCluster *vitistackv1alpha1.KubernetesCluster) string {
	return kubernetesCluster.Spec.Cluster.ClusterId
}

func (m *AKSClusterManagerImpl) getDNSPrefix(kubernetesCluster *vitistackv1alpha1.KubernetesCluster) string {
	return kubernetesCluster.Spec.Cluster.ClusterId
}

func (m *AKSClusterManagerImpl) getKubernetesVersion(kubernetesCluster *vitistackv1alpha1.KubernetesCluster) string {
	if kubernetesCluster.Spec.Topology.Version != "" {
		return kubernetesCluster.Spec.Topology.Version
	}
	if kubernetesCluster.Spec.Topology.ControlPlane.Version != "" {
		return kubernetesCluster.Spec.Topology.ControlPlane.Version
	}
	defaultVersion := os.Getenv(consts.DEFAULT_KUBERNETES_VERSION)
	if defaultVersion != "" {
		return defaultVersion
	}
	return "1.30"
}

// resolveVMSize resolves a MachineClass name to an Azure VM size using the mapper
func (m *AKSClusterManagerImpl) resolveVMSize(ctx context.Context, machineClass string, location string, preferredPurpose interfaces.VMPurpose) string {
	// If machineClass is already a Standard_* VM size, use it directly
	if strings.HasPrefix(machineClass, "Standard_") {
		return machineClass
	}

	// If no mapper is configured, use static fallback
	if m.machineClassMapper == nil {
		return m.staticFallbackVMSize(machineClass, preferredPurpose)
	}

	// Try to map using the MachineClass mapper (no logging - called frequently)
	mapping, err := m.machineClassMapper.MapMachineClassToVMSize(ctx, machineClass, location, preferredPurpose)
	if err != nil {
		return m.staticFallbackVMSize(machineClass, preferredPurpose)
	}

	return mapping.VMSize
}

// needsUpdate compares the desired state with the existing Azure cluster to determine if an update is needed
func (m *AKSClusterManagerImpl) needsUpdate(ctx context.Context, kubernetesCluster *vitistackv1alpha1.KubernetesCluster, existingCluster *armcontainerservice.ManagedCluster) bool {
	if existingCluster == nil || existingCluster.Properties == nil {
		return true
	}

	// Compare Kubernetes version
	desiredVersion := m.getKubernetesVersion(kubernetesCluster)
	if existingCluster.Properties.KubernetesVersion != nil {
		currentVersion := *existingCluster.Properties.KubernetesVersion
		if desiredVersion != currentVersion {
			vlog.Info("Kubernetes version change detected",
				"current", currentVersion,
				"desired", desiredVersion,
				"cluster", kubernetesCluster.Name)
			return true
		}
	}

	// Compare system node pool configuration
	if kubernetesCluster.Spec.Topology.ControlPlane.Replicas > 0 && existingCluster.Properties.AgentPoolProfiles != nil {
		// Safe conversion with bounds check (replicas should never exceed int32 max)
		replicas := kubernetesCluster.Spec.Topology.ControlPlane.Replicas
		if replicas > int(^int32(0)) {
			replicas = int(^int32(0)) // Cap at max int32
		}
		desiredCount := int32(replicas) // #nosec G115 -- bounds checked above
		for _, pool := range existingCluster.Properties.AgentPoolProfiles {
			if pool.Name != nil && *pool.Name == "system" {
				if pool.Count != nil && *pool.Count != desiredCount {
					vlog.Info("System node pool count change detected",
						"current", *pool.Count,
						"desired", desiredCount,
						"cluster", kubernetesCluster.Name)
					return true
				}
				// Compare VM size
				desiredVMSize := m.resolveVMSize(ctx,
					kubernetesCluster.Spec.Topology.ControlPlane.MachineClass,
					kubernetesCluster.Spec.Cluster.Region,
					interfaces.VMPurposeGeneralPurpose)
				if pool.VMSize != nil && *pool.VMSize != desiredVMSize {
					vlog.Info("System node pool VM size change detected",
						"current", *pool.VMSize,
						"desired", desiredVMSize,
						"cluster", kubernetesCluster.Name)
					return true
				}
				break
			}
		}
	}

	// Note: Worker node pool changes are handled separately by the AgentPoolManager
	// This function focuses on cluster-level settings

	return false
}

// determineNodePoolPurpose determines the preferred VM purpose for a node pool based on hints
func (m *AKSClusterManagerImpl) determineNodePoolPurpose(nodePool vitistackv1alpha1.KubernetesClusterNodePool) interfaces.VMPurpose {
	// Check for GPU-related taints or labels
	for _, taint := range nodePool.Taint {
		if strings.Contains(strings.ToLower(taint.Key), "gpu") ||
			strings.Contains(strings.ToLower(taint.Value), "gpu") {
			return interfaces.VMPurposeGPU
		}
	}

	for key, value := range nodePool.Metadata.Labels {
		keyLower := strings.ToLower(key)
		valueLower := strings.ToLower(value)

		if strings.Contains(keyLower, "gpu") || strings.Contains(valueLower, "gpu") {
			return interfaces.VMPurposeGPU
		}
		if strings.Contains(keyLower, "compute") || strings.Contains(valueLower, "compute") {
			return interfaces.VMPurposeComputeOptimized
		}
		if strings.Contains(keyLower, "memory") || strings.Contains(valueLower, "memory") {
			return interfaces.VMPurposeMemoryOptimized
		}
	}

	// Check machine class name for hints
	machineClassLower := strings.ToLower(nodePool.MachineClass)
	switch {
	case strings.Contains(machineClassLower, "gpu"):
		return interfaces.VMPurposeGPU
	case strings.Contains(machineClassLower, "cpu") || strings.Contains(machineClassLower, "compute"):
		return interfaces.VMPurposeComputeOptimized
	case strings.Contains(machineClassLower, "memory") || strings.Contains(machineClassLower, "mem"):
		return interfaces.VMPurposeMemoryOptimized
	default:
		return interfaces.VMPurposeGeneralPurpose
	}
}

// staticFallbackVMSize provides a static fallback mapping when mapper is not available
func (m *AKSClusterManagerImpl) staticFallbackVMSize(machineClass string, purpose interfaces.VMPurpose) string {
	// Define static mappings with purpose awareness
	type vmSizes struct {
		generalPurpose   string
		computeOptimized string
		memoryOptimized  string
		gpu              string
	}

	staticMappings := map[string]vmSizes{
		"small":       {"Standard_D2s_v5", "Standard_F2s_v2", "Standard_E2s_v5", "Standard_NC4as_T4_v3"},
		"medium":      {"Standard_D4s_v5", "Standard_F4s_v2", "Standard_E4s_v5", "Standard_NC8as_T4_v3"},
		"large":       {"Standard_D8s_v5", "Standard_F8s_v2", "Standard_E8s_v5", "Standard_NC16as_T4_v3"},
		"xlarge":      {"Standard_D16s_v5", "Standard_F16s_v2", "Standard_E16s_v5", "Standard_NC64as_T4_v3"},
		"largecpu":    {"Standard_F8s_v2", "Standard_F8s_v2", "Standard_F8s_v2", "Standard_NC6s_v3"},
		"largememory": {"Standard_E8s_v5", "Standard_E8s_v5", "Standard_E8s_v5", "Standard_NC6s_v3"},
		"gpu":         {"Standard_NC6s_v3", "Standard_NC6s_v3", "Standard_NC6s_v3", "Standard_NC6s_v3"},
	}

	name := strings.ToLower(machineClass)
	sizes, ok := staticMappings[name]
	if !ok {
		sizes = staticMappings["medium"]
	}

	switch purpose {
	case interfaces.VMPurposeComputeOptimized:
		return sizes.computeOptimized
	case interfaces.VMPurposeMemoryOptimized:
		return sizes.memoryOptimized
	case interfaces.VMPurposeGPU:
		return sizes.gpu
	default:
		return sizes.generalPurpose
	}
}
