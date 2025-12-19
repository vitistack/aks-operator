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
	"strings"

	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/compute/armcompute/v6"
)

// VMSizeClientImpl implements the VMSizeClient interface
type VMSizeClientImpl struct {
	client *armcompute.VirtualMachineSizesClient
}

// NewVMSizeClientImpl creates a new VMSizeClientImpl
func NewVMSizeClientImpl(client *armcompute.VirtualMachineSizesClient) *VMSizeClientImpl {
	return &VMSizeClientImpl{
		client: client,
	}
}

// List lists all available VM sizes in a location
func (c *VMSizeClientImpl) List(ctx context.Context, location string) ([]*armcompute.VirtualMachineSize, error) {
	var vmSizes []*armcompute.VirtualMachineSize

	pager := c.client.NewListPager(location, nil)
	for pager.More() {
		page, err := pager.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to list VM sizes: %w", err)
		}
		vmSizes = append(vmSizes, page.Value...)
	}

	return vmSizes, nil
}

// GetVMSize retrieves a specific VM size by name
func (c *VMSizeClientImpl) GetVMSize(
	ctx context.Context,
	location, vmSizeName string,
) (*armcompute.VirtualMachineSize, error) {
	vmSizes, err := c.List(ctx, location)
	if err != nil {
		return nil, err
	}

	for _, vmSize := range vmSizes {
		if vmSize.Name != nil && strings.EqualFold(*vmSize.Name, vmSizeName) {
			return vmSize, nil
		}
	}

	return nil, fmt.Errorf("VM size %s not found in location %s", vmSizeName, location)
}
