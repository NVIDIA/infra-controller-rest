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
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	cmbuiltin "github.com/NVIDIA/ncx-infra-controller-rest/rla/internal/task/componentmanager/builtin"
	"github.com/NVIDIA/ncx-infra-controller-rest/rla/internal/task/componentmanager/providerapi"
	"github.com/NVIDIA/ncx-infra-controller-rest/rla/internal/task/componentmanager/providers/nico"
	"github.com/NVIDIA/ncx-infra-controller-rest/rla/internal/task/componentmanager/providers/nvswitchmanager"
	"github.com/NVIDIA/ncx-infra-controller-rest/rla/internal/task/componentmanager/providers/psm"
	"github.com/NVIDIA/ncx-infra-controller-rest/rla/pkg/common/devicetypes"
)

type customProviderConfig struct {
	name string
}

func (c customProviderConfig) Name() string {
	return c.name
}

func (c customProviderConfig) NewProvider(context.Context) (Provider, error) {
	return nil, nil
}

type customProviderConfigDecoder struct {
	name string
}

func (d customProviderConfigDecoder) Name() string {
	return d.name
}

func (d customProviderConfigDecoder) DefaultConfig() ProviderConfig {
	return customProviderConfig{name: d.name}
}

func (d customProviderConfigDecoder) DecodeYAML(raw yaml.Node) (ProviderConfig, error) {
	return d.DefaultConfig(), nil
}

func TestParseConfigWithExplicitProviders(t *testing.T) {
	config, err := parseConfigWithBuiltins(t, `
component_managers:
  compute: nico
  nvlswitch: nvswitchmanager
  powershelf: psm
providers:
  nico:
    timeout: 45s
    compute_power_delay: 0s
  psm:
    timeout: 20s
  nvswitchmanager:
    timeout: 90s
`)
	require.NoError(t, err)

	assert.Equal(t, nico.ProviderName, config.ComponentManagers[devicetypes.ComponentTypeCompute])
	assert.Equal(t, nvswitchmanager.ProviderName, config.ComponentManagers[devicetypes.ComponentTypeNVLSwitch])
	assert.Equal(t, psm.ProviderName, config.ComponentManagers[devicetypes.ComponentTypePowerShelf])

	require.NotNil(t, config.Providers.NICo)
	assert.Equal(t, 45*time.Second, config.Providers.NICo.Timeout)
	assert.Equal(t, 0*time.Second, config.Providers.NICo.ComputePowerDelay)

	require.NotNil(t, config.Providers.PSM)
	assert.Equal(t, 20*time.Second, config.Providers.PSM.Timeout)

	require.NotNil(t, config.Providers.NVSwitchManager)
	assert.Equal(t, 90*time.Second, config.Providers.NVSwitchManager.Timeout)

	assert.Same(t, config.Providers.NICo, config.ProviderConfigs[nico.ProviderName])
	assert.Same(t, config.Providers.PSM, config.ProviderConfigs[psm.ProviderName])
	assert.Same(t, config.Providers.NVSwitchManager, config.ProviderConfigs[nvswitchmanager.ProviderName])
}

func TestParseConfigDerivesProviders(t *testing.T) {
	tests := []struct {
		name        string
		configYAML  string
		wantEnabled []string
	}{
		{
			name: "mock managers do not need providers",
			configYAML: `
component_managers:
  compute: mock
  nvlswitch: mock
  powershelf: mock
`,
			wantEnabled: nil,
		},
		{
			name: "nico",
			configYAML: `
component_managers:
  compute: nico
`,
			wantEnabled: []string{nico.ProviderName},
		},
		{
			name: "psm",
			configYAML: `
component_managers:
  powershelf: psm
`,
			wantEnabled: []string{psm.ProviderName},
		},
		{
			name: "nvswitchmanager",
			configYAML: `
component_managers:
  nvlswitch: nvswitchmanager
`,
			wantEnabled: []string{nvswitchmanager.ProviderName},
		},
		{
			name: "deduplicates providers",
			configYAML: `
component_managers:
  compute: nico
  nvlswitch: nico
  powershelf: psm
`,
			wantEnabled: []string{nico.ProviderName, psm.ProviderName},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			config, err := parseConfigWithBuiltins(t, tc.configYAML)
			require.NoError(t, err)
			assert.ElementsMatch(t, tc.wantEnabled, providerConfigNames(config))
		})
	}
}

func TestParseConfigKeepsExplicitProviderBehavior(t *testing.T) {
	config, err := parseConfigWithBuiltins(t, `
component_managers:
  compute: nico
  powershelf: psm
providers:
  psm:
    timeout: 20s
`)
	require.NoError(t, err)

	assert.Nil(t, config.Providers.NICo)
	require.NotNil(t, config.Providers.PSM)
	assert.Equal(t, 20*time.Second, config.Providers.PSM.Timeout)
	assert.False(t, config.HasProvider(nico.ProviderName))
	assert.True(t, config.HasProvider(psm.ProviderName))
}

func TestParseConfigTreatsEmptyProvidersAsExplicit(t *testing.T) {
	config, err := parseConfigWithBuiltins(t, `
component_managers:
  compute: nico
providers: {}
`)
	require.NoError(t, err)

	assert.Empty(t, config.ProviderConfigs)
	assert.Nil(t, config.Providers.NICo)
	assert.False(t, config.HasProvider(nico.ProviderName))
}

func TestParseConfigErrors(t *testing.T) {
	tests := []struct {
		name       string
		configYAML string
		wantErr    error
		checkErr   func(*testing.T, error)
	}{
		{
			name: "unknown provider",
			configYAML: `
component_managers:
  compute: mock
providers:
  madeup: {}
`,
			wantErr: ErrUnknownProvider,
			checkErr: func(t *testing.T, err error) {
				t.Helper()
				var providerErr UnknownProviderError
				require.True(t, errors.As(err, &providerErr))
				assert.Equal(t, "madeup", providerErr.Name)
			},
		},
		{
			name: "unknown component type",
			configYAML: `
component_managers:
  madeup: mock
`,
			wantErr: ErrUnknownComponentType,
			checkErr: func(t *testing.T, err error) {
				t.Helper()
				var componentTypeErr UnknownComponentTypeError
				require.True(t, errors.As(err, &componentTypeErr))
				assert.Equal(t, "madeup", componentTypeErr.Name)
			},
		},
		{
			name: "duplicate provider after trimming name",
			configYAML: `
component_managers:
  compute: mock
providers:
  nico:
    timeout: 30s
  " nico ":
    timeout: 45s
`,
			wantErr: ErrDuplicateProviderConfig,
			checkErr: func(t *testing.T, err error) {
				t.Helper()
				var duplicateErr DuplicateProviderConfigError
				require.True(t, errors.As(err, &duplicateErr))
				assert.Equal(t, nico.ProviderName, duplicateErr.Name)
			},
		},
		{
			name: "invalid nico timeout",
			configYAML: `
component_managers:
  compute: mock
providers:
  nico:
    timeout: nope
`,
			wantErr: providerapi.ErrInvalidProviderConfigField,
			checkErr: func(t *testing.T, err error) {
				t.Helper()
				assertInvalidProviderConfigField(t, err, nico.ProviderName, "timeout")
			},
		},
		{
			name: "invalid psm timeout",
			configYAML: `
component_managers:
  compute: mock
providers:
  psm:
    timeout: nope
`,
			wantErr: providerapi.ErrInvalidProviderConfigField,
			checkErr: func(t *testing.T, err error) {
				t.Helper()
				assertInvalidProviderConfigField(t, err, psm.ProviderName, "timeout")
			},
		},
		{
			name: "invalid nvswitchmanager timeout",
			configYAML: `
component_managers:
  compute: mock
providers:
  nvswitchmanager:
    timeout: nope
`,
			wantErr: providerapi.ErrInvalidProviderConfigField,
			checkErr: func(t *testing.T, err error) {
				t.Helper()
				assertInvalidProviderConfigField(t, err, nvswitchmanager.ProviderName, "timeout")
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := parseConfigWithBuiltins(t, tc.configYAML)
			require.Error(t, err)
			assert.True(t, errors.Is(err, tc.wantErr))
			if tc.checkErr != nil {
				tc.checkErr(t, err)
			}
		})
	}
}

func TestParseConfigAllowsCustomProviderDecoderRegistry(t *testing.T) {
	registry := providerapi.NewProviderConfigDecoderRegistry()
	require.NoError(t, registry.Register(customProviderConfigDecoder{name: "custom"}))

	config, err := ParseConfig([]byte(`
component_managers:
  compute: mock
providers:
  custom: {}
`), registry)
	require.NoError(t, err)

	assert.True(t, config.HasProvider("custom"))
	assert.Equal(t, customProviderConfig{name: "custom"}, config.ProviderConfigs["custom"])
	assert.Nil(t, config.Providers.NICo)
	assert.Nil(t, config.Providers.PSM)
	assert.Nil(t, config.Providers.NVSwitchManager)
}

func TestParseConfigRequiresDecoderRegistry(t *testing.T) {
	_, err := ParseConfig([]byte(`component_managers: {}`), nil)
	require.Error(t, err)
	assert.True(t, errors.Is(err, providerapi.ErrProviderConfigDecoderRegistryNotConfigured))
}

func assertInvalidProviderConfigField(
	t *testing.T,
	err error,
	provider string,
	field string,
) {
	t.Helper()

	var fieldErr providerapi.InvalidProviderConfigFieldError
	require.True(t, errors.As(err, &fieldErr))
	assert.Equal(t, provider, fieldErr.Provider)
	assert.Equal(t, field, fieldErr.Field)
}

func TestHasProviderFallsBackToLegacyFields(t *testing.T) {
	config := Config{
		Providers: LegacyProviderConfig{
			NICo: &nico.Config{},
		},
	}

	assert.True(t, config.HasProvider(nico.ProviderName))
	assert.False(t, config.HasProvider(psm.ProviderName))
}

func TestDefaultConfigCompatibility(t *testing.T) {
	prod := DefaultProdConfig()
	require.NotNil(t, prod.Providers.NICo)
	require.NotNil(t, prod.Providers.PSM)
	assert.True(t, prod.HasProvider(nico.ProviderName))
	assert.True(t, prod.HasProvider(psm.ProviderName))

	testConfig := defaultTestConfig()
	assert.Nil(t, testConfig.Providers.NICo)
	assert.Nil(t, testConfig.Providers.PSM)
	assert.Nil(t, testConfig.Providers.NVSwitchManager)
	assert.False(t, testConfig.HasProvider(nico.ProviderName))
}

func TestLoadConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "componentmanager.yaml")
	err := os.WriteFile(path, []byte(`
component_managers:
  compute: nico
`), 0o600)
	require.NoError(t, err)

	config, err := LoadConfig(path)
	require.NoError(t, err)
	assert.True(t, config.HasProvider(nico.ProviderName))
}

func providerConfigNames(config Config) []string {
	names := make([]string, 0, len(config.ProviderConfigs))
	for name := range config.ProviderConfigs {
		names = append(names, name)
	}
	return names
}

func parseConfigWithBuiltins(t *testing.T, data string) (Config, error) {
	t.Helper()
	decoders, err := cmbuiltin.NewServiceProviderConfigDecoderRegistry()
	if err != nil {
		return Config{}, err
	}
	return ParseConfig([]byte(data), decoders)
}

func defaultTestConfig() Config {
	return Config{
		ComponentManagers: map[devicetypes.ComponentType]string{
			devicetypes.ComponentTypeCompute:    "mock",
			devicetypes.ComponentTypeNVLSwitch:  "mock",
			devicetypes.ComponentTypePowerShelf: "mock",
		},
		Providers:       LegacyProviderConfig{},
		ProviderConfigs: map[string]ProviderConfig{},
	}
}
