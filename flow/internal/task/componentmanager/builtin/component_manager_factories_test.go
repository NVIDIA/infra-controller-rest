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

package builtin

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/NVIDIA/infra-controller-rest/flow/internal/task/componentmanager"
	computenico "github.com/NVIDIA/infra-controller-rest/flow/internal/task/componentmanager/compute/nico"
	cmconfig "github.com/NVIDIA/infra-controller-rest/flow/internal/task/componentmanager/config"
	"github.com/NVIDIA/infra-controller-rest/flow/internal/task/componentmanager/mock"
	nvlswitchnico "github.com/NVIDIA/infra-controller-rest/flow/internal/task/componentmanager/nvlswitch/nico"
	nvlswitchnsm "github.com/NVIDIA/infra-controller-rest/flow/internal/task/componentmanager/nvlswitch/nvswitchmanager"
	powershelfnico "github.com/NVIDIA/infra-controller-rest/flow/internal/task/componentmanager/powershelf/nico"
	powershelfpsm "github.com/NVIDIA/infra-controller-rest/flow/internal/task/componentmanager/powershelf/psm"
	"github.com/NVIDIA/infra-controller-rest/flow/internal/task/componentmanager/providerapi"
	nicoprovider "github.com/NVIDIA/infra-controller-rest/flow/internal/task/componentmanager/providers/nico"
	nsmprovider "github.com/NVIDIA/infra-controller-rest/flow/internal/task/componentmanager/providers/nvswitchmanager"
	psmprovider "github.com/NVIDIA/infra-controller-rest/flow/internal/task/componentmanager/providers/psm"
	"github.com/NVIDIA/infra-controller-rest/flow/pkg/common/devicetypes"
)

type testProviderConfig struct {
	name string
}

func (c testProviderConfig) Name() string {
	return c.name
}

func (c testProviderConfig) NewProvider(context.Context) (providerapi.Provider, error) {
	return nil, nil
}

func TestNewComponentManagerRegistryInitializesBuiltInMockManagers(t *testing.T) {
	config := cmconfig.Config{
		ComponentManagers: map[devicetypes.ComponentType]string{
			devicetypes.ComponentTypeCompute:    mock.ImplementationName,
			devicetypes.ComponentTypeNVLSwitch:  mock.ImplementationName,
			devicetypes.ComponentTypePowerShelf: mock.ImplementationName,
		},
	}

	registry, err := NewComponentManagerRegistry(
		config,
		providerapi.NewProviderRegistry(),
	)

	require.NoError(t, err)
	require.NotNil(t, registry)

	for componentType := range config.ComponentManagers {
		manager, err := registry.GetManager(componentType)
		require.NoError(t, err)
		assert.Equal(t, componentType, manager.Type())
	}
}

func TestNewComponentManagerRegistryRejectsImplementationForWrongType(t *testing.T) {
	config := cmconfig.Config{
		ComponentManagers: map[devicetypes.ComponentType]string{
			devicetypes.ComponentTypeCompute: nvlswitchnsm.ImplementationName,
		},
	}

	registry, err := NewComponentManagerRegistry(
		config,
		providerapi.NewProviderRegistry(),
	)

	require.Nil(t, registry)
	require.Error(t, err)
	require.True(t, errors.Is(err, componentmanager.ErrUnknownComponentManagerImplementation))

	var implErr componentmanager.UnknownComponentManagerImplementationError
	require.True(t, errors.As(err, &implErr))
	assert.Equal(t, devicetypes.ComponentTypeCompute, implErr.ComponentType)
	assert.Equal(t, nvlswitchnsm.ImplementationName, implErr.Implementation)
	assert.ElementsMatch(
		t,
		[]string{computenico.ImplementationName, mock.ImplementationName},
		implErr.Available,
	)
	assert.Equal(
		t,
		[]devicetypes.ComponentType{devicetypes.ComponentTypeNVLSwitch},
		implErr.RegisteredFor,
	)
}

func TestServiceCatalog(t *testing.T) {
	catalog, err := serviceCatalog(cmconfig.Config{})

	require.NoError(t, err)

	implementations := catalog.ListImplementations()
	assert.Equal(
		t,
		[]string{mock.ImplementationName, computenico.ImplementationName},
		implementations[devicetypes.ComponentTypeCompute],
	)
	assert.Equal(
		t,
		[]string{
			mock.ImplementationName,
			nvlswitchnico.ImplementationName,
			nvlswitchnsm.ImplementationName,
		},
		implementations[devicetypes.ComponentTypeNVLSwitch],
	)
	assert.Equal(
		t,
		[]string{
			mock.ImplementationName,
			powershelfnico.ImplementationName,
			powershelfpsm.ImplementationName,
		},
		implementations[devicetypes.ComponentTypePowerShelf],
	)

	assert.Equal(
		t,
		[]string{nicoprovider.ProviderName},
		requireDescriptor(
			t,
			catalog,
			devicetypes.ComponentTypeCompute,
			computenico.ImplementationName,
		).RequiredProviders,
	)
	assert.Equal(
		t,
		[]string{nicoprovider.ProviderName},
		requireDescriptor(
			t,
			catalog,
			devicetypes.ComponentTypeNVLSwitch,
			nvlswitchnico.ImplementationName,
		).RequiredProviders,
	)
	assert.Equal(
		t,
		[]string{nsmprovider.ProviderName},
		requireDescriptor(
			t,
			catalog,
			devicetypes.ComponentTypeNVLSwitch,
			nvlswitchnsm.ImplementationName,
		).RequiredProviders,
	)
	assert.Equal(
		t,
		[]string{nicoprovider.ProviderName},
		requireDescriptor(
			t,
			catalog,
			devicetypes.ComponentTypePowerShelf,
			powershelfnico.ImplementationName,
		).RequiredProviders,
	)
	assert.Equal(
		t,
		[]string{psmprovider.ProviderName},
		requireDescriptor(
			t,
			catalog,
			devicetypes.ComponentTypePowerShelf,
			powershelfpsm.ImplementationName,
		).RequiredProviders,
	)
	assert.Empty(t, requireDescriptor(
		t,
		catalog,
		devicetypes.ComponentTypeCompute,
		mock.ImplementationName,
	).RequiredProviders)
	assert.Empty(t, requireDescriptor(
		t,
		catalog,
		devicetypes.ComponentTypeNVLSwitch,
		mock.ImplementationName,
	).RequiredProviders)
	assert.Empty(t, requireDescriptor(
		t,
		catalog,
		devicetypes.ComponentTypePowerShelf,
		mock.ImplementationName,
	).RequiredProviders)
}

func requireDescriptor(
	t *testing.T,
	catalog componentmanager.Catalog,
	componentType devicetypes.ComponentType,
	implementation string,
) componentmanager.Descriptor {
	t.Helper()

	descriptor, ok := catalog.Get(componentType, implementation)
	require.True(t, ok)
	require.NotNil(t, descriptor.Factory)
	return descriptor
}

func TestNicoComputePowerDelayUsesProviderConfig(t *testing.T) {
	delay := 7 * time.Second
	config := cmconfig.Config{
		ProviderConfigs: map[string]providerapi.ProviderConfig{
			nicoprovider.ProviderName: &nicoprovider.Config{
				ComputePowerDelay: delay,
			},
		},
	}

	got, err := nicoComputePowerDelay(config)

	require.NoError(t, err)
	assert.Equal(t, delay, got)
}

func TestNicoComputePowerDelayDefaultsWhenProviderConfigMissing(t *testing.T) {
	got, err := nicoComputePowerDelay(cmconfig.Config{})

	require.NoError(t, err)
	assert.Equal(t, time.Duration(0), got)
}

func TestNicoComputePowerDelayRejectsUnexpectedConfigType(t *testing.T) {
	config := cmconfig.Config{
		ProviderConfigs: map[string]providerapi.ProviderConfig{
			nicoprovider.ProviderName: testProviderConfig{
				name: nicoprovider.ProviderName,
			},
		},
	}

	got, err := nicoComputePowerDelay(config)

	assert.Equal(t, time.Duration(0), got)
	require.Error(t, err)
	assert.True(t, errors.Is(err, componentmanager.ErrProviderConfigTypeMismatch))

	var mismatch componentmanager.ProviderConfigTypeMismatchError
	require.True(t, errors.As(err, &mismatch))
	assert.Equal(t, nicoprovider.ProviderName, mismatch.Name)
}
