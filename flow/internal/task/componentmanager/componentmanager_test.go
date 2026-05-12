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
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	cmconfig "github.com/NVIDIA/infra-controller-rest/flow/internal/task/componentmanager/config"
	"github.com/NVIDIA/infra-controller-rest/flow/internal/task/componentmanager/providerapi"
	"github.com/NVIDIA/infra-controller-rest/flow/internal/task/executor/temporalworkflow/common"
	"github.com/NVIDIA/infra-controller-rest/flow/internal/task/operations"
	"github.com/NVIDIA/infra-controller-rest/flow/pkg/common/devicetypes"
)

type testManager struct {
	componentType devicetypes.ComponentType
}

func (m testManager) Type() devicetypes.ComponentType {
	return m.componentType
}

func (m testManager) InjectExpectation(
	context.Context,
	common.Target,
	operations.InjectExpectationTaskInfo,
) error {
	return nil
}

func (m testManager) PowerControl(
	context.Context,
	common.Target,
	operations.PowerControlTaskInfo,
) error {
	return nil
}

func (m testManager) GetPowerStatus(
	context.Context,
	common.Target,
) (map[string]operations.PowerStatus, error) {
	return nil, nil
}

func (m testManager) FirmwareControl(
	context.Context,
	common.Target,
	operations.FirmwareControlTaskInfo,
) error {
	return nil
}

func (m testManager) GetFirmwareStatus(
	context.Context,
	common.Target,
) (map[string]operations.FirmwareUpdateStatus, error) {
	return nil, nil
}

func managerFactory(
	componentType devicetypes.ComponentType,
) ManagerFactory {
	return func(*providerapi.ProviderRegistry) (ComponentManager, error) {
		return testManager{componentType: componentType}, nil
	}
}

func TestNewCatalog(t *testing.T) {
	catalog, err := NewCatalog([]Descriptor{
		{
			Type:              devicetypes.ComponentTypeCompute,
			Implementation:    " custom ",
			RequiredProviders: []string{" beta ", "alpha", "beta"},
			Factory:           managerFactory(devicetypes.ComponentTypeCompute),
		},
	})

	require.NoError(t, err)

	descriptor, ok := catalog.Get(devicetypes.ComponentTypeCompute, "custom")
	require.True(t, ok)
	require.Equal(t, devicetypes.ComponentTypeCompute, descriptor.Type)
	require.Equal(t, "custom", descriptor.Implementation)
	require.Equal(t, []string{"alpha", "beta"}, descriptor.RequiredProviders)
	require.NotNil(t, descriptor.Factory)

	require.Equal(
		t,
		[]string{"custom"},
		catalog.Implementations(devicetypes.ComponentTypeCompute),
	)
}

func TestNewCatalogRejectsDuplicate(t *testing.T) {
	descriptor := Descriptor{
		Type:           devicetypes.ComponentTypeCompute,
		Implementation: "custom",
		Factory:        managerFactory(devicetypes.ComponentTypeCompute),
	}

	_, err := NewCatalog([]Descriptor{descriptor, descriptor})

	require.Error(t, err)
	require.True(t, errors.Is(err, ErrDuplicateDescriptor))

	var duplicateErr DuplicateDescriptorError
	require.True(t, errors.As(err, &duplicateErr))
	require.Equal(t, devicetypes.ComponentTypeCompute, duplicateErr.ComponentType)
	require.Equal(t, "custom", duplicateErr.Implementation)
}

func TestNewCatalogValidation(t *testing.T) {
	factory := managerFactory(devicetypes.ComponentTypeCompute)

	tests := []struct {
		name       string
		descriptor Descriptor
		wantErr    error
	}{
		{
			name: "unknown component type",
			descriptor: Descriptor{
				Type:           devicetypes.ComponentTypeUnknown,
				Implementation: "custom",
				Factory:        factory,
			},
			wantErr: ErrUnknownComponentType,
		},
		{
			name: "empty implementation",
			descriptor: Descriptor{
				Type:    devicetypes.ComponentTypeCompute,
				Factory: factory,
			},
			wantErr: ErrComponentManagerImplementationNameEmpty,
		},
		{
			name: "nil factory",
			descriptor: Descriptor{
				Type:           devicetypes.ComponentTypeCompute,
				Implementation: "custom",
			},
			wantErr: ErrComponentManagerFactoryNotConfigured,
		},
		{
			name: "empty required provider",
			descriptor: Descriptor{
				Type:              devicetypes.ComponentTypeCompute,
				Implementation:    "custom",
				RequiredProviders: []string{" "},
				Factory:           factory,
			},
			wantErr: ErrProviderNameEmpty,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewCatalog([]Descriptor{tt.descriptor})

			require.Error(t, err)
			require.True(t, errors.Is(err, tt.wantErr))
		})
	}
}

func TestRegistryGetManager(t *testing.T) {
	t.Run("nil registry", func(t *testing.T) {
		var registry *Registry

		manager, err := registry.GetManager(devicetypes.ComponentTypeCompute)

		require.Nil(t, manager)
		require.Error(t, err)
		require.True(t, errors.Is(err, ErrRegistryNotConfigured))
	})

	t.Run("missing active manager", func(t *testing.T) {
		registry, err := NewRegistry(
			Catalog{},
			cmconfig.Config{},
			providerapi.NewProviderRegistry(),
		)
		require.NoError(t, err)

		manager, err := registry.GetManager(devicetypes.ComponentTypeCompute)

		require.Nil(t, manager)
		require.Error(t, err)
		require.True(t, errors.Is(err, ErrManagerNotConfigured))

		var managerErr ManagerNotConfiguredError
		require.True(t, errors.As(err, &managerErr))
		require.Equal(t, devicetypes.ComponentTypeCompute, managerErr.ComponentType)
	})
}

func TestRegistryGetDescriptor(t *testing.T) {
	catalog, err := NewCatalog([]Descriptor{
		{
			Type:           devicetypes.ComponentTypeCompute,
			Implementation: "custom",
			Factory:        managerFactory(devicetypes.ComponentTypeCompute),
		},
	})
	require.NoError(t, err)

	registry, err := NewRegistry(
		catalog,
		cmconfig.Config{
			ComponentManagers: map[devicetypes.ComponentType]string{
				devicetypes.ComponentTypeCompute: "custom",
			},
		},
		providerapi.NewProviderRegistry(),
	)
	require.NoError(t, err)

	descriptor, err := registry.GetDescriptor(devicetypes.ComponentTypeCompute)

	require.NoError(t, err)
	require.Equal(t, devicetypes.ComponentTypeCompute, descriptor.Type)
	require.Equal(t, "custom", descriptor.Implementation)
}

func TestNewRegistryErrors(t *testing.T) {
	t.Run("factory not registered", func(t *testing.T) {
		_, err := NewRegistry(
			Catalog{},
			cmconfig.Config{
				ComponentManagers: map[devicetypes.ComponentType]string{
					devicetypes.ComponentTypeCompute: "mock",
				},
			},
			providerapi.NewProviderRegistry(),
		)

		require.Error(t, err)
		require.True(t, errors.Is(err, ErrComponentManagerFactoryNotRegistered))

		var factoryErr ComponentManagerFactoryNotRegisteredError
		require.True(t, errors.As(err, &factoryErr))
		require.Equal(t, devicetypes.ComponentTypeCompute, factoryErr.ComponentType)
	})

	t.Run("unknown implementation", func(t *testing.T) {
		catalog, err := NewCatalog([]Descriptor{
			{
				Type:           devicetypes.ComponentTypeCompute,
				Implementation: "known",
				Factory:        managerFactory(devicetypes.ComponentTypeCompute),
			},
		})
		require.NoError(t, err)

		_, err = NewRegistry(
			catalog,
			cmconfig.Config{
				ComponentManagers: map[devicetypes.ComponentType]string{
					devicetypes.ComponentTypeCompute: "missing",
				},
			},
			providerapi.NewProviderRegistry(),
		)

		require.Error(t, err)
		require.True(t, errors.Is(err, ErrUnknownComponentManagerImplementation))

		var implErr UnknownComponentManagerImplementationError
		require.True(t, errors.As(err, &implErr))
		require.Equal(t, devicetypes.ComponentTypeCompute, implErr.ComponentType)
		require.Equal(t, "missing", implErr.Implementation)
		require.ElementsMatch(t, []string{"known"}, implErr.Available)
	})

	t.Run("implementation registered for another type", func(t *testing.T) {
		catalog, err := NewCatalog([]Descriptor{
			{
				Type:           devicetypes.ComponentTypeCompute,
				Implementation: "nico",
				Factory:        managerFactory(devicetypes.ComponentTypeCompute),
			},
			{
				Type:           devicetypes.ComponentTypeNVLSwitch,
				Implementation: "nvswitchmanager",
				Factory:        managerFactory(devicetypes.ComponentTypeNVLSwitch),
			},
		})
		require.NoError(t, err)

		_, err = NewRegistry(
			catalog,
			cmconfig.Config{
				ComponentManagers: map[devicetypes.ComponentType]string{
					devicetypes.ComponentTypeCompute: "nvswitchmanager",
				},
			},
			providerapi.NewProviderRegistry(),
		)

		require.Error(t, err)
		require.True(t, errors.Is(err, ErrUnknownComponentManagerImplementation))

		var implErr UnknownComponentManagerImplementationError
		require.True(t, errors.As(err, &implErr))
		require.Equal(t, devicetypes.ComponentTypeCompute, implErr.ComponentType)
		require.Equal(t, "nvswitchmanager", implErr.Implementation)
		require.Equal(t, []string{"nico"}, implErr.Available)
		require.Equal(t, []devicetypes.ComponentType{
			devicetypes.ComponentTypeNVLSwitch,
		}, implErr.RegisteredFor)
	})

	t.Run("manager creation failed", func(t *testing.T) {
		rootErr := errors.New("boom")
		catalog, err := NewCatalog([]Descriptor{
			{
				Type:           devicetypes.ComponentTypeCompute,
				Implementation: "broken",
				Factory: func(*providerapi.ProviderRegistry) (ComponentManager, error) {
					return nil, rootErr
				},
			},
		})
		require.NoError(t, err)

		_, err = NewRegistry(
			catalog,
			cmconfig.Config{
				ComponentManagers: map[devicetypes.ComponentType]string{
					devicetypes.ComponentTypeCompute: "broken",
				},
			},
			providerapi.NewProviderRegistry(),
		)

		require.Error(t, err)
		require.True(t, errors.Is(err, ErrManagerCreationFailed))
		require.True(t, errors.Is(err, rootErr))

		var creationErr ManagerCreationError
		require.True(t, errors.As(err, &creationErr))
		require.Equal(t, devicetypes.ComponentTypeCompute, creationErr.ComponentType)
		require.Equal(t, "broken", creationErr.Implementation)
	})
}

func TestRegistryFindManager(t *testing.T) {
	t.Run("nil registry", func(t *testing.T) {
		var registry *Registry

		manager := registry.FindManager(devicetypes.ComponentTypeCompute)

		require.Nil(t, manager)
	})

	t.Run("missing active manager", func(t *testing.T) {
		registry, err := NewRegistry(
			Catalog{},
			cmconfig.Config{},
			providerapi.NewProviderRegistry(),
		)
		require.NoError(t, err)

		manager := registry.FindManager(devicetypes.ComponentTypeCompute)

		require.Nil(t, manager)
	})
}
