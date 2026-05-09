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
	"errors"
	"fmt"
	"sort"

	cmconfig "github.com/NVIDIA/infra-controller-rest/flow/internal/task/componentmanager/config"
	"github.com/NVIDIA/infra-controller-rest/flow/internal/task/componentmanager/providerapi"
	"github.com/NVIDIA/infra-controller-rest/flow/pkg/common/devicetypes"
)

var (
	// ErrRegistryNotConfigured reports that the component manager registry is
	// not available.
	ErrRegistryNotConfigured = errors.New("component manager registry is not configured")

	// ErrManagerNotConfigured reports that no active manager is configured for
	// the requested component type.
	ErrManagerNotConfigured = errors.New("component manager is not configured")

	// ErrComponentManagerFactoryNotRegistered reports that no factories were
	// registered for a component type.
	ErrComponentManagerFactoryNotRegistered = errors.New("component manager factory is not registered")

	// ErrUnknownComponentManagerImplementation reports that the configured
	// implementation name is not registered for a component type.
	ErrUnknownComponentManagerImplementation = errors.New("unknown component manager implementation")

	// ErrManagerCreationFailed reports that a registered manager factory failed.
	ErrManagerCreationFailed = errors.New("component manager creation failed")

	// ErrConfigNotConfigured reports that a nil component manager config was
	// provided where a config value is required.
	ErrConfigNotConfigured = cmconfig.ErrConfigNotConfigured

	// ErrUnknownComponentType reports an unrecognized component type in config.
	ErrUnknownComponentType = cmconfig.ErrUnknownComponentType

	// ErrComponentManagerImplementationNameEmpty reports that a component type
	// was configured without an implementation name.
	ErrComponentManagerImplementationNameEmpty = cmconfig.ErrComponentManagerImplementationNameEmpty

	// ErrComponentManagersNotConfigured reports that the service config has no
	// component manager entries.
	ErrComponentManagersNotConfigured = cmconfig.ErrComponentManagersNotConfigured

	// ErrProviderRegistryNotConfigured reports that the provider registry is not
	// available.
	ErrProviderRegistryNotConfigured = providerapi.ErrProviderRegistryNotConfigured

	// ErrProviderNotConfigured reports that a provider or provider config is not
	// available.
	ErrProviderNotConfigured = providerapi.ErrProviderNotConfigured

	// ErrUnknownProvider reports that a provider name is not known in the
	// current provider context.
	ErrUnknownProvider = providerapi.ErrUnknownProvider

	// ErrProviderTypeMismatch reports that a provider exists but has a different
	// concrete type than the caller requested.
	ErrProviderTypeMismatch = providerapi.ErrProviderTypeMismatch

	// ErrProviderNameEmpty reports an empty provider name.
	ErrProviderNameEmpty = providerapi.ErrProviderNameEmpty

	// ErrDuplicateProvider reports that a provider is already registered.
	ErrDuplicateProvider = providerapi.ErrDuplicateProvider

	// ErrProviderConfigNameMismatch reports that a provider config's name does
	// not match the name it was registered under.
	ErrProviderConfigNameMismatch = providerapi.ErrProviderConfigNameMismatch

	// ErrProviderNameMismatch reports that a created provider's name does not
	// match the provider config name.
	ErrProviderNameMismatch = providerapi.ErrProviderNameMismatch

	// ErrDuplicateProviderConfig reports duplicate provider configuration after
	// provider names are normalized.
	ErrDuplicateProviderConfig = cmconfig.ErrDuplicateProviderConfig

	// ErrProviderConfigDecoderNotRegistered reports that a provider is required
	// but no config decoder is registered for it.
	ErrProviderConfigDecoderNotRegistered = cmconfig.ErrProviderConfigDecoderNotRegistered

	// ErrProviderConfigTypeMismatch reports that a provider config has a
	// different concrete type than the caller expected.
	ErrProviderConfigTypeMismatch = errors.New("provider config type mismatch")
)

// ManagerNotConfiguredError includes the component type that has no active
// manager.
type ManagerNotConfiguredError struct {
	ComponentType devicetypes.ComponentType
}

func (e ManagerNotConfiguredError) Error() string {
	return fmt.Sprintf(
		"no active component manager configured for component type %s",
		devicetypes.ComponentTypeToString(e.ComponentType),
	)
}

func (e ManagerNotConfiguredError) Is(target error) bool {
	return target == ErrManagerNotConfigured
}

// ComponentManagerFactoryNotRegisteredError includes the component type that
// has no registered factories.
type ComponentManagerFactoryNotRegisteredError struct {
	ComponentType devicetypes.ComponentType
}

func (e ComponentManagerFactoryNotRegisteredError) Error() string {
	return fmt.Sprintf(
		"no factories registered for component type: %s",
		devicetypes.ComponentTypeToString(e.ComponentType),
	)
}

func (e ComponentManagerFactoryNotRegisteredError) Is(target error) bool {
	return target == ErrComponentManagerFactoryNotRegistered
}

// UnknownComponentManagerImplementationError includes the implementation name
// that was requested and the implementations that were available.
type UnknownComponentManagerImplementationError struct {
	ComponentType  devicetypes.ComponentType
	Implementation string
	Available      []string
}

func (e UnknownComponentManagerImplementationError) Error() string {
	available := append([]string(nil), e.Available...)
	sort.Strings(available)
	return fmt.Sprintf(
		"unknown implementation '%s' for component type %s, available: %v",
		e.Implementation,
		devicetypes.ComponentTypeToString(e.ComponentType),
		available,
	)
}

func (e UnknownComponentManagerImplementationError) Is(target error) bool {
	return target == ErrUnknownComponentManagerImplementation
}

// ManagerCreationError includes the configured manager identity and wraps the
// factory error.
type ManagerCreationError struct {
	ComponentType  devicetypes.ComponentType
	Implementation string
	Err            error
}

func (e ManagerCreationError) Error() string {
	msg := fmt.Sprintf(
		"failed to create manager for component type %s with implementation '%s'",
		devicetypes.ComponentTypeToString(e.ComponentType),
		e.Implementation,
	)
	if e.Err == nil {
		return msg
	}
	return fmt.Sprintf("%s: %v", msg, e.Err)
}

func (e ManagerCreationError) Unwrap() error {
	return e.Err
}

func (e ManagerCreationError) Is(target error) bool {
	return target == ErrManagerCreationFailed
}

// UnknownComponentTypeError includes the unrecognized component type string.
type UnknownComponentTypeError = cmconfig.UnknownComponentTypeError

// ComponentManagerImplementationNameEmptyError includes the component type
// whose configured implementation name is empty.
type ComponentManagerImplementationNameEmptyError = cmconfig.ComponentManagerImplementationNameEmptyError

// UnknownProviderError includes the unknown provider name.
type UnknownProviderError = providerapi.UnknownProviderError

// ProviderNotConfiguredError includes the provider name that is required but
// not configured.
type ProviderNotConfiguredError = providerapi.ProviderNotConfiguredError

// ProviderTypeMismatchError includes the provider name with the unexpected
// concrete type.
type ProviderTypeMismatchError = providerapi.ProviderTypeMismatchError

// DuplicateProviderError includes the duplicate provider name.
type DuplicateProviderError = providerapi.DuplicateProviderError

// ProviderConfigNameMismatchError includes the provider config map key and the
// name returned by the config.
type ProviderConfigNameMismatchError = providerapi.ProviderConfigNameMismatchError

// ProviderNameMismatchError includes the expected provider name and the name
// returned by the created provider.
type ProviderNameMismatchError = providerapi.ProviderNameMismatchError

// DuplicateProviderConfigError includes the normalized duplicate provider name.
type DuplicateProviderConfigError = cmconfig.DuplicateProviderConfigError

// ProviderConfigDecoderNotRegisteredError includes the provider name with no
// registered config decoder.
type ProviderConfigDecoderNotRegisteredError = cmconfig.ProviderConfigDecoderNotRegisteredError

// ProviderConfigTypeMismatchError includes the provider config type and the
// type expected by the caller.
type ProviderConfigTypeMismatchError struct {
	Name string
	Got  any
	Want string
}

func (e ProviderConfigTypeMismatchError) Error() string {
	return fmt.Sprintf(
		"provider %q returned config type %T, want %s",
		e.Name,
		e.Got,
		e.Want,
	)
}

func (e ProviderConfigTypeMismatchError) Is(target error) bool {
	return target == ErrProviderConfigTypeMismatch
}
