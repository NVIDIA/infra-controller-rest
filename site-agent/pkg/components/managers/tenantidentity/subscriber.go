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

package tenantidentity

import (
	swa "github.com/NVIDIA/infra-controller-rest/site-workflow/pkg/activity"
	sww "github.com/NVIDIA/infra-controller-rest/site-workflow/pkg/workflow"
)

// RegisterSubscriber registers one workflow+activity pair per Forge
// tenant-identity RPC with the Temporal worker.
func (mi *API) RegisterSubscriber() error {
	ManagerAccess.Data.EB.Log.Info().Msg("TenantIdentity: Registering the subscribers")

	manager := swa.NewManageTenantIdentity(ManagerAccess.Data.EB.Managers.CoreGrpc.Client)
	w := ManagerAccess.Data.EB.Managers.Workflow.Temporal.Worker

	w.RegisterWorkflow(sww.SetTenantIdentityConfiguration)
	w.RegisterActivity(manager.SetTenantIdentityConfigurationOnSite)
	ManagerAccess.Data.EB.Log.Info().Msg("TenantIdentity: Successfully registered SetTenantIdentityConfiguration workflow & activity")

	w.RegisterWorkflow(sww.GetTenantIdentityConfiguration)
	w.RegisterActivity(manager.GetTenantIdentityConfigurationFromSite)
	ManagerAccess.Data.EB.Log.Info().Msg("TenantIdentity: Successfully registered GetTenantIdentityConfiguration workflow & activity")

	w.RegisterWorkflow(sww.DeleteTenantIdentityConfiguration)
	w.RegisterActivity(manager.DeleteTenantIdentityConfigurationOnSite)
	ManagerAccess.Data.EB.Log.Info().Msg("TenantIdentity: Successfully registered DeleteTenantIdentityConfiguration workflow & activity")

	w.RegisterWorkflow(sww.SetTenantIdentityTokenDelegation)
	w.RegisterActivity(manager.SetTenantIdentityTokenDelegationOnSite)
	ManagerAccess.Data.EB.Log.Info().Msg("TenantIdentity: Successfully registered SetTenantIdentityTokenDelegation workflow & activity")

	w.RegisterWorkflow(sww.GetTenantIdentityTokenDelegation)
	w.RegisterActivity(manager.GetTenantIdentityTokenDelegationFromSite)
	ManagerAccess.Data.EB.Log.Info().Msg("TenantIdentity: Successfully registered GetTenantIdentityTokenDelegation workflow & activity")

	w.RegisterWorkflow(sww.DeleteTenantIdentityTokenDelegation)
	w.RegisterActivity(manager.DeleteTenantIdentityTokenDelegationOnSite)
	ManagerAccess.Data.EB.Log.Info().Msg("TenantIdentity: Successfully registered DeleteTenantIdentityTokenDelegation workflow & activity")

	w.RegisterWorkflow(sww.GetJWKS)
	w.RegisterActivity(manager.GetJWKSFromSite)
	ManagerAccess.Data.EB.Log.Info().Msg("TenantIdentity: Successfully registered GetJWKS workflow & activity")

	w.RegisterWorkflow(sww.GetOpenIDConfiguration)
	w.RegisterActivity(manager.GetOpenIDConfigurationFromSite)
	ManagerAccess.Data.EB.Log.Info().Msg("TenantIdentity: Successfully registered GetOpenIDConfiguration workflow & activity")

	return nil
}
