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

// AgentPoolClientImpl implements the AgentPoolClient interface
type AgentPoolClientImpl struct {
	client *armcontainerservice.AgentPoolsClient
}

// NewAgentPoolClientImpl creates a new AgentPoolClientImpl
func NewAgentPoolClientImpl(client *armcontainerservice.AgentPoolsClient) *AgentPoolClientImpl {
	return &AgentPoolClientImpl{
		client: client,
	}
}

// CreateOrUpdate creates or updates an agent pool
func (c *AgentPoolClientImpl) CreateOrUpdate(
	ctx context.Context,
	resourceGroupName, clusterName, agentPoolName string,
	agentPool armcontainerservice.AgentPool,
) (*armcontainerservice.AgentPool, error) {
	poller, err := c.client.BeginCreateOrUpdate(
		ctx, resourceGroupName, clusterName, agentPoolName, agentPool, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to begin create/update agent pool: %w", err)
	}

	resp, err := poller.PollUntilDone(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create/update agent pool: %w", err)
	}

	return &resp.AgentPool, nil
}

// Get retrieves an agent pool
func (c *AgentPoolClientImpl) Get(
	ctx context.Context,
	resourceGroupName, clusterName, agentPoolName string,
) (*armcontainerservice.AgentPool, error) {
	resp, err := c.client.Get(ctx, resourceGroupName, clusterName, agentPoolName, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get agent pool: %w", err)
	}

	return &resp.AgentPool, nil
}

// Delete deletes an agent pool
func (c *AgentPoolClientImpl) Delete(ctx context.Context, resourceGroupName, clusterName, agentPoolName string) error {
	poller, err := c.client.BeginDelete(ctx, resourceGroupName, clusterName, agentPoolName, nil)
	if err != nil {
		return fmt.Errorf("failed to begin delete agent pool: %w", err)
	}

	_, err = poller.PollUntilDone(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to delete agent pool: %w", err)
	}

	return nil
}

// List lists all agent pools in a managed cluster
func (c *AgentPoolClientImpl) List(
	ctx context.Context,
	resourceGroupName, clusterName string,
) ([]*armcontainerservice.AgentPool, error) {
	var agentPools []*armcontainerservice.AgentPool

	pager := c.client.NewListPager(resourceGroupName, clusterName, nil)
	for pager.More() {
		page, err := pager.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to list agent pools: %w", err)
		}
		agentPools = append(agentPools, page.Value...)
	}

	return agentPools, nil
}
