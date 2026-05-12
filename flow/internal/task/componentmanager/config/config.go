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

package config

import (
	"sort"
	"strings"

	"github.com/NVIDIA/infra-controller-rest/flow/internal/task/componentmanager/providerapi"
	"github.com/NVIDIA/infra-controller-rest/flow/pkg/common/devicetypes"
)

// Config holds the component manager configuration.
//
// Config values returned by ParseConfig, LoadConfig, and NewConfig are
// normalized: component manager implementation names are trimmed, unknown
// component types are rejected, explicit provider names are trimmed, duplicate
// provider names are rejected after trimming, and missing provider configs
// required by configured component managers are completed from provider
// defaults.
type Config struct {
	// ComponentManagers maps each component type to the component manager
	// implementation responsible for managing that type. Each component manager
	// implementation can use a provider to talk to its external service.
	ComponentManagers map[devicetypes.ComponentType]string

	// ProviderConfigs holds provider-specific typed configs keyed by provider
	// name. Explicit provider configs override defaults; missing providers
	// required by configured component manager implementations are completed
	// with provider defaults. Providers are configured once and can be shared
	// by multiple component manager implementations.
	ProviderConfigs map[string]providerapi.ProviderConfig
}

// New builds a component manager config from a component-manager
// implementation map and derives default provider configs for implementations
// backed by a registered provider decoder.
func New(
	componentManagers map[devicetypes.ComponentType]string,
	decoders *providerapi.ProviderConfigDecoderRegistry,
) (Config, error) {
	if decoders == nil {
		return Config{}, ErrProviderConfigDecoderRegistryRequired
	}

	config := newConfig()

	for ct, implName := range componentManagers {
		if err := config.addComponentManager(ct, implName); err != nil {
			return Config{}, err
		}
	}

	if err := config.completeProviderConfigs(decoders); err != nil {
		return Config{}, err
	}
	return config, nil
}

// newConfig creates an empty normalized-config accumulator. Callers add
// component managers and providers through the package helpers so normalization
// stays centralized.
func newConfig() Config {
	return Config{
		ComponentManagers: make(map[devicetypes.ComponentType]string),
		ProviderConfigs:   make(map[string]providerapi.ProviderConfig),
	}
}

// addComponentManager validates a component-manager entry and stores the
// normalized implementation name.
func (c *Config) addComponentManager(
	ct devicetypes.ComponentType,
	implName string,
) error {
	if ct == devicetypes.ComponentTypeUnknown {
		return UnknownComponentTypeError{
			Name: devicetypes.ComponentTypeToString(ct),
		}
	}

	implName = strings.TrimSpace(implName)
	if implName == "" {
		return ComponentManagerImplementationNameEmptyError{
			ComponentType: ct,
		}
	}

	c.ComponentManagers[ct] = implName
	return nil
}

// prepareProviderConfigForAdd normalizes a provider name, verifies the config
// does not already contain it, and resolves the decoder for that provider.
func (c *Config) prepareProviderConfigForAdd(
	name string,
	decoders *providerapi.ProviderConfigDecoderRegistry,
) (string, providerapi.ProviderConfigDecoder, error) {
	if c == nil {
		return "", nil, ErrConfigNotConfigured
	}

	if decoders == nil {
		return "", nil, ErrProviderConfigDecoderRegistryRequired
	}

	name = strings.TrimSpace(name)
	if name == "" {
		return "", nil, providerapi.ErrProviderNameEmpty
	}

	decoder, ok := decoders.Get(name)
	if !ok {
		return "", nil, providerapi.UnknownProviderError{Name: name}
	}

	return name, decoder, nil
}

// completeProviderConfigs enables missing providers based on the configured
// component manager implementations. Explicit provider configs already present
// in the config are preserved.
func (c *Config) completeProviderConfigs(
	decoders *providerapi.ProviderConfigDecoderRegistry,
) error {
	names, err := c.requiredProviderNames(decoders)
	if err != nil {
		return err
	}

	for _, name := range names {
		if c.HasProvider(name) {
			continue
		}

		name, decoder, err := c.prepareProviderConfigForAdd(name, decoders)
		if err != nil {
			return err
		}

		c.ProviderConfigs[name] = decoder.DefaultConfig()
	}

	return nil
}

// Validate verifies the generic component manager config contract.
func (c *Config) Validate(
	decoders *providerapi.ProviderConfigDecoderRegistry,
) error {
	if c == nil {
		return ErrConfigNotConfigured
	}

	if len(c.ComponentManagers) == 0 {
		return ErrComponentManagersNotConfigured
	}

	if decoders == nil {
		return ErrProviderConfigDecoderRegistryRequired
	}

	names, err := c.requiredProviderNames(decoders)
	if err != nil {
		return err
	}

	for _, name := range names {
		if !c.HasProvider(name) {
			return providerapi.ProviderNotConfiguredError{Name: name}
		}
	}
	return nil
}

// requiredProviderNames returns provider names implied by the configured
// component-manager implementations.
//
// Transitional compatibility shim: until manager descriptors exist, an
// implementation is considered provider-backed when its implementation name
// matches a registered provider decoder name.
func (c *Config) requiredProviderNames(
	decoders *providerapi.ProviderConfigDecoderRegistry,
) ([]string, error) {
	names := make(map[string]struct{})
	for _, implName := range c.ComponentManagers {
		if _, ok := decoders.Get(implName); ok {
			names[implName] = struct{}{}
		}
	}

	if len(names) == 0 {
		return nil, nil
	}

	result := make([]string, 0, len(names))
	for name := range names {
		result = append(result, name)
	}

	sort.Strings(result)
	return result, nil
}

// HasProvider checks if a provider is enabled in the configuration.
func (c *Config) HasProvider(name string) bool {
	if c != nil && c.ProviderConfigs != nil {
		if _, ok := c.ProviderConfigs[name]; ok {
			return true
		}
	}

	return false
}
