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
)

// ManagedClusterClient defines the interface for Azure AKS managed cluster operations
type ManagedClusterClient interface {
	// CreateOrUpdate creates or updates a managed cluster
	CreateOrUpdate(
		ctx context.Context,
		resourceGroupName, clusterName string,
		cluster armcontainerservice.ManagedCluster,
	) (*armcontainerservice.ManagedCluster, error)

	// Get retrieves a managed cluster
	Get(ctx context.Context, resourceGroupName, clusterName string) (*armcontainerservice.ManagedCluster, error)

	// Delete deletes a managed cluster
	Delete(ctx context.Context, resourceGroupName, clusterName string) error

	// GetAccessProfile retrieves the access profile for a managed cluster
	GetAccessProfile(
		ctx context.Context,
		resourceGroupName, clusterName, roleName string,
	) (*armcontainerservice.ManagedClusterAccessProfile, error)

	// ListClusterAdminCredentials lists the admin credentials
	ListClusterAdminCredentials(
		ctx context.Context,
		resourceGroupName, clusterName string,
	) (*armcontainerservice.CredentialResults, error)

	// ListClusterUserCredentials lists the user credentials
	ListClusterUserCredentials(
		ctx context.Context,
		resourceGroupName, clusterName string,
	) (*armcontainerservice.CredentialResults, error)
}

// AgentPoolClient defines the interface for Azure AKS agent pool operations
type AgentPoolClient interface {
	// CreateOrUpdate creates or updates an agent pool
	CreateOrUpdate(
		ctx context.Context,
		resourceGroupName, clusterName, agentPoolName string,
		agentPool armcontainerservice.AgentPool,
	) (*armcontainerservice.AgentPool, error)

	// Get retrieves an agent pool
	Get(
		ctx context.Context,
		resourceGroupName, clusterName, agentPoolName string,
	) (*armcontainerservice.AgentPool, error)

	// Delete deletes an agent pool
	Delete(ctx context.Context, resourceGroupName, clusterName, agentPoolName string) error

	// List lists all agent pools in a managed cluster
	List(ctx context.Context, resourceGroupName, clusterName string) ([]*armcontainerservice.AgentPool, error)
}

// VMSizeClient defines the interface for Azure VM size operations
type VMSizeClient interface {
	// List lists all available VM sizes in a location
	List(ctx context.Context, location string) ([]*armcompute.VirtualMachineSize, error)

	// GetVMSize retrieves a specific VM size by name
	GetVMSize(ctx context.Context, location, vmSizeName string) (*armcompute.VirtualMachineSize, error)
}

// AzureClientFactory defines the interface for creating Azure clients
type AzureClientFactory interface {
	// NewManagedClusterClient creates a new ManagedClusterClient
	NewManagedClusterClient() (ManagedClusterClient, error)

	// NewAgentPoolClient creates a new AgentPoolClient
	NewAgentPoolClient() (AgentPoolClient, error)

	// NewVMSizeClient creates a new VMSizeClient
	NewVMSizeClient() (VMSizeClient, error)
}
