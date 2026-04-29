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

package nvlinklogicalpartition

import (
	swa "github.com/NVIDIA/ncx-infra-controller-rest/site-workflow/pkg/activity"
	sww "github.com/NVIDIA/ncx-infra-controller-rest/site-workflow/pkg/workflow"
)

// RegisterSubscriber registers the NVLinkLogicalPartitionWorkflows with the Temporal client
func (api *API) RegisterSubscriber() error {
	// Register the subscribers here
	ManagerAccess.Data.EB.Log.Info().Msg("NVLinkLogicalPartition: Registering the subscribers")

	// Register workflows
	// Register CreateNVLinkLogicalPartition workflow
	ManagerAccess.Data.EB.Managers.Workflow.Temporal.Worker.RegisterWorkflow(sww.CreateNVLinkLogicalPartition)
	ManagerAccess.Data.EB.Log.Info().Msg("NVLinkLogicalPartition: successfully registered the CreateNVLinkLogicalPartition workflow")

	// Register UpdateNVLinkLogicalPartition workflow
	ManagerAccess.Data.EB.Managers.Workflow.Temporal.Worker.RegisterWorkflow(sww.UpdateNVLinkLogicalPartition)
	ManagerAccess.Data.EB.Log.Info().Msg("NVLinkLogicalPartition: successfully registered the UpdateNVLinkLogicalPartition workflow")

	// Register DeleteNVLinkLogicalPartition workflow
	ManagerAccess.Data.EB.Managers.Workflow.Temporal.Worker.RegisterWorkflow(sww.DeleteNVLinkLogicalPartition)
	ManagerAccess.Data.EB.Log.Info().Msg("NVLinkLogicalPartition: successfully registered the DeleteNVLinkLogicalPartition workflow")

	// Register activities
	nvlinkLogicalPartitionManager := swa.NewManageNVLinkLogicalPartition(ManagerAccess.Data.EB.Managers.NICo.Client)

	// Register CreateNVLinkLogicalPartitionOnSite activity
	ManagerAccess.Data.EB.Managers.Workflow.Temporal.Worker.RegisterActivity(nvlinkLogicalPartitionManager.CreateNVLinkLogicalPartitionOnSite)
	ManagerAccess.Data.EB.Log.Info().Msg("NVLinkLogicalPartition: successfully registered the CreateNVLinkLogicalPartitionOnSite activity")

	// Register UpdateNVLinkLogicalPartitionOnSite activity
	ManagerAccess.Data.EB.Managers.Workflow.Temporal.Worker.RegisterActivity(nvlinkLogicalPartitionManager.UpdateNVLinkLogicalPartitionOnSite)
	ManagerAccess.Data.EB.Log.Info().Msg("NVLinkLogicalPartition: successfully registered the UpdateNVLinkLogicalPartitionOnSite activity")

	// Register DeleteNVLinkLogicalPartitionOnSite activity
	ManagerAccess.Data.EB.Managers.Workflow.Temporal.Worker.RegisterActivity(nvlinkLogicalPartitionManager.DeleteNVLinkLogicalPartitionOnSite)
	ManagerAccess.Data.EB.Log.Info().Msg("NVLinkLogicalPartition: successfully registered the DeleteNVLinkLogicalPartitionOnSite activity")

	return nil
}
