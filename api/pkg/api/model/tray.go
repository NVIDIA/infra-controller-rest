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

package model

import (
	"fmt"
	"maps"
	"net/url"
	"slices"

	"github.com/google/uuid"

	validation "github.com/go-ozzo/ozzo-validation/v4"
	validationis "github.com/go-ozzo/ozzo-validation/v4/is"

	rlav1 "github.com/NVIDIA/infra-controller-rest/workflow-schema/rla/protobuf/v1"
)

// APIToProtoComponentTypeName maps API tray type strings to protobuf ComponentType enum names.
// These names match RLA's internal ComponentTypeFromString (case-insensitive).
var APIToProtoComponentTypeName = map[string]string{
	"Compute":    "COMPONENT_TYPE_COMPUTE",
	"NVLSwitch":  "COMPONENT_TYPE_NVLSWITCH",
	"PowerShelf": "COMPONENT_TYPE_POWERSHELF",
}

// ProtoToAPIComponentTypeName maps protobuf ComponentType to API tray type strings.
var ProtoToAPIComponentTypeName = map[rlav1.ComponentType]string{
	rlav1.ComponentType_COMPONENT_TYPE_UNKNOWN:    "Unknown",
	rlav1.ComponentType_COMPONENT_TYPE_COMPUTE:    "Compute",
	rlav1.ComponentType_COMPONENT_TYPE_NVLSWITCH:  "NVLSwitch",
	rlav1.ComponentType_COMPONENT_TYPE_POWERSHELF: "PowerShelf",
}

var validTrayTypesAny, ValidProtoComponentTypes = func() ([]interface{}, []rlav1.ComponentType) {
	anyTypes := make([]interface{}, 0, len(APIToProtoComponentTypeName))
	protoTypes := make([]rlav1.ComponentType, 0, len(APIToProtoComponentTypeName))
	for apiName, protoName := range APIToProtoComponentTypeName {
		anyTypes = append(anyTypes, apiName)
		protoTypes = append(protoTypes, rlav1.ComponentType(rlav1.ComponentType_value[protoName]))
	}
	return anyTypes, protoTypes
}()

// TrayFilterFieldMap maps API field names to RLA protobuf ComponentFilterField enum for tray validation queries
var TrayFilterFieldMap = map[string]rlav1.ComponentFilterField{
	"name":         rlav1.ComponentFilterField_COMPONENT_FILTER_FIELD_NAME,
	"manufacturer": rlav1.ComponentFilterField_COMPONENT_FILTER_FIELD_MANUFACTURER,
	"type":         rlav1.ComponentFilterField_COMPONENT_FILTER_FIELD_TYPE,
}

// GetProtoTrayFilter creates an RLA protobuf Filter for the given tray field and patterns.
// Multiple patterns are OR'd together.
func GetProtoTrayFilter(fieldName string, patterns []string) *rlav1.Filter {
	field, ok := TrayFilterFieldMap[fieldName]
	if !ok || len(patterns) == 0 {
		return nil
	}
	return &rlav1.Filter{
		Field: &rlav1.Filter_ComponentField{
			ComponentField: field,
		},
		QueryInfo: &rlav1.StringQueryInfo{
			Patterns:   patterns,
			IsWildcard: false,
			UseOr:      len(patterns) > 1,
		},
	}
}

// TrayOrderByFieldMap maps API field names to RLA protobuf ComponentOrderByField enum
var TrayOrderByFieldMap = map[string]rlav1.ComponentOrderByField{
	"name":         rlav1.ComponentOrderByField_COMPONENT_ORDER_BY_FIELD_NAME,
	"manufacturer": rlav1.ComponentOrderByField_COMPONENT_ORDER_BY_FIELD_MANUFACTURER,
	"model":        rlav1.ComponentOrderByField_COMPONENT_ORDER_BY_FIELD_MODEL,
	"type":         rlav1.ComponentOrderByField_COMPONENT_ORDER_BY_FIELD_TYPE,
}

// GetProtoTrayOrderByFromQueryParam creates an RLA protobuf OrderBy from API query parameters for tray (component) queries
func GetProtoTrayOrderByFromQueryParam(fieldName, direction string) *rlav1.OrderBy {
	field, ok := TrayOrderByFieldMap[fieldName]
	if !ok {
		return nil
	}
	return &rlav1.OrderBy{
		Field: &rlav1.OrderBy_ComponentField{
			ComponentField: field,
		},
		Direction: direction,
	}
}

// ========== Tray Filter (for batch operations) ==========

// TrayFilter specifies which trays to target in a batch operation.
// If nil or empty, the operation targets all trays in the site.
type TrayFilter struct {
	RackID       *string  `json:"rackId,omitempty"`
	RackName     *string  `json:"rackName,omitempty"`
	Type         *string  `json:"type,omitempty"`
	ComponentIDs []string `json:"componentIds,omitempty"`
	IDs          []string `json:"ids,omitempty"`
}

// Validate checks the tray filter fields.
func (f *TrayFilter) Validate() error {
	if f == nil {
		return nil
	}

	err := validation.ValidateStruct(f,
		validation.Field(&f.RackID,
			validation.When(f.RackID != nil, validationis.UUID.Error(validationErrorInvalidUUID))),
		validation.Field(&f.Type,
			validation.When(f.Type != nil, validation.In(validTrayTypesAny...).Error(
				fmt.Sprintf("must be one of %v", slices.Collect(maps.Keys(APIToProtoComponentTypeName)))))),
	)
	if err != nil {
		return err
	}

	for _, id := range f.IDs {
		if _, parseErr := uuid.Parse(id); parseErr != nil {
			return validation.Errors{"ids": fmt.Errorf("%s: %s", validationErrorInvalidUUID, id)}
		}
	}

	if f.RackID != nil && f.RackName != nil {
		return validation.Errors{"rackId": fmt.Errorf("rackId and rackName are mutually exclusive")}
	}

	hasRackParams := f.RackID != nil || f.RackName != nil
	hasComponentParams := len(f.IDs) > 0 || len(f.ComponentIDs) > 0
	if hasRackParams && hasComponentParams {
		return validation.Errors{"rackId": fmt.Errorf("rackId/rackName cannot be combined with ids/componentIds")}
	}

	if len(f.ComponentIDs) > 0 && f.Type == nil {
		return validation.Errors{"componentIds": fmt.Errorf("type is required when componentIds is provided")}
	}

	return nil
}

// ToTargetSpec converts the filter to an RLA OperationTargetSpec.
// Handles nil receiver gracefully (targets all trays).
func (f *TrayFilter) ToTargetSpec() *rlav1.OperationTargetSpec {
	if f == nil {
		return &rlav1.OperationTargetSpec{
			Targets: &rlav1.OperationTargetSpec_Racks{
				Racks: &rlav1.RackTargets{
					Targets: []*rlav1.RackTarget{{
						ComponentTypes: ValidProtoComponentTypes,
					}},
				},
			},
		}
	}

	hasIDs := len(f.IDs) > 0
	hasComponentIDsWithType := len(f.ComponentIDs) > 0 && f.Type != nil

	if hasIDs || hasComponentIDsWithType {
		componentTargets := make([]*rlav1.ComponentTarget, 0, len(f.IDs)+len(f.ComponentIDs))

		for _, id := range f.IDs {
			componentTargets = append(componentTargets, &rlav1.ComponentTarget{
				Identifier: &rlav1.ComponentTarget_Id{
					Id: &rlav1.UUID{Id: id},
				},
			})
		}

		if hasComponentIDsWithType {
			if protoName, ok := APIToProtoComponentTypeName[*f.Type]; ok {
				protoType := rlav1.ComponentType(rlav1.ComponentType_value[protoName])
				for _, cid := range f.ComponentIDs {
					componentTargets = append(componentTargets, &rlav1.ComponentTarget{
						Identifier: &rlav1.ComponentTarget_External{
							External: &rlav1.ExternalRef{
								Type: protoType,
								Id:   cid,
							},
						},
					})
				}
			}
		}

		return &rlav1.OperationTargetSpec{
			Targets: &rlav1.OperationTargetSpec_Components{
				Components: &rlav1.ComponentTargets{
					Targets: componentTargets,
				},
			},
		}
	}

	rackTarget := &rlav1.RackTarget{}

	if f.RackID != nil {
		rackTarget.Identifier = &rlav1.RackTarget_Id{
			Id: &rlav1.UUID{Id: *f.RackID},
		}
	} else if f.RackName != nil {
		rackTarget.Identifier = &rlav1.RackTarget_Name{
			Name: *f.RackName,
		}
	}

	if f.Type != nil {
		if protoName, ok := APIToProtoComponentTypeName[*f.Type]; ok {
			rackTarget.ComponentTypes = []rlav1.ComponentType{
				rlav1.ComponentType(rlav1.ComponentType_value[protoName]),
			}
		}
	} else {
		rackTarget.ComponentTypes = ValidProtoComponentTypes
	}

	return &rlav1.OperationTargetSpec{
		Targets: &rlav1.OperationTargetSpec_Racks{
			Racks: &rlav1.RackTargets{
				Targets: []*rlav1.RackTarget{rackTarget},
			},
		},
	}
}

// APITrayGetAllRequest captures query parameters for listing trays from RLA.
type APITrayGetAllRequest struct {
	SiteID       string   `query:"siteId"`
	RackID       *string  `query:"rackId"`
	RackName     *string  `query:"rackName"`
	Type         *string  `query:"type"`
	ComponentIDs []string `query:"componentId"`
	IDs          []string `query:"id"`
}

// Validate checks field formats and enforces the RLA protobuf oneof constraints:
//   - rackId must be a valid UUID
//   - rackId and rackName are mutually exclusive (RackTarget.oneof identifier)
//   - rackId/rackName cannot be combined with id/componentId (OperationTargetSpec.oneof targets)
//   - componentId requires type (ExternalRef needs type)
//   - type must be one of the supported tray types
//   - each entry in IDs must be a valid UUID
func (r *APITrayGetAllRequest) Validate() error {
	err := validation.ValidateStruct(r,
		validation.Field(&r.RackID,
			validation.When(r.RackID != nil, validationis.UUID.Error(validationErrorInvalidUUID))),
		validation.Field(&r.Type,
			validation.When(r.Type != nil, validation.In(validTrayTypesAny...).Error(
				fmt.Sprintf("must be one of %v", slices.Collect(maps.Keys(APIToProtoComponentTypeName)))))),
	)
	if err != nil {
		return err
	}

	for _, id := range r.IDs {
		if _, parseErr := uuid.Parse(id); parseErr != nil {
			return validation.Errors{"id": fmt.Errorf("%s: %s", validationErrorInvalidUUID, id)}
		}
	}

	if r.RackID != nil && r.RackName != nil {
		return validation.Errors{"rackId": fmt.Errorf("rackId and rackName are mutually exclusive")}
	}

	hasRackParams := r.RackID != nil || r.RackName != nil
	hasComponentParams := len(r.IDs) > 0 || len(r.ComponentIDs) > 0
	if hasRackParams && hasComponentParams {
		return validation.Errors{"rackId": fmt.Errorf("rackId/rackName cannot be combined with id/componentId")}
	}

	if len(r.ComponentIDs) > 0 && r.Type == nil {
		return validation.Errors{"componentId": fmt.Errorf("type is required when componentId is provided")}
	}

	return nil
}

// ToProto converts a validated APITrayGetAllRequest to an RLA GetComponentsRequest.
func (r *APITrayGetAllRequest) ToProto() *rlav1.GetComponentsRequest {
	rlaRequest := &rlav1.GetComponentsRequest{}

	hasIDs := len(r.IDs) > 0
	hasComponentIDsWithType := len(r.ComponentIDs) > 0 && r.Type != nil

	if hasIDs || hasComponentIDsWithType {
		componentTargets := make([]*rlav1.ComponentTarget, 0, len(r.IDs)+len(r.ComponentIDs))

		for _, id := range r.IDs {
			componentTargets = append(componentTargets, &rlav1.ComponentTarget{
				Identifier: &rlav1.ComponentTarget_Id{
					Id: &rlav1.UUID{Id: id},
				},
			})
		}

		if hasComponentIDsWithType {
			if protoName, ok := APIToProtoComponentTypeName[*r.Type]; ok {
				protoType := rlav1.ComponentType(rlav1.ComponentType_value[protoName])
				for _, cid := range r.ComponentIDs {
					componentTargets = append(componentTargets, &rlav1.ComponentTarget{
						Identifier: &rlav1.ComponentTarget_External{
							External: &rlav1.ExternalRef{
								Type: protoType,
								Id:   cid,
							},
						},
					})
				}
			}
		}

		rlaRequest.TargetSpec = &rlav1.OperationTargetSpec{
			Targets: &rlav1.OperationTargetSpec_Components{
				Components: &rlav1.ComponentTargets{
					Targets: componentTargets,
				},
			},
		}
		return rlaRequest
	}

	// When a specific rack is identified, use TargetSpec with a RackTarget.
	// When no rack identifier is provided, omit TargetSpec entirely so RLA
	// queries all components, and pass Type as a filter instead.
	if r.RackID != nil || r.RackName != nil {
		rackTarget := &rlav1.RackTarget{}
		if r.RackID != nil {
			rackTarget.Identifier = &rlav1.RackTarget_Id{
				Id: &rlav1.UUID{Id: *r.RackID},
			}
		} else {
			rackTarget.Identifier = &rlav1.RackTarget_Name{
				Name: *r.RackName,
			}
		}

		if r.Type != nil {
			if protoName, ok := APIToProtoComponentTypeName[*r.Type]; ok {
				rackTarget.ComponentTypes = []rlav1.ComponentType{
					rlav1.ComponentType(rlav1.ComponentType_value[protoName]),
				}
			}
		} else {
			rackTarget.ComponentTypes = ValidProtoComponentTypes
		}

		rlaRequest.TargetSpec = &rlav1.OperationTargetSpec{
			Targets: &rlav1.OperationTargetSpec_Racks{
				Racks: &rlav1.RackTargets{
					Targets: []*rlav1.RackTarget{rackTarget},
				},
			},
		}
		return rlaRequest
	}

	// No rack or component targeting — query all components via filters.
	if r.Type != nil {
		if f := GetProtoTrayFilter("type", []string{*r.Type}); f != nil {
			rlaRequest.Filters = append(rlaRequest.Filters, f)
		}
	}

	return rlaRequest
}

// QueryValues returns only the known query parameters as url.Values,
// suitable for deterministic workflow ID hashing without unknown param interference.
func (r *APITrayGetAllRequest) QueryValues() url.Values {
	v := url.Values{}
	v.Set("siteId", r.SiteID)
	if r.RackID != nil {
		v.Set("rackId", *r.RackID)
	}
	if r.RackName != nil {
		v.Set("rackName", *r.RackName)
	}
	if r.Type != nil {
		v.Set("type", *r.Type)
	}
	for _, cid := range r.ComponentIDs {
		v.Add("componentId", cid)
	}
	for _, id := range r.IDs {
		v.Add("id", id)
	}
	return v
}

// APITrayValidateAllRequest captures query parameters for validating trays.
type APITrayValidateAllRequest struct {
	SiteID       string   `query:"siteId"`
	RackID       *string  `query:"rackId"`
	RackName     *string  `query:"rackName"`
	Name         []string `query:"name"`
	Manufacturer []string `query:"manufacturer"`
	Type         *string  `query:"type"`
	ComponentIDs []string `query:"componentId"`
}

// Validate checks constraints on the request parameters.
func (r *APITrayValidateAllRequest) Validate() error {
	if r.SiteID == "" {
		return fmt.Errorf("siteId query parameter is required")
	}
	if err := validation.ValidateStruct(r,
		validation.Field(&r.RackID,
			validation.When(r.RackID != nil, validationis.UUID.Error(validationErrorInvalidUUID))),
		validation.Field(&r.Type,
			validation.When(r.Type != nil, validation.In(validTrayTypesAny...).Error(
				fmt.Sprintf("must be one of %v", slices.Collect(maps.Keys(APIToProtoComponentTypeName)))))),
	); err != nil {
		return err
	}
	if r.RackID != nil && r.RackName != nil {
		return validation.Errors{"rackId": fmt.Errorf("rackId and rackName are mutually exclusive")}
	}
	hasRackScope := r.RackID != nil || r.RackName != nil
	if hasRackScope && len(r.ComponentIDs) > 0 {
		return validation.Errors{"rackId": fmt.Errorf("rackId/rackName and componentId are mutually exclusive")}
	}
	if len(r.ComponentIDs) > 0 && r.Type == nil {
		return validation.Errors{"componentId": fmt.Errorf("type is required when componentId is provided")}
	}
	return nil
}

// ToTargetSpec converts the request's targeting fields to an RLA OperationTargetSpec.
func (r *APITrayValidateAllRequest) ToTargetSpec() *rlav1.OperationTargetSpec {
	if r.RackID != nil {
		return &rlav1.OperationTargetSpec{
			Targets: &rlav1.OperationTargetSpec_Racks{
				Racks: &rlav1.RackTargets{
					Targets: []*rlav1.RackTarget{
						{Identifier: &rlav1.RackTarget_Id{Id: &rlav1.UUID{Id: *r.RackID}}},
					},
				},
			},
		}
	}
	if r.RackName != nil {
		return &rlav1.OperationTargetSpec{
			Targets: &rlav1.OperationTargetSpec_Racks{
				Racks: &rlav1.RackTargets{
					Targets: []*rlav1.RackTarget{
						{Identifier: &rlav1.RackTarget_Name{Name: *r.RackName}},
					},
				},
			},
		}
	}
	if len(r.ComponentIDs) > 0 && r.Type != nil {
		protoName, ok := APIToProtoComponentTypeName[*r.Type]
		if !ok {
			return nil
		}
		protoType := rlav1.ComponentType(rlav1.ComponentType_value[protoName])
		targets := make([]*rlav1.ComponentTarget, 0, len(r.ComponentIDs))
		for _, cid := range r.ComponentIDs {
			targets = append(targets, &rlav1.ComponentTarget{
				Identifier: &rlav1.ComponentTarget_External{
					External: &rlav1.ExternalRef{
						Type: protoType,
						Id:   cid,
					},
				},
			})
		}
		return &rlav1.OperationTargetSpec{
			Targets: &rlav1.OperationTargetSpec_Components{
				Components: &rlav1.ComponentTargets{
					Targets: targets,
				},
			},
		}
	}
	return nil
}

// ToFilters converts the request's filter fields to RLA protobuf filters.
func (r *APITrayValidateAllRequest) ToFilters() []*rlav1.Filter {
	var filters []*rlav1.Filter
	if f := GetProtoTrayFilter("name", r.Name); f != nil {
		filters = append(filters, f)
	}
	if f := GetProtoTrayFilter("manufacturer", r.Manufacturer); f != nil {
		filters = append(filters, f)
	}
	if r.Type != nil {
		if f := GetProtoTrayFilter("type", []string{*r.Type}); f != nil {
			filters = append(filters, f)
		}
	}
	return filters
}

// QueryValues returns only the known query parameters as url.Values.
func (r *APITrayValidateAllRequest) QueryValues() url.Values {
	v := url.Values{}
	v.Set("siteId", r.SiteID)
	if r.RackID != nil {
		v.Set("rackId", *r.RackID)
	}
	if r.RackName != nil {
		v.Set("rackName", *r.RackName)
	}
	for _, n := range r.Name {
		v.Add("name", n)
	}
	for _, m := range r.Manufacturer {
		v.Add("manufacturer", m)
	}
	if r.Type != nil {
		v.Set("type", *r.Type)
	}
	for _, cid := range r.ComponentIDs {
		v.Add("componentId", cid)
	}
	return v
}

// APITrayPosition represents the position of a tray within a rack
type APITrayPosition struct {
	SlotID  int32 `json:"slotId"`
	TrayIdx int32 `json:"trayIdx"`
	HostID  int32 `json:"hostId"`
}

// FromProto converts a proto RackPosition to an APITrayPosition
func (atp *APITrayPosition) FromProto(protoPosition *rlav1.RackPosition) {
	if protoPosition == nil {
		return
	}
	atp.SlotID = protoPosition.GetSlotId()
	atp.TrayIdx = protoPosition.GetTrayIdx()
	atp.HostID = protoPosition.GetHostId()
}

// APITray is the API representation of a Tray (Component) from RLA
type APITray struct {
	ID              string           `json:"id"`
	ComponentID     string           `json:"componentId"`
	Type            string           `json:"type"`
	Name            string           `json:"name"`
	Manufacturer    string           `json:"manufacturer"`
	Model           string           `json:"model"`
	SerialNumber    string           `json:"serialNumber"`
	Description     string           `json:"description"`
	FirmwareVersion string           `json:"firmwareVersion"`
	PowerState      string           `json:"powerState"`
	Position        *APITrayPosition `json:"position"`
	BMCs            []*APIBMC        `json:"bmcs"`
	RackID          string           `json:"rackId"`
}

// FromProto converts an RLA protobuf Component to an APITray
func (at *APITray) FromProto(comp *rlav1.Component) {
	if comp == nil {
		return
	}

	at.Type = enumOr(ProtoToAPIComponentTypeName, comp.GetType(), "unknown")
	at.FirmwareVersion = comp.GetFirmwareVersion()
	at.PowerState = comp.GetPowerState()
	at.ComponentID = comp.GetComponentId()

	// Get info from DeviceInfo
	if comp.GetInfo() != nil {
		info := comp.GetInfo()
		if info.GetId() != nil {
			at.ID = info.GetId().GetId()
		}
		at.Name = info.GetName()
		at.Manufacturer = info.GetManufacturer()
		if info.Model != nil {
			at.Model = *info.Model
		}
		at.SerialNumber = info.GetSerialNumber()
		if info.Description != nil {
			at.Description = *info.Description
		}
	}

	// Get position
	if comp.GetPosition() != nil {
		at.Position = &APITrayPosition{}
		at.Position.FromProto(comp.GetPosition())
	}

	// Get BMCs
	if len(comp.GetBmcs()) > 0 {
		at.BMCs = make([]*APIBMC, 0, len(comp.GetBmcs()))
		for _, bmc := range comp.GetBmcs() {
			apiBMC := &APIBMC{}
			apiBMC.FromProto(bmc)
			at.BMCs = append(at.BMCs, apiBMC)
		}
	}

	// Get rack ID
	if comp.GetRackId() != nil {
		at.RackID = comp.GetRackId().GetId()
	}
}

// NewAPITray creates an APITray from the RLA protobuf Component
func NewAPITray(comp *rlav1.Component) *APITray {
	if comp == nil {
		return nil
	}
	apiTray := &APITray{}
	apiTray.FromProto(comp)
	return apiTray
}
