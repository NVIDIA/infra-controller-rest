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

package rla

import (
	swa "github.com/NVIDIA/infra-controller-rest/site-workflow/pkg/activity"
	sww "github.com/NVIDIA/infra-controller-rest/site-workflow/pkg/workflow"
)

// RegisterSubscriber registers RLA rack and tray workflows and activities with Temporal
func (api *API) RegisterSubscriber() error {
	// Check if RLA is enabled
	if !ManagerAccess.Conf.EB.RLA.Enabled {
		ManagerAccess.Data.EB.Log.Info().Msg("RLA: RLA is disabled, skipping workflow registration")
		return nil
	}

	// Register rack workflows
	ManagerAccess.Data.EB.Log.Info().Msg("RLA: Registering rack workflows")

	// Register GetRack workflow
	ManagerAccess.Data.EB.Managers.Workflow.Temporal.Worker.RegisterWorkflow(sww.GetRack)
	ManagerAccess.Data.EB.Log.Info().Msg("RLA: Successfully registered GetRack workflow")

	// Register GetRacks workflow
	ManagerAccess.Data.EB.Managers.Workflow.Temporal.Worker.RegisterWorkflow(sww.GetRacks)
	ManagerAccess.Data.EB.Log.Info().Msg("RLA: Successfully registered GetRacks workflow")

	// Register ValidateRackComponents workflow
	ManagerAccess.Data.EB.Managers.Workflow.Temporal.Worker.RegisterWorkflow(sww.ValidateRackComponents)
	ManagerAccess.Data.EB.Log.Info().Msg("RLA: Successfully registered ValidateRackComponents workflow")

	// Register PowerOnRack workflow
	ManagerAccess.Data.EB.Managers.Workflow.Temporal.Worker.RegisterWorkflow(sww.PowerOnRack)
	ManagerAccess.Data.EB.Log.Info().Msg("RLA: Successfully registered PowerOnRack workflow")

	// Register PowerOffRack workflow
	ManagerAccess.Data.EB.Managers.Workflow.Temporal.Worker.RegisterWorkflow(sww.PowerOffRack)
	ManagerAccess.Data.EB.Log.Info().Msg("RLA: Successfully registered PowerOffRack workflow")

	// Register PowerResetRack workflow
	ManagerAccess.Data.EB.Managers.Workflow.Temporal.Worker.RegisterWorkflow(sww.PowerResetRack)
	ManagerAccess.Data.EB.Log.Info().Msg("RLA: Successfully registered PowerResetRack workflow")

	// Register BringUpRack workflow
	ManagerAccess.Data.EB.Managers.Workflow.Temporal.Worker.RegisterWorkflow(sww.BringUpRack)
	ManagerAccess.Data.EB.Log.Info().Msg("RLA: Successfully registered BringUpRack workflow")

	// Register UpgradeFirmware workflow
	ManagerAccess.Data.EB.Managers.Workflow.Temporal.Worker.RegisterWorkflow(sww.UpgradeFirmware)
	ManagerAccess.Data.EB.Log.Info().Msg("RLA: Successfully registered UpgradeFirmware workflow")

	// Register GetRackTask workflow
	ManagerAccess.Data.EB.Managers.Workflow.Temporal.Worker.RegisterWorkflow(sww.GetRackTask)
	ManagerAccess.Data.EB.Log.Info().Msg("RLA: Successfully registered GetRackTask workflow")

	// Register activities
	rackManager := swa.NewManageRack(ManagerAccess.Data.EB.Managers.RLA.Client)

	// Register GetRack activity
	ManagerAccess.Data.EB.Managers.Workflow.Temporal.Worker.RegisterActivity(rackManager.GetRack)
	ManagerAccess.Data.EB.Log.Info().Msg("RLA: Successfully registered GetRack activity")

	// Register GetRacks activity
	ManagerAccess.Data.EB.Managers.Workflow.Temporal.Worker.RegisterActivity(rackManager.GetRacks)
	ManagerAccess.Data.EB.Log.Info().Msg("RLA: Successfully registered GetRacks activity")

	// Register ValidateRackComponents activity
	ManagerAccess.Data.EB.Managers.Workflow.Temporal.Worker.RegisterActivity(rackManager.ValidateRackComponents)
	ManagerAccess.Data.EB.Log.Info().Msg("RLA: Successfully registered ValidateRackComponents activity")

	// Register PowerOnRack activity
	ManagerAccess.Data.EB.Managers.Workflow.Temporal.Worker.RegisterActivity(rackManager.PowerOnRack)
	ManagerAccess.Data.EB.Log.Info().Msg("RLA: Successfully registered PowerOnRack activity")

	// Register PowerOffRack activity
	ManagerAccess.Data.EB.Managers.Workflow.Temporal.Worker.RegisterActivity(rackManager.PowerOffRack)
	ManagerAccess.Data.EB.Log.Info().Msg("RLA: Successfully registered PowerOffRack activity")

	// Register PowerResetRack activity
	ManagerAccess.Data.EB.Managers.Workflow.Temporal.Worker.RegisterActivity(rackManager.PowerResetRack)
	ManagerAccess.Data.EB.Log.Info().Msg("RLA: Successfully registered PowerResetRack activity")

	// Register BringUpRack activity
	ManagerAccess.Data.EB.Managers.Workflow.Temporal.Worker.RegisterActivity(rackManager.BringUpRack)
	ManagerAccess.Data.EB.Log.Info().Msg("RLA: Successfully registered BringUpRack activity")

	// Register UpgradeFirmware activity
	ManagerAccess.Data.EB.Managers.Workflow.Temporal.Worker.RegisterActivity(rackManager.UpgradeFirmware)
	ManagerAccess.Data.EB.Log.Info().Msg("RLA: Successfully registered UpgradeFirmware activity")

	// Register GetTaskByID activity
	ManagerAccess.Data.EB.Managers.Workflow.Temporal.Worker.RegisterActivity(rackManager.GetTaskByID)
	ManagerAccess.Data.EB.Log.Info().Msg("RLA: Successfully registered GetTaskByID activity")

	// Register the tray subscribers here
	ManagerAccess.Data.EB.Log.Info().Msg("RLA: Registering tray workflows")

	// Register Tray workflows

	// Register GetTray workflow
	ManagerAccess.Data.EB.Managers.Workflow.Temporal.Worker.RegisterWorkflow(sww.GetTray)
	ManagerAccess.Data.EB.Log.Info().Msg("RLA: Successfully registered GetTray workflow")

	// Register GetTrays workflow
	ManagerAccess.Data.EB.Managers.Workflow.Temporal.Worker.RegisterWorkflow(sww.GetTrays)
	ManagerAccess.Data.EB.Log.Info().Msg("RLA: Successfully registered GetTrays workflow")

	// Register tray activities
	trayManager := swa.NewManageTray(ManagerAccess.Data.EB.Managers.RLA.Client)

	// Register GetTray activity
	ManagerAccess.Data.EB.Managers.Workflow.Temporal.Worker.RegisterActivity(trayManager.GetTray)
	ManagerAccess.Data.EB.Log.Info().Msg("RLA: Successfully registered GetTray activity")

	// Register GetTrays activity
	ManagerAccess.Data.EB.Managers.Workflow.Temporal.Worker.RegisterActivity(trayManager.GetTrays)
	ManagerAccess.Data.EB.Log.Info().Msg("RLA: Successfully registered GetTrays activity")

	return nil
}
