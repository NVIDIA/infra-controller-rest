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

// ManageInfiniBandPartitionInventory is an activity wrapper for InfiniBand Partition inventory collection and publishing
type ManageInfiniBandPartitionInventory struct {
	config ManageInventoryConfig
}

// DiscoverInfiniBandPartitionInventory is an activity to collect InfiniBand Partition inventory and publish to Temporal queue
func (mmi *ManageInfiniBandPartitionInventory) DiscoverInfiniBandPartitionInventory(ctx context.Context) error {
	logger := log.With().Str("Activity", "DiscoverIBPartitionInventory").Logger()
	logger.Info().Msg("Starting activity")
	inventoryImpl := manageInventoryImpl[*cwssaws.IBPartitionId, *cwssaws.IBPartition, *cwssaws.InfiniBandPartitionInventory]{
		itemType:               "InfiniBandPartition",
		config:                 mmi.config,
		internalFindIDs:        ibpFindIDs,
		internalFindByIDs:      ibpFindByIDs,
		internalPagedInventory: ibpPagedInventory,
	}
	return inventoryImpl.CollectAndPublishInventory(ctx, &logger)
}

// NewManageInfiniBandPartitionInventory returns a ManageInventory implementation for InfiniBand Partition activity
func NewManageInfiniBandPartitionInventory(config ManageInventoryConfig) ManageInfiniBandPartitionInventory {
	return ManageInfiniBandPartitionInventory{
		config: config,
	}
}

func ibpFindIDs(ctx context.Context, carbideClient *cClient.CarbideClient) ([]*cwssaws.IBPartitionId, error) {
	// Call Site Controller gRPC endpoint
	forgeClient := carbideClient.Carbide()

	idList, err := forgeClient.FindIBPartitionIds(ctx, &cwssaws.IBPartitionSearchFilter{})
	if err != nil {
		return nil, err
	}

	return idList.GetIbPartitionIds(), nil
}

func ibpFindByIDs(ctx context.Context, carbideClient *cClient.CarbideClient, ids []*cwssaws.IBPartitionId) ([]*cwssaws.IBPartition, error) {
	// Call Site Controller gRPC endpoint
	forgeClient := carbideClient.Carbide()

	ibPartitionList, err := forgeClient.FindIBPartitionsByIds(ctx, &cwssaws.IBPartitionsByIdsRequest{
		IbPartitionIds: ids,
	})
	if err != nil {
		return nil, err
	}

	return ibPartitionList.GetIbPartitions(), nil
}

func ibpPagedInventory(allItemIDs []*cwssaws.IBPartitionId, pagedItems []*cwssaws.IBPartition, input *pagedInventoryInput) *cwssaws.InfiniBandPartitionInventory {
	itemIDs := []string{}
	for _, id := range allItemIDs {
		itemIDs = append(itemIDs, id.GetValue())
	}

	// Create an inventory page with the subset of VPCs
	inventory := &cwssaws.InfiniBandPartitionInventory{
		IbPartitions: pagedItems,
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

// ManageInfiniBandPartition is an activity wrapper for InfiniBand Partition management
type ManageInfiniBandPartition struct {
	CarbideAtomicClient *client.CarbideAtomicClient
}

// NewManageInfiniBandPartition returns a new ManageInfiniBandPartition client
func NewManageInfiniBandPartition(carbideClient *client.CarbideAtomicClient) ManageInfiniBandPartition {
	return ManageInfiniBandPartition{
		CarbideAtomicClient: carbideClient,
	}
}

// Function to create InfiniBand Partition with Carbide
func (mibp *ManageInfiniBandPartition) CreateInfiniBandPartitionOnSite(ctx context.Context, request *cwssaws.IBPartitionCreationRequest) error {
	logger := log.With().Str("Activity", "CreateInfiniBandPartitionOnSite").Logger()

	logger.Info().Msg("Starting activity")

	var err error

	// Validate request
	if request == nil {
		err = errors.New("received empty create InfiniBand Partition request")
	} else if request.Id == nil || request.GetId().GetValue() == "" {
		err = errors.New("received create InfiniBand Partition request without ID")
	} else if request.GetConfig() == nil {
		err = errors.New("received create InfiniBand Partition request without Config")
	} else if request.GetMetadata().GetName() == "" && request.GetConfig().GetName() == "" {
		// Backward compatibility: both Metadata.Name and Config.Name are accepted
		err = errors.New("received create InfiniBand Partition request without Name")
	} else if request.GetConfig().GetTenantOrganizationId() == "" {
		err = errors.New("received create InfiniBand Partition request without TenantOrganizationId")
	}

	if err != nil {
		return temporal.NewNonRetryableApplicationError(err.Error(), swe.ErrTypeInvalidRequest, err)
	}

	// Call Site Controller gRPC endpoint
	forgeClient, err := mibp.CarbideAtomicClient.GetForgeClient()
	if err != nil {
		return err
	}

	_, err = forgeClient.CreateIBPartition(ctx, request)
	if err != nil {
		logger.Warn().Err(err).Msg("Failed to create InfiniBand Partition using Site Controller API")
		return swe.WrapErr(err)
	}

	logger.Info().Msg("Completed activity")

	return nil
}

// UpdateInfiniBandPartitionOnSite applies an IB partition update on the site Forge controller
func (mibp *ManageInfiniBandPartition) UpdateInfiniBandPartitionOnSite(ctx context.Context, request *cwssaws.IBPartitionUpdateRequest) error {
	logger := log.With().Str("Activity", "UpdateInfiniBandPartitionOnSite").Logger()

	logger.Info().Msg("Starting activity")

	var err error

	if request == nil {
		err = errors.New("received empty update InfiniBand Partition request")
	} else if request.Id == nil || request.GetId().GetValue() == "" {
		err = errors.New("received update InfiniBand Partition request without ID")
	} else if request.GetConfig() == nil && request.GetMetadata() == nil {
		err = errors.New("received update InfiniBand Partition request without config or metadata")
	}

	if err != nil {
		return temporal.NewNonRetryableApplicationError(err.Error(), swe.ErrTypeInvalidRequest, err)
	}

	// Call Site Controller gRPC endpoint
	forgeClient, err := mibp.CarbideAtomicClient.GetForgeClient()
	if err != nil {
		return err
	}

	_, err = forgeClient.UpdateIBPartition(ctx, request)
	if err != nil {
		logger.Warn().Err(err).Msg("Failed to update InfiniBand Partition using Site Controller API")
		return swe.WrapErr(err)
	}

	logger.Info().Msg("Completed activity")

	return nil
}

// Function to delete InfiniBand Partition on Carbide
func (mipb *ManageInfiniBandPartition) DeleteInfiniBandPartitionOnSite(ctx context.Context, request *cwssaws.IBPartitionDeletionRequest) error {
	logger := log.With().Str("Activity", "DeleteInfiniBandPartitionOnSite").Logger()

	logger.Info().Msg("Starting activity")

	var err error

	// Validate request
	if request == nil {
		err = errors.New("received empty delete InfiniBand Partition request")
	} else if request.Id == nil || request.Id.GetValue() == "" {
		err = errors.New("received delete InfiniBand Partition request without ID")
	}

	if err != nil {
		return temporal.NewNonRetryableApplicationError(err.Error(), swe.ErrTypeInvalidRequest, err)
	}

	// Call Site Controller gRPC endpoint
	forgeClient, err := mipb.CarbideAtomicClient.GetForgeClient()
	if err != nil {
		return err
	}

	_, err = forgeClient.DeleteIBPartition(ctx, request)
	if err != nil {
		logger.Warn().Err(err).Msg("Failed to delete InfiniBand Partition using Site Controller API")
		return swe.WrapErr(err)
	}

	logger.Info().Msg("Completed activity")

	return nil
}
