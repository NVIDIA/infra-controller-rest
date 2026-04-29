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

package operatingsystem

import (
	swa "github.com/NVIDIA/ncx-infra-controller-rest/site-workflow/pkg/activity"
	sww "github.com/NVIDIA/ncx-infra-controller-rest/site-workflow/pkg/workflow"
)

// RegisterSubscriber registers the Machine workflows/activities with the Temporal client
func (api *API) RegisterSubscriber() error {
	// Register subscriber workflows
	ManagerAccess.Data.EB.Log.Info().Msg("Machine: Registering the subscribers")

	osImageManager := swa.NewManageOperatingSystem(ManagerAccess.Data.EB.Managers.NICo.Client)

	// Register workflows

	// Sync workflows
	// Register CreateOsImage workflow
	ManagerAccess.Data.EB.Managers.Workflow.Temporal.Worker.RegisterWorkflow(sww.CreateOsImage)
	ManagerAccess.Data.EB.Log.Info().Msg("OsImage: successfully registered the CreateOsImage workflow")

	// Register UpdateOsImage workflow
	ManagerAccess.Data.EB.Managers.Workflow.Temporal.Worker.RegisterWorkflow(sww.UpdateOsImage)
	ManagerAccess.Data.EB.Log.Info().Msg("OsImage: successfully registered the UpdateOsImage workflow")

	// Register DeleteOsImage workflow
	ManagerAccess.Data.EB.Managers.Workflow.Temporal.Worker.RegisterWorkflow(sww.DeleteOsImage)
	ManagerAccess.Data.EB.Log.Info().Msg("OsImage: successfully registered the DeleteOsImage workflow")

	// Register activities

	// Sync workflow activities
	// Register CreateOsImageOnSite
	ManagerAccess.Data.EB.Managers.Workflow.Temporal.Worker.RegisterActivity(osImageManager.CreateOsImageOnSite)
	ManagerAccess.Data.EB.Log.Info().Msg("OsImage: successfully registered CreateOsImageOnSite activity")

	// Register UpdateOsImageOnSite
	ManagerAccess.Data.EB.Managers.Workflow.Temporal.Worker.RegisterActivity(osImageManager.UpdateOsImageOnSite)
	ManagerAccess.Data.EB.Log.Info().Msg("OsImage: successfully registered UpdateOsImageOnSite activity")

	// Register DeleteOsImageOnSite
	ManagerAccess.Data.EB.Managers.Workflow.Temporal.Worker.RegisterActivity(osImageManager.DeleteOsImageOnSite)
	ManagerAccess.Data.EB.Log.Info().Msg("OsImage: successfully registered DeleteOsImageOnSite activity")

	return nil
}
