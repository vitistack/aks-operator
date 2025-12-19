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
	"math"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/compute/armcompute/v6"
	"github.com/vitistack/aks-operator/pkg/interfaces"
	"github.com/vitistack/common/pkg/loggers/vlog"
	vitistackv1alpha1 "github.com/vitistack/common/pkg/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// MachineClassMapperImpl implements the MachineClassMapper interface
type MachineClassMapperImpl struct {
	k8sClient     client.Client
	vmSizeManager interfaces.VMSizeManager
	cache         *machineClassCache
}

type machineClassCache struct {
	mu     sync.RWMutex
	data   map[string]*vitistackv1alpha1.MachineClass
	expiry time.Time
	ttl    time.Duration
}

func newMachineClassCache(ttl time.Duration) *machineClassCache {
	return &machineClassCache{
		data: make(map[string]*vitistackv1alpha1.MachineClass),
		ttl:  ttl,
	}
}

func (c *machineClassCache) get(name string) (*vitistackv1alpha1.MachineClass, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if time.Now().After(c.expiry) {
		return nil, false
	}

	mc, ok := c.data[name]
	return mc, ok
}

func (c *machineClassCache) getAll() (map[string]*vitistackv1alpha1.MachineClass, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if time.Now().After(c.expiry) {
		return nil, false
	}

	return c.data, len(c.data) > 0
}

func (c *machineClassCache) set(classes []*vitistackv1alpha1.MachineClass) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.data = make(map[string]*vitistackv1alpha1.MachineClass)
	for _, mc := range classes {
		c.data[mc.Name] = mc
	}
	c.expiry = time.Now().Add(c.ttl)
}

// NewMachineClassMapper creates a new MachineClassMapper
func NewMachineClassMapper(k8sClient client.Client, vmSizeManager interfaces.VMSizeManager) *MachineClassMapperImpl {
	return &MachineClassMapperImpl{
		k8sClient:     k8sClient,
		vmSizeManager: vmSizeManager,
		cache:         newMachineClassCache(5 * time.Minute),
	}
}

// MapMachineClassToVMSize maps a MachineClass name to the best matching Azure VM size
func (m *MachineClassMapperImpl) MapMachineClassToVMSize(ctx context.Context, machineClassName string, location string, preferredPurpose interfaces.VMPurpose) (*interfaces.MachineClassMapping, error) {
	machineClass, err := m.GetMachineClass(ctx, machineClassName)
	if err != nil {
		// If MachineClass not found, try static mapping as fallback
		vlog.Info("MachineClass not found, using static mapping", "machineClass", machineClassName)
		return m.staticMapping(machineClassName, preferredPurpose), nil
	}

	return m.MapMachineClassSpecToVMSize(ctx, machineClass, location, preferredPurpose)
}

// MapMachineClassSpecToVMSize maps a MachineClass spec directly to an Azure VM size
func (m *MachineClassMapperImpl) MapMachineClassSpecToVMSize(ctx context.Context, machineClass *vitistackv1alpha1.MachineClass, location string, preferredPurpose interfaces.VMPurpose) (*interfaces.MachineClassMapping, error) {
	// Get available VM sizes in the location
	vmSizes, err := m.vmSizeManager.ListVMSizes(ctx, location)
	if err != nil {
		return nil, fmt.Errorf("failed to list VM sizes: %w", err)
	}

	// Determine the purpose to use
	purpose := preferredPurpose
	if purpose == "" {
		purpose = m.GetDefaultPurposeForCategory(machineClass.Spec.Category)
	}

	// Extract requirements from MachineClass
	requiredCores := safeUintToInt(machineClass.Spec.CPU.Cores)
	requiredMemoryGB := float64(machineClass.Spec.Memory.Quantity.Value()) / (1024 * 1024 * 1024)
	hasGPU := machineClass.Spec.GPU.Cores > 0

	vlog.Info("Mapping MachineClass to VM size",
		"machineClass", machineClass.Name,
		"category", machineClass.Spec.Category,
		"cores", requiredCores,
		"memoryGB", requiredMemoryGB,
		"hasGPU", hasGPU,
		"preferredPurpose", purpose,
	)

	// Score and rank VM sizes
	type scoredVM struct {
		size  *armcompute.VirtualMachineSize
		score float64
	}

	candidates := make([]scoredVM, 0, len(vmSizes))
	for _, vmSize := range vmSizes {
		if vmSize.Name == nil || vmSize.NumberOfCores == nil || vmSize.MemoryInMB == nil {
			continue
		}

		// Filter by purpose
		if !m.vmSizeMatchesPurpose(*vmSize.Name, purpose, hasGPU) {
			continue
		}

		vmCores := int(*vmSize.NumberOfCores)
		vmMemoryGB := float64(*vmSize.MemoryInMB) / 1024

		// Skip VMs that don't meet minimum requirements
		if vmCores < requiredCores || vmMemoryGB < requiredMemoryGB {
			continue
		}

		// Calculate score (lower is better)
		score := m.calculateScore(requiredCores, requiredMemoryGB, vmCores, vmMemoryGB, *vmSize.Name)
		candidates = append(candidates, scoredVM{size: vmSize, score: score})
	}

	if len(candidates) == 0 {
		// Fallback to static mapping if no candidates found
		vlog.Info("No suitable VM sizes found, using static mapping", "machineClass", machineClass.Name)
		return m.staticMapping(machineClass.Name, purpose), nil
	}

	// Sort by score
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].score < candidates[j].score
	})

	// Build result
	best := candidates[0]
	mapping := &interfaces.MachineClassMapping{
		MachineClassName: machineClass.Name,
		VMSize:           *best.size.Name,
		VMPurpose:        purpose,
		Cores:            int(*best.size.NumberOfCores),
		MemoryGB:         float64(*best.size.MemoryInMB) / 1024,
		Score:            best.score,
	}

	// Add alternatives (next 3 best matches)
	for i := 1; i < len(candidates) && i <= 3; i++ {
		mapping.Alternatives = append(mapping.Alternatives, *candidates[i].size.Name)
	}

	if hasGPU {
		mapping.GPUCores = safeUintToInt(machineClass.Spec.GPU.Cores)
	}

	vlog.Info("Mapped MachineClass to VM size",
		"machineClass", machineClass.Name,
		"vmSize", mapping.VMSize,
		"purpose", mapping.VMPurpose,
		"score", mapping.Score,
		"alternatives", mapping.Alternatives,
	)

	return mapping, nil
}

// GetMachineClass retrieves a MachineClass CRD by name
func (m *MachineClassMapperImpl) GetMachineClass(ctx context.Context, name string) (*vitistackv1alpha1.MachineClass, error) {
	// Check cache first
	if mc, ok := m.cache.get(name); ok {
		return mc, nil
	}

	// Refresh cache
	if _, err := m.ListMachineClasses(ctx); err != nil {
		return nil, err
	}

	// Check cache again
	if mc, ok := m.cache.get(name); ok {
		return mc, nil
	}

	return nil, fmt.Errorf("MachineClass %s not found", name)
}

// ListMachineClasses lists all enabled MachineClasses
func (m *MachineClassMapperImpl) ListMachineClasses(ctx context.Context) ([]*vitistackv1alpha1.MachineClass, error) {
	if cached, ok := m.cache.getAll(); ok {
		result := make([]*vitistackv1alpha1.MachineClass, 0, len(cached))
		for _, mc := range cached {
			if mc.Spec.Enabled {
				result = append(result, mc)
			}
		}
		return result, nil
	}

	var machineClassList vitistackv1alpha1.MachineClassList
	if err := m.k8sClient.List(ctx, &machineClassList); err != nil {
		return nil, fmt.Errorf("failed to list MachineClasses: %w", err)
	}

	allClasses := make([]*vitistackv1alpha1.MachineClass, 0, len(machineClassList.Items))
	enabledClasses := make([]*vitistackv1alpha1.MachineClass, 0)
	for i := range machineClassList.Items {
		mc := &machineClassList.Items[i]
		allClasses = append(allClasses, mc)
		if mc.Spec.Enabled {
			enabledClasses = append(enabledClasses, mc)
		}
	}

	m.cache.set(allClasses)
	vlog.Info("Refreshed MachineClass cache", "total", len(allClasses), "enabled", len(enabledClasses))

	return enabledClasses, nil
}

// GetDefaultPurposeForCategory returns the default VM purpose for a MachineClass category
func (m *MachineClassMapperImpl) GetDefaultPurposeForCategory(category string) interfaces.VMPurpose {
	switch strings.ToLower(category) {
	case "cpu", "compute":
		return interfaces.VMPurposeComputeOptimized
	case "memory":
		return interfaces.VMPurposeMemoryOptimized
	case "gpu":
		return interfaces.VMPurposeGPU
	case "storage":
		return interfaces.VMPurposeStorageOptimized
	case "burstable":
		return interfaces.VMPurposeBurstable
	case "standard", "general":
		return interfaces.VMPurposeGeneralPurpose
	default:
		// Default to general purpose for unknown categories
		return interfaces.VMPurposeGeneralPurpose
	}
}

// ValidateMapping validates if a mapping is available in a location
func (m *MachineClassMapperImpl) ValidateMapping(ctx context.Context, machineClassName string, location string) (bool, error) {
	mapping, err := m.MapMachineClassToVMSize(ctx, machineClassName, location, "")
	if err != nil {
		return false, err
	}

	// Verify the VM size is available in the location
	return m.vmSizeManager.ValidateVMSize(ctx, location, mapping.VMSize)
}

// vmSizeMatchesPurpose checks if a VM size name matches the given purpose
func (m *MachineClassMapperImpl) vmSizeMatchesPurpose(vmSizeName string, purpose interfaces.VMPurpose, hasGPU bool) bool {
	name := strings.ToUpper(vmSizeName)

	// If GPU is required, only match GPU VMs
	if hasGPU {
		return strings.Contains(name, "STANDARD_N") || strings.Contains(name, "_GPU")
	}

	switch purpose {
	case interfaces.VMPurposeGeneralPurpose:
		// D-series, Ds-series, Dv2-series, Dsv2-series, Dv3-series, Dsv3-series, etc.
		return (strings.HasPrefix(name, "STANDARD_D") &&
			!strings.Contains(name, "_DC") &&
			!strings.Contains(name, "_ND")) ||
			strings.HasPrefix(name, "STANDARD_A")

	case interfaces.VMPurposeComputeOptimized:
		// F-series, Fsv2-series
		return strings.HasPrefix(name, "STANDARD_F")

	case interfaces.VMPurposeMemoryOptimized:
		// E-series, Esv3-series, M-series
		return strings.HasPrefix(name, "STANDARD_E") ||
			strings.HasPrefix(name, "STANDARD_M")

	case interfaces.VMPurposeGPU:
		// N-series (NC, ND, NV)
		return strings.HasPrefix(name, "STANDARD_N")

	case interfaces.VMPurposeBurstable:
		// B-series
		return strings.HasPrefix(name, "STANDARD_B")

	case interfaces.VMPurposeStorageOptimized:
		// L-series
		return strings.HasPrefix(name, "STANDARD_L")

	default:
		// Default: accept general purpose VMs
		return strings.HasPrefix(name, "STANDARD_D") || strings.HasPrefix(name, "STANDARD_A")
	}
}

// calculateScore calculates a matching score for a VM size (lower is better)
func (m *MachineClassMapperImpl) calculateScore(
	requiredCores int,
	requiredMemoryGB float64,
	vmCores int,
	vmMemoryGB float64,
	vmSizeName string,
) float64 {
	// Calculate how much over the requirements the VM is
	coreExcess := float64(vmCores-requiredCores) / float64(requiredCores)
	memExcess := (vmMemoryGB - requiredMemoryGB) / requiredMemoryGB

	// Base score is the weighted sum of excess resources
	// We want to minimize waste but prioritize meeting requirements
	score := coreExcess*0.4 + memExcess*0.4

	// Bonus for v3/v4/v5 series (newer, more efficient)
	if strings.Contains(vmSizeName, "_v5") {
		score -= 0.15
	} else if strings.Contains(vmSizeName, "_v4") {
		score -= 0.10
	} else if strings.Contains(vmSizeName, "_v3") {
		score -= 0.05
	} else if strings.Contains(vmSizeName, "_v2") {
		score -= 0.02
	}

	// Bonus for "s" suffix (premium storage capable)
	if strings.Contains(vmSizeName, "s_") || strings.HasSuffix(vmSizeName, "s") {
		score -= 0.05
	}

	// Penalty for promo SKUs
	if strings.Contains(strings.ToLower(vmSizeName), "promo") {
		score += 0.3
	}

	// Penalty for very old series
	if strings.HasPrefix(vmSizeName, "Standard_A") && !strings.Contains(vmSizeName, "_v") {
		score += 0.5
	}

	// Ensure score is not negative
	return math.Max(score, 0)
}

// staticMapping provides a static fallback mapping when MachineClass is not found
func (m *MachineClassMapperImpl) staticMapping(machineClassName string, purpose interfaces.VMPurpose) *interfaces.MachineClassMapping {
	// Define static mappings based on machine class name and purpose
	type staticDef struct {
		generalPurpose   string
		computeOptimized string
		memoryOptimized  string
		cores            int
		memoryGB         float64
	}

	staticMappings := map[string]staticDef{
		"small": {
			generalPurpose:   "Standard_D2s_v5",
			computeOptimized: "Standard_F2s_v2",
			memoryOptimized:  "Standard_E2s_v5",
			cores:            2,
			memoryGB:         8,
		},
		"medium": {
			generalPurpose:   "Standard_D4s_v5",
			computeOptimized: "Standard_F4s_v2",
			memoryOptimized:  "Standard_E4s_v5",
			cores:            4,
			memoryGB:         16,
		},
		"large": {
			generalPurpose:   "Standard_D8s_v5",
			computeOptimized: "Standard_F8s_v2",
			memoryOptimized:  "Standard_E8s_v5",
			cores:            8,
			memoryGB:         32,
		},
		"xlarge": {
			generalPurpose:   "Standard_D16s_v5",
			computeOptimized: "Standard_F16s_v2",
			memoryOptimized:  "Standard_E16s_v5",
			cores:            16,
			memoryGB:         64,
		},
		"largecpu": {
			generalPurpose:   "Standard_F8s_v2",
			computeOptimized: "Standard_F8s_v2",
			memoryOptimized:  "Standard_F8s_v2",
			cores:            8,
			memoryGB:         16,
		},
		"largememory": {
			generalPurpose:   "Standard_E8s_v5",
			computeOptimized: "Standard_E8s_v5",
			memoryOptimized:  "Standard_E8s_v5",
			cores:            8,
			memoryGB:         64,
		},
		"gpu": {
			generalPurpose:   "Standard_NC6s_v3",
			computeOptimized: "Standard_NC6s_v3",
			memoryOptimized:  "Standard_NC6s_v3",
			cores:            6,
			memoryGB:         112,
		},
	}

	name := strings.ToLower(machineClassName)
	def, ok := staticMappings[name]
	if !ok {
		// Default to medium
		def = staticMappings["medium"]
	}

	var vmSize string
	switch purpose {
	case interfaces.VMPurposeComputeOptimized:
		vmSize = def.computeOptimized
	case interfaces.VMPurposeMemoryOptimized:
		vmSize = def.memoryOptimized
	case interfaces.VMPurposeGPU:
		if name == "gpu" {
			vmSize = def.generalPurpose
		} else {
			vmSize = "Standard_NC6s_v3"
		}
	default:
		vmSize = def.generalPurpose
	}

	// If machineClassName is already a Standard_* VM size, use it directly
	if strings.HasPrefix(machineClassName, "Standard_") {
		vmSize = machineClassName
	}

	return &interfaces.MachineClassMapping{
		MachineClassName: machineClassName,
		VMSize:           vmSize,
		VMPurpose:        purpose,
		Cores:            def.cores,
		MemoryGB:         def.memoryGB,
	}
}
