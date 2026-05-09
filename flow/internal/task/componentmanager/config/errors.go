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
	"errors"
	"fmt"

	"github.com/NVIDIA/infra-controller-rest/flow/pkg/common/devicetypes"
)

var (
	// ErrConfigNotConfigured reports that a nil component manager config was
	// provided where a Config value is required.
	ErrConfigNotConfigured = errors.New("component manager config is not configured")

	// ErrUnknownComponentType reports an unrecognized component type in config.
	ErrUnknownComponentType = errors.New("unknown component type")

	// ErrComponentManagerImplementationNameEmpty reports that a component type
	// was configured without an implementation name.
	ErrComponentManagerImplementationNameEmpty = errors.New("component manager implementation name is empty")

	// ErrComponentManagersNotConfigured reports that the service config has no
	// component manager entries.
	ErrComponentManagersNotConfigured = errors.New("component managers are not configured")

	// ErrDuplicateProviderConfig reports duplicate provider configuration after
	// provider names are normalized.
	ErrDuplicateProviderConfig = errors.New("duplicate provider config")

	// ErrProviderConfigDecoderNotRegistered reports that a provider is required
	// but no config decoder is registered for it.
	ErrProviderConfigDecoderNotRegistered = errors.New("provider config decoder is not registered")

	// ErrProviderConfigDecoderRegistryRequired reports that a config operation
	// requires a provider config decoder registry argument.
	ErrProviderConfigDecoderRegistryRequired = errors.New("provider config decoder registry is required")
)

// UnknownComponentTypeError includes the unrecognized component type string.
type UnknownComponentTypeError struct {
	// Name is the component type name read from config.
	Name string
}

func (e UnknownComponentTypeError) Error() string {
	return fmt.Sprintf("%s: %s", ErrUnknownComponentType, e.Name)
}

func (e UnknownComponentTypeError) Is(target error) bool {
	return target == ErrUnknownComponentType
}

// ComponentManagerImplementationNameEmptyError includes the component type
// whose configured implementation name is empty.
type ComponentManagerImplementationNameEmptyError struct {
	// ComponentType is the component type with an empty implementation name.
	ComponentType devicetypes.ComponentType
}

func (e ComponentManagerImplementationNameEmptyError) Error() string {
	return fmt.Sprintf(
		"%s for component type %s",
		ErrComponentManagerImplementationNameEmpty,
		devicetypes.ComponentTypeToString(e.ComponentType),
	)
}

func (e ComponentManagerImplementationNameEmptyError) Is(target error) bool {
	return target == ErrComponentManagerImplementationNameEmpty
}

// DuplicateProviderConfigError includes the normalized duplicate provider name.
type DuplicateProviderConfigError struct {
	// Name is the duplicate provider name after trimming whitespace.
	Name string
}

func (e DuplicateProviderConfigError) Error() string {
	return fmt.Sprintf("duplicate provider config for %q", e.Name)
}

func (e DuplicateProviderConfigError) Is(target error) bool {
	return target == ErrDuplicateProviderConfig
}

// ProviderConfigDecoderNotRegisteredError includes the provider name with no
// registered config decoder.
type ProviderConfigDecoderNotRegisteredError struct {
	// Name is the provider name that has no registered config decoder.
	Name string
}

func (e ProviderConfigDecoderNotRegisteredError) Error() string {
	return fmt.Sprintf("provider config decoder %q is not registered", e.Name)
}

func (e ProviderConfigDecoderNotRegisteredError) Is(target error) bool {
	return target == ErrProviderConfigDecoderNotRegistered
}
