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

package mock

import (
	"context"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/NVIDIA/infra-controller-rest/rla/internal/task/componentmanager"
	"github.com/NVIDIA/infra-controller-rest/rla/internal/task/executor/temporalworkflow/common"
	"github.com/NVIDIA/infra-controller-rest/rla/internal/task/operations"
	"github.com/NVIDIA/infra-controller-rest/rla/pkg/common/devicetypes"
)

const (
	// ImplementationName is the name used to identify mock implementations.
	ImplementationName = "mock"

	// DefaultDelay is the simulated delay for mock operations.
	DefaultDelay = time.Second
)

// Manager is a mock component manager for testing and development.
type Manager struct {
	componentType devicetypes.ComponentType
	delay         time.Duration
}

// New creates a new mock Manager for the specified component type.
func New(componentType devicetypes.ComponentType) *Manager {
	return &Manager{
		componentType: componentType,
		delay:         DefaultDelay,
	}
}

// NewWithDelay creates a new mock Manager with a custom delay.
func NewWithDelay(componentType devicetypes.ComponentType, delay time.Duration) *Manager {
	return &Manager{
		componentType: componentType,
		delay:         delay,
	}
}

// FactoryFor creates a factory function for the specified component type.
func FactoryFor(componentType devicetypes.ComponentType) componentmanager.ManagerFactory {
	return func(providers *componentmanager.ProviderRegistry) (componentmanager.ComponentManager, error) {
		return New(componentType), nil
	}
}

// RegisterAll registers mock factories for all component types.
func RegisterAll(registry *componentmanager.Registry) {
	for _, ct := range []devicetypes.ComponentType{
		devicetypes.ComponentTypeCompute,
		devicetypes.ComponentTypeNVLSwitch,
		devicetypes.ComponentTypePowerShelf,
	} {
		registry.RegisterFactory(ct, ImplementationName, FactoryFor(ct))
	}
}

// Type returns the component type this manager handles.
func (m *Manager) Type() devicetypes.ComponentType {
	return m.componentType
}

// InjectExpectation simulates injecting expected configuration.
func (m *Manager) InjectExpectation(
	ctx context.Context,
	target common.Target,
	info operations.InjectExpectationTaskInfo,
) error {
	log.Debug().
		Str("component_type", m.componentType.String()).
		Str("target", target.String()).
		Msg("Mock: InjectExpectation")

	time.Sleep(m.delay)
	return nil
}

// PowerControl simulates power operations.
func (m *Manager) PowerControl(
	ctx context.Context,
	target common.Target,
	info operations.PowerControlTaskInfo,
) error {
	log.Debug().
		Str("component_type", m.componentType.String()).
		Str("target", target.String()).
		Str("operation", info.Operation.String()).
		Msg("Mock: PowerControl")

	time.Sleep(m.delay)

	log.Info().
		Str("component_type", m.componentType.String()).
		Str("target", target.String()).
		Str("operation", info.Operation.String()).
		Msg("Mock: PowerControl completed")

	return nil
}

// GetPowerStatus simulates getting power status.
func (m *Manager) GetPowerStatus(
	ctx context.Context,
	target common.Target,
) (map[string]operations.PowerStatus, error) {
	log.Debug().
		Str("component_type", m.componentType.String()).
		Str("target", target.String()).
		Msg("Mock: GetPowerStatus")

	time.Sleep(m.delay)

	result := make(map[string]operations.PowerStatus)
	for _, componentID := range target.ComponentIDs {
		result[componentID] = operations.PowerStatusOn
	}

	log.Info().
		Str("component_type", m.componentType.String()).
		Str("target", target.String()).
		Int("component_count", len(result)).
		Msg("Mock: GetPowerStatus completed")

	return result, nil
}

// FirmwareControl simulates initiating firmware update without waiting for completion.
func (m *Manager) FirmwareControl(
	ctx context.Context,
	target common.Target,
	info operations.FirmwareControlTaskInfo,
) error {
	log.Debug().
		Str("component_type", m.componentType.String()).
		Str("target", target.String()).
		Str("target_version", info.TargetVersion).
		Msg("Mock: FirmwareControl")

	time.Sleep(m.delay)

	log.Info().
		Str("component_type", m.componentType.String()).
		Str("target", target.String()).
		Msg("Mock: FirmwareControl completed")

	return nil
}

// BringUpControl simulates opening the bring-up gate.
func (m *Manager) BringUpControl(
	ctx context.Context,
	target common.Target,
) error {
	log.Debug().
		Str("component_type", m.componentType.String()).
		Str("target", target.String()).
		Msg("Mock: BringUpControl")
	time.Sleep(m.delay)
	return nil
}

// GetBringUpStatus simulates getting bring-up status.
func (m *Manager) GetBringUpStatus(
	ctx context.Context,
	target common.Target,
) (map[string]operations.MachineBringUpState, error) {
	log.Debug().
		Str("component_type", m.componentType.String()).
		Str("target", target.String()).
		Msg("Mock: GetBringUpStatus")
	time.Sleep(m.delay)

	result := make(
		map[string]operations.MachineBringUpState,
		len(target.ComponentIDs),
	)
	for _, id := range target.ComponentIDs {
		result[id] = operations.MachineBringUpStateMachineCreated
	}
	return result, nil
}

// GetFirmwareStatus simulates getting firmware update status.
func (m *Manager) GetFirmwareStatus(
	ctx context.Context,
	target common.Target,
) (map[string]operations.FirmwareUpdateStatus, error) {
	log.Debug().
		Str("component_type", m.componentType.String()).
		Str("target", target.String()).
		Msg("Mock: GetFirmwareStatus")

	time.Sleep(m.delay)

	result := make(map[string]operations.FirmwareUpdateStatus)
	for _, componentID := range target.ComponentIDs {
		result[componentID] = operations.FirmwareUpdateStatus{
			ComponentID: componentID,
			State:       operations.FirmwareUpdateStateCompleted,
			Error:       "",
		}
	}

	log.Info().
		Str("component_type", m.componentType.String()).
		Str("target", target.String()).
		Int("component_count", len(result)).
		Msg("Mock: GetFirmwareStatus completed")

	return result, nil
}
