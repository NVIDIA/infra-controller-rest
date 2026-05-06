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

package server

import (
	"context"
	"net"
	"time"

	"github.com/gogo/status"
	"github.com/google/uuid"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/reflection"

	emptypb "google.golang.org/protobuf/types/known/emptypb"

	"github.com/rs/zerolog/log"

	rlav1 "github.com/NVIDIA/infra-controller-rest/workflow-schema/rla/protobuf/v1"
)

var (
	// RlaDefaultPort is the default port that the RLA server listens at
	RlaDefaultPort = ":11080"
)

// RlaServerImpl implements interface RLAServer
type RlaServerImpl struct {
	rlav1.UnimplementedRLAServer
	racks           map[string]*rlav1.Rack
	components      map[string]*rlav1.Component
	nvlDomains      map[string]*rlav1.NVLDomain
	tasks           map[string]*rlav1.Task
	rackToDomainMap map[string]string // Maps rack ID to domain ID
}

var rlaLogger = log.With().Str("Component", "Mock RLA gRPC Server").Logger()

// Version implements interface RLAServer
func (r *RlaServerImpl) Version(ctx context.Context, req *rlav1.VersionRequest) (*rlav1.BuildInfo, error) {
	return &rlav1.BuildInfo{
		Version:   "1.0.0",
		BuildTime: time.Now().Format(time.RFC3339),
		GitCommit: "test-commit",
	}, nil
}

// CreateExpectedRack implements interface RLAServer
func (r *RlaServerImpl) CreateExpectedRack(ctx context.Context, req *rlav1.CreateExpectedRackRequest) (*rlav1.CreateExpectedRackResponse, error) {
	if req == nil || req.Rack == nil {
		return nil, status.Errorf(codes.InvalidArgument, "Invalid request argument")
	}

	rackID := uuid.NewString()
	if req.Rack.Info != nil && req.Rack.Info.Id != nil {
		rackID = req.Rack.Info.Id.Id
	}

	rack := &rlav1.Rack{
		Info: &rlav1.DeviceInfo{
			Id: &rlav1.UUID{Id: rackID},
		},
		Location:   req.Rack.Location,
		Components: req.Rack.Components,
	}

	if req.Rack.Info != nil {
		rack.Info.Name = req.Rack.Info.Name
		rack.Info.Manufacturer = req.Rack.Info.Manufacturer
		rack.Info.SerialNumber = req.Rack.Info.SerialNumber
		if req.Rack.Info.Model != nil {
			rack.Info.Model = req.Rack.Info.Model
		}
		if req.Rack.Info.Description != nil {
			rack.Info.Description = req.Rack.Info.Description
		}
	}

	r.racks[rackID] = rack

	// Store components
	for _, comp := range rack.Components {
		if comp.ComponentId != "" {
			r.components[comp.ComponentId] = comp
		}
	}

	return &rlav1.CreateExpectedRackResponse{
		Id: &rlav1.UUID{Id: rackID},
	}, nil
}

// PatchRack implements interface RLAServer
func (r *RlaServerImpl) PatchRack(ctx context.Context, req *rlav1.PatchRackRequest) (*rlav1.PatchRackResponse, error) {
	if req == nil || req.Rack == nil || req.Rack.Info == nil || req.Rack.Info.Id == nil {
		return nil, status.Errorf(codes.InvalidArgument, "Invalid request argument")
	}

	rackID := req.Rack.Info.Id.Id
	rack, ok := r.racks[rackID]
	if !ok {
		return nil, status.Errorf(codes.NotFound, "Rack with ID not found")
	}

	// Update rack fields
	if req.Rack.Info.Name != "" {
		rack.Info.Name = req.Rack.Info.Name
	}
	if req.Rack.Location != nil {
		rack.Location = req.Rack.Location
	}
	if len(req.Rack.Components) > 0 {
		rack.Components = req.Rack.Components
	}

	return &rlav1.PatchRackResponse{
		Report: "Rack patched successfully",
	}, nil
}

// GetRackInfoByID implements interface RLAServer
func (r *RlaServerImpl) GetRackInfoByID(ctx context.Context, req *rlav1.GetRackInfoByIDRequest) (*rlav1.GetRackInfoResponse, error) {
	if req == nil || req.Id == nil {
		return nil, status.Errorf(codes.InvalidArgument, "Invalid request argument")
	}

	rack, ok := r.racks[req.Id.Id]
	if !ok {
		return nil, status.Errorf(codes.NotFound, "Rack with ID not found")
	}

	response := &rlav1.GetRackInfoResponse{
		Rack: rack,
	}

	if !req.WithComponents {
		// Return rack without components
		response.Rack = &rlav1.Rack{
			Info:     rack.Info,
			Location: rack.Location,
		}
	}

	return response, nil
}

// GetRackInfoBySerial implements interface RLAServer
func (r *RlaServerImpl) GetRackInfoBySerial(ctx context.Context, req *rlav1.GetRackInfoBySerialRequest) (*rlav1.GetRackInfoResponse, error) {
	if req == nil || req.SerialInfo == nil {
		return nil, status.Errorf(codes.InvalidArgument, "Invalid request argument")
	}

	// Find rack by serial number
	for _, rack := range r.racks {
		if rack.Info != nil && rack.Info.SerialNumber == req.SerialInfo.SerialNumber {
			response := &rlav1.GetRackInfoResponse{
				Rack: rack,
			}
			if !req.WithComponents {
				response.Rack = &rlav1.Rack{
					Info:     rack.Info,
					Location: rack.Location,
				}
			}
			return response, nil
		}
	}

	return nil, status.Errorf(codes.NotFound, "Rack with serial number not found")
}

// GetComponentInfoByID implements interface RLAServer
func (r *RlaServerImpl) GetComponentInfoByID(ctx context.Context, req *rlav1.GetComponentInfoByIDRequest) (*rlav1.GetComponentInfoResponse, error) {
	if req == nil || req.Id == nil {
		return nil, status.Errorf(codes.InvalidArgument, "Invalid request argument")
	}

	// Find component by UUID
	for _, comp := range r.components {
		if comp.Info != nil && comp.Info.Id != nil && comp.Info.Id.Id == req.Id.Id {
			response := &rlav1.GetComponentInfoResponse{
				Component: comp,
			}
			if req.WithRack {
				// Find the rack containing this component
				for _, rack := range r.racks {
					for _, rackComp := range rack.Components {
						if rackComp.ComponentId == comp.ComponentId {
							response.Rack = rack
							break
						}
					}
					if response.Rack != nil {
						break
					}
				}
			}
			return response, nil
		}
	}

	return nil, status.Errorf(codes.NotFound, "Component with ID not found")
}

// GetComponentInfoBySerial implements interface RLAServer
func (r *RlaServerImpl) GetComponentInfoBySerial(ctx context.Context, req *rlav1.GetComponentInfoBySerialRequest) (*rlav1.GetComponentInfoResponse, error) {
	if req == nil || req.SerialInfo == nil {
		return nil, status.Errorf(codes.InvalidArgument, "Invalid request argument")
	}

	// Find component by serial number
	for _, comp := range r.components {
		if comp.Info != nil && comp.Info.SerialNumber == req.SerialInfo.SerialNumber {
			response := &rlav1.GetComponentInfoResponse{
				Component: comp,
			}
			if req.WithRack {
				// Find the rack containing this component
				for _, rack := range r.racks {
					for _, rackComp := range rack.Components {
						if rackComp.ComponentId == comp.ComponentId {
							response.Rack = rack
							break
						}
					}
					if response.Rack != nil {
						break
					}
				}
			}
			return response, nil
		}
	}

	return nil, status.Errorf(codes.NotFound, "Component with serial number not found")
}

// GetListOfRacks implements interface RLAServer
func (r *RlaServerImpl) GetListOfRacks(ctx context.Context, req *rlav1.GetListOfRacksRequest) (*rlav1.GetListOfRacksResponse, error) {
	if req == nil {
		return nil, status.Errorf(codes.InvalidArgument, "Invalid request argument")
	}

	var racks []*rlav1.Rack
	for _, rack := range r.racks {
		if req.WithComponents {
			racks = append(racks, rack)
		} else {
			racks = append(racks, &rlav1.Rack{
				Info:     rack.Info,
				Location: rack.Location,
			})
		}
	}

	return &rlav1.GetListOfRacksResponse{
		Racks: racks,
		Total: int32(len(racks)),
	}, nil
}

// CreateNVLDomain implements interface RLAServer
func (r *RlaServerImpl) CreateNVLDomain(ctx context.Context, req *rlav1.CreateNVLDomainRequest) (*rlav1.CreateNVLDomainResponse, error) {
	if req == nil || req.NvlDomain == nil || req.NvlDomain.Identifier == nil {
		return nil, status.Errorf(codes.InvalidArgument, "Invalid request argument")
	}

	domainID := uuid.NewString()
	if req.NvlDomain.Identifier.Id != nil {
		domainID = req.NvlDomain.Identifier.Id.Id
	}

	domain := &rlav1.NVLDomain{
		Identifier: &rlav1.Identifier{
			Id:   &rlav1.UUID{Id: domainID},
			Name: req.NvlDomain.Identifier.Name,
		},
	}

	r.nvlDomains[domainID] = domain

	return &rlav1.CreateNVLDomainResponse{
		Id: &rlav1.UUID{Id: domainID},
	}, nil
}

// AttachRacksToNVLDomain implements interface RLAServer
func (r *RlaServerImpl) AttachRacksToNVLDomain(ctx context.Context, req *rlav1.AttachRacksToNVLDomainRequest) (*emptypb.Empty, error) {
	if req == nil || req.NvlDomainIdentifier == nil {
		return nil, status.Errorf(codes.InvalidArgument, "Invalid request argument")
	}

	domainID := ""
	if req.NvlDomainIdentifier.Id != nil {
		domainID = req.NvlDomainIdentifier.Id.Id
	} else if req.NvlDomainIdentifier.Name != "" {
		// Find domain by name
		for id, domain := range r.nvlDomains {
			if domain.Identifier != nil && domain.Identifier.Name == req.NvlDomainIdentifier.Name {
				domainID = id
				break
			}
		}
	}

	if domainID == "" {
		return nil, status.Errorf(codes.NotFound, "NVL Domain not found")
	}

	// Attach racks to domain
	for _, rackIdentifier := range req.RackIdentifiers {
		rackID := ""
		if rackIdentifier.Id != nil {
			rackID = rackIdentifier.Id.Id
		} else if rackIdentifier.Name != "" {
			// Find rack by name
			for id, rack := range r.racks {
				if rack.Info != nil && rack.Info.Name == rackIdentifier.Name {
					rackID = id
					break
				}
			}
		}

		if rackID != "" {
			r.rackToDomainMap[rackID] = domainID
		}
	}

	return &emptypb.Empty{}, nil
}

// DetachRacksFromNVLDomain implements interface RLAServer
func (r *RlaServerImpl) DetachRacksFromNVLDomain(ctx context.Context, req *rlav1.DetachRacksFromNVLDomainRequest) (*emptypb.Empty, error) {
	if req == nil {
		return nil, status.Errorf(codes.InvalidArgument, "Invalid request argument")
	}

	// Detach racks from domain
	for _, rackIdentifier := range req.RackIdentifiers {
		rackID := ""
		if rackIdentifier.Id != nil {
			rackID = rackIdentifier.Id.Id
		} else if rackIdentifier.Name != "" {
			// Find rack by name
			for id, rack := range r.racks {
				if rack.Info != nil && rack.Info.Name == rackIdentifier.Name {
					rackID = id
					break
				}
			}
		}

		if rackID != "" {
			delete(r.rackToDomainMap, rackID)
		}
	}

	return &emptypb.Empty{}, nil
}

// GetListOfNVLDomains implements interface RLAServer
func (r *RlaServerImpl) GetListOfNVLDomains(ctx context.Context, req *rlav1.GetListOfNVLDomainsRequest) (*rlav1.GetListOfNVLDomainsResponse, error) {
	if req == nil {
		return nil, status.Errorf(codes.InvalidArgument, "Invalid request argument")
	}

	var domains []*rlav1.NVLDomain
	for _, domain := range r.nvlDomains {
		domains = append(domains, domain)
	}

	return &rlav1.GetListOfNVLDomainsResponse{
		NvlDomains: domains,
		Total:      int32(len(domains)),
	}, nil
}

// GetRacksForNVLDomain implements interface RLAServer
func (r *RlaServerImpl) GetRacksForNVLDomain(ctx context.Context, req *rlav1.GetRacksForNVLDomainRequest) (*rlav1.GetRacksForNVLDomainResponse, error) {
	if req == nil || req.NvlDomainIdentifier == nil {
		return nil, status.Errorf(codes.InvalidArgument, "Invalid request argument")
	}

	domainID := ""
	if req.NvlDomainIdentifier.Id != nil {
		domainID = req.NvlDomainIdentifier.Id.Id
	} else if req.NvlDomainIdentifier.Name != "" {
		// Find domain by name
		for id, domain := range r.nvlDomains {
			if domain.Identifier != nil && domain.Identifier.Name == req.NvlDomainIdentifier.Name {
				domainID = id
				break
			}
		}
	}

	if domainID == "" {
		return nil, status.Errorf(codes.NotFound, "NVL Domain not found")
	}

	// Find all racks attached to this domain
	var racks []*rlav1.Rack
	for rackID, attachedDomainID := range r.rackToDomainMap {
		if attachedDomainID == domainID {
			if rack, ok := r.racks[rackID]; ok {
				racks = append(racks, rack)
			}
		}
	}

	return &rlav1.GetRacksForNVLDomainResponse{
		Racks: racks,
	}, nil
}

// UpgradeFirmware implements interface RLAServer
func (r *RlaServerImpl) UpgradeFirmware(ctx context.Context, req *rlav1.UpgradeFirmwareRequest) (*rlav1.SubmitTaskResponse, error) {
	if req == nil || req.TargetSpec == nil {
		return nil, status.Errorf(codes.InvalidArgument, "Invalid request argument")
	}

	taskID := uuid.NewString()
	task := &rlav1.Task{
		Id:           &rlav1.UUID{Id: taskID},
		Operation:    "UpgradeFirmware",
		Status:       rlav1.TaskStatus_TASK_STATUS_PENDING,
		ExecutorType: rlav1.TaskExecutorType_TASK_EXECUTOR_TYPE_TEMPORAL,
		Message:      "Firmware upgrade task created",
	}
	r.tasks[taskID] = task

	return &rlav1.SubmitTaskResponse{
		TaskIds: []*rlav1.UUID{{Id: taskID}},
	}, nil
}

// GetComponents implements interface RLAServer
func (r *RlaServerImpl) GetComponents(ctx context.Context, req *rlav1.GetComponentsRequest) (*rlav1.GetComponentsResponse, error) {
	if req == nil {
		return nil, status.Errorf(codes.InvalidArgument, "Invalid request argument")
	}

	var components []*rlav1.Component
	for _, comp := range r.components {
		components = append(components, comp)
	}

	return &rlav1.GetComponentsResponse{
		Components: components,
		Total:      int32(len(components)),
	}, nil
}

// AddComponent implements interface RLAServer
func (r *RlaServerImpl) AddComponent(ctx context.Context, req *rlav1.AddComponentRequest) (*rlav1.AddComponentResponse, error) {
	if req == nil || req.Component == nil {
		return nil, status.Errorf(codes.InvalidArgument, "Invalid request argument")
	}

	if req.Component.RackId == nil || req.Component.RackId.Id == "" {
		return nil, status.Errorf(codes.InvalidArgument, "component.rack_id must be set")
	}

	componentID := req.Component.ComponentId
	if componentID == "" {
		componentID = uuid.NewString()
	}

	component := &rlav1.Component{
		Type:            req.Component.Type,
		Info:            req.Component.Info,
		FirmwareVersion: req.Component.FirmwareVersion,
		Position:        req.Component.Position,
		Bmcs:            req.Component.Bmcs,
		ComponentId:     componentID,
		RackId:          req.Component.RackId,
		PowerState:      req.Component.PowerState,
	}

	r.components[componentID] = component

	return &rlav1.AddComponentResponse{
		Component: component,
	}, nil
}

// PatchComponent implements interface RLAServer
func (r *RlaServerImpl) PatchComponent(ctx context.Context, req *rlav1.PatchComponentRequest) (*rlav1.PatchComponentResponse, error) {
	if req == nil || req.Id == nil || req.Id.Id == "" {
		return nil, status.Errorf(codes.InvalidArgument, "Invalid request argument")
	}

	// Find component by UUID
	var comp *rlav1.Component
	for _, c := range r.components {
		if c.Info != nil && c.Info.Id != nil && c.Info.Id.Id == req.Id.Id {
			comp = c
			break
		}
	}

	if comp == nil {
		return nil, status.Errorf(codes.NotFound, "Component with ID not found")
	}

	// Apply patch fields
	if req.FirmwareVersion != nil {
		comp.FirmwareVersion = *req.FirmwareVersion
	}
	if req.Position != nil {
		comp.Position = req.Position
	}
	if req.RackId != nil {
		comp.RackId = req.RackId
	}

	return &rlav1.PatchComponentResponse{
		Component: comp,
	}, nil
}

// DeleteComponent implements interface RLAServer
func (r *RlaServerImpl) DeleteComponent(ctx context.Context, req *rlav1.DeleteComponentRequest) (*rlav1.DeleteComponentResponse, error) {
	if req == nil || req.Id == nil || req.Id.Id == "" {
		return nil, status.Errorf(codes.InvalidArgument, "Invalid request argument")
	}

	// Find and delete component by UUID
	found := false
	for key, comp := range r.components {
		if comp.Info != nil && comp.Info.Id != nil && comp.Info.Id.Id == req.Id.Id {
			delete(r.components, key)
			found = true
			break
		}
	}

	if !found {
		return nil, status.Errorf(codes.NotFound, "Component with ID not found")
	}

	return &rlav1.DeleteComponentResponse{}, nil
}

// ValidateComponents implements interface RLAServer
func (r *RlaServerImpl) ValidateComponents(ctx context.Context, req *rlav1.ValidateComponentsRequest) (*rlav1.ValidateComponentsResponse, error) {
	if req == nil || req.TargetSpec == nil {
		return nil, status.Errorf(codes.InvalidArgument, "Invalid request argument")
	}

	// Get components
	componentsResp, err := r.GetComponents(ctx, &rlav1.GetComponentsRequest{
		TargetSpec: req.TargetSpec,
	})
	if err != nil {
		return nil, err
	}

	// For validation, we treat the components as both expected and actual
	// In the new proto, actual is also a Component (ActualComponent was removed)
	actualComponents := make([]*rlav1.Component, 0, len(componentsResp.Components))
	for _, comp := range componentsResp.Components {
		actualComp := &rlav1.Component{
			Type:            comp.Type,
			Info:            comp.Info,
			FirmwareVersion: comp.FirmwareVersion,
			Position:        comp.Position,
			Bmcs:            comp.Bmcs,
			ComponentId:     comp.ComponentId,
			RackId:          comp.RackId,
			PowerState:      "on",
		}
		actualComponents = append(actualComponents, actualComp)
	}

	expectedMap := make(map[string]*rlav1.Component)
	for _, comp := range componentsResp.Components {
		if comp.ComponentId != "" {
			expectedMap[comp.ComponentId] = comp
		}
	}

	actualMap := make(map[string]*rlav1.Component)
	for _, comp := range actualComponents {
		if comp.ComponentId != "" {
			actualMap[comp.ComponentId] = comp
		}
	}

	var diffs []*rlav1.ComponentDiff
	missingCount := 0
	unexpectedCount := 0
	driftCount := 0
	matchCount := 0

	// Find components only in expected
	for compID, expectedComp := range expectedMap {
		if _, exists := actualMap[compID]; !exists {
			diffs = append(diffs, &rlav1.ComponentDiff{
				Type:        rlav1.DiffType_DIFF_TYPE_MISSING,
				ComponentId: compID,
				Expected:    expectedComp,
			})
			missingCount++
		}
	}

	// Find components only in actual
	for compID, actualComp := range actualMap {
		if _, exists := expectedMap[compID]; !exists {
			diffs = append(diffs, &rlav1.ComponentDiff{
				Type:        rlav1.DiffType_DIFF_TYPE_UNEXPECTED,
				ComponentId: compID,
				Actual:      actualComp,
			})
			unexpectedCount++
		}
	}

	// Find components in both (check for drift)
	for compID, expectedComp := range expectedMap {
		if actualComp, exists := actualMap[compID]; exists {
			// Simple comparison: check if firmware version differs
			if expectedComp.FirmwareVersion != actualComp.FirmwareVersion {
				var fieldDiffs []*rlav1.FieldDiff
				fieldDiffs = append(fieldDiffs, &rlav1.FieldDiff{
					FieldName:     "firmware_version",
					ExpectedValue: expectedComp.FirmwareVersion,
					ActualValue:   actualComp.FirmwareVersion,
				})
				diffs = append(diffs, &rlav1.ComponentDiff{
					Type:        rlav1.DiffType_DIFF_TYPE_DRIFT,
					ComponentId: compID,
					FieldDiffs:  fieldDiffs,
				})
				driftCount++
			} else {
				matchCount++
			}
		}
	}

	return &rlav1.ValidateComponentsResponse{
		Diffs:           diffs,
		TotalDiffs:      int32(len(diffs)),
		MissingCount:    int32(missingCount),
		UnexpectedCount: int32(unexpectedCount),
		DriftCount:      int32(driftCount),
		MatchCount:      int32(matchCount),
	}, nil
}

// PowerOnRack implements interface RLAServer
func (r *RlaServerImpl) PowerOnRack(ctx context.Context, req *rlav1.PowerOnRackRequest) (*rlav1.SubmitTaskResponse, error) {
	if req == nil || req.TargetSpec == nil {
		return nil, status.Errorf(codes.InvalidArgument, "Invalid request argument")
	}

	taskID := uuid.NewString()
	task := &rlav1.Task{
		Id:           &rlav1.UUID{Id: taskID},
		Operation:    "PowerOnRack",
		Status:       rlav1.TaskStatus_TASK_STATUS_PENDING,
		ExecutorType: rlav1.TaskExecutorType_TASK_EXECUTOR_TYPE_TEMPORAL,
		Message:      "Power on task created",
	}
	r.tasks[taskID] = task

	return &rlav1.SubmitTaskResponse{
		TaskIds: []*rlav1.UUID{{Id: taskID}},
	}, nil
}

// PowerOffRack implements interface RLAServer
func (r *RlaServerImpl) PowerOffRack(ctx context.Context, req *rlav1.PowerOffRackRequest) (*rlav1.SubmitTaskResponse, error) {
	if req == nil || req.TargetSpec == nil {
		return nil, status.Errorf(codes.InvalidArgument, "Invalid request argument")
	}

	taskID := uuid.NewString()
	task := &rlav1.Task{
		Id:           &rlav1.UUID{Id: taskID},
		Operation:    "PowerOffRack",
		Status:       rlav1.TaskStatus_TASK_STATUS_PENDING,
		ExecutorType: rlav1.TaskExecutorType_TASK_EXECUTOR_TYPE_TEMPORAL,
		Message:      "Power off task created",
	}
	r.tasks[taskID] = task

	return &rlav1.SubmitTaskResponse{
		TaskIds: []*rlav1.UUID{{Id: taskID}},
	}, nil
}

// PowerResetRack implements interface RLAServer
func (r *RlaServerImpl) PowerResetRack(ctx context.Context, req *rlav1.PowerResetRackRequest) (*rlav1.SubmitTaskResponse, error) {
	if req == nil || req.TargetSpec == nil {
		return nil, status.Errorf(codes.InvalidArgument, "Invalid request argument")
	}

	taskID := uuid.NewString()
	task := &rlav1.Task{
		Id:           &rlav1.UUID{Id: taskID},
		Operation:    "PowerResetRack",
		Status:       rlav1.TaskStatus_TASK_STATUS_PENDING,
		ExecutorType: rlav1.TaskExecutorType_TASK_EXECUTOR_TYPE_TEMPORAL,
		Message:      "Power reset task created",
	}
	r.tasks[taskID] = task

	return &rlav1.SubmitTaskResponse{
		TaskIds: []*rlav1.UUID{{Id: taskID}},
	}, nil
}

// BringUpRack implements interface RLAServer
func (r *RlaServerImpl) BringUpRack(ctx context.Context, req *rlav1.BringUpRackRequest) (*rlav1.SubmitTaskResponse, error) {
	if req == nil || req.TargetSpec == nil {
		return nil, status.Errorf(codes.InvalidArgument, "Invalid request argument")
	}

	taskID := uuid.NewString()
	task := &rlav1.Task{
		Id:           &rlav1.UUID{Id: taskID},
		Operation:    "BringUpRack",
		Status:       rlav1.TaskStatus_TASK_STATUS_PENDING,
		ExecutorType: rlav1.TaskExecutorType_TASK_EXECUTOR_TYPE_TEMPORAL,
		Message:      "Bring up task created",
	}
	r.tasks[taskID] = task

	return &rlav1.SubmitTaskResponse{
		TaskIds: []*rlav1.UUID{{Id: taskID}},
	}, nil
}

// ListTasks implements interface RLAServer
func (r *RlaServerImpl) ListTasks(ctx context.Context, req *rlav1.ListTasksRequest) (*rlav1.ListTasksResponse, error) {
	if req == nil {
		return nil, status.Errorf(codes.InvalidArgument, "Invalid request argument")
	}

	var tasks []*rlav1.Task
	for _, task := range r.tasks {
		if req.ActiveOnly && (task.Status == rlav1.TaskStatus_TASK_STATUS_COMPLETED || task.Status == rlav1.TaskStatus_TASK_STATUS_FAILED) {
			continue
		}
		if req.RackId != nil && task.RackId != nil && task.RackId.Id != req.RackId.Id {
			continue
		}
		tasks = append(tasks, task)
	}

	return &rlav1.ListTasksResponse{
		Tasks: tasks,
		Total: int32(len(tasks)),
	}, nil
}

// GetTasksByIDs implements interface RLAServer
func (r *RlaServerImpl) GetTasksByIDs(ctx context.Context, req *rlav1.GetTasksByIDsRequest) (*rlav1.GetTasksByIDsResponse, error) {
	if req == nil {
		return nil, status.Errorf(codes.InvalidArgument, "Invalid request argument")
	}

	var tasks []*rlav1.Task
	for _, taskID := range req.TaskIds {
		if task, ok := r.tasks[taskID.Id]; ok {
			tasks = append(tasks, task)
		}
	}

	return &rlav1.GetTasksByIDsResponse{
		Tasks: tasks,
	}, nil
}

// RlaTest starts the RLA test gRPC server
func RlaTest(secs int) {
	listener, err := net.Listen("tcp", RlaDefaultPort)
	if err != nil {
		panic(err)
	}

	s := grpc.NewServer()
	reflection.Register(s)
	rlav1.RegisterRLAServer(s, &RlaServerImpl{
		racks:           make(map[string]*rlav1.Rack),
		components:      make(map[string]*rlav1.Component),
		nvlDomains:      make(map[string]*rlav1.NVLDomain),
		tasks:           make(map[string]*rlav1.Task),
		rackToDomainMap: make(map[string]string),
	})

	if secs != 0 {
		timer := time.AfterFunc(time.Second*time.Duration(secs), func() {
			s.GracefulStop()
			rlaLogger.Info().Msgf("Timer started for: %v seconds", secs)
		})
		defer timer.Stop()
	}

	rlaLogger.Info().Msg("Started RLA API server")

	err = s.Serve(listener)
	if err != nil {
		rlaLogger.Fatal().Err(err).Msg("Failed to start RLA API server")
	}

	rlaLogger.Info().Msg("Stopped RLA API server")
}
