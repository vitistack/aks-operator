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
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/runtime"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/containerservice/armcontainerservice/v6"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// ManagedClusterClientImpl implements the ManagedClusterClient interface
type ManagedClusterClientImpl struct {
	client *armcontainerservice.ManagedClustersClient
}

// Polling configuration for long-running operations
const (
	// pollInterval is how often to poll Azure for operation status
	pollInterval = 30 * time.Second
	// logInterval is how often to log progress messages
	logInterval = 60 * time.Second
)

// NewManagedClusterClientImpl creates a new ManagedClusterClientImpl
func NewManagedClusterClientImpl(client *armcontainerservice.ManagedClustersClient) *ManagedClusterClientImpl {
	return &ManagedClusterClientImpl{
		client: client,
	}
}

// pollWithLogging polls a long-running operation with periodic logging
func pollWithLogging[T any](ctx context.Context, poller *runtime.Poller[T], operation, resourceName string) (T, error) {
	logger := log.FromContext(ctx)
	startTime := time.Now()
	lastLogTime := startTime

	logger.Info("Starting long-running Azure operation",
		"operation", operation,
		"resource", resourceName)

	for {
		// Check if context is cancelled
		select {
		case <-ctx.Done():
			var zero T
			return zero, ctx.Err()
		default:
		}

		// Poll for completion
		resp, err := poller.Poll(ctx)
		if err != nil {
			var zero T
			return zero, fmt.Errorf("polling failed: %w", err)
		}

		elapsed := time.Since(startTime).Round(time.Second)

		if poller.Done() {
			logger.Info("Azure operation completed",
				"operation", operation,
				"resource", resourceName,
				"duration", elapsed.String())

			result, err := poller.Result(ctx)
			if err != nil {
				var zero T
				return zero, fmt.Errorf("failed to get operation result: %w", err)
			}
			return result, nil
		}

		// Log progress periodically
		if time.Since(lastLogTime) >= logInterval {
			logger.Info("Azure operation in progress, waiting...",
				"operation", operation,
				"resource", resourceName,
				"elapsed", elapsed.String(),
				"status", resp.Status)
			lastLogTime = time.Now()
		}

		// Wait before next poll
		select {
		case <-ctx.Done():
			var zero T
			return zero, ctx.Err()
		case <-time.After(pollInterval):
		}
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

	resp, err := pollWithLogging(ctx, poller, "CreateOrUpdate", clusterName)
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
	logger := log.FromContext(ctx)
	logger.Info("🗑️ Starting Azure AKS deletion API call",
		"resourceGroup", resourceGroupName,
		"clusterName", clusterName)

	poller, err := c.client.BeginDelete(ctx, resourceGroupName, clusterName, nil)
	if err != nil {
		logger.Error(err, "❌ Failed to begin delete managed cluster",
			"resourceGroup", resourceGroupName,
			"clusterName", clusterName)
		return fmt.Errorf("failed to begin delete managed cluster: %w", err)
	}

	logger.Info("✅ Azure accepted delete request, polling for completion",
		"resourceGroup", resourceGroupName,
		"clusterName", clusterName)

	_, err = pollWithLogging(ctx, poller, "Delete", clusterName)
	if err != nil {
		return fmt.Errorf("failed to delete managed cluster: %w", err)
	}

	logger.Info("🗑️ Azure AKS cluster deletion completed",
		"resourceGroup", resourceGroupName,
		"clusterName", clusterName)

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
