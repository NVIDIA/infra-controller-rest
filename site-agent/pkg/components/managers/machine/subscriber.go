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

package machine

import (
	swa "github.com/NVIDIA/ncx-infra-controller-rest/site-workflow/pkg/activity"
	sww "github.com/NVIDIA/ncx-infra-controller-rest/site-workflow/pkg/workflow"
)

// RegisterSubscriber registers the Machine workflows/activities with the Temporal client
func (api *API) RegisterSubscriber() error {
	// Register subscriber workflows
	ManagerAccess.Data.EB.Log.Info().Msg("Machine: Registering the subscribers")

	machineManager := swa.NewManageMachine(ManagerAccess.Data.EB.Managers.NICo.Client)

	// Register workflows

	// Sync workflows
	// Set Maintenance Mode workflow
	ManagerAccess.Data.EB.Managers.Workflow.Temporal.Worker.RegisterWorkflow(sww.SetMachineMaintenance)
	ManagerAccess.Data.EB.Log.Info().Msg("Machine: successfully registered the Set Machine Maintenance workflow")

	ManagerAccess.Data.EB.Managers.Workflow.Temporal.Worker.RegisterWorkflow(sww.UpdateMachineMetadata)
	ManagerAccess.Data.EB.Log.Info().Msg("Machine: successfully registered the Update Machine Metadata workflow")

	// Register activities

	// Sync workflow activities
	// Register Machine activities
	ManagerAccess.Data.EB.Managers.Workflow.Temporal.Worker.RegisterActivity(machineManager.SetMachineMaintenanceOnSite)
	ManagerAccess.Data.EB.Log.Info().Msg("Machine: successfully registered the Set Machine Maintenance activity")

	ManagerAccess.Data.EB.Managers.Workflow.Temporal.Worker.RegisterActivity(machineManager.UpdateMachineMetadataOnSite)
	ManagerAccess.Data.EB.Log.Info().Msg("Machine: successfully registered the Update Machine Metadata activity")

	return nil
}
