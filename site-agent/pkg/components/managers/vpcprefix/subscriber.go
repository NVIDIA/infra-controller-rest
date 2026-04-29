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

package vpcprefix

import (
	swa "github.com/NVIDIA/ncx-infra-controller-rest/site-workflow/pkg/activity"
	sww "github.com/NVIDIA/ncx-infra-controller-rest/site-workflow/pkg/workflow"
)

// RegisterSubscriber registers the VpcPrefixWorkflows with the Temporal client
func (api *API) RegisterSubscriber() error {
	// Register the subscribers here
	ManagerAccess.Data.EB.Log.Info().Msg("VpcPrefix: Registering the subscribers")

	vpcPrefixManager := swa.NewManageVpcPrefix(ManagerAccess.Data.EB.Managers.NICo.Client)

	//  Register Workflows

	// Sync workflows
	// Register CreateVpcPrefix worfklow
	ManagerAccess.Data.EB.Managers.Workflow.Temporal.Worker.RegisterWorkflow(sww.CreateVpcPrefix)
	ManagerAccess.Data.EB.Log.Info().Msg("VpcPrefix: successfully registered Create VpcPrefix workflow")

	// Register UpdateVpcPrefix worfklow
	ManagerAccess.Data.EB.Managers.Workflow.Temporal.Worker.RegisterWorkflow(sww.UpdateVpcPrefix)
	ManagerAccess.Data.EB.Log.Info().Msg("VpcPrefix: successfully registered Update VpcPrefix workflow")

	// Register DeleteVpcPrefix worfklow
	ManagerAccess.Data.EB.Managers.Workflow.Temporal.Worker.RegisterWorkflow(sww.DeleteVpcPrefix)
	ManagerAccess.Data.EB.Log.Info().Msg("VpcPrefix: successfully registered Delete VpcPrefix workflow")

	// Regsiter Activities

	// Sync workflow activities
	// Register CreateVpcPrefixOnSite
	ManagerAccess.Data.EB.Managers.Workflow.Temporal.Worker.RegisterActivity(vpcPrefixManager.CreateVpcPrefixOnSite)
	ManagerAccess.Data.EB.Log.Info().Msg("VpcPrefix: successfully registered Create VpcPrefix activity")

	// Register UpdateVpcPrefixOnSite
	ManagerAccess.Data.EB.Managers.Workflow.Temporal.Worker.RegisterActivity(vpcPrefixManager.UpdateVpcPrefixOnSite)
	ManagerAccess.Data.EB.Log.Info().Msg("VpcPrefix: successfully registered Update VpcPrefix activity")

	// Register DeleteVpcPrefixOnSite
	ManagerAccess.Data.EB.Managers.Workflow.Temporal.Worker.RegisterActivity(vpcPrefixManager.DeleteVpcPrefixOnSite)
	ManagerAccess.Data.EB.Log.Info().Msg("VpcPrefix: successfully registered Delete VpcPrefix activity")

	return nil
}
