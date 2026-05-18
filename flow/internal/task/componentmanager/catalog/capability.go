/*
 * SPDX-FileCopyrightText: Copyright (c) 2026 NVIDIA CORPORATION & AFFILIATES. All rights reserved.
 * SPDX-License-Identifier: Apache-2.0
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 * http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package catalog

import (
	"slices"
	"strings"
)

// Capability identifies an operation supported by a component manager
// implementation.
type Capability string

const (
	CapabilityInjectExpectation        Capability = "InjectExpectation"
	CapabilityPowerControl             Capability = "PowerControl"
	CapabilityPowerStatus              Capability = "PowerStatus"
	CapabilityFirmwareControl          Capability = "FirmwareControl"
	CapabilityFirmwareStatus           Capability = "FirmwareStatus"
	CapabilityBringUpControl           Capability = "BringUpControl"
	CapabilityBringUpStatus            Capability = "BringUpStatus"
	CapabilityFirmwareConsistencyCheck Capability = "FirmwareConsistencyCheck"
)

// ParseCapability trims and validates a component manager capability name.
func ParseCapability(name string) (Capability, error) {
	s := strings.TrimSpace(name)
	if s == "" {
		return "", CapabilityNameEmptyError{}
	}

	c := Capability(s)
	if !c.Valid() {
		return "", UnknownCapabilityError{Capability: c}
	}

	return c, nil
}

// String returns the capability name.
func (c Capability) String() string {
	return string(c)
}

// Valid reports whether c is one of the known component manager capabilities.
func (c Capability) Valid() bool {
	switch c {
	case CapabilityInjectExpectation,
		CapabilityPowerControl,
		CapabilityPowerStatus,
		CapabilityFirmwareControl,
		CapabilityFirmwareStatus,
		CapabilityBringUpControl,
		CapabilityBringUpStatus,
		CapabilityFirmwareConsistencyCheck:
		return true
	default:
		return false
	}
}

// Normalize trims and validates a capability name.
func (c Capability) Normalize() (Capability, error) {
	return ParseCapability(c.String())
}

// CapabilitySet is the normalized set of operations supported by a component
// manager implementation.
type CapabilitySet []Capability

// Normalize returns capabilities trimmed, deduplicated, and sorted.
func (s CapabilitySet) Normalize() (CapabilitySet, error) {
	if len(s) == 0 {
		return nil, nil
	}

	capabilities := make(CapabilitySet, 0, len(s))
	seen := make(map[Capability]struct{}, len(s))
	for _, capability := range s {
		capability, err := capability.Normalize()
		if err != nil {
			return nil, err
		}
		if _, ok := seen[capability]; ok {
			continue
		}
		seen[capability] = struct{}{}
		capabilities = append(capabilities, capability)
	}

	slices.Sort(capabilities)

	return capabilities, nil
}

// Clone returns a capability set copy that does not share storage with the
// source set.
func (s CapabilitySet) Clone() CapabilitySet {
	return slices.Clone(s)
}

// Strings returns the capability names as strings.
func (s CapabilitySet) Strings() []string {
	names := make([]string, 0, len(s))
	for _, c := range s {
		names = append(names, c.String())
	}
	return names
}
