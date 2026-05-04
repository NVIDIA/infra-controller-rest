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

package workflow

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"

	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/testsuite"

	rActivity "github.com/NVIDIA/infra-controller-rest/site-workflow/pkg/activity"
	rlav1 "github.com/NVIDIA/infra-controller-rest/workflow-schema/rla/protobuf/v1"
)

// GetRackTestSuite tests the GetRack workflow
type GetRackTestSuite struct {
	suite.Suite
	testsuite.WorkflowTestSuite

	env *testsuite.TestWorkflowEnvironment
}

func (s *GetRackTestSuite) SetupTest() {
	s.env = s.NewTestWorkflowEnvironment()
}

func (s *GetRackTestSuite) AfterTest(suiteName, testName string) {
	s.env.AssertExpectations(s.T())
}

func (s *GetRackTestSuite) Test_GetRack_Success() {
	var rackManager rActivity.ManageRack

	rackID := "test-rack-id"
	request := &rlav1.GetRackInfoByIDRequest{
		Id: &rlav1.UUID{Id: rackID},
	}

	expectedResponse := &rlav1.GetRackInfoResponse{
		Rack: &rlav1.Rack{
			Info: &rlav1.DeviceInfo{
				Id:   &rlav1.UUID{Id: rackID},
				Name: "test-rack",
			},
		},
	}

	// Mock GetRack activity
	s.env.RegisterActivity(rackManager.GetRack)
	s.env.OnActivity(rackManager.GetRack, mock.Anything, mock.Anything).Return(expectedResponse, nil)

	// Execute GetRack workflow
	s.env.ExecuteWorkflow(GetRack, request)
	s.True(s.env.IsWorkflowCompleted())
	s.NoError(s.env.GetWorkflowError())

	// Verify result
	var response rlav1.GetRackInfoResponse
	s.NoError(s.env.GetWorkflowResult(&response))
	s.NotNil(response.Rack)
	s.NotNil(response.Rack.Info)
	s.Equal(rackID, response.Rack.Info.Id.Id)
}

func (s *GetRackTestSuite) Test_GetRack_ActivityFails() {
	var rackManager rActivity.ManageRack

	rackID := "test-rack-id"
	request := &rlav1.GetRackInfoByIDRequest{
		Id: &rlav1.UUID{Id: rackID},
	}

	errMsg := "RLA connection failed"

	// Mock GetRack activity failure
	s.env.RegisterActivity(rackManager.GetRack)
	s.env.OnActivity(rackManager.GetRack, mock.Anything, mock.Anything).Return(nil, errors.New(errMsg))

	// Execute GetRack workflow
	s.env.ExecuteWorkflow(GetRack, request)
	s.True(s.env.IsWorkflowCompleted())
	err := s.env.GetWorkflowError()
	s.Error(err)

	var applicationErr *temporal.ApplicationError
	s.True(errors.As(err, &applicationErr))
	s.Equal(errMsg, applicationErr.Error())
}

func TestGetRackTestSuite(t *testing.T) {
	suite.Run(t, new(GetRackTestSuite))
}

// GetRacksTestSuite tests the GetRacks workflow
type GetRacksTestSuite struct {
	suite.Suite
	testsuite.WorkflowTestSuite

	env *testsuite.TestWorkflowEnvironment
}

func (s *GetRacksTestSuite) SetupTest() {
	s.env = s.NewTestWorkflowEnvironment()
}

func (s *GetRacksTestSuite) AfterTest(suiteName, testName string) {
	s.env.AssertExpectations(s.T())
}

func (s *GetRacksTestSuite) Test_GetRacks_Success() {
	var rackManager rActivity.ManageRack

	request := &rlav1.GetListOfRacksRequest{}

	expectedResponse := &rlav1.GetListOfRacksResponse{
		Racks: []*rlav1.Rack{
			{
				Info: &rlav1.DeviceInfo{
					Id:   &rlav1.UUID{Id: "rack-1"},
					Name: "Rack 1",
				},
			},
			{
				Info: &rlav1.DeviceInfo{
					Id:   &rlav1.UUID{Id: "rack-2"},
					Name: "Rack 2",
				},
			},
		},
		Total: 2,
	}

	// Mock GetRacks activity
	s.env.RegisterActivity(rackManager.GetRacks)
	s.env.OnActivity(rackManager.GetRacks, mock.Anything, mock.Anything).Return(expectedResponse, nil)

	// Execute GetRacks workflow
	s.env.ExecuteWorkflow(GetRacks, request)
	s.True(s.env.IsWorkflowCompleted())
	s.NoError(s.env.GetWorkflowError())

	// Verify result
	var response rlav1.GetListOfRacksResponse
	s.NoError(s.env.GetWorkflowResult(&response))
	s.NotNil(response.Racks)
	s.Equal(int32(2), response.Total)
	s.Equal(2, len(response.Racks))
}

func (s *GetRacksTestSuite) Test_GetRacks_ActivityFails() {
	var rackManager rActivity.ManageRack

	request := &rlav1.GetListOfRacksRequest{}

	errMsg := "RLA connection failed"

	// Mock GetRacks activity failure
	s.env.RegisterActivity(rackManager.GetRacks)
	s.env.OnActivity(rackManager.GetRacks, mock.Anything, mock.Anything).Return(nil, errors.New(errMsg))

	// Execute GetRacks workflow
	s.env.ExecuteWorkflow(GetRacks, request)
	s.True(s.env.IsWorkflowCompleted())
	err := s.env.GetWorkflowError()
	s.Error(err)

	var applicationErr *temporal.ApplicationError
	s.True(errors.As(err, &applicationErr))
	s.Equal(errMsg, applicationErr.Error())
}

func TestGetRacksTestSuite(t *testing.T) {
	suite.Run(t, new(GetRacksTestSuite))
}

// ValidateRackComponentsTestSuite tests the ValidateRackComponents workflow
type ValidateRackComponentsTestSuite struct {
	suite.Suite
	testsuite.WorkflowTestSuite

	env *testsuite.TestWorkflowEnvironment
}

func (s *ValidateRackComponentsTestSuite) SetupTest() {
	s.env = s.NewTestWorkflowEnvironment()
}

func (s *ValidateRackComponentsTestSuite) AfterTest(suiteName, testName string) {
	s.env.AssertExpectations(s.T())
}

func (s *ValidateRackComponentsTestSuite) Test_ValidateRackComponents_Success_NoDiffs() {
	var rackManager rActivity.ManageRack

	request := &rlav1.ValidateComponentsRequest{
		TargetSpec: &rlav1.OperationTargetSpec{
			Targets: &rlav1.OperationTargetSpec_Racks{
				Racks: &rlav1.RackTargets{
					Targets: []*rlav1.RackTarget{
						{
							Identifier: &rlav1.RackTarget_Id{
								Id: &rlav1.UUID{Id: "test-rack-id"},
							},
						},
					},
				},
			},
		},
	}

	expectedResponse := &rlav1.ValidateComponentsResponse{
		Diffs:           []*rlav1.ComponentDiff{},
		TotalDiffs:      0,
		MissingCount:    0,
		UnexpectedCount: 0,
		DriftCount:      0,
		MatchCount:      5,
	}

	// Mock ValidateRackComponents activity
	s.env.RegisterActivity(rackManager.ValidateRackComponents)
	s.env.OnActivity(rackManager.ValidateRackComponents, mock.Anything, mock.Anything).Return(expectedResponse, nil)

	// Execute ValidateRackComponents workflow
	s.env.ExecuteWorkflow(ValidateRackComponents, request)
	s.True(s.env.IsWorkflowCompleted())
	s.NoError(s.env.GetWorkflowError())

	// Verify result
	var response rlav1.ValidateComponentsResponse
	s.NoError(s.env.GetWorkflowResult(&response))
	s.Equal(int32(0), response.TotalDiffs)
	s.Equal(int32(5), response.MatchCount)
	s.Equal(0, len(response.Diffs))
}

func (s *ValidateRackComponentsTestSuite) Test_ValidateRackComponents_Success_WithDiffs() {
	var rackManager rActivity.ManageRack

	request := &rlav1.ValidateComponentsRequest{
		TargetSpec: &rlav1.OperationTargetSpec{
			Targets: &rlav1.OperationTargetSpec_Racks{
				Racks: &rlav1.RackTargets{
					Targets: []*rlav1.RackTarget{
						{
							Identifier: &rlav1.RackTarget_Id{
								Id: &rlav1.UUID{Id: "test-rack-id"},
							},
						},
					},
				},
			},
		},
	}

	expectedResponse := &rlav1.ValidateComponentsResponse{
		Diffs: []*rlav1.ComponentDiff{
			{
				Type:        rlav1.DiffType_DIFF_TYPE_MISSING,
				ComponentId: "comp-1",
			},
			{
				Type:        rlav1.DiffType_DIFF_TYPE_DRIFT,
				ComponentId: "comp-2",
				FieldDiffs: []*rlav1.FieldDiff{
					{
						FieldName:     "firmware_version",
						ExpectedValue: "1.0.0",
						ActualValue:   "2.0.0",
					},
				},
			},
		},
		TotalDiffs:      2,
		MissingCount:    1,
		UnexpectedCount: 0,
		DriftCount:      1,
		MatchCount:      3,
	}

	// Mock ValidateRackComponents activity
	s.env.RegisterActivity(rackManager.ValidateRackComponents)
	s.env.OnActivity(rackManager.ValidateRackComponents, mock.Anything, mock.Anything).Return(expectedResponse, nil)

	// Execute ValidateRackComponents workflow
	s.env.ExecuteWorkflow(ValidateRackComponents, request)
	s.True(s.env.IsWorkflowCompleted())
	s.NoError(s.env.GetWorkflowError())

	// Verify result
	var response rlav1.ValidateComponentsResponse
	s.NoError(s.env.GetWorkflowResult(&response))
	s.Equal(int32(2), response.TotalDiffs)
	s.Equal(int32(1), response.MissingCount)
	s.Equal(int32(1), response.DriftCount)
	s.Equal(int32(3), response.MatchCount)
	s.Equal(2, len(response.Diffs))
}

func (s *ValidateRackComponentsTestSuite) Test_ValidateRackComponents_ActivityFails() {
	var rackManager rActivity.ManageRack

	request := &rlav1.ValidateComponentsRequest{
		TargetSpec: &rlav1.OperationTargetSpec{
			Targets: &rlav1.OperationTargetSpec_Racks{
				Racks: &rlav1.RackTargets{
					Targets: []*rlav1.RackTarget{
						{
							Identifier: &rlav1.RackTarget_Id{
								Id: &rlav1.UUID{Id: "test-rack-id"},
							},
						},
					},
				},
			},
		},
	}

	errMsg := "RLA connection failed"

	// Mock ValidateRackComponents activity failure
	s.env.RegisterActivity(rackManager.ValidateRackComponents)
	s.env.OnActivity(rackManager.ValidateRackComponents, mock.Anything, mock.Anything).Return(nil, errors.New(errMsg))

	// Execute ValidateRackComponents workflow
	s.env.ExecuteWorkflow(ValidateRackComponents, request)
	s.True(s.env.IsWorkflowCompleted())
	err := s.env.GetWorkflowError()
	s.Error(err)

	var applicationErr *temporal.ApplicationError
	s.True(errors.As(err, &applicationErr))
	s.Equal(errMsg, applicationErr.Error())
}

func TestValidateRackComponentsTestSuite(t *testing.T) {
	suite.Run(t, new(ValidateRackComponentsTestSuite))
}

// PowerOnRackTestSuite tests the PowerOnRack workflow
type PowerOnRackTestSuite struct {
	suite.Suite
	testsuite.WorkflowTestSuite

	env *testsuite.TestWorkflowEnvironment
}

func (s *PowerOnRackTestSuite) SetupTest() {
	s.env = s.NewTestWorkflowEnvironment()
}

func (s *PowerOnRackTestSuite) AfterTest(suiteName, testName string) {
	s.env.AssertExpectations(s.T())
}

func (s *PowerOnRackTestSuite) Test_PowerOnRack_Success() {
	var rackManager rActivity.ManageRack

	request := &rlav1.PowerOnRackRequest{
		TargetSpec: &rlav1.OperationTargetSpec{
			Targets: &rlav1.OperationTargetSpec_Racks{
				Racks: &rlav1.RackTargets{
					Targets: []*rlav1.RackTarget{
						{
							Identifier: &rlav1.RackTarget_Id{
								Id: &rlav1.UUID{Id: "test-rack-id"},
							},
						},
					},
				},
			},
		},
		Description: "API power on Rack",
	}

	expectedResponse := &rlav1.SubmitTaskResponse{
		TaskIds: []*rlav1.UUID{{Id: "task-1"}},
	}

	s.env.RegisterActivity(rackManager.PowerOnRack)
	s.env.OnActivity(rackManager.PowerOnRack, mock.Anything, mock.Anything).Return(expectedResponse, nil)

	s.env.ExecuteWorkflow(PowerOnRack, request)
	s.True(s.env.IsWorkflowCompleted())
	s.NoError(s.env.GetWorkflowError())

	var response rlav1.SubmitTaskResponse
	s.NoError(s.env.GetWorkflowResult(&response))
	s.Equal(1, len(response.GetTaskIds()))
}

func (s *PowerOnRackTestSuite) Test_PowerOnRack_ActivityFails() {
	var rackManager rActivity.ManageRack

	request := &rlav1.PowerOnRackRequest{
		TargetSpec: &rlav1.OperationTargetSpec{
			Targets: &rlav1.OperationTargetSpec_Racks{
				Racks: &rlav1.RackTargets{
					Targets: []*rlav1.RackTarget{
						{
							Identifier: &rlav1.RackTarget_Id{
								Id: &rlav1.UUID{Id: "test-rack-id"},
							},
						},
					},
				},
			},
		},
	}

	errMsg := "RLA connection failed"

	s.env.RegisterActivity(rackManager.PowerOnRack)
	s.env.OnActivity(rackManager.PowerOnRack, mock.Anything, mock.Anything).Return(nil, errors.New(errMsg))

	s.env.ExecuteWorkflow(PowerOnRack, request)
	s.True(s.env.IsWorkflowCompleted())
	err := s.env.GetWorkflowError()
	s.Error(err)

	var applicationErr *temporal.ApplicationError
	s.True(errors.As(err, &applicationErr))
	s.Equal(errMsg, applicationErr.Error())
}

func TestPowerOnRackTestSuite(t *testing.T) {
	suite.Run(t, new(PowerOnRackTestSuite))
}

// PowerOffRackTestSuite tests the PowerOffRack workflow
type PowerOffRackTestSuite struct {
	suite.Suite
	testsuite.WorkflowTestSuite

	env *testsuite.TestWorkflowEnvironment
}

func (s *PowerOffRackTestSuite) SetupTest() {
	s.env = s.NewTestWorkflowEnvironment()
}

func (s *PowerOffRackTestSuite) AfterTest(suiteName, testName string) {
	s.env.AssertExpectations(s.T())
}

func (s *PowerOffRackTestSuite) Test_PowerOffRack_Success() {
	var rackManager rActivity.ManageRack

	request := &rlav1.PowerOffRackRequest{
		TargetSpec: &rlav1.OperationTargetSpec{
			Targets: &rlav1.OperationTargetSpec_Racks{
				Racks: &rlav1.RackTargets{
					Targets: []*rlav1.RackTarget{
						{
							Identifier: &rlav1.RackTarget_Id{
								Id: &rlav1.UUID{Id: "test-rack-id"},
							},
						},
					},
				},
			},
		},
		Description: "API power off Rack",
	}

	expectedResponse := &rlav1.SubmitTaskResponse{
		TaskIds: []*rlav1.UUID{{Id: "task-1"}},
	}

	s.env.RegisterActivity(rackManager.PowerOffRack)
	s.env.OnActivity(rackManager.PowerOffRack, mock.Anything, mock.Anything).Return(expectedResponse, nil)

	s.env.ExecuteWorkflow(PowerOffRack, request)
	s.True(s.env.IsWorkflowCompleted())
	s.NoError(s.env.GetWorkflowError())

	var response rlav1.SubmitTaskResponse
	s.NoError(s.env.GetWorkflowResult(&response))
	s.Equal(1, len(response.GetTaskIds()))
}

func (s *PowerOffRackTestSuite) Test_PowerOffRack_Forced() {
	var rackManager rActivity.ManageRack

	request := &rlav1.PowerOffRackRequest{
		TargetSpec: &rlav1.OperationTargetSpec{
			Targets: &rlav1.OperationTargetSpec_Racks{
				Racks: &rlav1.RackTargets{
					Targets: []*rlav1.RackTarget{
						{
							Identifier: &rlav1.RackTarget_Id{
								Id: &rlav1.UUID{Id: "test-rack-id"},
							},
						},
					},
				},
			},
		},
		Forced:      true,
		Description: "API force power off Rack",
	}

	expectedResponse := &rlav1.SubmitTaskResponse{
		TaskIds: []*rlav1.UUID{{Id: "task-1"}},
	}

	s.env.RegisterActivity(rackManager.PowerOffRack)
	s.env.OnActivity(rackManager.PowerOffRack, mock.Anything, mock.Anything).Return(expectedResponse, nil)

	s.env.ExecuteWorkflow(PowerOffRack, request)
	s.True(s.env.IsWorkflowCompleted())
	s.NoError(s.env.GetWorkflowError())

	var response rlav1.SubmitTaskResponse
	s.NoError(s.env.GetWorkflowResult(&response))
	s.Equal(1, len(response.GetTaskIds()))
}

func (s *PowerOffRackTestSuite) Test_PowerOffRack_ActivityFails() {
	var rackManager rActivity.ManageRack

	request := &rlav1.PowerOffRackRequest{
		TargetSpec: &rlav1.OperationTargetSpec{
			Targets: &rlav1.OperationTargetSpec_Racks{
				Racks: &rlav1.RackTargets{
					Targets: []*rlav1.RackTarget{
						{
							Identifier: &rlav1.RackTarget_Id{
								Id: &rlav1.UUID{Id: "test-rack-id"},
							},
						},
					},
				},
			},
		},
	}

	errMsg := "RLA connection failed"

	s.env.RegisterActivity(rackManager.PowerOffRack)
	s.env.OnActivity(rackManager.PowerOffRack, mock.Anything, mock.Anything).Return(nil, errors.New(errMsg))

	s.env.ExecuteWorkflow(PowerOffRack, request)
	s.True(s.env.IsWorkflowCompleted())
	err := s.env.GetWorkflowError()
	s.Error(err)

	var applicationErr *temporal.ApplicationError
	s.True(errors.As(err, &applicationErr))
	s.Equal(errMsg, applicationErr.Error())
}

func TestPowerOffRackTestSuite(t *testing.T) {
	suite.Run(t, new(PowerOffRackTestSuite))
}

// PowerResetRackTestSuite tests the PowerResetRack workflow
type PowerResetRackTestSuite struct {
	suite.Suite
	testsuite.WorkflowTestSuite

	env *testsuite.TestWorkflowEnvironment
}

func (s *PowerResetRackTestSuite) SetupTest() {
	s.env = s.NewTestWorkflowEnvironment()
}

func (s *PowerResetRackTestSuite) AfterTest(suiteName, testName string) {
	s.env.AssertExpectations(s.T())
}

func (s *PowerResetRackTestSuite) Test_PowerResetRack_Success() {
	var rackManager rActivity.ManageRack

	request := &rlav1.PowerResetRackRequest{
		TargetSpec: &rlav1.OperationTargetSpec{
			Targets: &rlav1.OperationTargetSpec_Racks{
				Racks: &rlav1.RackTargets{
					Targets: []*rlav1.RackTarget{
						{
							Identifier: &rlav1.RackTarget_Id{
								Id: &rlav1.UUID{Id: "test-rack-id"},
							},
						},
					},
				},
			},
		},
		Description: "API power cycle Rack",
	}

	expectedResponse := &rlav1.SubmitTaskResponse{
		TaskIds: []*rlav1.UUID{{Id: "task-1"}},
	}

	s.env.RegisterActivity(rackManager.PowerResetRack)
	s.env.OnActivity(rackManager.PowerResetRack, mock.Anything, mock.Anything).Return(expectedResponse, nil)

	s.env.ExecuteWorkflow(PowerResetRack, request)
	s.True(s.env.IsWorkflowCompleted())
	s.NoError(s.env.GetWorkflowError())

	var response rlav1.SubmitTaskResponse
	s.NoError(s.env.GetWorkflowResult(&response))
	s.Equal(1, len(response.GetTaskIds()))
}

func (s *PowerResetRackTestSuite) Test_PowerResetRack_Forced() {
	var rackManager rActivity.ManageRack

	request := &rlav1.PowerResetRackRequest{
		TargetSpec: &rlav1.OperationTargetSpec{
			Targets: &rlav1.OperationTargetSpec_Racks{
				Racks: &rlav1.RackTargets{
					Targets: []*rlav1.RackTarget{
						{
							Identifier: &rlav1.RackTarget_Id{
								Id: &rlav1.UUID{Id: "test-rack-id"},
							},
						},
					},
				},
			},
		},
		Forced:      true,
		Description: "API force power cycle Rack",
	}

	expectedResponse := &rlav1.SubmitTaskResponse{
		TaskIds: []*rlav1.UUID{{Id: "task-1"}, {Id: "task-2"}},
	}

	s.env.RegisterActivity(rackManager.PowerResetRack)
	s.env.OnActivity(rackManager.PowerResetRack, mock.Anything, mock.Anything).Return(expectedResponse, nil)

	s.env.ExecuteWorkflow(PowerResetRack, request)
	s.True(s.env.IsWorkflowCompleted())
	s.NoError(s.env.GetWorkflowError())

	var response rlav1.SubmitTaskResponse
	s.NoError(s.env.GetWorkflowResult(&response))
	s.Equal(2, len(response.GetTaskIds()))
}

func (s *PowerResetRackTestSuite) Test_PowerResetRack_ActivityFails() {
	var rackManager rActivity.ManageRack

	request := &rlav1.PowerResetRackRequest{
		TargetSpec: &rlav1.OperationTargetSpec{
			Targets: &rlav1.OperationTargetSpec_Racks{
				Racks: &rlav1.RackTargets{
					Targets: []*rlav1.RackTarget{
						{
							Identifier: &rlav1.RackTarget_Id{
								Id: &rlav1.UUID{Id: "test-rack-id"},
							},
						},
					},
				},
			},
		},
	}

	errMsg := "RLA connection failed"

	s.env.RegisterActivity(rackManager.PowerResetRack)
	s.env.OnActivity(rackManager.PowerResetRack, mock.Anything, mock.Anything).Return(nil, errors.New(errMsg))

	s.env.ExecuteWorkflow(PowerResetRack, request)
	s.True(s.env.IsWorkflowCompleted())
	err := s.env.GetWorkflowError()
	s.Error(err)

	var applicationErr *temporal.ApplicationError
	s.True(errors.As(err, &applicationErr))
	s.Equal(errMsg, applicationErr.Error())
}

func TestPowerResetRackTestSuite(t *testing.T) {
	suite.Run(t, new(PowerResetRackTestSuite))
}

// BringUpRackTestSuite tests the BringUpRack workflow
type BringUpRackTestSuite struct {
	suite.Suite
	testsuite.WorkflowTestSuite

	env *testsuite.TestWorkflowEnvironment
}

func (s *BringUpRackTestSuite) SetupTest() {
	s.env = s.NewTestWorkflowEnvironment()
}

func (s *BringUpRackTestSuite) AfterTest(suiteName, testName string) {
	s.env.AssertExpectations(s.T())
}

func (s *BringUpRackTestSuite) Test_BringUpRack_Success() {
	var rackManager rActivity.ManageRack

	request := &rlav1.BringUpRackRequest{
		TargetSpec: &rlav1.OperationTargetSpec{
			Targets: &rlav1.OperationTargetSpec_Racks{
				Racks: &rlav1.RackTargets{
					Targets: []*rlav1.RackTarget{
						{
							Identifier: &rlav1.RackTarget_Id{
								Id: &rlav1.UUID{Id: "test-rack-id"},
							},
						},
					},
				},
			},
		},
		Description: "API bring up Rack",
	}

	expectedResponse := &rlav1.SubmitTaskResponse{
		TaskIds: []*rlav1.UUID{{Id: "task-1"}},
	}

	s.env.RegisterActivity(rackManager.BringUpRack)
	s.env.OnActivity(rackManager.BringUpRack, mock.Anything, mock.Anything).Return(expectedResponse, nil)

	s.env.ExecuteWorkflow(BringUpRack, request)
	s.True(s.env.IsWorkflowCompleted())
	s.NoError(s.env.GetWorkflowError())

	var response rlav1.SubmitTaskResponse
	s.NoError(s.env.GetWorkflowResult(&response))
	s.Equal(1, len(response.GetTaskIds()))
}

func (s *BringUpRackTestSuite) Test_BringUpRack_ActivityFails() {
	var rackManager rActivity.ManageRack

	request := &rlav1.BringUpRackRequest{
		TargetSpec: &rlav1.OperationTargetSpec{
			Targets: &rlav1.OperationTargetSpec_Racks{
				Racks: &rlav1.RackTargets{
					Targets: []*rlav1.RackTarget{
						{
							Identifier: &rlav1.RackTarget_Id{
								Id: &rlav1.UUID{Id: "test-rack-id"},
							},
						},
					},
				},
			},
		},
	}

	errMsg := "RLA connection failed"

	s.env.RegisterActivity(rackManager.BringUpRack)
	s.env.OnActivity(rackManager.BringUpRack, mock.Anything, mock.Anything).Return(nil, errors.New(errMsg))

	s.env.ExecuteWorkflow(BringUpRack, request)
	s.True(s.env.IsWorkflowCompleted())
	err := s.env.GetWorkflowError()
	s.Error(err)

	var applicationErr *temporal.ApplicationError
	s.True(errors.As(err, &applicationErr))
	s.Equal(errMsg, applicationErr.Error())
}

func TestBringUpRackTestSuite(t *testing.T) {
	suite.Run(t, new(BringUpRackTestSuite))
}

// GetRackTaskTestSuite tests the GetRackTask workflow
type GetRackTaskTestSuite struct {
	suite.Suite
	testsuite.WorkflowTestSuite

	env *testsuite.TestWorkflowEnvironment
}

func (s *GetRackTaskTestSuite) SetupTest() {
	s.env = s.NewTestWorkflowEnvironment()
}

func (s *GetRackTaskTestSuite) AfterTest(suiteName, testName string) {
	s.env.AssertExpectations(s.T())
}

func (s *GetRackTaskTestSuite) Test_GetRackTask_Success() {
	var rackManager rActivity.ManageRack

	taskID := "test-task-id"
	request := &rlav1.GetTasksByIDsRequest{
		TaskIds: []*rlav1.UUID{{Id: taskID}},
	}

	expectedResponse := &rlav1.GetTasksByIDsResponse{
		Tasks: []*rlav1.Task{
			{
				Id:          &rlav1.UUID{Id: taskID},
				Operation:   "power_on",
				Description: "Power on rack",
				Status:      rlav1.TaskStatus_TASK_STATUS_RUNNING,
				Message:     "Processing",
			},
		},
	}

	s.env.RegisterActivity(rackManager.GetTaskByID)
	s.env.OnActivity(rackManager.GetTaskByID, mock.Anything, mock.Anything).Return(expectedResponse, nil)

	s.env.ExecuteWorkflow(GetRackTask, request)
	s.True(s.env.IsWorkflowCompleted())
	s.NoError(s.env.GetWorkflowError())

	var response rlav1.GetTasksByIDsResponse
	s.NoError(s.env.GetWorkflowResult(&response))
	s.Equal(1, len(response.GetTasks()))
	s.Equal(taskID, response.GetTasks()[0].GetId().GetId())
}

func (s *GetRackTaskTestSuite) Test_GetRackTask_EmptyResult() {
	var rackManager rActivity.ManageRack

	request := &rlav1.GetTasksByIDsRequest{
		TaskIds: []*rlav1.UUID{{Id: "nonexistent-task"}},
	}

	expectedResponse := &rlav1.GetTasksByIDsResponse{
		Tasks: []*rlav1.Task{},
	}

	s.env.RegisterActivity(rackManager.GetTaskByID)
	s.env.OnActivity(rackManager.GetTaskByID, mock.Anything, mock.Anything).Return(expectedResponse, nil)

	s.env.ExecuteWorkflow(GetRackTask, request)
	s.True(s.env.IsWorkflowCompleted())
	s.NoError(s.env.GetWorkflowError())

	var response rlav1.GetTasksByIDsResponse
	s.NoError(s.env.GetWorkflowResult(&response))
	s.Equal(0, len(response.GetTasks()))
}

func (s *GetRackTaskTestSuite) Test_GetRackTask_ActivityFails() {
	var rackManager rActivity.ManageRack

	request := &rlav1.GetTasksByIDsRequest{
		TaskIds: []*rlav1.UUID{{Id: "test-task-id"}},
	}

	errMsg := "RLA connection failed"

	s.env.RegisterActivity(rackManager.GetTaskByID)
	s.env.OnActivity(rackManager.GetTaskByID, mock.Anything, mock.Anything).Return(nil, errors.New(errMsg))

	s.env.ExecuteWorkflow(GetRackTask, request)
	s.True(s.env.IsWorkflowCompleted())
	err := s.env.GetWorkflowError()
	s.Error(err)

	var applicationErr *temporal.ApplicationError
	s.True(errors.As(err, &applicationErr))
	s.Equal(errMsg, applicationErr.Error())
}

func TestGetRackTaskTestSuite(t *testing.T) {
	suite.Run(t, new(GetRackTaskTestSuite))
}

// UpgradeFirmwareTestSuite tests the UpgradeFirmware workflow
type UpgradeFirmwareTestSuite struct {
	suite.Suite
	testsuite.WorkflowTestSuite

	env *testsuite.TestWorkflowEnvironment
}

func (s *UpgradeFirmwareTestSuite) SetupTest() {
	s.env = s.NewTestWorkflowEnvironment()
}

func (s *UpgradeFirmwareTestSuite) AfterTest(suiteName, testName string) {
	s.env.AssertExpectations(s.T())
}

func (s *UpgradeFirmwareTestSuite) Test_UpgradeFirmware_Success() {
	var rackManager rActivity.ManageRack

	request := &rlav1.UpgradeFirmwareRequest{
		TargetSpec: &rlav1.OperationTargetSpec{
			Targets: &rlav1.OperationTargetSpec_Racks{
				Racks: &rlav1.RackTargets{
					Targets: []*rlav1.RackTarget{
						{
							Identifier: &rlav1.RackTarget_Id{
								Id: &rlav1.UUID{Id: "test-rack-id"},
							},
						},
					},
				},
			},
		},
		Description: "API firmware upgrade Rack",
	}

	expectedResponse := &rlav1.SubmitTaskResponse{
		TaskIds: []*rlav1.UUID{{Id: "task-1"}},
	}

	s.env.RegisterActivity(rackManager.UpgradeFirmware)
	s.env.OnActivity(rackManager.UpgradeFirmware, mock.Anything, mock.Anything).Return(expectedResponse, nil)

	s.env.ExecuteWorkflow(UpgradeFirmware, request)
	s.True(s.env.IsWorkflowCompleted())
	s.NoError(s.env.GetWorkflowError())

	var response rlav1.SubmitTaskResponse
	s.NoError(s.env.GetWorkflowResult(&response))
	s.Equal(1, len(response.GetTaskIds()))
}

func (s *UpgradeFirmwareTestSuite) Test_UpgradeFirmware_WithVersion() {
	var rackManager rActivity.ManageRack

	version := "24.11.0"
	request := &rlav1.UpgradeFirmwareRequest{
		TargetSpec: &rlav1.OperationTargetSpec{
			Targets: &rlav1.OperationTargetSpec_Racks{
				Racks: &rlav1.RackTargets{
					Targets: []*rlav1.RackTarget{
						{
							Identifier: &rlav1.RackTarget_Id{
								Id: &rlav1.UUID{Id: "test-rack-id"},
							},
						},
					},
				},
			},
		},
		TargetVersion: &version,
		Description:   "API firmware upgrade Rack",
	}

	expectedResponse := &rlav1.SubmitTaskResponse{
		TaskIds: []*rlav1.UUID{{Id: "task-1"}},
	}

	s.env.RegisterActivity(rackManager.UpgradeFirmware)
	s.env.OnActivity(rackManager.UpgradeFirmware, mock.Anything, mock.Anything).Return(expectedResponse, nil)

	s.env.ExecuteWorkflow(UpgradeFirmware, request)
	s.True(s.env.IsWorkflowCompleted())
	s.NoError(s.env.GetWorkflowError())

	var response rlav1.SubmitTaskResponse
	s.NoError(s.env.GetWorkflowResult(&response))
	s.Equal(1, len(response.GetTaskIds()))
}

func (s *UpgradeFirmwareTestSuite) Test_UpgradeFirmware_ActivityFails() {
	var rackManager rActivity.ManageRack

	request := &rlav1.UpgradeFirmwareRequest{
		TargetSpec: &rlav1.OperationTargetSpec{
			Targets: &rlav1.OperationTargetSpec_Racks{
				Racks: &rlav1.RackTargets{
					Targets: []*rlav1.RackTarget{
						{
							Identifier: &rlav1.RackTarget_Id{
								Id: &rlav1.UUID{Id: "test-rack-id"},
							},
						},
					},
				},
			},
		},
	}

	errMsg := "RLA connection failed"

	s.env.RegisterActivity(rackManager.UpgradeFirmware)
	s.env.OnActivity(rackManager.UpgradeFirmware, mock.Anything, mock.Anything).Return(nil, errors.New(errMsg))

	s.env.ExecuteWorkflow(UpgradeFirmware, request)
	s.True(s.env.IsWorkflowCompleted())
	err := s.env.GetWorkflowError()
	s.Error(err)

	var applicationErr *temporal.ApplicationError
	s.True(errors.As(err, &applicationErr))
	s.Equal(errMsg, applicationErr.Error())
}

func TestUpgradeFirmwareTestSuite(t *testing.T) {
	suite.Run(t, new(UpgradeFirmwareTestSuite))
}
