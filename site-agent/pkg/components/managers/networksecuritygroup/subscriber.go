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

package networksecuritygroup

import (
	swa "github.com/NVIDIA/ncx-infra-controller-rest/site-workflow/pkg/activity"
	sww "github.com/NVIDIA/ncx-infra-controller-rest/site-workflow/pkg/workflow"
)

// RegisterSubscriber registers the NetworkSecurityGroup workflows and activities with the Temporal client
func (api *API) RegisterSubscriber() error {
	// Register the subscribers here
	ManagerAccess.Data.EB.Log.Info().Msg("NetworkSecurityGroup: Registering the subscribers")

	networkSecurityGroupManager := swa.NewManageNetworkSecurityGroup(ManagerAccess.Data.EB.Managers.NICo.Client)

	//  Register Workflows

	// Sync workflows
	// Register CreateNetworkSecurityGroup worfklow
	ManagerAccess.Data.EB.Managers.Workflow.Temporal.Worker.RegisterWorkflow(sww.CreateNetworkSecurityGroup)
	ManagerAccess.Data.EB.Log.Info().Msg("NetworkSecurityGroup: successfully registered Create NetworkSecurityGroup workflow")

	// Register UpdateNetworkSecurityGroup worfklow
	ManagerAccess.Data.EB.Managers.Workflow.Temporal.Worker.RegisterWorkflow(sww.UpdateNetworkSecurityGroup)
	ManagerAccess.Data.EB.Log.Info().Msg("NetworkSecurityGroup: successfully registered Update NetworkSecurityGroup workflow")

	// Register DeleteNetworkSecurityGroup worfklow
	ManagerAccess.Data.EB.Managers.Workflow.Temporal.Worker.RegisterWorkflow(sww.DeleteNetworkSecurityGroup)
	ManagerAccess.Data.EB.Log.Info().Msg("NetworkSecurityGroup: successfully registered Delete NetworkSecurityGroup workflow")

	// Register Activities

	// Sync workflow activities
	// Register CreateNetworkSecurityGroupOnSite
	ManagerAccess.Data.EB.Managers.Workflow.Temporal.Worker.RegisterActivity(networkSecurityGroupManager.CreateNetworkSecurityGroupOnSite)
	ManagerAccess.Data.EB.Log.Info().Msg("NetworkSecurityGroup: successfully registered Create NetworkSecurityGroup activity")

	// Register UpdateNetworkSecurityGroupOnSite
	ManagerAccess.Data.EB.Managers.Workflow.Temporal.Worker.RegisterActivity(networkSecurityGroupManager.UpdateNetworkSecurityGroupOnSite)
	ManagerAccess.Data.EB.Log.Info().Msg("NetworkSecurityGroup: successfully registered Update NetworkSecurityGroup activity")

	// Register DeleteNetworkSecurityGroupOnSite
	ManagerAccess.Data.EB.Managers.Workflow.Temporal.Worker.RegisterActivity(networkSecurityGroupManager.DeleteNetworkSecurityGroupOnSite)
	ManagerAccess.Data.EB.Log.Info().Msg("NetworkSecurityGroup: successfully registered Delete NetworkSecurityGroup activity")

	return nil
}
