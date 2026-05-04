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

// Package builtin registers the component manager extensions compiled into the
// RLA binary.
package builtin

import (
	"fmt"

	"github.com/NVIDIA/ncx-infra-controller-rest/rla/internal/task/componentmanager/providerapi"
	"github.com/NVIDIA/ncx-infra-controller-rest/rla/internal/task/componentmanager/providers/carbide"
	"github.com/NVIDIA/ncx-infra-controller-rest/rla/internal/task/componentmanager/providers/nvswitchmanager"
	"github.com/NVIDIA/ncx-infra-controller-rest/rla/internal/task/componentmanager/providers/psm"
)

// NewServiceProviderConfigDecoderRegistry creates the provider config decoder
// registry used by the RLA service.
func NewServiceProviderConfigDecoderRegistry() (*providerapi.ProviderConfigDecoderRegistry, error) {
	registry := providerapi.NewProviderConfigDecoderRegistry()

	for _, decoder := range serviceProviderConfigDecoders() {
		if err := registry.Register(decoder); err != nil {
			return nil, fmt.Errorf(
				"register service provider config decoder %q: %w",
				decoder.Name(),
				err,
			)
		}
	}

	return registry, nil
}

// serviceProviderConfigDecoders returns all provider config decoders supported
// by the RLA service. Add a new provider's decoder here when adding a provider
// compiled into the service.
func serviceProviderConfigDecoders() []providerapi.ProviderConfigDecoder {
	return []providerapi.ProviderConfigDecoder{
		carbide.ConfigDecoder{},
		psm.ConfigDecoder{},
		nvswitchmanager.ConfigDecoder{},
	}
}
