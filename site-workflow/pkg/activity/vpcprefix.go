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
	"github.com/NVIDIA/ncx-infra-controller-rest/site-workflow/pkg/grpc/client"
	cClient "github.com/NVIDIA/ncx-infra-controller-rest/site-workflow/pkg/grpc/client"
	cwssaws "github.com/NVIDIA/ncx-infra-controller-rest/workflow-schema/schema/site-agent/workflows/v1"
	"github.com/rs/zerolog/log"
	"go.temporal.io/sdk/temporal"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// ManageVpcPrefix is an activity wrapper for VpcPrefix management
type ManageVpcPrefix struct {
	CarbideAtomicClient *client.CarbideAtomicClient
}

// NewManageVpcPrefix returns a new ManageVpcPrefix client
func NewManageVpcPrefix(carbideClient *client.CarbideAtomicClient) ManageVpcPrefix {
	return ManageVpcPrefix{
		CarbideAtomicClient: carbideClient,
	}
}

// Function to create VpcPrefix with Carbide
func (mvp *ManageVpcPrefix) CreateVpcPrefixOnSite(ctx context.Context, request *cwssaws.VpcPrefixCreationRequest) error {
	logger := log.With().Str("Activity", "CreateVpcPrefixOnSite").Logger()

	logger.Info().Msg("Starting activity")

	var err error

	// Validate request
	if request == nil {
		err = errors.New("received empty create VPC Prefix request")
	} else if request.Id == nil || request.Id.Value == "" {
		// Don't let a request come in without an ID or Site will generate one and REST won't know the relationship
		err = errors.New("received create VPC Prefix request without ID")
	}

	if err != nil {
		return temporal.NewNonRetryableApplicationError(err.Error(), swe.ErrTypeInvalidRequest, err)
	}

	// Call Site Controller gRPC endpoint
	forgeClient, err := mvp.CarbideAtomicClient.GetForgeClient()
	if err != nil {
		return err
	}

	_, err = forgeClient.CreateVpcPrefix(ctx, request)
	if err != nil {
		logger.Warn().Err(err).Msg("Failed to create VPC Prefix using Site Controller API")
		return swe.WrapErr(err)
	}

	logger.Info().Msg("Completed activity")

	return nil
}

// Function to update VpcPrefix with Carbide
func (mvp *ManageVpcPrefix) UpdateVpcPrefixOnSite(ctx context.Context, request *cwssaws.VpcPrefixUpdateRequest) error {
	logger := log.With().Str("Activity", "UpdateVpcPrefixOnSite").Logger()

	logger.Info().Msg("Starting activity")

	var err error

	// Validate request
	if request == nil {
		err = errors.New("received empty update VPC Prefix request")
	} else if request.Id == nil || request.Id.Value == "" {
		err = errors.New("received update VPC Prefix request without ID")
	}

	if err != nil {
		return temporal.NewNonRetryableApplicationError(err.Error(), swe.ErrTypeInvalidRequest, err)
	}

	// Call Site Controller gRPC endpoint
	forgeClient, err := mvp.CarbideAtomicClient.GetForgeClient()
	if err != nil {
		return err
	}

	_, err = forgeClient.UpdateVpcPrefix(ctx, request)
	if err != nil {
		logger.Warn().Err(err).Msg("Failed to update VPC Prefix using Site Controller API")
		return swe.WrapErr(err)
	}

	logger.Info().Msg("Completed activity")

	return nil
}

// Function to delete VpcPrefix on Carbide
func (mvp *ManageVpcPrefix) DeleteVpcPrefixOnSite(ctx context.Context, request *cwssaws.VpcPrefixDeletionRequest) error {
	logger := log.With().Str("Activity", "DeleteVpcPrefixOnSite").Logger()

	logger.Info().Msg("Starting activity")

	var err error

	// Validate request
	if request == nil {
		err = errors.New("received empty delete VPC Prefix request")
	} else if request.Id == nil || request.Id.Value == "" {
		err = errors.New("received delete VPC Prefix request without ID")
	}

	if err != nil {
		return temporal.NewNonRetryableApplicationError(err.Error(), swe.ErrTypeInvalidRequest, err)
	}

	// Call Site Controller gRPC endpoint
	forgeClient, err := mvp.CarbideAtomicClient.GetForgeClient()
	if err != nil {
		return err
	}

	_, err = forgeClient.DeleteVpcPrefix(ctx, request)
	if err != nil {
		logger.Warn().Err(err).Msg("Failed to delete VPC Prefix using Site Controller API")
		return swe.WrapErr(err)
	}

	logger.Info().Msg("Completed activity")

	return nil
}

// ManageVpcPrefixInventory is an activity wrapper for VpcPrefix inventory collection and publishing
type ManageVpcPrefixInventory struct {
	config ManageInventoryConfig
}

// NewManageVpcPrefixInventory returns a ManageInventory implementation for VpcPrefix
func NewManageVpcPrefixInventory(config ManageInventoryConfig) ManageVpcPrefixInventory {
	return ManageVpcPrefixInventory{
		config: config,
	}
}

// DiscoverVpcPrefixInventory is an activity to collect VpcPrefix inventory and publish to Temporal queue
func (moii *ManageVpcPrefixInventory) DiscoverVpcPrefixInventory(ctx context.Context) error {
	logger := log.With().Str("Activity", "DiscoverVpcPrefixInventory").Logger()
	logger.Info().Msg("Starting activity")

	inventoryImpl := manageInventoryImpl[*cwssaws.VpcPrefixId, *cwssaws.VpcPrefix, *cwssaws.VpcPrefixInventory]{
		itemType:               "VpcPrefix",
		config:                 moii.config,
		internalFindIDs:        VpcPrefixFindIDs,
		internalFindByIDs:      VpcPrefixFindByIDs,
		internalPagedInventory: VpcPrefixPagedInventory,
		internalFindFallback:   VpcPrefixFindFallback,
	}
	return inventoryImpl.CollectAndPublishInventory(ctx, &logger)
}

func VpcPrefixFindIDs(ctx context.Context, carbideClient *cClient.CarbideClient) ([]*cwssaws.VpcPrefixId, error) {
	idList, err := carbideClient.Carbide().SearchVpcPrefixes(ctx, &cwssaws.VpcPrefixSearchQuery{})
	if err != nil {
		return nil, err
	}
	return idList.VpcPrefixIds, nil
}

func VpcPrefixFindByIDs(ctx context.Context, carbideClient *cClient.CarbideClient, ids []*cwssaws.VpcPrefixId) ([]*cwssaws.VpcPrefix, error) {
	list, err := carbideClient.Carbide().GetVpcPrefixes(ctx, &cwssaws.VpcPrefixGetRequest{
		VpcPrefixIds: ids,
	})

	if err != nil {
		return nil, err
	}
	return list.GetVpcPrefixes(), nil
}

func VpcPrefixPagedInventory(allItemIDs []*cwssaws.VpcPrefixId, pagedItems []*cwssaws.VpcPrefix, input *pagedInventoryInput) *cwssaws.VpcPrefixInventory {
	itemIDs := []string{}
	for _, id := range allItemIDs {
		itemIDs = append(itemIDs, id.GetValue())
	}

	// Create an inventory page with the subset of VpcPrefixs
	inventory := &cwssaws.VpcPrefixInventory{
		VpcPrefixes: pagedItems,
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

func VpcPrefixFindFallback(ctx context.Context, carbideClient *cClient.CarbideClient) ([]*cwssaws.VpcPrefixId, []*cwssaws.VpcPrefix, error) {
	request := &cwssaws.VpcPrefixGetRequest{}
	items, err := carbideClient.Carbide().GetVpcPrefixes(ctx, request)
	if err != nil {
		return nil, nil, err
	}

	var ids []*cwssaws.VpcPrefixId
	for _, it := range items.GetVpcPrefixes() {
		ids = append(ids, it.GetId())
	}
	return ids, items.GetVpcPrefixes(), nil
}
