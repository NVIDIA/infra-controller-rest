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

package flowgrpc

import (
	"context"

	"github.com/NVIDIA/infra-controller-rest/site-workflow/pkg/grpc/client"

	"go.temporal.io/sdk/activity"
	"go.temporal.io/sdk/log"
)

// CreateGrpcClientActivity is an activity to create a Flow gRPC client
func (flowgrpc *API) CreateGrpcClientActivity(ctx context.Context, ResourceID string) (client *client.FlowGrpcClient, err error) {
	// Use temporal logger for temporal logs
	logger := activity.GetLogger(ctx)

	slogger := log.With(logger, "Activity", "CreateGrpcClientActivity", "ResourceID", ResourceID)
	slogger.Info("Flow: Starting the gRPC connection Activity")

	// Create the client
	slogger.Info("Flow: Creating gRPC client")

	err = flowgrpc.CreateGrpcClient()
	if err != nil {
		return nil, err
	}
	return flowgrpc.GetGrpcClient(), nil
}

// RegisterGrpc - Register gRPC client activity
func (flowgrpc *API) RegisterGrpc() {
	// Register activity
	activityRegisterOptions := activity.RegisterOptions{
		Name: "CreateRlaGrpcClientActivity",
	}

	ManagerAccess.Data.EB.Managers.Workflow.Temporal.Worker.RegisterActivityWithOptions(
		flowgrpc.CreateGrpcClientActivity, activityRegisterOptions,
	)
	ManagerAccess.Data.EB.Log.Info().Msg("Flow gRPC: successfully registered gRPC client activity")
}
