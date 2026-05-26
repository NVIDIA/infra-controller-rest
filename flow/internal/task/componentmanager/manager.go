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

// Package componentmanager defines the manager contracts used to dispatch task
// operations. Each manager owns its descriptor metadata, which is used by the
// registry to validate configured implementations and supported capabilities.
package componentmanager

import (
	cmcatalog "github.com/NVIDIA/infra-controller-rest/flow/internal/task/componentmanager/catalog"
)

// ComponentManager defines the common identity and metadata every component
// manager must expose. Operation methods live on capability-specific
// interfaces so managers only implement the operations they support.
type ComponentManager interface {
	// Descriptor returns the component manager metadata for this manager.
	Descriptor() cmcatalog.Descriptor
}
