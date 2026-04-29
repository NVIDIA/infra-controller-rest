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
	"fmt"
	"os"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	cmbuiltin "github.com/NVIDIA/ncx-infra-controller-rest/rla/internal/task/componentmanager/builtin"
	"github.com/NVIDIA/ncx-infra-controller-rest/rla/internal/task/componentmanager/providerapi"
	"github.com/NVIDIA/ncx-infra-controller-rest/rla/internal/task/componentmanager/providers/nico"
	"github.com/NVIDIA/ncx-infra-controller-rest/rla/internal/task/componentmanager/providers/nvswitchmanager"
	"github.com/NVIDIA/ncx-infra-controller-rest/rla/internal/task/componentmanager/providers/psm"
	"github.com/NVIDIA/ncx-infra-controller-rest/rla/pkg/common/devicetypes"
)

// LegacyProviderConfig holds the typed configuration fields used by the
// current bootstrap path.
// A nil pointer means the provider is not enabled.
type LegacyProviderConfig struct {
	// NICo holds NICo-specific configuration. Nil means disabled.
	NICo *nico.Config

	// PSM holds PSM-specific configuration. Nil means disabled.
	PSM *psm.Config

	// NVSwitchManager holds NV-Switch Manager-specific configuration. Nil means disabled.
	NVSwitchManager *nvswitchmanager.Config
}

// Config holds the component manager configuration.
type Config struct {
	// ComponentManagers maps component types to their implementation names.
	ComponentManagers map[devicetypes.ComponentType]string

	// Providers holds provider-specific configuration.
	Providers LegacyProviderConfig

	// ProviderConfigs holds provider-specific typed configs keyed by provider
	// name. It is the bridge to future generic provider bootstrap.
	ProviderConfigs map[string]ProviderConfig
}

// rawConfig is the raw YAML structure before conversion.
type rawConfig struct {
	ComponentManagers map[string]string    `yaml:"component_managers"`
	Providers         map[string]yaml.Node `yaml:"providers"`
}

// LoadConfig loads the component manager configuration from a YAML file.
func LoadConfig(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, fmt.Errorf("failed to read config file: %w", err)
	}

	// TODO: In the provider bootstrap PR, pass the decoder registry into
	// LoadConfig instead of constructing the built-in registry here.
	decoders, err := cmbuiltin.NewServiceProviderConfigDecoderRegistry()
	if err != nil {
		return Config{}, fmt.Errorf(
			"failed to create provider config decoder registry: %w", err,
		)
	}

	return ParseConfig(data, decoders)
}

// ParseConfig parses the component manager configuration from YAML data using
// the supplied provider config decoders.
func ParseConfig(
	data []byte,
	decoders *ProviderConfigDecoderRegistry,
) (Config, error) {
	if decoders == nil {
		return Config{}, providerapi.ErrProviderConfigDecoderRegistryNotConfigured
	}

	var raw rawConfig
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return Config{}, fmt.Errorf("failed to parse config: %w", err)
	}

	config := Config{
		ComponentManagers: make(map[devicetypes.ComponentType]string),
		ProviderConfigs:   make(map[string]ProviderConfig),
	}

	// Parse component managers.
	for typeStr, implName := range raw.ComponentManagers {
		componentType := devicetypes.ComponentTypeFromString(typeStr)
		if componentType == devicetypes.ComponentTypeUnknown {
			return Config{}, UnknownComponentTypeError{Name: typeStr}
		}
		config.ComponentManagers[componentType] = strings.TrimSpace(implName)
	}

	if raw.Providers != nil {
		// Preserve the current config semantics: when a providers section is
		// present, it is treated as the complete explicit provider set. Missing
		// providers are not auto-derived here; manager/provider requirement
		// validation should report those missing dependencies in a later PR.
		if err := decodeConfiguredProviders(&config, raw.Providers, decoders); err != nil {
			return Config{}, err
		}
		return config, nil
	}

	if err := deriveProviders(&config, decoders); err != nil {
		return Config{}, err
	}

	return config, nil
}

func decodeConfiguredProviders(
	config *Config,
	rawProviders map[string]yaml.Node,
	decoders *ProviderConfigDecoderRegistry,
) error {
	providers, err := normalizeConfiguredProviders(rawProviders)
	if err != nil {
		return err
	}

	for _, provider := range providers {
		name := provider.name
		decoder, ok := decoders.Get(name)
		if !ok {
			return UnknownProviderError{Name: name}
		}

		decoded, err := decoder.DecodeYAML(provider.raw)
		if err != nil {
			return err
		}

		if err := applyProviderConfig(config, name, decoded); err != nil {
			return err
		}
	}
	return nil
}

type configuredProvider struct {
	name string
	raw  yaml.Node
}

// normalizeConfiguredProviders trims provider names, rejects duplicate
// normalized names, and returns providers in deterministic order before any
// provider-specific decoding runs.
func normalizeConfiguredProviders(
	rawProviders map[string]yaml.Node,
) ([]configuredProvider, error) {
	providersByName := make(map[string]yaml.Node, len(rawProviders))
	for rawName, rawNode := range rawProviders {
		name := strings.TrimSpace(rawName)
		if name == "" {
			return nil, ErrProviderNameEmpty
		}
		if _, exists := providersByName[name]; exists {
			return nil, DuplicateProviderConfigError{Name: name}
		}
		providersByName[name] = rawNode
	}

	names := make([]string, 0, len(providersByName))
	for name := range providersByName {
		names = append(names, name)
	}
	sort.Strings(names)

	providers := make([]configuredProvider, 0, len(names))
	for _, name := range names {
		providers = append(providers, configuredProvider{
			name: name,
			raw:  providersByName[name],
		})
	}
	return providers, nil
}

// deriveProviders enables providers based on the component manager
// implementations configured.
func deriveProviders(config *Config, decoders *ProviderConfigDecoderRegistry) error {
	for _, name := range deriveProviderNames(*config) {
		decoder, ok := decoders.Get(name)
		if !ok {
			return ProviderConfigDecoderNotRegisteredError{Name: name}
		}

		if err := applyProviderConfig(config, name, decoder.DefaultConfig()); err != nil {
			return err
		}
	}
	return nil
}

func deriveProviderNames(config Config) []string {
	// Transitional compatibility shim. The final architecture should derive
	// required providers from manager descriptors rather than assuming an
	// implementation name maps directly to a provider name.
	names := make(map[string]struct{})
	for _, implName := range config.ComponentManagers {
		switch implName {
		case nico.ProviderName:
			names[nico.ProviderName] = struct{}{}
		case psm.ProviderName:
			names[psm.ProviderName] = struct{}{}
		case nvswitchmanager.ProviderName:
			names[nvswitchmanager.ProviderName] = struct{}{}
		}
	}

	result := make([]string, 0, len(names))
	for name := range names {
		result = append(result, name)
	}
	sort.Strings(result)
	return result
}

func applyProviderConfig(config *Config, name string, decoded ProviderConfig) error {
	if config.ProviderConfigs == nil {
		config.ProviderConfigs = make(map[string]ProviderConfig)
	}
	config.ProviderConfigs[name] = decoded
	return applyLegacyProviderConfig(config, name, decoded)
}

func applyLegacyProviderConfig(config *Config, name string, decoded ProviderConfig) error {
	switch name {
	case nico.ProviderName:
		nicoConfig, ok := decoded.(*nico.Config)
		if !ok {
			return ProviderConfigTypeMismatchError{
				Name: name,
				Got:  decoded,
				Want: "*nico.Config",
			}
		}
		config.Providers.NICo = nicoConfig
	case psm.ProviderName:
		psmConfig, ok := decoded.(*psm.Config)
		if !ok {
			return ProviderConfigTypeMismatchError{
				Name: name,
				Got:  decoded,
				Want: "*psm.Config",
			}
		}
		config.Providers.PSM = psmConfig
	case nvswitchmanager.ProviderName:
		nsmConfig, ok := decoded.(*nvswitchmanager.Config)
		if !ok {
			return ProviderConfigTypeMismatchError{
				Name: name,
				Got:  decoded,
				Want: "*nvswitchmanager.Config",
			}
		}
		config.Providers.NVSwitchManager = nsmConfig
	}

	return nil
}

// HasProvider checks if a provider is enabled in the configuration.
func (c *Config) HasProvider(name string) bool {
	if c.ProviderConfigs != nil {
		if _, ok := c.ProviderConfigs[name]; ok {
			return true
		}
	}

	switch name {
	case nico.ProviderName:
		return c.Providers.NICo != nil
	case psm.ProviderName:
		return c.Providers.PSM != nil
	case nvswitchmanager.ProviderName:
		return c.Providers.NVSwitchManager != nil
	}
	return false
}

// DefaultProdConfig returns the embedded production configuration.
// Used when no config file is specified. Connects to external services
//
// Timing parameters for operations are configured per-rule via action parameters.
func DefaultProdConfig() Config {
	nicoConfig := &nico.Config{
		Timeout:           nico.DefaultTimeout,
		ComputePowerDelay: nico.DefaultComputePowerDelay,
	}
	psmConfig := &psm.Config{
		Timeout: psm.DefaultTimeout,
	}

	return Config{
		ComponentManagers: map[devicetypes.ComponentType]string{
			devicetypes.ComponentTypeCompute:    nico.ProviderName,
			devicetypes.ComponentTypeNVLSwitch:  nico.ProviderName,
			devicetypes.ComponentTypePowerShelf: nico.ProviderName,
		},
		Providers: LegacyProviderConfig{
			NICo: nicoConfig,
			PSM:  psmConfig,
		},
		ProviderConfigs: map[string]ProviderConfig{
			nico.ProviderName: nicoConfig,
			psm.ProviderName:  psmConfig,
		},
	}
}
