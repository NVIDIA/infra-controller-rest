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

package componentmanager

import (
	"context"
	"slices"
	"strings"
	"sync"

	cmconfig "github.com/NVIDIA/infra-controller-rest/flow/internal/task/componentmanager/config"
	"github.com/NVIDIA/infra-controller-rest/flow/internal/task/componentmanager/providerapi"
	"github.com/NVIDIA/infra-controller-rest/flow/internal/task/executor/temporalworkflow/common"
	"github.com/NVIDIA/infra-controller-rest/flow/internal/task/operations"
	"github.com/NVIDIA/infra-controller-rest/flow/pkg/common/devicetypes"
)

// ComponentManager defines the interface for managing various types of
// components. Implementations handle component-specific operations like
// power control, firmware management, and status monitoring.
type ComponentManager interface {
	// Type returns the component type this manager is responsible for.
	Type() devicetypes.ComponentType

	// InjectExpectation registers expected component configurations with the
	// component manager service for the target components.
	InjectExpectation(ctx context.Context, target common.Target, info operations.InjectExpectationTaskInfo) error //nolint

	// PowerControl applies a power state transition to the target components.
	PowerControl(ctx context.Context, target common.Target, info operations.PowerControlTaskInfo) error //nolint

	// GetPowerStatus queries the current power state of each component in the
	// target. Returns a map of component ID to PowerStatus.
	GetPowerStatus(ctx context.Context, target common.Target) (map[string]operations.PowerStatus, error) //nolint

	// FirmwareControl initiates a firmware update without waiting for completion.
	// Returns immediately after the update request is accepted.
	FirmwareControl(ctx context.Context, target common.Target, info operations.FirmwareControlTaskInfo) error //nolint

	// GetFirmwareStatus returns the current firmware update state for each
	// component in the target. Returns a map of component ID to FirmwareUpdateStatus.
	GetFirmwareStatus(ctx context.Context, target common.Target) (map[string]operations.FirmwareUpdateStatus, error) //nolint
}

// BringUpController is an optional interface for component managers that support
// bring-up operations.
type BringUpController interface {
	// BringUpControl opens the power-on gate for the target components, allowing
	// them to proceed through the bring-up sequence.
	BringUpControl(ctx context.Context, target common.Target) error

	// GetBringUpStatus returns the current bring-up state for each component in
	// the target. Returns a map of component ID to MachineBringUpState.
	GetBringUpStatus(ctx context.Context, target common.Target) (map[string]operations.MachineBringUpState, error)
}

// FirmwareConsistencyChecker is an optional interface for component managers
// that can verify firmware version consistency across a set of components.
type FirmwareConsistencyChecker interface {
	// VerifyFirmwareConsistency checks that all target components report the same
	// firmware version set. Returns an error if versions diverge.
	VerifyFirmwareConsistency(ctx context.Context, target common.Target) error
}

// ManagerFactory is a function that creates a ComponentManager instance.
// It receives a ProviderRegistry from which it can retrieve the providers it needs.
type ManagerFactory func(providers *providerapi.ProviderRegistry) (ComponentManager, error)

// Descriptor describes a component manager implementation registered in
// this process. The descriptor identity is Type plus Implementation; provider
// names stay separate because one manager can require multiple providers and
// one provider can serve multiple component manager implementations.
type Descriptor struct {
	Type              devicetypes.ComponentType
	Implementation    string
	RequiredProviders []string
	Factory           ManagerFactory
}

// Catalog contains the component manager implementations supported by a
// particular binary. Service-specific packages such as builtin own the list of
// descriptors that goes into a catalog.
type Catalog struct {
	descriptors map[devicetypes.ComponentType]map[string]Descriptor // type -> impl_name -> descriptor
}

// NewCatalog validates descriptors and indexes them by component type and
// implementation.
func NewCatalog(descriptors []Descriptor) (Catalog, error) {
	catalog := Catalog{
		descriptors: make(map[devicetypes.ComponentType]map[string]Descriptor),
	}

	for _, descriptor := range descriptors {
		descriptor, err := descriptor.normalize()
		if err != nil {
			return Catalog{}, err
		}

		if _, ok := catalog.descriptors[descriptor.Type]; !ok {
			catalog.descriptors[descriptor.Type] = make(map[string]Descriptor)
		}

		if _, exists := catalog.descriptors[descriptor.Type][descriptor.Implementation]; exists {
			return Catalog{}, DuplicateDescriptorError{
				ComponentType:  descriptor.Type,
				Implementation: descriptor.Implementation,
			}
		}

		catalog.descriptors[descriptor.Type][descriptor.Implementation] = descriptor
	}

	return catalog, nil
}

// Get returns the descriptor for a component type and implementation.
func (c Catalog) Get(
	componentType devicetypes.ComponentType,
	implementation string,
) (Descriptor, bool) {
	descriptors := c.descriptors[componentType]
	if descriptors == nil {
		return Descriptor{}, false
	}

	descriptor, ok := descriptors[implementation]
	return descriptor, ok
}

// Implementations returns the implementations registered for a component type.
func (c Catalog) Implementations(
	componentType devicetypes.ComponentType,
) []string {
	return descriptorImplementationNames(c.descriptors[componentType])
}

// ListImplementations returns all registered implementation names by component
// type.
func (c Catalog) ListImplementations() map[devicetypes.ComponentType][]string {
	result := make(map[devicetypes.ComponentType][]string)
	for componentType, descriptors := range c.descriptors {
		result[componentType] = descriptorImplementationNames(descriptors)
	}
	return result
}

func (c Catalog) componentTypesForImplementation(
	implementation string,
) []devicetypes.ComponentType {
	types := make([]devicetypes.ComponentType, 0)
	for componentType, descriptors := range c.descriptors {
		if _, ok := descriptors[implementation]; ok {
			types = append(types, componentType)
		}
	}
	slices.Sort(types)
	return types
}

type activeManager struct {
	descriptor Descriptor
	manager    ComponentManager
}

// Registry maintains the active component managers selected from a catalog.
type Registry struct {
	mu     sync.RWMutex
	active map[devicetypes.ComponentType]activeManager
}

// NewRegistry creates and initializes a Registry from the supplied catalog and
// component manager configuration.
func NewRegistry(
	catalog Catalog,
	config cmconfig.Config,
	providers *providerapi.ProviderRegistry,
) (*Registry, error) {
	registry := &Registry{
		active: make(map[devicetypes.ComponentType]activeManager),
	}

	if err := registry.initialize(catalog, config, providers); err != nil {
		return nil, err
	}

	return registry, nil
}

func (r *Registry) initialize(
	catalog Catalog,
	config cmconfig.Config,
	providers *providerapi.ProviderRegistry,
) error {
	activeManagers := make(
		map[devicetypes.ComponentType]activeManager,
		len(config.ComponentManagers),
	)

	for componentType, implName := range config.ComponentManagers {
		descriptor, ok := catalog.Get(componentType, implName)
		if !ok {
			available := catalog.Implementations(componentType)
			if len(available) == 0 {
				return ComponentManagerFactoryNotRegisteredError{
					ComponentType: componentType,
				}
			}

			return UnknownComponentManagerImplementationError{
				ComponentType:  componentType,
				Implementation: implName,
				Available:      available,
				RegisteredFor:  catalog.componentTypesForImplementation(implName),
			}
		}

		if descriptor.Factory == nil {
			return ComponentManagerFactoryNotRegisteredError{
				ComponentType: componentType,
			}
		}

		manager, err := descriptor.Factory(providers)
		if err != nil {
			return ManagerCreationError{
				ComponentType:  componentType,
				Implementation: implName,
				Err:            err,
			}
		}

		activeManagers[componentType] = activeManager{
			descriptor: descriptor,
			manager:    manager,
		}
	}

	r.mu.Lock()
	r.active = activeManagers
	r.mu.Unlock()

	return nil
}

// FindManager returns the active manager for the specified component type.
// It returns nil when the registry is nil or when no manager is active for the
// type. Use GetManager when the caller needs a descriptive configuration error.
func (r *Registry) FindManager(
	componentType devicetypes.ComponentType,
) ComponentManager {
	if r == nil {
		return nil
	}

	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.active[componentType].manager
}

// GetManager returns the active manager for the specified component type.
// It returns a descriptive error when the registry is nil or when no manager is
// active for the type.
func (r *Registry) GetManager(
	componentType devicetypes.ComponentType,
) (ComponentManager, error) {
	if r == nil {
		return nil, ErrRegistryNotConfigured
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	active := r.active[componentType]
	if active.manager == nil {
		return nil, ManagerNotConfiguredError{ComponentType: componentType}
	}

	return active.manager, nil
}

// GetDescriptor returns the descriptor selected for the specified component
// type.
func (r *Registry) GetDescriptor(
	componentType devicetypes.ComponentType,
) (Descriptor, error) {
	if r == nil {
		return Descriptor{}, ErrRegistryNotConfigured
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	active, ok := r.active[componentType]
	if !ok {
		return Descriptor{}, ManagerNotConfiguredError{ComponentType: componentType}
	}

	return active.descriptor, nil
}

// GetAllManagers returns all active managers.
func (r *Registry) GetAllManagers() []ComponentManager {
	r.mu.RLock()
	defer r.mu.RUnlock()

	managers := make([]ComponentManager, 0, len(r.active))
	for _, active := range r.active {
		managers = append(managers, active.manager)
	}
	return managers
}

func (d Descriptor) normalize() (Descriptor, error) {
	if d.Type == devicetypes.ComponentTypeUnknown {
		return Descriptor{}, UnknownComponentTypeError{
			Name: devicetypes.ComponentTypeToString(d.Type),
		}
	}

	d.Implementation = strings.TrimSpace(d.Implementation)
	if d.Implementation == "" {
		return Descriptor{}, ComponentManagerImplementationNameEmptyError{
			ComponentType: d.Type,
		}
	}

	if d.Factory == nil {
		return Descriptor{}, ComponentManagerFactoryNotConfiguredError{
			ComponentType:  d.Type,
			Implementation: d.Implementation,
		}
	}

	requiredProviders := make([]string, 0, len(d.RequiredProviders))
	seen := make(map[string]struct{}, len(d.RequiredProviders))
	for _, name := range d.RequiredProviders {
		name = strings.TrimSpace(name)
		if name == "" {
			return Descriptor{}, providerapi.ErrProviderNameEmpty
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		requiredProviders = append(requiredProviders, name)
	}
	slices.Sort(requiredProviders)
	d.RequiredProviders = requiredProviders

	return d, nil
}

func descriptorImplementationNames(
	descriptors map[string]Descriptor,
) []string {
	names := make([]string, 0, len(descriptors))
	for name := range descriptors {
		names = append(names, name)
	}
	slices.Sort(names)
	return names
}
