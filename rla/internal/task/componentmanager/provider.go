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
	"sync"

	"github.com/rs/zerolog/log"

	"github.com/NVIDIA/infra-controller-rest/rla/internal/task/componentmanager/providerapi"
)

// Provider is a marker interface for API client providers.
// Each provider wraps an API client and exposes it to component manager implementations.
type Provider = providerapi.Provider

// ProviderConfig is a decoded provider-specific configuration that can create
// its provider.
type ProviderConfig = providerapi.ProviderConfig

// ProviderConfigDecoder owns provider-specific config defaults and YAML
// decoding.
type ProviderConfigDecoder = providerapi.ProviderConfigDecoder

// ProviderConfigDecoderRegistry manages provider config decoders by provider name.
type ProviderConfigDecoderRegistry = providerapi.ProviderConfigDecoderRegistry

// ProviderRegistry manages API providers for component manager implementations.
// It allows implementations to request their required providers by name.
type ProviderRegistry struct {
	mu        sync.RWMutex
	providers map[string]Provider
}

// NewProviderRegistry creates a new ProviderRegistry instance.
func NewProviderRegistry() *ProviderRegistry {
	return &ProviderRegistry{
		providers: make(map[string]Provider),
	}
}

// Register adds a provider to the registry.
// Returns false if a provider with the same name already exists.
func (pr *ProviderRegistry) Register(provider Provider) bool {
	pr.mu.Lock()
	defer pr.mu.Unlock()

	name := provider.Name()
	if _, exists := pr.providers[name]; exists {
		log.Warn().
			Str("provider", name).
			Msg("Provider already registered, skipping")
		return false
	}

	pr.providers[name] = provider
	log.Debug().
		Str("provider", name).
		Msg("Registered provider")
	return true
}

// Get retrieves a provider by name.
// Returns nil if the provider is not found.
func (pr *ProviderRegistry) Get(name string) Provider {
	pr.mu.RLock()
	defer pr.mu.RUnlock()
	return pr.providers[name]
}

// GetTyped retrieves a provider by name and casts it to the expected type.
// Returns an error if the provider is not found or cannot be cast to the expected type.
func GetTyped[T Provider](pr *ProviderRegistry, name string) (T, error) {
	var zero T
	if pr == nil {
		return zero, ErrProviderRegistryNotConfigured
	}

	provider := pr.Get(name)
	if provider == nil {
		return zero, UnknownProviderError{Name: name}
	}

	typed, ok := provider.(T)
	if !ok {
		return zero, ProviderTypeMismatchError{Name: name}
	}

	return typed, nil
}

// Has checks if a provider with the given name is registered.
func (pr *ProviderRegistry) Has(name string) bool {
	pr.mu.RLock()
	defer pr.mu.RUnlock()
	_, exists := pr.providers[name]
	return exists
}

// List returns the names of all registered providers.
func (pr *ProviderRegistry) List() []string {
	pr.mu.RLock()
	defer pr.mu.RUnlock()

	names := make([]string, 0, len(pr.providers))
	for name := range pr.providers {
		names = append(names, name)
	}
	return names
}
