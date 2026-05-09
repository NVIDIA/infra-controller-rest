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
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	cmconfig "github.com/NVIDIA/infra-controller-rest/flow/internal/task/componentmanager/config"
	"github.com/NVIDIA/infra-controller-rest/flow/internal/task/componentmanager/providers/nico"
	"github.com/NVIDIA/infra-controller-rest/flow/internal/task/componentmanager/providers/psm"
	"github.com/NVIDIA/infra-controller-rest/flow/pkg/common/devicetypes"
)

func TestDefaultServiceComponentManagers(t *testing.T) {
	componentManagers := defaultServiceComponentManagers()

	assert.Equal(t, nico.ProviderName, componentManagers[devicetypes.ComponentTypeCompute])
	assert.Equal(t, nico.ProviderName, componentManagers[devicetypes.ComponentTypeNVLSwitch])
	assert.Equal(t, nico.ProviderName, componentManagers[devicetypes.ComponentTypePowerShelf])

	componentManagers[devicetypes.ComponentTypeCompute] = "mutated"
	assert.Equal(
		t,
		nico.ProviderName,
		defaultServiceComponentManagers()[devicetypes.ComponentTypeCompute],
	)
}

func TestLoadConfigUsesDefaultsWithoutPath(t *testing.T) {
	config, err := LoadConfig("")
	require.NoError(t, err)

	assert.Equal(
		t,
		defaultServiceComponentManagers(),
		config.ComponentManagers,
	)
	assert.True(t, config.HasProvider(nico.ProviderName))
	assert.False(t, config.HasProvider(psm.ProviderName))

	nicoConfig, ok := config.ProviderConfigs[nico.ProviderName].(*nico.Config)
	require.True(t, ok)
	assert.Equal(t, nico.DefaultTimeout, nicoConfig.Timeout)
	assert.Equal(
		t,
		nico.DefaultComputePowerDelay,
		nicoConfig.ComputePowerDelay,
	)
}

func TestLoadConfigUsesAuthoritativeFile(t *testing.T) {
	path := writeServiceConfig(t, `
component_managers:
  compute: mock
providers: {}
`)

	config, err := LoadConfig(path)
	require.NoError(t, err)

	assert.Equal(t, "mock", config.ComponentManagers[devicetypes.ComponentTypeCompute])
	assert.Empty(t, config.ProviderConfigs)
	assert.False(t, config.HasProvider(nico.ProviderName))
}

func TestLoadConfigRequiresComponentManagers(t *testing.T) {
	path := writeServiceConfig(t, `
providers: {}
`)

	config, err := LoadConfig(path)

	require.Empty(t, config.ComponentManagers)
	require.Error(t, err)
	assert.True(t, errors.Is(err, cmconfig.ErrComponentManagersNotConfigured))
}

func TestLoadConfigCompletesMissingProviders(t *testing.T) {
	path := writeServiceConfig(t, `
component_managers:
  compute: nico
providers: {}
`)

	config, err := LoadConfig(path)

	require.NoError(t, err)
	assert.Equal(t, nico.ProviderName, config.ComponentManagers[devicetypes.ComponentTypeCompute])
	assert.True(t, config.HasProvider(nico.ProviderName))
}

func writeServiceConfig(t *testing.T, data string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "componentmanager.yaml")
	err := os.WriteFile(path, []byte(data), 0o600)
	require.NoError(t, err)
	return path
}
