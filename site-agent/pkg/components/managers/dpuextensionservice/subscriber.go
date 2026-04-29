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

package dpuextensionservice

import (
	swa "github.com/NVIDIA/ncx-infra-controller-rest/site-workflow/pkg/activity"
	sww "github.com/NVIDIA/ncx-infra-controller-rest/site-workflow/pkg/workflow"
)

// RegisterSubscriber registers DPU Extension Service workflows Site Agent subscribes to execute
func (api *API) RegisterSubscriber() error {
	//  Register Workflows

	// Sync workflows
	// Register CreateDpuExtensionService workflow
	ManagerAccess.Data.EB.Managers.Workflow.Temporal.Worker.RegisterWorkflow(sww.CreateDpuExtensionService)
	ManagerAccess.Data.EB.Log.Info().Msg("DpuExtensionService: successfully registered CreateDpuExtensionService workflow")

	// Register UpdateDpuExtensionService workflow
	ManagerAccess.Data.EB.Managers.Workflow.Temporal.Worker.RegisterWorkflow(sww.UpdateDpuExtensionService)
	ManagerAccess.Data.EB.Log.Info().Msg("DpuExtensionService: successfully registered UpdateDpuExtensionService workflow")

	// Register DeleteDpuExtensionService workflow
	ManagerAccess.Data.EB.Managers.Workflow.Temporal.Worker.RegisterWorkflow(sww.DeleteDpuExtensionService)
	ManagerAccess.Data.EB.Log.Info().Msg("DpuExtensionService: successfully registered DeleteDpuExtensionService workflow")

	// Register GetDpuExtensionServiceVersionsInfo workflow
	ManagerAccess.Data.EB.Managers.Workflow.Temporal.Worker.RegisterWorkflow(sww.GetDpuExtensionServiceVersionsInfo)
	ManagerAccess.Data.EB.Log.Info().Msg("DpuExtensionService: successfully registered GetDpuExtensionServiceVersionsInfo workflow")

	// Register Activities
	dpuExtServiceManager := swa.NewManageDpuExtensionService(ManagerAccess.Data.EB.Managers.NICo.Client)

	// Sync workflow activities
	// Register CreateDpuExtensionServiceOnSite
	ManagerAccess.Data.EB.Managers.Workflow.Temporal.Worker.RegisterActivity(dpuExtServiceManager.CreateDpuExtensionServiceOnSite)
	ManagerAccess.Data.EB.Log.Info().Msg("DpuExtensionService: successfully registered CreateDpuExtensionServiceOnSite activity")

	// Register UpdateDpuExtensionServiceOnSite
	ManagerAccess.Data.EB.Managers.Workflow.Temporal.Worker.RegisterActivity(dpuExtServiceManager.UpdateDpuExtensionServiceOnSite)
	ManagerAccess.Data.EB.Log.Info().Msg("DpuExtensionService: successfully registered UpdateDpuExtensionServiceOnSite activity")

	// Register DeleteDpuExtensionServiceOnSite
	ManagerAccess.Data.EB.Managers.Workflow.Temporal.Worker.RegisterActivity(dpuExtServiceManager.DeleteDpuExtensionServiceOnSite)
	ManagerAccess.Data.EB.Log.Info().Msg("DpuExtensionService: successfully registered DeleteDpuExtensionServiceOnSite activity")

	// Register GetDpuExtensionServiceVersionsInfoOnSite activity
	ManagerAccess.Data.EB.Managers.Workflow.Temporal.Worker.RegisterActivity(dpuExtServiceManager.GetDpuExtensionServiceVersionsInfoOnSite)
	ManagerAccess.Data.EB.Log.Info().Msg("DpuExtensionService: successfully registered GetDpuExtensionServiceVersionsInfoOnSite activity")

	return nil
}
