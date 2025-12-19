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

import "math"

// safeIntToInt32 safely converts an int to int32, clamping to valid range.
// This prevents integer overflow issues (G115) when converting to Azure SDK types.
func safeIntToInt32(v int) int32 {
	if v > math.MaxInt32 {
		return math.MaxInt32
	}
	if v < math.MinInt32 {
		return math.MinInt32
	}
	return int32(v) // #nosec G115 -- bounds checked above
}

// safeUintToInt safely converts a uint to int, clamping to valid range.
// This prevents integer overflow issues (G115) when converting from MachineClass specs.
func safeUintToInt(v uint) int {
	if v > math.MaxInt {
		return math.MaxInt
	}
	return int(v) // #nosec G115 -- bounds checked above
}
