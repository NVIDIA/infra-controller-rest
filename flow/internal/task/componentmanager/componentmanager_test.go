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
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	cmconfig "github.com/NVIDIA/infra-controller-rest/flow/internal/task/componentmanager/config"
	"github.com/NVIDIA/infra-controller-rest/flow/internal/task/componentmanager/providerapi"
	"github.com/NVIDIA/infra-controller-rest/flow/pkg/common/devicetypes"
)

func TestRegistryGetManager(t *testing.T) {
	t.Run("nil registry", func(t *testing.T) {
		var registry *Registry

		manager, err := registry.GetManager(devicetypes.ComponentTypeCompute)

		require.Nil(t, manager)
		require.Error(t, err)
		require.True(t, errors.Is(err, ErrRegistryNotConfigured))
	})

	t.Run("missing active manager", func(t *testing.T) {
		registry := NewRegistry()

		manager, err := registry.GetManager(devicetypes.ComponentTypeCompute)

		require.Nil(t, manager)
		require.Error(t, err)
		require.True(t, errors.Is(err, ErrManagerNotConfigured))

		var managerErr ManagerNotConfiguredError
		require.True(t, errors.As(err, &managerErr))
		require.Equal(t, devicetypes.ComponentTypeCompute, managerErr.ComponentType)
	})
}

func TestRegistryInitializeErrors(t *testing.T) {
	t.Run("factory not registered", func(t *testing.T) {
		registry := NewRegistry()

		err := registry.Initialize(cmconfig.Config{
			ComponentManagers: map[devicetypes.ComponentType]string{
				devicetypes.ComponentTypeCompute: "mock",
			},
		}, providerapi.NewProviderRegistry())

		require.Error(t, err)
		require.True(t, errors.Is(err, ErrComponentManagerFactoryNotRegistered))

		var factoryErr ComponentManagerFactoryNotRegisteredError
		require.True(t, errors.As(err, &factoryErr))
		require.Equal(t, devicetypes.ComponentTypeCompute, factoryErr.ComponentType)
	})

	t.Run("unknown implementation", func(t *testing.T) {
		registry := NewRegistry()
		registry.RegisterFactory(
			devicetypes.ComponentTypeCompute,
			"known",
			func(*providerapi.ProviderRegistry) (ComponentManager, error) {
				return nil, nil
			},
		)

		err := registry.Initialize(cmconfig.Config{
			ComponentManagers: map[devicetypes.ComponentType]string{
				devicetypes.ComponentTypeCompute: "missing",
			},
		}, providerapi.NewProviderRegistry())

		require.Error(t, err)
		require.True(t, errors.Is(err, ErrUnknownComponentManagerImplementation))

		var implErr UnknownComponentManagerImplementationError
		require.True(t, errors.As(err, &implErr))
		require.Equal(t, devicetypes.ComponentTypeCompute, implErr.ComponentType)
		require.Equal(t, "missing", implErr.Implementation)
		require.ElementsMatch(t, []string{"known"}, implErr.Available)
	})

	t.Run("manager creation failed", func(t *testing.T) {
		rootErr := errors.New("boom")
		registry := NewRegistry()
		registry.RegisterFactory(
			devicetypes.ComponentTypeCompute,
			"broken",
			func(*providerapi.ProviderRegistry) (ComponentManager, error) {
				return nil, rootErr
			},
		)

		err := registry.Initialize(cmconfig.Config{
			ComponentManagers: map[devicetypes.ComponentType]string{
				devicetypes.ComponentTypeCompute: "broken",
			},
		}, providerapi.NewProviderRegistry())

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
		registry := NewRegistry()

		manager := registry.FindManager(devicetypes.ComponentTypeCompute)

		require.Nil(t, manager)
	})
}
