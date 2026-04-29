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

package instancetype

import (
	swa "github.com/NVIDIA/ncx-infra-controller-rest/site-workflow/pkg/activity"
	sww "github.com/NVIDIA/ncx-infra-controller-rest/site-workflow/pkg/workflow"
)

// RegisterSubscriber registers the InstanceType workflows and activities with the Temporal client
func (api *API) RegisterSubscriber() error {
	// Register the subscribers here
	ManagerAccess.Data.EB.Log.Info().Msg("InstanceType: Registering the subscribers")

	instanceTypeManager := swa.NewManageInstanceType(ManagerAccess.Data.EB.Managers.NICo.Client)

	//  Register Workflows

	// Sync workflows
	// Register CreateInstanceType worfklow
	ManagerAccess.Data.EB.Managers.Workflow.Temporal.Worker.RegisterWorkflow(sww.CreateInstanceType)
	ManagerAccess.Data.EB.Log.Info().Msg("InstanceType: successfully registered Create InstanceType workflow")

	// Register UpdateInstanceType worfklow
	ManagerAccess.Data.EB.Managers.Workflow.Temporal.Worker.RegisterWorkflow(sww.UpdateInstanceType)
	ManagerAccess.Data.EB.Log.Info().Msg("InstanceType: successfully registered Update InstanceType workflow")

	// Register DeleteInstanceType worfklow
	ManagerAccess.Data.EB.Managers.Workflow.Temporal.Worker.RegisterWorkflow(sww.DeleteInstanceType)
	ManagerAccess.Data.EB.Log.Info().Msg("InstanceType: successfully registered Delete InstanceType workflow")

	// Register AssociateMachinesWithInstanceType worfklow
	ManagerAccess.Data.EB.Managers.Workflow.Temporal.Worker.RegisterWorkflow(sww.AssociateMachinesWithInstanceType)

	// Register RemoveMachineInstanceTypeAssociation worfklow
	ManagerAccess.Data.EB.Managers.Workflow.Temporal.Worker.RegisterWorkflow(sww.RemoveMachineInstanceTypeAssociation)

	// Regsiter Activities

	// Sync workflow activities
	// Register CreateInstanceTypeOnSite
	ManagerAccess.Data.EB.Managers.Workflow.Temporal.Worker.RegisterActivity(instanceTypeManager.CreateInstanceTypeOnSite)
	ManagerAccess.Data.EB.Log.Info().Msg("InstanceType: successfully registered Create InstanceType activity")

	// Register UpdateInstanceTypeOnSite
	ManagerAccess.Data.EB.Managers.Workflow.Temporal.Worker.RegisterActivity(instanceTypeManager.UpdateInstanceTypeOnSite)
	ManagerAccess.Data.EB.Log.Info().Msg("InstanceType: successfully registered Update InstanceType activity")

	// Register DeleteInstanceTypeOnSite
	ManagerAccess.Data.EB.Managers.Workflow.Temporal.Worker.RegisterActivity(instanceTypeManager.DeleteInstanceTypeOnSite)
	ManagerAccess.Data.EB.Log.Info().Msg("InstanceType: successfully registered Delete InstanceType activity")

	// Register AssociateMachinesWithInstanceTypeOnSite
	ManagerAccess.Data.EB.Managers.Workflow.Temporal.Worker.RegisterActivity(instanceTypeManager.AssociateMachinesWithInstanceTypeOnSite)
	ManagerAccess.Data.EB.Log.Info().Msg("InstanceType: successfully registered AssociateMachinesWithInstanceType activity")

	// Register RemoveMachineInstanceTypeAssociationOnSite
	ManagerAccess.Data.EB.Managers.Workflow.Temporal.Worker.RegisterActivity(instanceTypeManager.RemoveMachineInstanceTypeAssociationOnSite)
	ManagerAccess.Data.EB.Log.Info().Msg("InstanceType: successfully registered RemoveMachineInstanceTypeAssociation activity")

	return nil
}
