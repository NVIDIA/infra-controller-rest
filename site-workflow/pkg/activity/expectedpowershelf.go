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
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
	"go.temporal.io/sdk/client"
	tClient "go.temporal.io/sdk/client"
	"go.temporal.io/sdk/temporal"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"

	swe "github.com/NVIDIA/ncx-infra-controller-rest/site-workflow/pkg/error"
	cclient "github.com/NVIDIA/ncx-infra-controller-rest/site-workflow/pkg/grpc/client"
	rlav1 "github.com/NVIDIA/ncx-infra-controller-rest/workflow-schema/rla/protobuf/v1"
	cwssaws "github.com/NVIDIA/ncx-infra-controller-rest/workflow-schema/schema/site-agent/workflows/v1"
)

// ManageExpectedPowerShelfInventory is an activity wrapper for Expected Power Shelf inventory collection and publishing
type ManageExpectedPowerShelfInventory struct {
	siteID                uuid.UUID
	carbideAtomicClient   *cclient.CarbideAtomicClient
	temporalPublishClient tClient.Client
	temporalPublishQueue  string
	cloudPageSize         int
}

type linkedExpectedPowerShelfInfo struct {
	expectedPowerShelf       *cwssaws.ExpectedPowerShelf
	linkedExpectedPowerShelf *cwssaws.LinkedExpectedPowerShelf
}

// DiscoverExpectedPowerShelfInventory is an activity to collect Expected Power Shelf inventory and publish to Temporal queue
func (mepsi *ManageExpectedPowerShelfInventory) DiscoverExpectedPowerShelfInventory(ctx context.Context) error {
	logger := log.With().Str("Activity", "DiscoverExpectedPowerShelfInventory").Logger()
	logger.Info().Msg("Starting activity")

	// Define workflow options
	workflowOptions := tClient.StartWorkflowOptions{
		ID:        "update-expectedpowershelf-inventory-" + mepsi.siteID.String(),
		TaskQueue: mepsi.temporalPublishQueue,
	}

	// Get Site Controller gRPC client
	forgeClient, err := mepsi.carbideAtomicClient.GetForgeClient()
	if err != nil {
		return err
	}

	// Call GetAllExpectedPowerShelves to get full list of ExpectedPowerShelves on Site
	epsList, err := forgeClient.GetAllExpectedPowerShelves(ctx, &emptypb.Empty{})
	if err != nil {
		logger.Warn().Err(err).Msg("Failed to retrieve ExpectedPowerShelves using Site Controller API")

		// Error encountered before we've published anything, report inventory collection error to Cloud
		inventory := &cwssaws.ExpectedPowerShelfInventory{
			Timestamp: &timestamppb.Timestamp{
				Seconds: time.Now().Unix(),
			},
			InventoryStatus: cwssaws.InventoryStatus_INVENTORY_STATUS_FAILED,
			StatusMsg:       err.Error(),
		}

		_, serr := mepsi.temporalPublishClient.ExecuteWorkflow(context.Background(), workflowOptions, "UpdateExpectedPowerShelfInventory", mepsi.siteID, inventory)
		if serr != nil {
			logger.Error().Err(serr).Msg("Failed to publish ExpectedPowerShelf inventory error to Cloud")
			return serr
		}
		return err
	}

	// Call GetAllExpectedPowerShelvesLinked to get linked Power Shelf IDs
	linkedList, lerr := forgeClient.GetAllExpectedPowerShelvesLinked(ctx, &emptypb.Empty{})
	if lerr != nil {
		logger.Warn().Err(lerr).Msg("Failed to retrieve linked Power Shelf IDs using Site Controller API")

		// Fatal error - report inventory collection error to Cloud
		inventory := &cwssaws.ExpectedPowerShelfInventory{
			Timestamp: &timestamppb.Timestamp{
				Seconds: time.Now().Unix(),
			},
			InventoryStatus: cwssaws.InventoryStatus_INVENTORY_STATUS_FAILED,
			StatusMsg:       lerr.Error(),
		}

		_, serr := mepsi.temporalPublishClient.ExecuteWorkflow(context.Background(), workflowOptions, "UpdateExpectedPowerShelfInventory", mepsi.siteID, inventory)
		if serr != nil {
			logger.Error().Err(serr).Msg("Failed to publish ExpectedPowerShelf inventory error to Cloud")
			return serr
		}
		return lerr
	}

	// LinkedExpectedPowerShelf data is missing ExpectedPowerShelf ID so we build an intermediate map using MAC address
	linkedPowerShelvesByKey := make(map[string]*cwssaws.LinkedExpectedPowerShelf)
	for _, linked := range linkedList.ExpectedPowerShelves {
		linkedPowerShelvesByKey[linked.BmcMacAddress] = linked
	}

	// Build list of ExpectedPowerShelf paired with LinkedExpectedPowerShelf
	linkedExpectedPowerShelvesInfo := []linkedExpectedPowerShelfInfo{}
	allExpectedPowerShelfIDs := []string{}
	for _, eps := range epsList.ExpectedPowerShelves {
		// Discard records without ID
		if eps.ExpectedPowerShelfId == nil || eps.ExpectedPowerShelfId.Value == "" {
			logger.Warn().Str("MAC", eps.BmcMacAddress).Str("Serial", eps.ShelfSerialNumber).Msg("Discarding ExpectedPowerShelf without ID")
			continue
		}
		allExpectedPowerShelfIDs = append(allExpectedPowerShelfIDs, eps.ExpectedPowerShelfId.Value)
		// Find matching LinkedPowerShelf record by MAC address if it exists
		linked := linkedPowerShelvesByKey[eps.BmcMacAddress]
		linkedExpectedPowerShelvesInfo = append(linkedExpectedPowerShelvesInfo, linkedExpectedPowerShelfInfo{
			expectedPowerShelf:       eps,
			linkedExpectedPowerShelf: linked,
		})
	}
	totalCount := len(linkedExpectedPowerShelvesInfo)

	logger.Info().Int("ExpectedPowerShelf Count", totalCount).Msg("Built ExpectedPowerShelf list")

	if totalCount == 0 {
		inventoryPage := getPagedExpectedPowerShelfInventory([]linkedExpectedPowerShelfInfo{}, allExpectedPowerShelfIDs, totalCount, 1, mepsi.cloudPageSize, cwssaws.InventoryStatus_INVENTORY_STATUS_SUCCESS, "No ExpectedPowerShelves reported by Site Controller")

		_, serr := mepsi.temporalPublishClient.ExecuteWorkflow(context.Background(), workflowOptions, "UpdateExpectedPowerShelfInventory", mepsi.siteID, inventoryPage)
		if serr != nil {
			logger.Error().Err(serr).Msg("Failed to publish ExpectedPowerShelf inventory to Cloud")
			return serr
		}
		return nil
	}

	// Calculate total pages needed for Cloud
	totalCloudPages := totalCount / mepsi.cloudPageSize
	if totalCount%mepsi.cloudPageSize > 0 {
		totalCloudPages++
	}

	// Publish ExpectedPowerShelf inventory to Cloud in separate chunks
	for cloudPage := 1; cloudPage <= totalCloudPages; cloudPage++ {
		startIndex := (cloudPage - 1) * mepsi.cloudPageSize
		endIndex := startIndex + mepsi.cloudPageSize
		if endIndex > totalCount {
			endIndex = totalCount
		}

		pagedWorkflowOptions := client.StartWorkflowOptions{
			ID:        fmt.Sprintf("%v-%v", workflowOptions.ID, cloudPage),
			TaskQueue: workflowOptions.TaskQueue,
		}

		// Create an inventory page with the subset of ExpectedPowerShelves
		// Slice the list directly for this page
		pagedInfo := linkedExpectedPowerShelvesInfo[startIndex:endIndex]
		inventoryPage := getPagedExpectedPowerShelfInventory(
			pagedInfo,
			allExpectedPowerShelfIDs,
			totalCount,
			cloudPage,
			mepsi.cloudPageSize,
			cwssaws.InventoryStatus_INVENTORY_STATUS_SUCCESS,
			"Successfully retrieved ExpectedPowerShelves from Site Controller",
		)

		logger.Info().Msgf("Publishing ExpectedPowerShelf inventory page %d to Cloud", cloudPage)

		_, serr := mepsi.temporalPublishClient.ExecuteWorkflow(context.Background(), pagedWorkflowOptions, "UpdateExpectedPowerShelfInventory", mepsi.siteID, inventoryPage)
		if serr != nil {
			logger.Error().Err(serr).Int("Cloud Page", cloudPage).Msg("Failed to publish ExpectedPowerShelf inventory to Cloud")
			return serr
		}
	}

	return nil
}

// getPagedExpectedPowerShelfInventory returns a subset of ExpectedPowerShelfInventory for a given page
func getPagedExpectedPowerShelfInventory(
	pagedInfo []linkedExpectedPowerShelfInfo,
	allExpectedPowerShelfIDs []string,
	totalCount int,
	page int,
	pageSize int,
	status cwssaws.InventoryStatus,
	statusMessage string,
) *cwssaws.ExpectedPowerShelfInventory {
	totalPages := totalCount / pageSize
	if totalCount%pageSize > 0 {
		totalPages++
	}

	// Build lists for this page from the sliced info list
	pagedExpectedPowerShelves := make([]*cwssaws.ExpectedPowerShelf, 0, len(pagedInfo))
	pagedLinkedPowerShelves := make([]*cwssaws.LinkedExpectedPowerShelf, 0, len(pagedInfo))

	for _, info := range pagedInfo {
		pagedExpectedPowerShelves = append(pagedExpectedPowerShelves, info.expectedPowerShelf)
		// Only add LinkedExpectedPowerShelf if it exists (it may be nil if no match was found)
		if info.linkedExpectedPowerShelf != nil {
			pagedLinkedPowerShelves = append(pagedLinkedPowerShelves, info.linkedExpectedPowerShelf)
		}
	}

	// Create an inventory page with the subset of ExpectedPowerShelves and matching LinkedPowerShelves
	inventoryPage := &cwssaws.ExpectedPowerShelfInventory{
		ExpectedPowerShelves: pagedExpectedPowerShelves,
		LinkedPowerShelves:   pagedLinkedPowerShelves,
		Timestamp: &timestamppb.Timestamp{
			Seconds: time.Now().Unix(),
		},
		InventoryStatus: status,
		StatusMsg:       statusMessage,
		InventoryPage: &cwssaws.InventoryPage{
			TotalPages:  int32(totalPages),
			CurrentPage: int32(page),
			PageSize:    int32(pageSize),
			TotalItems:  int32(totalCount),
			ItemIds:     allExpectedPowerShelfIDs,
		},
	}

	return inventoryPage
}

// NewManageExpectedPowerShelfInventory returns a ManageInventory implementation for Expected Power Shelf activity
func NewManageExpectedPowerShelfInventory(siteID uuid.UUID, carbideAtomicClient *cclient.CarbideAtomicClient, temporalPublishClient tClient.Client, temporalPublishQueue string, cloudPageSize int) ManageExpectedPowerShelfInventory {
	return ManageExpectedPowerShelfInventory{
		siteID:                siteID,
		carbideAtomicClient:   carbideAtomicClient,
		temporalPublishClient: temporalPublishClient,
		temporalPublishQueue:  temporalPublishQueue,
		cloudPageSize:         cloudPageSize,
	}
}

// ManageExpectedPowerShelf is an activity wrapper for Expected Power Shelf management
type ManageExpectedPowerShelf struct {
	CarbideAtomicClient *cclient.CarbideAtomicClient
	RlaAtomicClient     *cclient.RlaAtomicClient
}

// NewManageExpectedPowerShelf returns a new ManageExpectedPowerShelf client
func NewManageExpectedPowerShelf(carbideClient *cclient.CarbideAtomicClient, rlaClient *cclient.RlaAtomicClient) ManageExpectedPowerShelf {
	return ManageExpectedPowerShelf{
		CarbideAtomicClient: carbideClient,
		RlaAtomicClient:     rlaClient,
	}
}

// CreateExpectedPowerShelfOnSite creates Expected Power Shelf with Carbide
func (meps *ManageExpectedPowerShelf) CreateExpectedPowerShelfOnSite(ctx context.Context, request *cwssaws.ExpectedPowerShelf) error {
	logger := log.With().Str("Activity", "CreateExpectedPowerShelfOnSite").Logger()

	logger.Info().Msg("Starting activity")

	var err error

	// Validate request
	if request == nil {
		err = errors.New("received empty create Expected Power Shelf request")
	} else if request.GetExpectedPowerShelfId().GetValue() == "" {
		err = errors.New("received create Expected Power Shelf request without required id field")
	} else if request.GetBmcMacAddress() == "" || request.GetShelfSerialNumber() == "" {
		err = errors.New("received create Expected Power Shelf request with missing MAC or serial")
	}

	if err != nil {
		return temporal.NewNonRetryableApplicationError(err.Error(), swe.ErrTypeInvalidRequest, err)
	}

	// Call Site Controller gRPC endpoint
	forgeClient, err := meps.CarbideAtomicClient.GetForgeClient()
	if err != nil {
		return err
	}

	// Call Forge gRPC endpoint
	_, err = forgeClient.AddExpectedPowerShelf(ctx, request)
	if err != nil {
		logger.Warn().Err(err).Msg("Failed to create Expected Power Shelf using Site Controller API")
		return swe.WrapErr(err)
	}

	logger.Info().Msg("Completed activity")

	return nil
}

// UpdateExpectedPowerShelfOnSite updates Expected Power Shelf on Carbide
func (meps *ManageExpectedPowerShelf) UpdateExpectedPowerShelfOnSite(ctx context.Context, request *cwssaws.ExpectedPowerShelf) error {
	logger := log.With().Str("Activity", "UpdateExpectedPowerShelfOnSite").Logger()

	logger.Info().Msg("Starting activity")

	var err error

	// Validate request
	if request == nil {
		err = errors.New("received empty update Expected Power Shelf request")
	} else if request.GetExpectedPowerShelfId().GetValue() == "" {
		err = errors.New("received update Expected Power Shelf request without required id field")
	} else if request.GetBmcMacAddress() == "" || request.GetShelfSerialNumber() == "" {
		err = errors.New("received update Expected Power Shelf request with missing MAC or serial")
	}

	if err != nil {
		return temporal.NewNonRetryableApplicationError(err.Error(), swe.ErrTypeInvalidRequest, err)
	}

	// Call Site Controller gRPC endpoint
	forgeClient, err := meps.CarbideAtomicClient.GetForgeClient()
	if err != nil {
		return err
	}

	_, err = forgeClient.UpdateExpectedPowerShelf(ctx, request)
	if err != nil {
		logger.Warn().Err(err).Msg("Failed to update Expected Power Shelf using Site Controller API")
		return swe.WrapErr(err)
	}

	logger.Info().Msg("Completed activity")

	return nil
}

// CreateExpectedPowerShelfOnRLA creates an Expected Power Shelf as a component in RLA via AddComponent
func (meps *ManageExpectedPowerShelf) CreateExpectedPowerShelfOnRLA(ctx context.Context, request *cwssaws.ExpectedPowerShelf) error {
	logger := log.With().Str("Activity", "CreateExpectedPowerShelfOnRLA").Logger()

	logger.Info().Msg("Starting activity")

	// Validate request
	if request == nil {
		return temporal.NewNonRetryableApplicationError("received empty create Expected Power Shelf request for RLA", swe.ErrTypeInvalidRequest, errors.New("nil request"))
	}

	// If RLA client is not configured, skip gracefully
	if meps.RlaAtomicClient == nil {
		logger.Warn().Msg("RLA client not configured, skipping RLA component creation")
		return nil
	}

	rlaClient := meps.RlaAtomicClient.GetClient()
	if rlaClient == nil {
		logger.Warn().Msg("RLA client not connected, skipping RLA component creation")
		return nil
	}

	component := expectedPowerShelfToRLAComponent(request)
	_, err := rlaClient.Rla().AddComponent(ctx, &rlav1.AddComponentRequest{Component: component})
	if err != nil {
		logger.Warn().Err(err).Msg("Failed to create Expected Power Shelf component on RLA")
		return swe.WrapErr(err)
	}

	logger.Info().Msg("Completed activity")
	return nil
}

// expectedPowerShelfToRLAComponent converts a Forge ExpectedPowerShelf proto to an RLA Component proto
func expectedPowerShelfToRLAComponent(eps *cwssaws.ExpectedPowerShelf) *rlav1.Component {
	component := &rlav1.Component{
		Type: rlav1.ComponentType_COMPONENT_TYPE_POWERSHELF,
		Info: &rlav1.DeviceInfo{
			Id:           &rlav1.UUID{Id: eps.GetExpectedPowerShelfId().GetValue()},
			SerialNumber: eps.GetShelfSerialNumber(),
		},
		Bmcs: []*rlav1.BMCInfo{
			{
				Type:       rlav1.BMCType_BMC_TYPE_HOST,
				MacAddress: eps.GetBmcMacAddress(),
			},
		},
		ComponentId: eps.GetExpectedPowerShelfId().GetValue(),
	}

	// DeviceInfo fields
	if name := eps.GetName(); name != "" {
		component.Info.Name = name
	}
	if manufacturer := eps.GetManufacturer(); manufacturer != "" {
		component.Info.Manufacturer = manufacturer
	}
	if eps.Model != nil {
		component.Info.Model = eps.Model
	}
	if eps.Description != nil {
		component.Info.Description = eps.Description
	}

	// Firmware version
	if fv := eps.GetFirmwareVersion(); fv != "" {
		component.FirmwareVersion = fv
	}

	// Rack position
	if eps.SlotId != nil || eps.TrayIdx != nil || eps.HostId != nil {
		pos := &rlav1.RackPosition{}
		if eps.SlotId != nil {
			pos.SlotId = *eps.SlotId
		}
		if eps.TrayIdx != nil {
			pos.TrayIdx = *eps.TrayIdx
		}
		if eps.HostId != nil {
			pos.HostId = *eps.HostId
		}
		component.Position = pos
	}

	if eps.GetBmcIpAddress() != "" {
		ipAddr := eps.GetBmcIpAddress()
		component.Bmcs[0].IpAddress = &ipAddr
	}

	if rackID := eps.GetRackId().GetId(); rackID != "" {
		component.RackId = &rlav1.UUID{Id: rackID}
	}

	return component
}

// DeleteExpectedPowerShelfOnSite deletes Expected Power Shelf on Carbide
func (meps *ManageExpectedPowerShelf) DeleteExpectedPowerShelfOnSite(ctx context.Context, request *cwssaws.ExpectedPowerShelfRequest) error {
	logger := log.With().Str("Activity", "DeleteExpectedPowerShelfOnSite").Logger()

	logger.Info().Msg("Starting activity")

	var err error

	// Validate request
	if request == nil {
		err = errors.New("received empty delete Expected Power Shelf request")
	} else if request.GetExpectedPowerShelfId().GetValue() == "" {
		err = errors.New("received delete Expected Power Shelf request without required id field")
	}

	if err != nil {
		return temporal.NewNonRetryableApplicationError(err.Error(), swe.ErrTypeInvalidRequest, err)
	}

	// Call Site Controller gRPC endpoint
	forgeClient, err := meps.CarbideAtomicClient.GetForgeClient()
	if err != nil {
		return err
	}

	_, err = forgeClient.DeleteExpectedPowerShelf(ctx, request)
	if err != nil {
		logger.Warn().Err(err).Msg("Failed to delete Expected Power Shelf using Site Controller API")
		return swe.WrapErr(err)
	}

	logger.Info().Msg("Completed activity")

	return nil
}
