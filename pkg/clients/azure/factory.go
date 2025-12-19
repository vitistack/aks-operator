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
	"fmt"
	"os"

	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/compute/armcompute/v6"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/containerservice/armcontainerservice/v6"
	"github.com/vitistack/aks-operator/internal/consts"
	"github.com/vitistack/aks-operator/pkg/interfaces"
)

// ClientFactory implements the AzureClientFactory interface
type ClientFactory struct {
	subscriptionID          string
	credential              *azidentity.DefaultAzureCredential
	containerServiceFactory *armcontainerservice.ClientFactory
	computeFactory          *armcompute.ClientFactory
}

// NewClientFactory creates a new Azure client factory
func NewClientFactory() (*ClientFactory, error) {
	subscriptionID := os.Getenv(consts.AZURE_SUBSCRIPTION_ID)
	if subscriptionID == "" {
		return nil, fmt.Errorf("environment variable %s is not set", consts.AZURE_SUBSCRIPTION_ID)
	}

	// Create Azure credential using DefaultAzureCredential
	// This supports multiple authentication methods: environment variables, managed identity, Azure CLI, etc.
	cred, err := azidentity.NewDefaultAzureCredential(nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create Azure credential: %w", err)
	}

	// Create container service client factory
	containerServiceFactory, err := armcontainerservice.NewClientFactory(subscriptionID, cred, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create container service client factory: %w", err)
	}

	// Create compute client factory
	computeFactory, err := armcompute.NewClientFactory(subscriptionID, cred, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create compute client factory: %w", err)
	}

	return &ClientFactory{
		subscriptionID:          subscriptionID,
		credential:              cred,
		containerServiceFactory: containerServiceFactory,
		computeFactory:          computeFactory,
	}, nil
}

// NewManagedClusterClient creates a new ManagedClusterClient
func (f *ClientFactory) NewManagedClusterClient() (interfaces.ManagedClusterClient, error) {
	client := f.containerServiceFactory.NewManagedClustersClient()
	return NewManagedClusterClientImpl(client), nil
}

// NewAgentPoolClient creates a new AgentPoolClient
func (f *ClientFactory) NewAgentPoolClient() (interfaces.AgentPoolClient, error) {
	client := f.containerServiceFactory.NewAgentPoolsClient()
	return NewAgentPoolClientImpl(client), nil
}

// NewVMSizeClient creates a new VMSizeClient
func (f *ClientFactory) NewVMSizeClient() (interfaces.VMSizeClient, error) {
	client := f.computeFactory.NewVirtualMachineSizesClient()
	return NewVMSizeClientImpl(client), nil
}

// GetSubscriptionID returns the subscription ID
func (f *ClientFactory) GetSubscriptionID() string {
	return f.subscriptionID
}
