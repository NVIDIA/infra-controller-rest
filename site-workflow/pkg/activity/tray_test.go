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
	"testing"

	cClient "github.com/NVIDIA/ncx-infra-controller-rest/site-workflow/pkg/grpc/client"
	rlav1 "github.com/NVIDIA/ncx-infra-controller-rest/workflow-schema/rla/protobuf/v1"
	"github.com/stretchr/testify/assert"
)

func TestManageTray_GetTray(t *testing.T) {
	tests := []struct {
		name        string
		request     *rlav1.GetComponentInfoByIDRequest
		mockResp    *rlav1.GetComponentInfoResponse
		mockErr     error
		wantErr     bool
		errContains string
	}{
		{
			name:        "nil request returns error",
			request:     nil,
			mockResp:    nil,
			mockErr:     nil,
			wantErr:     true,
			errContains: "empty get tray request",
		},
		{
			name: "request with nil ID returns error",
			request: &rlav1.GetComponentInfoByIDRequest{
				Id: nil,
			},
			mockResp:    nil,
			mockErr:     nil,
			wantErr:     true,
			errContains: "missing tray ID",
		},
		{
			name: "request with empty ID returns error",
			request: &rlav1.GetComponentInfoByIDRequest{
				Id: &rlav1.UUID{Id: ""},
			},
			mockResp:    nil,
			mockErr:     nil,
			wantErr:     true,
			errContains: "missing tray ID",
		},
		{
			name: "successful request - compute tray",
			request: &rlav1.GetComponentInfoByIDRequest{
				Id: &rlav1.UUID{Id: "test-tray-id"},
			},
			mockResp: &rlav1.GetComponentInfoResponse{
				Component: &rlav1.Component{
					Type: rlav1.ComponentType_COMPONENT_TYPE_COMPUTE,
					Info: &rlav1.DeviceInfo{
						Id:           &rlav1.UUID{Id: "test-tray-id"},
						Name:         "Test Compute Tray",
						Manufacturer: "NVIDIA",
						SerialNumber: "TSN001",
					},
					FirmwareVersion: "2.0.0",
					ComponentId:     "nico-machine-123",
					Position: &rlav1.RackPosition{
						SlotId:  1,
						TrayIdx: 0,
						HostId:  1,
					},
				},
			},
			mockErr: nil,
			wantErr: false,
		},
		{
			name: "successful request - switch tray",
			request: &rlav1.GetComponentInfoByIDRequest{
				Id: &rlav1.UUID{Id: "switch-tray-id"},
			},
			mockResp: &rlav1.GetComponentInfoResponse{
				Component: &rlav1.Component{
					Type: rlav1.ComponentType_COMPONENT_TYPE_NVLSWITCH,
					Info: &rlav1.DeviceInfo{
						Id:   &rlav1.UUID{Id: "switch-tray-id"},
						Name: "NVSwitch Tray",
					},
				},
			},
			mockErr: nil,
			wantErr: false,
		},
		{
			name: "RLA client error",
			request: &rlav1.GetComponentInfoByIDRequest{
				Id: &rlav1.UUID{Id: "test-tray-id"},
			},
			mockResp:    nil,
			mockErr:     errors.New("connection refused"),
			wantErr:     true,
			errContains: "connection refused",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create mock RLA client
			mockRlaClient := cClient.NewMockRlaClient()

			// Create atomic client and swap with mock
			rlaAtomicClient := cClient.NewRlaAtomicClient(&cClient.RlaClientConfig{})
			rlaAtomicClient.SwapClient(mockRlaClient)

			// Create ManageTray instance
			manageTray := NewManageTray(rlaAtomicClient)

			// Execute activity with context injection
			ctx := context.Background()
			if tt.mockErr != nil {
				ctx = context.WithValue(ctx, "wantError", tt.mockErr)
			}
			if tt.mockResp != nil {
				ctx = context.WithValue(ctx, "wantResponse", tt.mockResp)
			}
			result, err := manageTray.GetTray(ctx, tt.request)

			if tt.wantErr {
				assert.Error(t, err)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
				return
			}

			assert.NoError(t, err)
			assert.NotNil(t, result)
			assert.Equal(t, tt.mockResp.GetComponent().GetInfo().GetId().GetId(), result.GetComponent().GetInfo().GetId().GetId())
		})
	}
}

func TestManageTray_GetTrays(t *testing.T) {
	tests := []struct {
		name        string
		request     *rlav1.GetComponentsRequest
		mockResp    *rlav1.GetComponentsResponse
		mockErr     error
		wantErr     bool
		errContains string
	}{
		{
			name:    "successful request - nil request (gets all trays)",
			request: nil,
			mockResp: &rlav1.GetComponentsResponse{
				Components: []*rlav1.Component{},
				Total:      0,
			},
			mockErr: nil,
			wantErr: false,
		},
		{
			name:    "successful request - empty request",
			request: &rlav1.GetComponentsRequest{},
			mockResp: &rlav1.GetComponentsResponse{
				Components: []*rlav1.Component{},
				Total:      0,
			},
			mockErr: nil,
			wantErr: false,
		},
		{
			name:    "successful request - multiple trays",
			request: &rlav1.GetComponentsRequest{},
			mockResp: &rlav1.GetComponentsResponse{
				Components: []*rlav1.Component{
					{
						Type: rlav1.ComponentType_COMPONENT_TYPE_COMPUTE,
						Info: &rlav1.DeviceInfo{
							Id:   &rlav1.UUID{Id: "tray-1"},
							Name: "Compute Tray 1",
						},
						FirmwareVersion: "1.0.0",
						Position: &rlav1.RackPosition{
							SlotId: 1,
						},
					},
					{
						Type: rlav1.ComponentType_COMPONENT_TYPE_NVLSWITCH,
						Info: &rlav1.DeviceInfo{
							Id:   &rlav1.UUID{Id: "tray-2"},
							Name: "Switch Tray 1",
						},
						Position: &rlav1.RackPosition{
							SlotId: 24,
						},
					},
					{
						Type: rlav1.ComponentType_COMPONENT_TYPE_POWERSHELF,
						Info: &rlav1.DeviceInfo{
							Id:   &rlav1.UUID{Id: "tray-3"},
							Name: "Power Shelf 1",
						},
						Position: &rlav1.RackPosition{
							SlotId: 48,
						},
					},
				},
				Total: 3,
			},
			mockErr: nil,
			wantErr: false,
		},
		{
			name: "successful request - with target spec filter",
			request: &rlav1.GetComponentsRequest{
				TargetSpec: &rlav1.OperationTargetSpec{
					Targets: &rlav1.OperationTargetSpec_Racks{
						Racks: &rlav1.RackTargets{
							Targets: []*rlav1.RackTarget{
								{
									Identifier: &rlav1.RackTarget_Id{
										Id: &rlav1.UUID{Id: "rack-123"},
									},
									ComponentTypes: []rlav1.ComponentType{
										rlav1.ComponentType_COMPONENT_TYPE_COMPUTE,
									},
								},
							},
						},
					},
				},
			},
			mockResp: &rlav1.GetComponentsResponse{
				Components: []*rlav1.Component{
					{
						Type: rlav1.ComponentType_COMPONENT_TYPE_COMPUTE,
						Info: &rlav1.DeviceInfo{
							Id:   &rlav1.UUID{Id: "compute-tray-1"},
							Name: "Compute Tray",
						},
						RackId: &rlav1.UUID{Id: "rack-123"},
					},
				},
				Total: 1,
			},
			mockErr: nil,
			wantErr: false,
		},
		{
			name:        "RLA client error",
			request:     &rlav1.GetComponentsRequest{},
			mockResp:    nil,
			mockErr:     errors.New("internal server error"),
			wantErr:     true,
			errContains: "internal server error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create mock RLA client
			mockRlaClient := cClient.NewMockRlaClient()

			// Create atomic client and swap with mock
			rlaAtomicClient := cClient.NewRlaAtomicClient(&cClient.RlaClientConfig{})
			rlaAtomicClient.SwapClient(mockRlaClient)

			// Create ManageTray instance
			manageTray := NewManageTray(rlaAtomicClient)

			// Execute activity with context injection
			ctx := context.Background()
			if tt.mockErr != nil {
				ctx = context.WithValue(ctx, "wantError", tt.mockErr)
			}
			if tt.mockResp != nil {
				ctx = context.WithValue(ctx, "wantResponse", tt.mockResp)
			}
			result, err := manageTray.GetTrays(ctx, tt.request)

			if tt.wantErr {
				assert.Error(t, err)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
				return
			}

			assert.NoError(t, err)
			assert.NotNil(t, result)
			assert.Equal(t, tt.mockResp.GetTotal(), result.GetTotal())
			assert.Equal(t, len(tt.mockResp.GetComponents()), len(result.GetComponents()))
		})
	}
}

func TestNewManageTray(t *testing.T) {
	// Create a mock RLA client
	rlaAtomicClient := cClient.NewRlaAtomicClient(&cClient.RlaClientConfig{})

	// Test constructor
	manageTray := NewManageTray(rlaAtomicClient)

	assert.NotNil(t, manageTray.RlaAtomicClient)
	assert.Equal(t, rlaAtomicClient, manageTray.RlaAtomicClient)
}
