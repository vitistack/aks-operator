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
	vlog.Info("Creating AKS cluster", "cluster", kubernetesCluster.Name, "namespace", kubernetesCluster.Namespace)

	resourceGroupName := m.getResourceGroupName(kubernetesCluster)
	clusterName := m.getClusterName(kubernetesCluster)
	location := kubernetesCluster.Spec.Cluster.Region

	managedCluster := m.buildManagedCluster(ctx, kubernetesCluster, location)

	result, err := m.managedClusterClient.CreateOrUpdate(ctx, resourceGroupName, clusterName, managedCluster)
	if err != nil {
		return nil, fmt.Errorf("failed to create AKS cluster: %w", err)
	}

	vlog.Info("AKS cluster created successfully", "cluster", clusterName, "id", *result.ID)
	return result, nil
}

// UpdateCluster updates an existing AKS cluster
func (m *AKSClusterManagerImpl) UpdateCluster(ctx context.Context, kubernetesCluster *vitistackv1alpha1.KubernetesCluster) (*armcontainerservice.ManagedCluster, error) {
	vlog.Info("Updating AKS cluster", "cluster", kubernetesCluster.Name, "namespace", kubernetesCluster.Namespace)

	resourceGroupName := m.getResourceGroupName(kubernetesCluster)
	clusterName := m.getClusterName(kubernetesCluster)
	location := kubernetesCluster.Spec.Cluster.Region

	managedCluster := m.buildManagedCluster(ctx, kubernetesCluster, location)

	result, err := m.managedClusterClient.CreateOrUpdate(ctx, resourceGroupName, clusterName, managedCluster)
	if err != nil {
		return nil, fmt.Errorf("failed to update AKS cluster: %w", err)
	}

	vlog.Info("AKS cluster updated successfully", "cluster", clusterName, "id", *result.ID)
	return result, nil
}

// DeleteCluster deletes an AKS cluster
func (m *AKSClusterManagerImpl) DeleteCluster(ctx context.Context, kubernetesCluster *vitistackv1alpha1.KubernetesCluster) error {
	vlog.Info("Deleting AKS cluster", "cluster", kubernetesCluster.Name, "namespace", kubernetesCluster.Namespace)

	resourceGroupName := m.getResourceGroupName(kubernetesCluster)
	clusterName := m.getClusterName(kubernetesCluster)

	err := m.managedClusterClient.Delete(ctx, resourceGroupName, clusterName)
	if err != nil {
		return fmt.Errorf("failed to delete AKS cluster: %w", err)
	}

	vlog.Info("AKS cluster deleted successfully", "cluster", clusterName)
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
	vlog.Info("Reconciling AKS cluster", "cluster", kubernetesCluster.Name, "namespace", kubernetesCluster.Namespace)

	existingCluster, err := m.GetCluster(ctx, kubernetesCluster)
	if err != nil {
		if strings.Contains(err.Error(), "not found") || strings.Contains(err.Error(), "ResourceNotFound") {
			return m.CreateCluster(ctx, kubernetesCluster)
		}
		return nil, err
	}

	result, err := m.UpdateCluster(ctx, kubernetesCluster)
	if err != nil {
		return nil, err
	}

	if m.agentPoolManager != nil {
		if err := m.agentPoolManager.ReconcileAgentPools(ctx, kubernetesCluster); err != nil {
			vlog.Error(err, "failed to reconcile agent pools")
			return result, fmt.Errorf("failed to reconcile agent pools: %w", err)
		}
	}

	vlog.Info("AKS cluster reconciled successfully", "cluster", kubernetesCluster.Name, "provisioningState", *existingCluster.Properties.ProvisioningState)
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

	objectID := os.Getenv(consts.AZURE_OBJECT_ID)
	clientSecret := os.Getenv(consts.AZURE_CLIENT_SECRET)
	if objectID != "" && clientSecret != "" {
		managedCluster.Properties.ServicePrincipalProfile = &armcontainerservice.ManagedClusterServicePrincipalProfile{
			ClientID: to.Ptr(objectID),
			Secret:   to.Ptr(clientSecret),
		}
		managedCluster.Identity = nil
	}

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
	return fmt.Sprintf("aks-%s-%s", kubernetesCluster.Namespace, kubernetesCluster.Name)
}

func (m *AKSClusterManagerImpl) getDNSPrefix(kubernetesCluster *vitistackv1alpha1.KubernetesCluster) string {
	return fmt.Sprintf("aks-%s-%s", kubernetesCluster.Namespace, kubernetesCluster.Name)
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

	// Try to map using the MachineClass mapper
	mapping, err := m.machineClassMapper.MapMachineClassToVMSize(ctx, machineClass, location, preferredPurpose)
	if err != nil {
		vlog.Error(err, "Failed to map MachineClass, using fallback", "machineClass", machineClass)
		return m.staticFallbackVMSize(machineClass, preferredPurpose)
	}

	vlog.Info("Resolved MachineClass to VM size",
		"machineClass", machineClass,
		"vmSize", mapping.VMSize,
		"purpose", mapping.VMPurpose,
	)

	return mapping.VMSize
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
