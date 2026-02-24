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
	"strings"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/containerservice/armcontainerservice/v6"
	"github.com/vitistack/aks-operator/pkg/interfaces"
	"github.com/vitistack/common/pkg/loggers/vlog"
	vitistackv1alpha1 "github.com/vitistack/common/pkg/v1alpha1"
)

// AgentPoolManagerImpl implements the AgentPoolManager interface
type AgentPoolManagerImpl struct {
	agentPoolClient    interfaces.AgentPoolClient
	vmSizeManager      interfaces.VMSizeManager
	machineClassMapper interfaces.MachineClassMapper
}

// NewAgentPoolManager creates a new AgentPoolManager
func NewAgentPoolManager(agentPoolClient interfaces.AgentPoolClient, vmSizeManager interfaces.VMSizeManager, machineClassMapper interfaces.MachineClassMapper) *AgentPoolManagerImpl {
	return &AgentPoolManagerImpl{
		agentPoolClient:    agentPoolClient,
		vmSizeManager:      vmSizeManager,
		machineClassMapper: machineClassMapper,
	}
}

// CreateAgentPool creates an agent pool from NodePool spec
func (m *AgentPoolManagerImpl) CreateAgentPool(ctx context.Context, kubernetesCluster *vitistackv1alpha1.KubernetesCluster, nodePool vitistackv1alpha1.KubernetesClusterNodePool) (*armcontainerservice.AgentPool, error) {
	vlog.Info("Creating agent pool", "cluster", kubernetesCluster.Name, "nodePool", nodePool.Name, "clusterid", kubernetesCluster.Spec.Cluster.ClusterId)

	resourceGroupName := m.getResourceGroupName(kubernetesCluster)
	clusterName := m.getClusterName(kubernetesCluster)
	location := kubernetesCluster.Spec.Cluster.Region

	vmSize := m.resolveVMSize(ctx, nodePool, location)
	if m.vmSizeManager != nil {
		valid, err := m.vmSizeManager.ValidateVMSize(ctx, location, vmSize)
		if err != nil {
			vlog.Error(err, "failed to validate VM size", "vmSize", vmSize, "location", location, "clusterid", kubernetesCluster.Spec.Cluster.ClusterId)
		} else if !valid {
			return nil, fmt.Errorf("VM size %s is not available in location %s", vmSize, location)
		}
	}

	agentPool := m.buildAgentPool(nodePool, vmSize)

	result, err := m.agentPoolClient.CreateOrUpdate(ctx, resourceGroupName, clusterName, nodePool.Name, agentPool)
	if err != nil {
		return nil, fmt.Errorf("failed to create agent pool: %w", err)
	}

	vlog.Info("Agent pool created successfully", "cluster", clusterName, "nodePool", nodePool.Name, "id", *result.ID, "clusterid", kubernetesCluster.Spec.Cluster.ClusterId)
	return result, nil
}

// UpdateAgentPool updates an existing agent pool
func (m *AgentPoolManagerImpl) UpdateAgentPool(ctx context.Context, kubernetesCluster *vitistackv1alpha1.KubernetesCluster, nodePool vitistackv1alpha1.KubernetesClusterNodePool) (*armcontainerservice.AgentPool, error) {
	vlog.Info("Updating agent pool", "cluster", kubernetesCluster.Name, "nodePool", nodePool.Name, "clusterid", kubernetesCluster.Spec.Cluster.ClusterId)

	resourceGroupName := m.getResourceGroupName(kubernetesCluster)
	clusterName := m.getClusterName(kubernetesCluster)
	location := kubernetesCluster.Spec.Cluster.Region

	vmSize := m.resolveVMSize(ctx, nodePool, location)
	agentPool := m.buildAgentPool(nodePool, vmSize)

	result, err := m.agentPoolClient.CreateOrUpdate(ctx, resourceGroupName, clusterName, nodePool.Name, agentPool)
	if err != nil {
		return nil, fmt.Errorf("failed to update agent pool: %w", err)
	}

	vlog.Info("Agent pool updated successfully", "cluster", clusterName, "nodePool", nodePool.Name, "clusterid", kubernetesCluster.Spec.Cluster.ClusterId)
	return result, nil
}

// DeleteAgentPool deletes an agent pool
func (m *AgentPoolManagerImpl) DeleteAgentPool(ctx context.Context, kubernetesCluster *vitistackv1alpha1.KubernetesCluster, nodePoolName string) error {
	vlog.Info("Deleting agent pool", "cluster", kubernetesCluster.Name, "nodePool", nodePoolName, "clusterid", kubernetesCluster.Spec.Cluster.ClusterId)

	resourceGroupName := m.getResourceGroupName(kubernetesCluster)
	clusterName := m.getClusterName(kubernetesCluster)

	err := m.agentPoolClient.Delete(ctx, resourceGroupName, clusterName, nodePoolName)
	if err != nil {
		return fmt.Errorf("failed to delete agent pool: %w", err)
	}

	vlog.Info("Agent pool deleted successfully", "cluster", clusterName, "nodePool", nodePoolName, "clusterid", kubernetesCluster.Spec.Cluster.ClusterId)
	return nil
}

// GetAgentPool retrieves an agent pool
func (m *AgentPoolManagerImpl) GetAgentPool(ctx context.Context, kubernetesCluster *vitistackv1alpha1.KubernetesCluster, nodePoolName string) (*armcontainerservice.AgentPool, error) {
	resourceGroupName := m.getResourceGroupName(kubernetesCluster)
	clusterName := m.getClusterName(kubernetesCluster)

	result, err := m.agentPoolClient.Get(ctx, resourceGroupName, clusterName, nodePoolName)
	if err != nil {
		return nil, fmt.Errorf("failed to get agent pool: %w", err)
	}

	return result, nil
}

// ListAgentPools lists all agent pools in a cluster
func (m *AgentPoolManagerImpl) ListAgentPools(ctx context.Context, kubernetesCluster *vitistackv1alpha1.KubernetesCluster) ([]*armcontainerservice.AgentPool, error) {
	resourceGroupName := m.getResourceGroupName(kubernetesCluster)
	clusterName := m.getClusterName(kubernetesCluster)

	pools, err := m.agentPoolClient.List(ctx, resourceGroupName, clusterName)
	if err != nil {
		return nil, fmt.Errorf("failed to list agent pools: %w", err)
	}

	return pools, nil
}

// ReconcileAgentPools reconciles all agent pools for a cluster
func (m *AgentPoolManagerImpl) ReconcileAgentPools(ctx context.Context, kubernetesCluster *vitistackv1alpha1.KubernetesCluster) error {
	vlog.Info("Reconciling agent pools", "cluster", kubernetesCluster.Name, "clusterid", kubernetesCluster.Spec.Cluster.ClusterId)

	existingPools, err := m.ListAgentPools(ctx, kubernetesCluster)
	if err != nil {
		if strings.Contains(err.Error(), "not found") || strings.Contains(err.Error(), "ResourceNotFound") {
			vlog.Info("Cluster not found, skipping agent pool reconciliation")
			return nil
		}
		return err
	}

	existingPoolMap := make(map[string]*armcontainerservice.AgentPool)
	for _, pool := range existingPools {
		if pool.Name != nil {
			existingPoolMap[*pool.Name] = pool
		}
	}

	desiredPools := kubernetesCluster.Spec.Topology.Workers.NodePools
	desiredPoolNames := make(map[string]bool)

	for _, nodePool := range desiredPools {
		desiredPoolNames[nodePool.Name] = true

		if _, exists := existingPoolMap[nodePool.Name]; exists {
			if _, err := m.UpdateAgentPool(ctx, kubernetesCluster, nodePool); err != nil {
				vlog.Error(err, "failed to update agent pool", "nodePool", nodePool.Name, "clusterid", kubernetesCluster.Spec.Cluster.ClusterId)
			}
		} else {
			if _, err := m.CreateAgentPool(ctx, kubernetesCluster, nodePool); err != nil {
				vlog.Error(err, "failed to create agent pool", "nodePool", nodePool.Name, "clusterid", kubernetesCluster.Spec.Cluster.ClusterId)
			}
		}
	}

	for poolName := range existingPoolMap {
		if !desiredPoolNames[poolName] && poolName != "system" {
			if err := m.DeleteAgentPool(ctx, kubernetesCluster, poolName); err != nil {
				vlog.Error(err, "failed to delete agent pool", "nodePool", poolName, "clusterid", kubernetesCluster.Spec.Cluster.ClusterId)
			}
		}
	}

	vlog.Info("Agent pools reconciled successfully", "cluster", kubernetesCluster.Name, "clusterid", kubernetesCluster.Spec.Cluster.ClusterId)
	return nil
}

func (m *AgentPoolManagerImpl) buildAgentPool(nodePool vitistackv1alpha1.KubernetesClusterNodePool, vmSize string) armcontainerservice.AgentPool {
	agentPool := armcontainerservice.AgentPool{
		Properties: &armcontainerservice.ManagedClusterAgentPoolProfileProperties{
			Count:  new(safeIntToInt32(nodePool.Replicas)),
			VMSize: new(vmSize),
			Mode:   to.Ptr(armcontainerservice.AgentPoolModeUser),
			OSType: to.Ptr(armcontainerservice.OSTypeLinux),
			Type:   to.Ptr(armcontainerservice.AgentPoolTypeVirtualMachineScaleSets),
		},
	}

	if nodePool.Autoscaling.Enabled {
		agentPool.Properties.EnableAutoScaling = new(true)
		agentPool.Properties.MinCount = new(safeIntToInt32(nodePool.Autoscaling.MinReplicas))
		agentPool.Properties.MaxCount = new(safeIntToInt32(nodePool.Autoscaling.MaxReplicas))
	} else {
		agentPool.Properties.EnableAutoScaling = new(false)
	}

	if nodePool.Version != "" {
		agentPool.Properties.OrchestratorVersion = new(nodePool.Version)
	}

	if len(nodePool.Taint) > 0 {
		taints := make([]*string, 0, len(nodePool.Taint))
		for _, taint := range nodePool.Taint {
			taintStr := fmt.Sprintf("%s=%s:%s", taint.Key, taint.Value, taint.Effect)
			taints = append(taints, new(taintStr))
		}
		agentPool.Properties.NodeTaints = taints
	}

	if len(nodePool.Metadata.Labels) > 0 {
		agentPool.Properties.NodeLabels = make(map[string]*string)
		for k, v := range nodePool.Metadata.Labels {
			agentPool.Properties.NodeLabels[k] = new(v)
		}
	}

	agentPool.Properties.AvailabilityZones = []*string{
		new("1"),
		new("2"),
		new("3"),
	}

	return agentPool
}

func (m *AgentPoolManagerImpl) getResourceGroupName(kubernetesCluster *vitistackv1alpha1.KubernetesCluster) string {
	if kubernetesCluster.Spec.Cluster.Project != "" {
		return kubernetesCluster.Spec.Cluster.Project
	}
	return kubernetesCluster.Namespace
}

func (m *AgentPoolManagerImpl) getClusterName(kubernetesCluster *vitistackv1alpha1.KubernetesCluster) string {
	return kubernetesCluster.Spec.Cluster.ClusterId
}

// resolveVMSize resolves a MachineClass name to an Azure VM size using the mapper
func (m *AgentPoolManagerImpl) resolveVMSize(ctx context.Context, nodePool vitistackv1alpha1.KubernetesClusterNodePool, location string) string {
	machineClass := nodePool.MachineClass

	// If machineClass is already a Standard_* VM size, use it directly
	if strings.HasPrefix(machineClass, "Standard_") {
		return machineClass
	}

	// Determine preferred purpose based on node pool hints
	purpose := m.determineNodePoolPurpose(nodePool)

	// If no mapper is configured, use static fallback
	if m.machineClassMapper == nil {
		return m.staticFallbackVMSize(machineClass, purpose)
	}

	// Try to map using the MachineClass mapper
	mapping, err := m.machineClassMapper.MapMachineClassToVMSize(ctx, machineClass, location, purpose)
	if err != nil {
		vlog.Error(err, "Failed to map MachineClass, using fallback", "machineClass", machineClass)
		return m.staticFallbackVMSize(machineClass, purpose)
	}

	vlog.Info("Resolved MachineClass to VM size",
		"machineClass", machineClass,
		"vmSize", mapping.VMSize,
		"purpose", mapping.VMPurpose,
	)

	return mapping.VMSize
}

// determineNodePoolPurpose determines the preferred VM purpose for a node pool based on hints
func (m *AgentPoolManagerImpl) determineNodePoolPurpose(nodePool vitistackv1alpha1.KubernetesClusterNodePool) interfaces.VMPurpose {
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
func (m *AgentPoolManagerImpl) staticFallbackVMSize(machineClass string, purpose interfaces.VMPurpose) string {
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
