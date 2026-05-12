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

// MachineClass size names — the lower-case keys used across the static
// fallback mappings. Kept here so the literal "small"/"medium"/... only
// appears once in the package.
//
// Note: the "gpu" key is intentionally not duplicated here; use
// workloadTypeGPU (defined in vm_size_manager.go) for that value.
const (
	machineClassSmall       = "small"
	machineClassMedium      = "medium"
	machineClassLarge       = "large"
	machineClassXLarge      = "xlarge"
	machineClassLargeCPU    = "largecpu"
	machineClassLargeMemory = "largememory"
)

// Azure VM SKU names referenced by the static fallback mappings. These are
// duplicated across agent_pool_manager, aks_cluster_manager, and
// machineclass_mapper; declaring them once keeps the lists trivially
// reviewable when Azure deprecates a SKU.
const (
	vmSizeStandardD2sV5  = "Standard_D2s_v5"
	vmSizeStandardD4sV5  = "Standard_D4s_v5"
	vmSizeStandardD8sV5  = "Standard_D8s_v5"
	vmSizeStandardD16sV5 = "Standard_D16s_v5"

	vmSizeStandardF2sV2  = "Standard_F2s_v2"
	vmSizeStandardF4sV2  = "Standard_F4s_v2"
	vmSizeStandardF8sV2  = "Standard_F8s_v2"
	vmSizeStandardF16sV2 = "Standard_F16s_v2"

	vmSizeStandardE2sV5  = "Standard_E2s_v5"
	vmSizeStandardE4sV5  = "Standard_E4s_v5"
	vmSizeStandardE8sV5  = "Standard_E8s_v5"
	vmSizeStandardE16sV5 = "Standard_E16s_v5"

	vmSizeStandardNC6sV3     = "Standard_NC6s_v3"
	vmSizeStandardNC4asT4V3  = "Standard_NC4as_T4_v3"
	vmSizeStandardNC8asT4V3  = "Standard_NC8as_T4_v3"
	vmSizeStandardNC16asT4V3 = "Standard_NC16as_T4_v3"
	vmSizeStandardNC64asT4V3 = "Standard_NC64as_T4_v3"
)
