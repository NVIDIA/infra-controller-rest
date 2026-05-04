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
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/NVIDIA/infra-controller-rest/rla/internal/task/componentmanager/providers/nico"
	"github.com/NVIDIA/infra-controller-rest/rla/internal/task/componentmanager/providers/nvswitchmanager"
	"github.com/NVIDIA/infra-controller-rest/rla/internal/task/componentmanager/providers/psm"
)

func TestNewServiceProviderConfigDecoderRegistry(t *testing.T) {
	registry, err := NewServiceProviderConfigDecoderRegistry()
	require.NoError(t, err)

	assert.ElementsMatch(
		t,
		[]string{
			nico.ProviderName,
			psm.ProviderName,
			nvswitchmanager.ProviderName,
		},
		registry.List(),
	)

	_, ok := registry.Get(nico.ProviderName)
	assert.True(t, ok)

	_, ok = registry.Get(psm.ProviderName)
	assert.True(t, ok)

	_, ok = registry.Get(nvswitchmanager.ProviderName)
	assert.True(t, ok)
}
