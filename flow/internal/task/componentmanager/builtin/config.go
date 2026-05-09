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

	computenico "github.com/NVIDIA/infra-controller-rest/flow/internal/task/componentmanager/compute/nico"
	cmconfig "github.com/NVIDIA/infra-controller-rest/flow/internal/task/componentmanager/config"
	nvlswitchnico "github.com/NVIDIA/infra-controller-rest/flow/internal/task/componentmanager/nvlswitch/nico"
	powershelfnico "github.com/NVIDIA/infra-controller-rest/flow/internal/task/componentmanager/powershelf/nico"
	"github.com/NVIDIA/infra-controller-rest/flow/pkg/common/devicetypes"
)

// defaultServiceComponentManagers returns the component manager implementation
// map used when the RLA service is started without a component manager config
// file. A configured file is authoritative and does not merge with this map.
func defaultServiceComponentManagers() map[devicetypes.ComponentType]string {
	return map[devicetypes.ComponentType]string{
		devicetypes.ComponentTypeCompute:    computenico.ImplementationName,
		devicetypes.ComponentTypeNVLSwitch:  nvlswitchnico.ImplementationName,
		devicetypes.ComponentTypePowerShelf: powershelfnico.ImplementationName,
	}
}

// LoadConfig loads the component manager config for the RLA service.
// If path is empty, the embedded service defaults are used. If path is set, the
// YAML file is authoritative and must satisfy service config validation.
func LoadConfig(path string) (cmconfig.Config, error) {
	decoders, err := newServiceProviderConfigDecoderRegistry()
	if err != nil {
		return cmconfig.Config{}, fmt.Errorf(
			"initialize service provider config decoders: %w",
			err,
		)
	}

	var config cmconfig.Config
	if path != "" {
		config, err = cmconfig.LoadConfig(path, decoders)
		if err != nil {
			return cmconfig.Config{}, fmt.Errorf("load config from file: %w", err)
		}
	} else {
		config, err = cmconfig.New(defaultServiceComponentManagers(), decoders)
		if err != nil {
			return cmconfig.Config{}, fmt.Errorf("get default config: %w", err)
		}
	}

	if err := config.Validate(decoders); err != nil {
		return cmconfig.Config{}, fmt.Errorf("validate loaded config: %w", err)
	}

	return config, nil
}
