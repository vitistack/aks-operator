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

package interfaces

import (
	"context"

	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/compute/armcompute/v6"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/containerservice/armcontainerservice/v6"
	vitistackv1alpha1 "github.com/vitistack/common/pkg/v1alpha1"
)

// AKSClusterManager defines the interface for managing AKS clusters
type AKSClusterManager interface {
	// CreateCluster creates an AKS cluster from KubernetesCluster spec
	CreateCluster(
		ctx context.Context,
		kubernetesCluster *vitistackv1alpha1.KubernetesCluster,
	) (*armcontainerservice.ManagedCluster, error)

	// UpdateCluster updates an existing AKS cluster
	UpdateCluster(
		ctx context.Context,
		kubernetesCluster *vitistackv1alpha1.KubernetesCluster,
	) (*armcontainerservice.ManagedCluster, error)

	// DeleteCluster deletes an AKS cluster
	DeleteCluster(ctx context.Context, kubernetesCluster *vitistackv1alpha1.KubernetesCluster) error

	// GetCluster retrieves an AKS cluster
	GetCluster(
		ctx context.Context,
		kubernetesCluster *vitistackv1alpha1.KubernetesCluster,
	) (*armcontainerservice.ManagedCluster, error)

	// GetClusterCredentials retrieves the admin credentials for an AKS cluster
	GetClusterCredentials(
		ctx context.Context,
		kubernetesCluster *vitistackv1alpha1.KubernetesCluster,
	) (*armcontainerservice.CredentialResults, error)

	// ReconcileCluster reconciles the cluster state
	ReconcileCluster(
		ctx context.Context,
		kubernetesCluster *vitistackv1alpha1.KubernetesCluster,
	) (*armcontainerservice.ManagedCluster, error)
}

// AgentPoolManager defines the interface for managing AKS agent pools
type AgentPoolManager interface {
	// CreateAgentPool creates an agent pool from NodePool spec
	CreateAgentPool(
		ctx context.Context,
		kubernetesCluster *vitistackv1alpha1.KubernetesCluster,
		nodePool vitistackv1alpha1.KubernetesClusterNodePool,
	) (*armcontainerservice.AgentPool, error)

	// UpdateAgentPool updates an existing agent pool
	UpdateAgentPool(
		ctx context.Context,
		kubernetesCluster *vitistackv1alpha1.KubernetesCluster,
		nodePool vitistackv1alpha1.KubernetesClusterNodePool,
	) (*armcontainerservice.AgentPool, error)

	// DeleteAgentPool deletes an agent pool
	DeleteAgentPool(
		ctx context.Context,
		kubernetesCluster *vitistackv1alpha1.KubernetesCluster,
		nodePoolName string,
	) error

	// GetAgentPool retrieves an agent pool
	GetAgentPool(
		ctx context.Context,
		kubernetesCluster *vitistackv1alpha1.KubernetesCluster,
		nodePoolName string,
	) (*armcontainerservice.AgentPool, error)

	// ListAgentPools lists all agent pools in a cluster
	ListAgentPools(
		ctx context.Context,
		kubernetesCluster *vitistackv1alpha1.KubernetesCluster,
	) ([]*armcontainerservice.AgentPool, error)

	// ReconcileAgentPools reconciles all agent pools for a cluster
	ReconcileAgentPools(ctx context.Context, kubernetesCluster *vitistackv1alpha1.KubernetesCluster) error
}

// VMSizeManager defines the interface for managing Azure VM sizes
type VMSizeManager interface {
	// ListVMSizes lists all available VM sizes in a location
	ListVMSizes(ctx context.Context, location string) ([]*armcompute.VirtualMachineSize, error)

	// GetVMSize retrieves a specific VM size
	GetVMSize(ctx context.Context, location, vmSizeName string) (*armcompute.VirtualMachineSize, error)

	// ValidateVMSize validates if a VM size is available in a location
	ValidateVMSize(ctx context.Context, location, vmSizeName string) (bool, error)

	// GetRecommendedVMSizes returns recommended VM sizes for a workload type
	GetRecommendedVMSizes(
		ctx context.Context,
		location string,
		workloadType string,
	) ([]*armcompute.VirtualMachineSize, error)
}

// VMPurpose represents the preferred VM purpose/category
type VMPurpose string

const (
	// VMPurposeGeneralPurpose prefers general purpose VMs (D-series, Dsv3, etc.)
	VMPurposeGeneralPurpose VMPurpose = "general-purpose"
	// VMPurposeComputeOptimized prefers compute optimized VMs (F-series)
	VMPurposeComputeOptimized VMPurpose = "compute-optimized"
	// VMPurposeMemoryOptimized prefers memory optimized VMs (E-series)
	VMPurposeMemoryOptimized VMPurpose = "memory-optimized"
	// VMPurposeGPU prefers GPU VMs (N-series)
	VMPurposeGPU VMPurpose = "gpu"
	// VMPurposeBurstable prefers burstable VMs (B-series)
	VMPurposeBurstable VMPurpose = "burstable"
	// VMPurposeStorageOptimized prefers storage optimized VMs (L-series)
	VMPurposeStorageOptimized VMPurpose = "storage-optimized"
)

// MachineClassMapping represents the result of a MachineClass to VM size mapping
type MachineClassMapping struct {
	// MachineClassName is the name of the vitistack MachineClass
	MachineClassName string
	// VMSize is the recommended Azure VM size
	VMSize string
	// VMPurpose is the VM purpose category
	VMPurpose VMPurpose
	// Cores is the number of CPU cores
	Cores int
	// MemoryGB is the memory in GB
	MemoryGB float64
	// GPUCores is the number of GPU cores (0 if no GPU)
	GPUCores int
	// Score is the match score (lower is better)
	Score float64
	// Alternatives are alternative VM sizes that also match
	Alternatives []string
}

// MachineClassMapper defines the interface for mapping vitistack MachineClasses to Azure VM sizes
type MachineClassMapper interface {
	// MapMachineClassToVMSize maps a MachineClass name to the best matching Azure VM size
	// It reads the MachineClass CRD from the cluster and finds the best match
	MapMachineClassToVMSize(
		ctx context.Context,
		machineClassName string,
		location string,
		preferredPurpose VMPurpose,
	) (*MachineClassMapping, error)

	// MapMachineClassSpecToVMSize maps a MachineClass spec directly to an Azure VM size
	MapMachineClassSpecToVMSize(
		ctx context.Context,
		machineClass *vitistackv1alpha1.MachineClass,
		location string,
		preferredPurpose VMPurpose,
	) (*MachineClassMapping, error)

	// GetMachineClass retrieves a MachineClass CRD by name
	GetMachineClass(ctx context.Context, name string) (*vitistackv1alpha1.MachineClass, error)

	// ListMachineClasses lists all enabled MachineClasses
	ListMachineClasses(ctx context.Context) ([]*vitistackv1alpha1.MachineClass, error)

	// GetDefaultPurposeForCategory returns the default VM purpose for a MachineClass category
	GetDefaultPurposeForCategory(category string) VMPurpose

	// ValidateMapping validates if a mapping is available in a location
	ValidateMapping(ctx context.Context, machineClassName string, location string) (bool, error)
}
