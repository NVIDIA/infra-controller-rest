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

	"github.com/rs/zerolog/log"

	swe "github.com/NVIDIA/infra-controller-rest/site-workflow/pkg/error"
	cClient "github.com/NVIDIA/infra-controller-rest/site-workflow/pkg/grpc/client"
	rlav1 "github.com/NVIDIA/infra-controller-rest/workflow-schema/rla/protobuf/v1"
	"go.temporal.io/sdk/temporal"
)

// ManageRack is an activity wrapper for Rack management via RLA
type ManageRack struct {
	RlaAtomicClient *cClient.RlaAtomicClient
}

// NewManageRack returns a new ManageRack client
func NewManageRack(rlaClient *cClient.RlaAtomicClient) ManageRack {
	return ManageRack{
		RlaAtomicClient: rlaClient,
	}
}

// GetRack retrieves a rack by its UUID from RLA
func (mr *ManageRack) GetRack(ctx context.Context, request *rlav1.GetRackInfoByIDRequest) (*rlav1.GetRackInfoResponse, error) {
	logger := log.With().Str("Activity", "GetRack").Logger()
	logger.Info().Msg("Starting activity")

	var err error

	// Validate request
	switch {
	case request == nil:
		err = errors.New("received empty get rack request")
	case request.Id == nil || request.Id.Id == "":
		err = errors.New("received get rack request without rack ID")
	}

	if err != nil {
		return nil, temporal.NewNonRetryableApplicationError(err.Error(), swe.ErrTypeInvalidRequest, err)
	}

	// Call RLA gRPC endpoint
	rla, err := mr.RlaAtomicClient.GetRLAClient()
	if err != nil {
		return nil, err
	}

	response, err := rla.GetRackInfoByID(ctx, request)
	if err != nil {
		logger.Warn().Err(err).Msg("Failed to get rack by ID using RLA API")
		return nil, swe.WrapErr(err)
	}

	logger.Info().Msg("Completed activity")

	return response, nil
}

// GetRacks retrieves a list of racks from RLA with optional filters
func (mr *ManageRack) GetRacks(ctx context.Context, request *rlav1.GetListOfRacksRequest) (*rlav1.GetListOfRacksResponse, error) {
	logger := log.With().Str("Activity", "GetRacks").Logger()
	logger.Info().Msg("Starting activity")

	// Request can be nil or empty for getting all racks
	if request == nil {
		request = &rlav1.GetListOfRacksRequest{}
	}

	// Call RLA gRPC endpoint
	rla, err := mr.RlaAtomicClient.GetRLAClient()
	if err != nil {
		return nil, err
	}

	response, err := rla.GetListOfRacks(ctx, request)
	if err != nil {
		logger.Warn().Err(err).Msg("Failed to get list of racks using RLA API")
		return nil, swe.WrapErr(err)
	}

	logger.Info().Int32("Total", response.GetTotal()).Msg("Completed activity")

	return response, nil
}

// ValidateRackComponents validates rack components by comparing expected vs actual state via RLA.
// Supports validating a single rack, multiple racks with filters, or all racks in a site.
func (mr *ManageRack) ValidateRackComponents(ctx context.Context, request *rlav1.ValidateComponentsRequest) (*rlav1.ValidateComponentsResponse, error) {
	logger := log.With().Str("Activity", "ValidateRackComponents").Logger()
	logger.Info().Msg("Starting activity")

	var err error

	// Validate request
	switch {
	case request == nil:
		err = errors.New("received empty validate rack components request")
	}

	if err != nil {
		return nil, temporal.NewNonRetryableApplicationError(err.Error(), swe.ErrTypeInvalidRequest, err)
	}

	// Call RLA gRPC endpoint
	rla, err := mr.RlaAtomicClient.GetRLAClient()
	if err != nil {
		return nil, err
	}

	response, err := rla.ValidateComponents(ctx, request)
	if err != nil {
		logger.Warn().Err(err).Msg("Failed to validate rack components using RLA API")
		return nil, swe.WrapErr(err)
	}

	logger.Info().Int32("TotalDiffs", response.GetTotalDiffs()).Msg("Completed activity")

	return response, nil
}

// PowerOnRack powers on a rack or its specified components via RLA
func (mr *ManageRack) PowerOnRack(ctx context.Context, request *rlav1.PowerOnRackRequest) (*rlav1.SubmitTaskResponse, error) {
	logger := log.With().Str("Activity", "PowerOnRack").Logger()
	logger.Info().Msg("Starting activity")

	var err error

	// Validate request
	switch {
	case request == nil:
		err = errors.New("received empty power on rack request")
	}

	if err != nil {
		return nil, temporal.NewNonRetryableApplicationError(err.Error(), swe.ErrTypeInvalidRequest, err)
	}

	// Call RLA gRPC endpoint
	rla, err := mr.RlaAtomicClient.GetRLAClient()
	if err != nil {
		return nil, err
	}

	response, err := rla.PowerOnRack(ctx, request)
	if err != nil {
		logger.Warn().Err(err).Msg("Failed to power on rack using RLA API")
		return nil, swe.WrapErr(err)
	}

	logger.Info().Int("TaskCount", len(response.GetTaskIds())).Msg("Completed activity")

	return response, nil
}

// PowerOffRack powers off a rack or its specified components via RLA
func (mr *ManageRack) PowerOffRack(ctx context.Context, request *rlav1.PowerOffRackRequest) (*rlav1.SubmitTaskResponse, error) {
	logger := log.With().Str("Activity", "PowerOffRack").Logger()
	logger.Info().Msg("Starting activity")

	var err error

	// Validate request
	switch {
	case request == nil:
		err = errors.New("received empty power off rack request")
	}

	if err != nil {
		return nil, temporal.NewNonRetryableApplicationError(err.Error(), swe.ErrTypeInvalidRequest, err)
	}

	// Call RLA gRPC endpoint
	rla, err := mr.RlaAtomicClient.GetRLAClient()
	if err != nil {
		return nil, err
	}

	response, err := rla.PowerOffRack(ctx, request)
	if err != nil {
		logger.Warn().Err(err).Msg("Failed to power off rack using RLA API")
		return nil, swe.WrapErr(err)
	}

	logger.Info().Int("TaskCount", len(response.GetTaskIds())).Msg("Completed activity")

	return response, nil
}

// PowerResetRack resets (power cycles) a rack or its specified components via RLA
func (mr *ManageRack) PowerResetRack(ctx context.Context, request *rlav1.PowerResetRackRequest) (*rlav1.SubmitTaskResponse, error) {
	logger := log.With().Str("Activity", "PowerResetRack").Logger()
	logger.Info().Msg("Starting activity")

	var err error

	// Validate request
	switch {
	case request == nil:
		err = errors.New("received empty power reset rack request")
	}

	if err != nil {
		return nil, temporal.NewNonRetryableApplicationError(err.Error(), swe.ErrTypeInvalidRequest, err)
	}

	// Call RLA gRPC endpoint
	rla, err := mr.RlaAtomicClient.GetRLAClient()
	if err != nil {
		return nil, err
	}

	response, err := rla.PowerResetRack(ctx, request)
	if err != nil {
		logger.Warn().Err(err).Msg("Failed to power reset rack using RLA API")
		return nil, swe.WrapErr(err)
	}

	logger.Info().Int("TaskCount", len(response.GetTaskIds())).Msg("Completed activity")

	return response, nil
}

// BringUpRack brings up a rack or its specified components via RLA
func (mr *ManageRack) BringUpRack(ctx context.Context, request *rlav1.BringUpRackRequest) (*rlav1.SubmitTaskResponse, error) {
	logger := log.With().Str("Activity", "BringUpRack").Logger()
	logger.Info().Msg("Starting activity")

	var err error

	switch {
	case request == nil:
		err = errors.New("received empty bring up rack request")
	}

	if err != nil {
		return nil, temporal.NewNonRetryableApplicationError(err.Error(), swe.ErrTypeInvalidRequest, err)
	}

	rla, err := mr.RlaAtomicClient.GetRLAClient()
	if err != nil {
		return nil, err
	}

	response, err := rla.BringUpRack(ctx, request)
	if err != nil {
		logger.Warn().Err(err).Msg("Failed to bring up rack using RLA API")
		return nil, swe.WrapErr(err)
	}

	logger.Info().Int("TaskCount", len(response.GetTaskIds())).Msg("Completed activity")

	return response, nil
}

// GetTaskByID retrieves a task by its UUID from RLA
func (mr *ManageRack) GetTaskByID(ctx context.Context, request *rlav1.GetTasksByIDsRequest) (*rlav1.GetTasksByIDsResponse, error) {
	logger := log.With().Str("Activity", "GetTaskByID").Logger()
	logger.Info().Msg("Starting activity")

	var err error

	switch {
	case request == nil:
		err = errors.New("received empty get task request")
	case len(request.GetTaskIds()) == 0:
		err = errors.New("received get task request without task IDs")
	}

	if err != nil {
		return nil, temporal.NewNonRetryableApplicationError(err.Error(), swe.ErrTypeInvalidRequest, err)
	}

	rla, err := mr.RlaAtomicClient.GetRLAClient()
	if err != nil {
		return nil, err
	}

	response, err := rla.GetTasksByIDs(ctx, request)
	if err != nil {
		logger.Warn().Err(err).Msg("Failed to get task by ID using RLA API")
		return nil, swe.WrapErr(err)
	}

	logger.Info().Int("TaskCount", len(response.GetTasks())).Msg("Completed activity")

	return response, nil
}

// UpgradeFirmware upgrades firmware on racks or components via RLA
func (mr *ManageRack) UpgradeFirmware(ctx context.Context, request *rlav1.UpgradeFirmwareRequest) (*rlav1.SubmitTaskResponse, error) {
	logger := log.With().Str("Activity", "UpgradeFirmware").Logger()
	logger.Info().Msg("Starting activity")

	var err error

	switch {
	case request == nil:
		err = errors.New("received empty upgrade firmware request")
	}

	if err != nil {
		return nil, temporal.NewNonRetryableApplicationError(err.Error(), swe.ErrTypeInvalidRequest, err)
	}

	rla, err := mr.RlaAtomicClient.GetRLAClient()
	if err != nil {
		return nil, err
	}

	response, err := rla.UpgradeFirmware(ctx, request)
	if err != nil {
		logger.Warn().Err(err).Msg("Failed to upgrade firmware using RLA API")
		return nil, swe.WrapErr(err)
	}

	logger.Info().Int("TaskCount", len(response.GetTaskIds())).Msg("Completed activity")

	return response, nil
}
