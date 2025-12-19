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

package azure

import (
	"context"
	"fmt"

	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/containerservice/armcontainerservice/v6"
)

// ManagedClusterClientImpl implements the ManagedClusterClient interface
type ManagedClusterClientImpl struct {
	client *armcontainerservice.ManagedClustersClient
}

// NewManagedClusterClientImpl creates a new ManagedClusterClientImpl
func NewManagedClusterClientImpl(client *armcontainerservice.ManagedClustersClient) *ManagedClusterClientImpl {
	return &ManagedClusterClientImpl{
		client: client,
	}
}

// CreateOrUpdate creates or updates a managed cluster
func (c *ManagedClusterClientImpl) CreateOrUpdate(
	ctx context.Context,
	resourceGroupName, clusterName string,
	cluster armcontainerservice.ManagedCluster,
) (*armcontainerservice.ManagedCluster, error) {
	poller, err := c.client.BeginCreateOrUpdate(ctx, resourceGroupName, clusterName, cluster, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to begin create/update managed cluster: %w", err)
	}

	resp, err := poller.PollUntilDone(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create/update managed cluster: %w", err)
	}

	return &resp.ManagedCluster, nil
}

// Get retrieves a managed cluster
func (c *ManagedClusterClientImpl) Get(
	ctx context.Context,
	resourceGroupName, clusterName string,
) (*armcontainerservice.ManagedCluster, error) {
	resp, err := c.client.Get(ctx, resourceGroupName, clusterName, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get managed cluster: %w", err)
	}

	return &resp.ManagedCluster, nil
}

// Delete deletes a managed cluster
func (c *ManagedClusterClientImpl) Delete(ctx context.Context, resourceGroupName, clusterName string) error {
	poller, err := c.client.BeginDelete(ctx, resourceGroupName, clusterName, nil)
	if err != nil {
		return fmt.Errorf("failed to begin delete managed cluster: %w", err)
	}

	_, err = poller.PollUntilDone(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to delete managed cluster: %w", err)
	}

	return nil
}

// GetAccessProfile retrieves the access profile for a managed cluster
func (c *ManagedClusterClientImpl) GetAccessProfile(
	ctx context.Context,
	resourceGroupName, clusterName, roleName string,
) (*armcontainerservice.ManagedClusterAccessProfile, error) {
	resp, err := c.client.GetAccessProfile(ctx, resourceGroupName, clusterName, roleName, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get access profile: %w", err)
	}

	return &resp.ManagedClusterAccessProfile, nil
}

// ListClusterAdminCredentials lists the admin credentials
func (c *ManagedClusterClientImpl) ListClusterAdminCredentials(
	ctx context.Context,
	resourceGroupName, clusterName string,
) (*armcontainerservice.CredentialResults, error) {
	resp, err := c.client.ListClusterAdminCredentials(ctx, resourceGroupName, clusterName, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to list cluster admin credentials: %w", err)
	}

	return &resp.CredentialResults, nil
}

// ListClusterUserCredentials lists the user credentials
func (c *ManagedClusterClientImpl) ListClusterUserCredentials(
	ctx context.Context,
	resourceGroupName, clusterName string,
) (*armcontainerservice.CredentialResults, error) {
	resp, err := c.client.ListClusterUserCredentials(ctx, resourceGroupName, clusterName, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to list cluster user credentials: %w", err)
	}

	return &resp.CredentialResults, nil
}
