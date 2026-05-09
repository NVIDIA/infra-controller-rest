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

type componentManagerRegistrar func(*componentmanager.Registry)

// NewComponentManagerRegistry creates the component manager registry for the
// RLA service using all component manager implementations compiled into the
// binary.
func NewComponentManagerRegistry(
	config cmconfig.Config,
	providers *providerapi.ProviderRegistry,
) (*componentmanager.Registry, error) {
	registry := componentmanager.NewRegistry()

	if err := registerServiceComponentManagers(registry, config); err != nil {
		return nil, err
	}

	if err := registry.Initialize(config, providers); err != nil {
		return nil, fmt.Errorf("initialize component managers: %w", err)
	}

	impls := registry.ListRegisteredImplementations()
	for compType, names := range impls {
		log.Debug().
			Str("component_type", compType.String()).
			Strs("implementations", names).
			Msg("Registered component manager implementations")
	}

	return registry, nil
}

// registerServiceComponentManagers registers all component manager factories
// supported by the RLA service. Add a new compiled-in component manager here
// when adding a service-supported implementation.
func registerServiceComponentManagers(
	registry *componentmanager.Registry,
	config cmconfig.Config,
) error {
	registrars, err := serviceComponentManagerRegistrars(config)
	if err != nil {
		return err
	}

	for _, registrar := range registrars {
		registrar(registry)
	}
	return nil
}

// serviceComponentManagerRegistrars returns all component manager factory
// registrars supported by the RLA service. Add a new compiled-in component
// manager here when adding a service-supported implementation.
func serviceComponentManagerRegistrars(
	config cmconfig.Config,
) ([]componentManagerRegistrar, error) {
	computePowerDelay, err := nicoComputePowerDelay(config)
	if err != nil {
		return nil, err
	}

	return []componentManagerRegistrar{
		func(registry *componentmanager.Registry) {
			computenico.Register(registry, computePowerDelay)
		},
		nvlswitchnico.Register,
		nvlswitchnsm.Register,
		powershelfnico.Register,
		powershelfpsm.Register,
		mock.RegisterAll,
	}, nil
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
