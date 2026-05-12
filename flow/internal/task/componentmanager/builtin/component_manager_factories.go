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
	"fmt"
	"time"

	"github.com/rs/zerolog/log"

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
)

// NewComponentManagerRegistry creates the component manager registry for the
// Flow service using all component manager implementations compiled into the
// binary.
func NewComponentManagerRegistry(
	config cmconfig.Config,
	providers *providerapi.ProviderRegistry,
) (*componentmanager.Registry, error) {
	catalog, err := serviceCatalog(config)
	if err != nil {
		return nil, err
	}

	registry, err := componentmanager.NewRegistry(catalog, config, providers)
	if err != nil {
		return nil, fmt.Errorf("initialize component managers: %w", err)
	}

	impls := catalog.ListImplementations()
	for compType, names := range impls {
		log.Debug().
			Str("component_type", compType.String()).
			Strs("implementations", names).
			Msg("Registered component manager implementations")
	}

	return registry, nil
}

// serviceCatalog builds the component manager catalog for the flow service.
// The catalog contains the descriptors for all the built-in component
// managers supported by the flow service.
func serviceCatalog(
	config cmconfig.Config,
) (componentmanager.Catalog, error) {
	computePowerDelay, err := nicoComputePowerDelay(config)
	if err != nil {
		return componentmanager.Catalog{}, err
	}

	// Add all component manager descriptors supported by the flow service.
	descriptors := []componentmanager.Descriptor{
		computenico.Descriptor(computePowerDelay),
		nvlswitchnico.Descriptor(),
		nvlswitchnsm.Descriptor(),
		powershelfnico.Descriptor(),
		powershelfpsm.Descriptor(),
	}

	// Add all mock component manager descriptors.
	descriptors = append(descriptors, mock.Descriptors()...)

	catalog, err := componentmanager.NewCatalog(descriptors)
	if err != nil {
		return componentmanager.Catalog{}, fmt.Errorf(
			"build component manager catalog: %w",
			err,
		)
	}

	return catalog, nil
}

func nicoComputePowerDelay(config cmconfig.Config) (time.Duration, error) {
	providerConfig, ok := config.ProviderConfigs[nicoprovider.ProviderName]
	if !ok {
		return 0, nil
	}
	if providerConfig == nil {
		return 0, providerapi.ProviderNotConfiguredError{Name: nicoprovider.ProviderName}
	}

	nicoConfig, ok := providerConfig.(*nicoprovider.Config)
	if !ok {
		return 0, componentmanager.ProviderConfigTypeMismatchError{
			Name: nicoprovider.ProviderName,
			Got:  providerConfig,
			Want: "*nico.Config",
		}
	}
	return nicoConfig.ComputePowerDelay, nil
}
