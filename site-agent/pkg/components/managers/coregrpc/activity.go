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

package coregrpc

import (
	"context"

	"github.com/NVIDIA/infra-controller-rest/site-workflow/pkg/grpc/client"

	"go.temporal.io/sdk/activity"
	"go.temporal.io/sdk/log"
)

// CreateGRPCClientActivity - Create GRPC client Activity
func (coregrpc *API) CreateGrpcClientActivity(ctx context.Context, ResourceID string) (client *client.CoreGrpcClient, err error) {
	// Create the VPC
	ManagerAccess.Data.EB.Log.Info().Interface("Request", ResourceID).Msg("Core gRPC: Starting  the gRPC connection Activity")

	// Use temporal logger for temporal logs
	logger := activity.GetLogger(ctx)
	withLogger := log.With(logger, "Activity", "CreateGrpcClientActivity", "ResourceReq", ResourceID)
	withLogger.Info("Core gRPC: Starting gRPC connection Activity")

	// Create the client
	ManagerAccess.Data.EB.Log.Info().Interface("Request", ResourceID).Msg("Core gRPC: Creating gRPC client")

	err = coregrpc.CreateGrpcClient()
	if err != nil {
		return nil, err
	}
	return coregrpc.GetGrpcClient(), nil
}

// RegisterGRPC - Register GRPC
func (coregrpc *API) RegisterGrpc() {
	// Register activity
	activityRegisterOptions := activity.RegisterOptions{
		Name: "CreateGrpcClientActivity",
	}

	ManagerAccess.Data.EB.Managers.Workflow.Temporal.Worker.RegisterActivityWithOptions(
		ManagerAccess.API.CoreGrpc.CreateGrpcClientActivity, activityRegisterOptions,
	)
	ManagerAccess.Data.EB.Log.Info().Msg("Core gRPC: successfully registered gRPC client activity")
}
