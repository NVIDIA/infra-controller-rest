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

// ManageExpectedSwitchInventory is an activity wrapper for Expected Switch inventory collection and publishing
type ManageExpectedSwitchInventory struct {
	siteID                uuid.UUID
	carbideAtomicClient   *cclient.CarbideAtomicClient
	temporalPublishClient tClient.Client
	temporalPublishQueue  string
	cloudPageSize         int
}

type linkedExpectedSwitchInfo struct {
	expectedSwitch       *cwssaws.ExpectedSwitch
	linkedExpectedSwitch *cwssaws.LinkedExpectedSwitch
}

// DiscoverExpectedSwitchInventory is an activity to collect Expected Switch inventory and publish to Temporal queue
func (mesi *ManageExpectedSwitchInventory) DiscoverExpectedSwitchInventory(ctx context.Context) error {
	logger := log.With().Str("Activity", "DiscoverExpectedSwitchInventory").Logger()
	logger.Info().Msg("Starting activity")

	// Define workflow options
	workflowOptions := tClient.StartWorkflowOptions{
		ID:        "update-expectedswitch-inventory-" + mesi.siteID.String(),
		TaskQueue: mesi.temporalPublishQueue,
	}

	// Get Site Controller gRPC client
	forgeClient, err := mesi.carbideAtomicClient.GetForgeClient()
	if err != nil {
		return err
	}

	// Call GetAllExpectedSwitches to get full list of ExpectedSwitches on Site
	esList, err := forgeClient.GetAllExpectedSwitches(ctx, &emptypb.Empty{})
	if err != nil {
		logger.Warn().Err(err).Msg("Failed to retrieve ExpectedSwitches using Site Controller API")

		// Error encountered before we've published anything, report inventory collection error to Cloud
		inventory := &cwssaws.ExpectedSwitchInventory{
			Timestamp: &timestamppb.Timestamp{
				Seconds: time.Now().Unix(),
			},
			InventoryStatus: cwssaws.InventoryStatus_INVENTORY_STATUS_FAILED,
			StatusMsg:       err.Error(),
		}

		_, serr := mesi.temporalPublishClient.ExecuteWorkflow(context.Background(), workflowOptions, "UpdateExpectedSwitchInventory", mesi.siteID, inventory)
		if serr != nil {
			logger.Error().Err(serr).Msg("Failed to publish ExpectedSwitch inventory error to Cloud")
			return serr
		}
		return err
	}

	// Call GetAllExpectedSwitchesLinked to get linked Switch IDs
	linkedList, lerr := forgeClient.GetAllExpectedSwitchesLinked(ctx, &emptypb.Empty{})
	if lerr != nil {
		logger.Warn().Err(lerr).Msg("Failed to retrieve linked Switch IDs using Site Controller API")

		// Fatal error - report inventory collection error to Cloud
		inventory := &cwssaws.ExpectedSwitchInventory{
			Timestamp: &timestamppb.Timestamp{
				Seconds: time.Now().Unix(),
			},
			InventoryStatus: cwssaws.InventoryStatus_INVENTORY_STATUS_FAILED,
			StatusMsg:       lerr.Error(),
		}

		_, serr := mesi.temporalPublishClient.ExecuteWorkflow(context.Background(), workflowOptions, "UpdateExpectedSwitchInventory", mesi.siteID, inventory)
		if serr != nil {
			logger.Error().Err(serr).Msg("Failed to publish ExpectedSwitch inventory error to Cloud")
			return serr
		}
		return lerr
	}

	// LinkedExpectedSwitch data is missing ExpectedSwitch ID so we build an intermediate map using MAC address
	linkedSwitchesByKey := make(map[string]*cwssaws.LinkedExpectedSwitch)
	for _, linked := range linkedList.ExpectedSwitches {
		linkedSwitchesByKey[linked.BmcMacAddress] = linked
	}

	// Build list of ExpectedSwitch paired with LinkedExpectedSwitch
	linkedExpectedSwitchesInfo := []linkedExpectedSwitchInfo{}
	allExpectedSwitchIDs := []string{}
	for _, es := range esList.ExpectedSwitches {
		// Discard records without ID
		if es.ExpectedSwitchId == nil || es.ExpectedSwitchId.Value == "" {
			logger.Warn().Str("MAC", es.BmcMacAddress).Str("Serial", es.SwitchSerialNumber).Msg("Discarding ExpectedSwitch without ID")
			continue
		}
		allExpectedSwitchIDs = append(allExpectedSwitchIDs, es.ExpectedSwitchId.Value)
		// Find matching LinkedSwitch record by MAC address if it exists
		linked := linkedSwitchesByKey[es.BmcMacAddress]
		linkedExpectedSwitchesInfo = append(linkedExpectedSwitchesInfo, linkedExpectedSwitchInfo{
			expectedSwitch:       es,
			linkedExpectedSwitch: linked,
		})
	}
	totalCount := len(linkedExpectedSwitchesInfo)

	logger.Info().Int("ExpectedSwitch Count", totalCount).Msg("Built ExpectedSwitch list")

	if totalCount == 0 {
		inventoryPage := getPagedExpectedSwitchInventory([]linkedExpectedSwitchInfo{}, allExpectedSwitchIDs, totalCount, 1, mesi.cloudPageSize, cwssaws.InventoryStatus_INVENTORY_STATUS_SUCCESS, "No ExpectedSwitches reported by Site Controller")

		_, serr := mesi.temporalPublishClient.ExecuteWorkflow(context.Background(), workflowOptions, "UpdateExpectedSwitchInventory", mesi.siteID, inventoryPage)
		if serr != nil {
			logger.Error().Err(serr).Msg("Failed to publish ExpectedSwitch inventory to Cloud")
			return serr
		}
		return nil
	}

	// Calculate total pages needed for Cloud
	totalCloudPages := totalCount / mesi.cloudPageSize
	if totalCount%mesi.cloudPageSize > 0 {
		totalCloudPages++
	}

	// Publish ExpectedSwitch inventory to Cloud in separate chunks
	for cloudPage := 1; cloudPage <= totalCloudPages; cloudPage++ {
		startIndex := (cloudPage - 1) * mesi.cloudPageSize
		endIndex := startIndex + mesi.cloudPageSize
		if endIndex > totalCount {
			endIndex = totalCount
		}

		pagedWorkflowOptions := client.StartWorkflowOptions{
			ID:        fmt.Sprintf("%v-%v", workflowOptions.ID, cloudPage),
			TaskQueue: workflowOptions.TaskQueue,
		}

		// Create an inventory page with the subset of ExpectedSwitches
		// Slice the list directly for this page
		pagedInfo := linkedExpectedSwitchesInfo[startIndex:endIndex]
		inventoryPage := getPagedExpectedSwitchInventory(
			pagedInfo,
			allExpectedSwitchIDs,
			totalCount,
			cloudPage,
			mesi.cloudPageSize,
			cwssaws.InventoryStatus_INVENTORY_STATUS_SUCCESS,
			"Successfully retrieved ExpectedSwitches from Site Controller",
		)

		logger.Info().Msgf("Publishing ExpectedSwitch inventory page %d to Cloud", cloudPage)

		_, serr := mesi.temporalPublishClient.ExecuteWorkflow(context.Background(), pagedWorkflowOptions, "UpdateExpectedSwitchInventory", mesi.siteID, inventoryPage)
		if serr != nil {
			logger.Error().Err(serr).Int("Cloud Page", cloudPage).Msg("Failed to publish ExpectedSwitch inventory to Cloud")
			return serr
		}
	}

	return nil
}

// getPagedExpectedSwitchInventory returns a subset of ExpectedSwitchInventory for a given page
func getPagedExpectedSwitchInventory(
	pagedInfo []linkedExpectedSwitchInfo,
	allExpectedSwitchIDs []string,
	totalCount int,
	page int,
	pageSize int,
	status cwssaws.InventoryStatus,
	statusMessage string,
) *cwssaws.ExpectedSwitchInventory {
	totalPages := totalCount / pageSize
	if totalCount%pageSize > 0 {
		totalPages++
	}

	// Build lists for this page from the sliced info list
	pagedExpectedSwitches := make([]*cwssaws.ExpectedSwitch, 0, len(pagedInfo))
	pagedLinkedSwitches := make([]*cwssaws.LinkedExpectedSwitch, 0, len(pagedInfo))

	for _, info := range pagedInfo {
		pagedExpectedSwitches = append(pagedExpectedSwitches, info.expectedSwitch)
		// Only add LinkedExpectedSwitch if it exists (it may be nil if no match was found)
		if info.linkedExpectedSwitch != nil {
			pagedLinkedSwitches = append(pagedLinkedSwitches, info.linkedExpectedSwitch)
		}
	}

	// Create an inventory page with the subset of ExpectedSwitches and matching LinkedSwitches
	inventoryPage := &cwssaws.ExpectedSwitchInventory{
		ExpectedSwitches: pagedExpectedSwitches,
		LinkedSwitches:   pagedLinkedSwitches,
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
			ItemIds:     allExpectedSwitchIDs,
		},
	}

	return inventoryPage
}

// NewManageExpectedSwitchInventory returns a ManageInventory implementation for Expected Switch activity
func NewManageExpectedSwitchInventory(siteID uuid.UUID, carbideAtomicClient *cclient.CarbideAtomicClient, temporalPublishClient tClient.Client, temporalPublishQueue string, cloudPageSize int) ManageExpectedSwitchInventory {
	return ManageExpectedSwitchInventory{
		siteID:                siteID,
		carbideAtomicClient:   carbideAtomicClient,
		temporalPublishClient: temporalPublishClient,
		temporalPublishQueue:  temporalPublishQueue,
		cloudPageSize:         cloudPageSize,
	}
}

// ManageExpectedSwitch is an activity wrapper for Expected Switch management
type ManageExpectedSwitch struct {
	CarbideAtomicClient *cclient.CarbideAtomicClient
	RlaAtomicClient     *cclient.RlaAtomicClient
}

// NewManageExpectedSwitch returns a new ManageExpectedSwitch client
func NewManageExpectedSwitch(carbideClient *cclient.CarbideAtomicClient, rlaClient *cclient.RlaAtomicClient) ManageExpectedSwitch {
	return ManageExpectedSwitch{
		CarbideAtomicClient: carbideClient,
		RlaAtomicClient:     rlaClient,
	}
}

// CreateExpectedSwitchOnSite creates Expected Switch with Carbide
func (mes *ManageExpectedSwitch) CreateExpectedSwitchOnSite(ctx context.Context, request *cwssaws.ExpectedSwitch) error {
	logger := log.With().Str("Activity", "CreateExpectedSwitchOnSite").Logger()

	logger.Info().Msg("Starting activity")

	var err error

	// Validate request
	if request == nil {
		err = errors.New("received empty create Expected Switch request")
	} else if request.GetExpectedSwitchId().GetValue() == "" {
		err = errors.New("received create Expected Switch request without required id field")
	} else if request.GetBmcMacAddress() == "" || request.GetSwitchSerialNumber() == "" {
		err = errors.New("received create Expected Switch request with missing MAC or serial")
	}

	if err != nil {
		return temporal.NewNonRetryableApplicationError(err.Error(), swe.ErrTypeInvalidRequest, err)
	}

	// Call Site Controller gRPC endpoint
	forgeClient, err := mes.CarbideAtomicClient.GetForgeClient()
	if err != nil {
		return err
	}

	// Call Forge gRPC endpoint
	_, err = forgeClient.AddExpectedSwitch(ctx, request)
	if err != nil {
		logger.Warn().Err(err).Msg("Failed to create Expected Switch using Site Controller API")
		return swe.WrapErr(err)
	}

	logger.Info().Msg("Completed activity")

	return nil
}

// UpdateExpectedSwitchOnSite updates Expected Switch on Carbide
func (mes *ManageExpectedSwitch) UpdateExpectedSwitchOnSite(ctx context.Context, request *cwssaws.ExpectedSwitch) error {
	logger := log.With().Str("Activity", "UpdateExpectedSwitchOnSite").Logger()

	logger.Info().Msg("Starting activity")

	var err error

	// Validate request
	if request == nil {
		err = errors.New("received empty update Expected Switch request")
	} else if request.GetExpectedSwitchId().GetValue() == "" {
		err = errors.New("received update Expected Switch request without required id field")
	} else if request.GetBmcMacAddress() == "" || request.GetSwitchSerialNumber() == "" {
		err = errors.New("received update Expected Switch request with missing MAC or serial")
	}

	if err != nil {
		return temporal.NewNonRetryableApplicationError(err.Error(), swe.ErrTypeInvalidRequest, err)
	}

	// Call Site Controller gRPC endpoint
	forgeClient, err := mes.CarbideAtomicClient.GetForgeClient()
	if err != nil {
		return err
	}

	_, err = forgeClient.UpdateExpectedSwitch(ctx, request)
	if err != nil {
		logger.Warn().Err(err).Msg("Failed to update Expected Switch using Site Controller API")
		return swe.WrapErr(err)
	}

	logger.Info().Msg("Completed activity")

	return nil
}

// CreateExpectedSwitchOnRLA creates an Expected Switch as a component in RLA via AddComponent
func (mes *ManageExpectedSwitch) CreateExpectedSwitchOnRLA(ctx context.Context, request *cwssaws.ExpectedSwitch) error {
	logger := log.With().Str("Activity", "CreateExpectedSwitchOnRLA").Logger()

	logger.Info().Msg("Starting activity")

	// Validate request
	if request == nil {
		return temporal.NewNonRetryableApplicationError("received empty create Expected Switch request for RLA", swe.ErrTypeInvalidRequest, errors.New("nil request"))
	}

	// If RLA client is not configured, skip gracefully
	if mes.RlaAtomicClient == nil {
		logger.Warn().Msg("RLA client not configured, skipping RLA component creation")
		return nil
	}

	rlaClient := mes.RlaAtomicClient.GetClient()
	if rlaClient == nil {
		logger.Warn().Msg("RLA client not connected, skipping RLA component creation")
		return nil
	}

	component := expectedSwitchToRLAComponent(request)
	_, err := rlaClient.Rla().AddComponent(ctx, &rlav1.AddComponentRequest{Component: component})
	if err != nil {
		logger.Warn().Err(err).Msg("Failed to create Expected Switch component on RLA")
		return swe.WrapErr(err)
	}

	logger.Info().Msg("Completed activity")
	return nil
}

// expectedSwitchToRLAComponent converts a Forge ExpectedSwitch proto to an RLA Component proto
func expectedSwitchToRLAComponent(es *cwssaws.ExpectedSwitch) *rlav1.Component {
	component := &rlav1.Component{
		Type: rlav1.ComponentType_COMPONENT_TYPE_NVLSWITCH,
		Info: &rlav1.DeviceInfo{
			Id:           &rlav1.UUID{Id: es.GetExpectedSwitchId().GetValue()},
			SerialNumber: es.GetSwitchSerialNumber(),
		},
		Bmcs: []*rlav1.BMCInfo{
			{
				Type:       rlav1.BMCType_BMC_TYPE_HOST,
				MacAddress: es.GetBmcMacAddress(),
			},
		},
		ComponentId: es.GetExpectedSwitchId().GetValue(),
	}

	// DeviceInfo fields
	if name := es.GetName(); name != "" {
		component.Info.Name = name
	}
	if manufacturer := es.GetManufacturer(); manufacturer != "" {
		component.Info.Manufacturer = manufacturer
	}
	if es.Model != nil {
		component.Info.Model = es.Model
	}
	if es.Description != nil {
		component.Info.Description = es.Description
	}

	// Firmware version
	if fv := es.GetFirmwareVersion(); fv != "" {
		component.FirmwareVersion = fv
	}

	// Rack position
	if es.SlotId != nil || es.TrayIdx != nil || es.HostId != nil {
		pos := &rlav1.RackPosition{}
		if es.SlotId != nil {
			pos.SlotId = *es.SlotId
		}
		if es.TrayIdx != nil {
			pos.TrayIdx = *es.TrayIdx
		}
		if es.HostId != nil {
			pos.HostId = *es.HostId
		}
		component.Position = pos
	}

	if rackID := es.GetRackId().GetId(); rackID != "" {
		component.RackId = &rlav1.UUID{Id: rackID}
	}

	return component
}

// DeleteExpectedSwitchOnSite deletes Expected Switch on Carbide
func (mes *ManageExpectedSwitch) DeleteExpectedSwitchOnSite(ctx context.Context, request *cwssaws.ExpectedSwitchRequest) error {
	logger := log.With().Str("Activity", "DeleteExpectedSwitchOnSite").Logger()

	logger.Info().Msg("Starting activity")

	var err error

	// Validate request
	if request == nil {
		err = errors.New("received empty delete Expected Switch request")
	} else if request.GetExpectedSwitchId().GetValue() == "" {
		err = errors.New("received delete Expected Switch request without required id field")
	}

	if err != nil {
		return temporal.NewNonRetryableApplicationError(err.Error(), swe.ErrTypeInvalidRequest, err)
	}

	// Call Site Controller gRPC endpoint
	forgeClient, err := mes.CarbideAtomicClient.GetForgeClient()
	if err != nil {
		return err
	}

	_, err = forgeClient.DeleteExpectedSwitch(ctx, request)
	if err != nil {
		logger.Warn().Err(err).Msg("Failed to delete Expected Switch using Site Controller API")
		return swe.WrapErr(err)
	}

	logger.Info().Msg("Completed activity")

	return nil
}
