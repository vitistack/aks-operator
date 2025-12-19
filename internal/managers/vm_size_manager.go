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
	"sync"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/compute/armcompute/v6"
	"github.com/vitistack/aks-operator/pkg/interfaces"
	"github.com/vitistack/common/pkg/loggers/vlog"
)

// Workload type constants
const (
	workloadTypeGPU = "gpu"
)

// VMSizeManagerImpl implements the VMSizeManager interface
type VMSizeManagerImpl struct {
	vmSizeClient interfaces.VMSizeClient
	cache        *vmSizeCache
}

type vmSizeCache struct {
	mu     sync.RWMutex
	data   map[string][]*armcompute.VirtualMachineSize
	expiry map[string]time.Time
	ttl    time.Duration
}

func newVMSizeCache(ttl time.Duration) *vmSizeCache {
	return &vmSizeCache{
		data:   make(map[string][]*armcompute.VirtualMachineSize),
		expiry: make(map[string]time.Time),
		ttl:    ttl,
	}
}

func (c *vmSizeCache) get(location string) ([]*armcompute.VirtualMachineSize, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if expiry, ok := c.expiry[location]; ok {
		if time.Now().Before(expiry) {
			return c.data[location], true
		}
	}
	return nil, false
}

func (c *vmSizeCache) set(location string, sizes []*armcompute.VirtualMachineSize) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.data[location] = sizes
	c.expiry[location] = time.Now().Add(c.ttl)
}

// NewVMSizeManager creates a new VMSizeManager
func NewVMSizeManager(vmSizeClient interfaces.VMSizeClient) *VMSizeManagerImpl {
	return &VMSizeManagerImpl{
		vmSizeClient: vmSizeClient,
		cache:        newVMSizeCache(30 * time.Minute),
	}
}

// ListVMSizes lists all available VM sizes in a location
func (m *VMSizeManagerImpl) ListVMSizes(ctx context.Context, location string) ([]*armcompute.VirtualMachineSize, error) {
	if sizes, ok := m.cache.get(location); ok {
		vlog.Debug("VM sizes retrieved from cache", "location", location, "count", len(sizes))
		return sizes, nil
	}

	sizes, err := m.vmSizeClient.List(ctx, location)
	if err != nil {
		return nil, fmt.Errorf("failed to list VM sizes: %w", err)
	}

	m.cache.set(location, sizes)

	vlog.Info("VM sizes fetched from Azure", "location", location, "count", len(sizes))
	return sizes, nil
}

// GetVMSize retrieves a specific VM size
func (m *VMSizeManagerImpl) GetVMSize(ctx context.Context, location, vmSizeName string) (*armcompute.VirtualMachineSize, error) {
	sizes, err := m.ListVMSizes(ctx, location)
	if err != nil {
		return nil, err
	}

	for _, size := range sizes {
		if size.Name != nil && strings.EqualFold(*size.Name, vmSizeName) {
			return size, nil
		}
	}

	return nil, fmt.Errorf("VM size %s not found in location %s", vmSizeName, location)
}

// ValidateVMSize validates if a VM size is available in a location
func (m *VMSizeManagerImpl) ValidateVMSize(ctx context.Context, location, vmSizeName string) (bool, error) {
	size, err := m.GetVMSize(ctx, location, vmSizeName)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			return false, nil
		}
		return false, err
	}
	return size != nil, nil
}

// GetRecommendedVMSizes returns recommended VM sizes for a workload type
func (m *VMSizeManagerImpl) GetRecommendedVMSizes(ctx context.Context, location string, workloadType string) ([]*armcompute.VirtualMachineSize, error) {
	sizes, err := m.ListVMSizes(ctx, location)
	if err != nil {
		return nil, err
	}

	var recommended []*armcompute.VirtualMachineSize
	for _, size := range sizes {
		if size.Name == nil {
			continue
		}
		name := *size.Name

		switch strings.ToLower(workloadType) {
		case "general", "general-purpose":
			if strings.HasPrefix(name, "Standard_D") || strings.HasPrefix(name, "Standard_DS") {
				recommended = append(recommended, size)
			}
		case "compute", "compute-optimized":
			if strings.HasPrefix(name, "Standard_F") {
				recommended = append(recommended, size)
			}
		case "memory", "memory-optimized":
			if strings.HasPrefix(name, "Standard_E") {
				recommended = append(recommended, size)
			}
		case workloadTypeGPU:
			if strings.HasPrefix(name, "Standard_N") {
				recommended = append(recommended, size)
			}
		case "burstable":
			if strings.HasPrefix(name, "Standard_B") {
				recommended = append(recommended, size)
			}
		case "storage", "storage-optimized":
			if strings.HasPrefix(name, "Standard_L") {
				recommended = append(recommended, size)
			}
		default:
			if strings.HasPrefix(name, "Standard_DS") && strings.Contains(name, "_v2") {
				recommended = append(recommended, size)
			}
		}
	}

	vlog.Info("Recommended VM sizes for workload type", "location", location, "workloadType", workloadType, "count", len(recommended))
	return recommended, nil
}

// VMSizeSpecs represents the specifications of a VM size
type VMSizeSpecs struct {
	Name               string `json:"name"`
	Cores              int    `json:"cores"`
	MemoryMB           int    `json:"memoryMB"`
	MaxDataDisks       int    `json:"maxDataDisks"`
	OSDiskSizeMB       int    `json:"osDiskSizeMB"`
	ResourceDiskSizeMB int    `json:"resourceDiskSizeMB"`
}

// GetVMSizeSpecs returns the specifications of a VM size
func (m *VMSizeManagerImpl) GetVMSizeSpecs(ctx context.Context, location, vmSizeName string) (*VMSizeSpecs, error) {
	size, err := m.GetVMSize(ctx, location, vmSizeName)
	if err != nil {
		return nil, err
	}

	specs := &VMSizeSpecs{
		Name: vmSizeName,
	}

	if size.NumberOfCores != nil {
		specs.Cores = int(*size.NumberOfCores)
	}
	if size.MemoryInMB != nil {
		specs.MemoryMB = int(*size.MemoryInMB)
	}
	if size.MaxDataDiskCount != nil {
		specs.MaxDataDisks = int(*size.MaxDataDiskCount)
	}
	if size.OSDiskSizeInMB != nil {
		specs.OSDiskSizeMB = int(*size.OSDiskSizeInMB)
	}
	if size.ResourceDiskSizeInMB != nil {
		specs.ResourceDiskSizeMB = int(*size.ResourceDiskSizeInMB)
	}

	return specs, nil
}
