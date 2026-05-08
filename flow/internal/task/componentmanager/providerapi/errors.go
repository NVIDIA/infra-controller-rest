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

package providerapi

import (
	"errors"
	"fmt"
)

var (
	// ErrProviderConfigDecoderRegistryNotConfigured reports that a decoder
	// registry was required but not configured.
	ErrProviderConfigDecoderRegistryNotConfigured = errors.New("provider config decoder registry is not configured")

	// ErrProviderConfigDecoderNotConfigured reports that a nil decoder was
	// provided for registration.
	ErrProviderConfigDecoderNotConfigured = errors.New("provider config decoder is not configured")

	// ErrProviderConfigDecoderNameEmpty reports that a decoder returned an empty
	// provider name.
	ErrProviderConfigDecoderNameEmpty = errors.New("provider config decoder name is empty")

	// ErrProviderConfigDecoderAlreadyRegistered reports a duplicate decoder
	// registration.
	ErrProviderConfigDecoderAlreadyRegistered = errors.New("provider config decoder already registered")

	// ErrInvalidProviderConfig reports that provider-specific YAML was invalid.
	ErrInvalidProviderConfig = errors.New("invalid provider config")

	// ErrInvalidProviderConfigField reports that a provider config field value
	// was invalid.
	ErrInvalidProviderConfigField = errors.New("invalid provider config field")
)

// ProviderConfigDecoderAlreadyRegisteredError includes the duplicate provider
// decoder name.
type ProviderConfigDecoderAlreadyRegisteredError struct {
	Name string
}

func (e ProviderConfigDecoderAlreadyRegisteredError) Error() string {
	return fmt.Sprintf("provider config decoder %q already registered", e.Name)
}

func (e ProviderConfigDecoderAlreadyRegisteredError) Is(target error) bool {
	return target == ErrProviderConfigDecoderAlreadyRegistered
}

// InvalidProviderConfigError wraps provider-specific YAML decode errors.
type InvalidProviderConfigError struct {
	Provider string
	Err      error
}

func (e InvalidProviderConfigError) Error() string {
	msg := fmt.Sprintf("invalid %s config", e.Provider)
	if e.Err == nil {
		return msg
	}
	return fmt.Sprintf("%s: %v", msg, e.Err)
}

func (e InvalidProviderConfigError) Unwrap() error {
	return e.Err
}

func (e InvalidProviderConfigError) Is(target error) bool {
	return target == ErrInvalidProviderConfig
}

// InvalidProviderConfigFieldError wraps invalid provider config field values.
type InvalidProviderConfigFieldError struct {
	Provider string
	Field    string
	Err      error
}

func (e InvalidProviderConfigFieldError) Error() string {
	msg := fmt.Sprintf("invalid %s %s", e.Provider, e.Field)
	if e.Err == nil {
		return msg
	}
	return fmt.Sprintf("%s: %v", msg, e.Err)
}

func (e InvalidProviderConfigFieldError) Unwrap() error {
	return e.Err
}

func (e InvalidProviderConfigFieldError) Is(target error) bool {
	return target == ErrInvalidProviderConfigField
}
