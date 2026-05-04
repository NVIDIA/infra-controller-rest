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

package activity

import (
	"context"
	"errors"
	"time"

	swe "github.com/NVIDIA/ncx-infra-controller-rest/site-workflow/pkg/error"
	cClient "github.com/NVIDIA/ncx-infra-controller-rest/site-workflow/pkg/grpc/client"
	cwssaws "github.com/NVIDIA/ncx-infra-controller-rest/workflow-schema/schema/site-agent/workflows/v1"
	"github.com/rs/zerolog/log"
	"go.temporal.io/sdk/temporal"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// ManageVPC is an activity wrapper for VPC management
// TODO: Do we really need a distinction between general management and inventory?
// The pattern is elsewhere as well, but it seems like we could condense them since
// Manage*Inventory.config has a property that holds a *client.CarbideAtomicClient.
type ManageVPC struct {
	CarbideAtomicClient *cClient.CarbideAtomicClient
}

// ManageVPCInventory is an activity wrapper for VPC inventory collection and publishing
type ManageVPCInventory struct {
	config ManageInventoryConfig
}

// NewManageVPC returns a new ManageVPC client
func NewManageVPC(carbideClient *cClient.CarbideAtomicClient) ManageVPC {
	return ManageVPC{
		CarbideAtomicClient: carbideClient,
	}
}

// DiscoverVPCInventory is an activity to collect VPC inventory and publish to Temporal queue
func (mvi *ManageVPCInventory) DiscoverVPCInventory(ctx context.Context) error {
	logger := log.With().Str("Activity", "DiscoverVPCInventory").Logger()
	logger.Info().Msg("Starting activity")
	inventoryImpl := manageInventoryImpl[*cwssaws.VpcId, *cwssaws.Vpc, *cwssaws.VPCInventory]{
		itemType:                          "Vpc",
		config:                            mvi.config,
		internalFindIDs:                   vpcFindIDs,
		internalFindByIDs:                 vpcFindByIDs,
		internalPagedInventory:            vpcPagedInventory,
		internalPagedInventoryPostProcess: vpcPagedInventoryPostProcess,
	}
	return inventoryImpl.CollectAndPublishInventory(ctx, &logger)
}

// NewManageVPCInventory returns a ManageInventory implementation for VPC activity
func NewManageVPCInventory(config ManageInventoryConfig) ManageVPCInventory {
	return ManageVPCInventory{
		config: config,
	}
}

func vpcFindIDs(ctx context.Context, carbideClient *cClient.CarbideClient) ([]*cwssaws.VpcId, error) {
	forgeClient := carbideClient.Carbide()

	idList, err := forgeClient.FindVpcIds(ctx, &cwssaws.VpcSearchFilter{})
	if err != nil {
		return nil, err
	}
	return idList.GetVpcIds(), nil
}

func vpcFindByIDs(ctx context.Context, carbideClient *cClient.CarbideClient, ids []*cwssaws.VpcId) ([]*cwssaws.Vpc, error) {
	forgeClient := carbideClient.Carbide()

	list, err := forgeClient.FindVpcsByIds(ctx, &cwssaws.VpcsByIdsRequest{
		VpcIds: ids,
	})
	if err != nil {
		return nil, err
	}

	return list.GetVpcs(), nil
}

// instancePagedInventoryPostProcess will attach NSG propagation
// information for the inventory page of VPCs.
// This will only be called for pages with inventory.
func vpcPagedInventoryPostProcess(ctx context.Context, carbideClient *cClient.CarbideClient, inventory *cwssaws.VPCInventory) (*cwssaws.VPCInventory, error) {

	vpcIds := make([]string, len(inventory.GetVpcs()))

	for i, vpc := range inventory.GetVpcs() {
		vpcIds[i] = vpc.GetId().GetValue()
	}

	propList, err := carbideClient.Carbide().GetNetworkSecurityGroupPropagationStatus(ctx, &cwssaws.GetNetworkSecurityGroupPropagationStatusRequest{
		VpcIds: vpcIds,
	})

	if err != nil {
		return nil, err
	}

	inventory.NetworkSecurityGroupPropagations = propList.GetVpcs()

	return inventory, nil
}

func vpcPagedInventory(allItemIDs []*cwssaws.VpcId, pagedItems []*cwssaws.Vpc, input *pagedInventoryInput) *cwssaws.VPCInventory {
	itemIDs := []string{}
	for _, id := range allItemIDs {
		itemIDs = append(itemIDs, id.GetValue())
	}

	// Create an inventory page with the subset of VPCs
	inventory := &cwssaws.VPCInventory{
		Vpcs: pagedItems,
		Timestamp: &timestamppb.Timestamp{
			Seconds: time.Now().Unix(),
		},
		InventoryStatus: input.status,
		StatusMsg:       input.statusMessage,
		InventoryPage:   input.buildPage(),
	}
	if inventory.InventoryPage != nil {
		inventory.InventoryPage.ItemIds = itemIDs
	}
	return inventory
}

// Function to create VPCS with Carbide
func (mv *ManageVPC) CreateVpcOnSite(ctx context.Context, request *cwssaws.VpcCreationRequest) (*cwssaws.Vpc, error) {
	logger := log.With().Str("Activity", "CreateVpcOnSite").Logger()

	logger.Info().Msg("Starting activity")

	var err error

	// Validate request
	switch {
	case request == nil:
		err = errors.New("received empty create VPC request")
	case request.Name == "":
		err = errors.New("received create VPC request missing name")
	case request.TenantOrganizationId == "":
		err = errors.New("received create VPC request missing TenantOrganizationId")
	case request.Id == nil || request.Id.Value == "":
		// Don't let a request come in without a cloud-provided ID
		// or carbide will generate one and cloud won't know the relationship.
		err = errors.New("received create VPC request missing VPC ID")
	}

	if err != nil {
		return nil, temporal.NewNonRetryableApplicationError(err.Error(), swe.ErrTypeInvalidRequest, err)
	}

	// Call Site Controller gRPC endpoint
	forgeClient, err := mv.CarbideAtomicClient.GetForgeClient()
	if err != nil {
		return nil, err
	}

	controllerVpc, err := forgeClient.CreateVpc(ctx, request)
	if err != nil {
		logger.Warn().Err(err).Msg("Failed to create VPC using Site Controller API")
		return nil, swe.WrapErr(err)
	}

	logger.Info().Msg("Completed activity")

	return controllerVpc, nil
}

// Function to update VPCS with Carbide
func (mv *ManageVPC) UpdateVpcOnSite(ctx context.Context, request *cwssaws.VpcUpdateRequest) error {
	logger := log.With().Str("Activity", "UpdateVpcOnSite").Logger()

	logger.Info().Msg("Starting activity")

	var err error

	// Validate request
	switch {
	case request == nil:
		err = errors.New("received empty update VPC request")
	case request.Id == nil || request.Id.Value == "":
		// Don't let a request come in without a cloud-provided ID
		// or carbide will generate one and cloud won't know the relationship.
		err = errors.New("received update VPC request missing VPC ID")
	}

	if err != nil {
		return temporal.NewNonRetryableApplicationError(err.Error(), swe.ErrTypeInvalidRequest, err)
	}

	// Call Site Controller gRPC endpoint
	forgeClient, err := mv.CarbideAtomicClient.GetForgeClient()
	if err != nil {
		return err
	}

	_, err = forgeClient.UpdateVpc(ctx, request)
	if err != nil {
		logger.Warn().Err(err).Msg("Failed to update VPC using Site Controller API")
		return swe.WrapErr(err)
	}

	logger.Info().Msg("Completed activity")

	return nil
}

// Function to delete VPCS with Carbide
func (mv *ManageVPC) DeleteVpcOnSite(ctx context.Context, request *cwssaws.VpcDeletionRequest) error {
	logger := log.With().Str("Activity", "DeleteVpcOnSite").Logger()

	logger.Info().Msg("Starting activity")

	var err error

	// Validate request
	switch {
	case request == nil:
		err = errors.New("received empty delete VPC request")
	case request.Id == nil || request.Id.Value == "":

		err = errors.New("received delete VPC request missing VPC ID")
	}

	if err != nil {
		return temporal.NewNonRetryableApplicationError(err.Error(), swe.ErrTypeInvalidRequest, err)
	}

	// Call Site Controller gRPC endpoint
	forgeClient, err := mv.CarbideAtomicClient.GetForgeClient()
	if err != nil {
		return err
	}

	_, err = forgeClient.DeleteVpc(ctx, request)
	if err != nil {
		logger.Warn().Err(err).Msg("Failed to delete VPC using Site Controller API")
		return swe.WrapErr(err)
	}

	logger.Info().Msg("Completed activity")

	return nil
}

// UpdateVpcVirtualizationOnSite updates VPC virtualization on Site
func (mv *ManageVPC) UpdateVpcVirtualizationOnSite(ctx context.Context, request *cwssaws.VpcUpdateVirtualizationRequest) error {
	logger := log.With().Str("Activity", "UpdateVpcOnSite").Logger()

	logger.Info().Msg("Starting activity")

	var err error

	// Validate request
	switch {
	case request == nil:
		err = errors.New("received empty update VPC virtualization request")
	case request.Id == nil || request.Id.Value == "":
		err = errors.New("received update VPC virtualization request missing VPC ID")
	}

	if err != nil {
		return temporal.NewNonRetryableApplicationError(err.Error(), swe.ErrTypeInvalidRequest, err)
	}

	// Call Site Controller gRPC endpoint
	forgeClient, err := mv.CarbideAtomicClient.GetForgeClient()
	if err != nil {
		return err
	}

	_, err = forgeClient.UpdateVpcVirtualization(ctx, request)
	if err != nil {
		logger.Warn().Err(err).Msg("Failed to update VPC virtualization using Site Controller API")
		return swe.WrapErr(err)
	}

	logger.Info().Msg("Completed activity")

	return nil
}
