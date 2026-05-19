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
	"fmt"

	computils "github.com/NVIDIA/infra-controller-rest/site-agent/pkg/components/utils"
	"github.com/prometheus/client_golang/prometheus"
)

const (
	// MetricCoreGrpcStatus - Metric Core GRPC Status
	MetricCoreGrpcStatus = "carbide_health_status"
)

// Init - initialize carbide manager
func (coregrpc *API) Init() {
	ManagerAccess.Data.EB.Log.Info().Msg("Core gRPC: Initializing Core gRPC client")

	prometheus.MustRegister(
		prometheus.NewGaugeFunc(prometheus.GaugeOpts{
			Namespace: "elektra_site_agent",
			Name:      MetricCoreGrpcStatus,
			Help:      "Core gRPC health status",
		},
			func() float64 {
				return float64(ManagerAccess.Data.EB.Managers.CoreGrpc.State.HealthStatus.Load())
			}))
	ManagerAccess.Data.EB.Managers.CoreGrpc.State.HealthStatus.Store(uint64(computils.CompNotKnown))

	// initialize workflow metrics
	ManagerAccess.Data.EB.Managers.CoreGrpc.State.WflowMetrics = newWorkflowMetrics()
}

// Start - start carbide manager
func (coregrpc *API) Start() {
	ManagerAccess.Data.EB.Log.Info().Msg("Core gRPC: Starting the core gRPC client")

	// Create the client here
	// Each workflow will check and reinitialize the client if needed
	if err := coregrpc.CreateGrpcClient(); err != nil {
		ManagerAccess.Data.EB.Log.Error().Msgf("Core gRPC: failed to create gRPC client: %v", err)
	}
}

// GetState Machine
func (coregrpc *API) GetState() []string {
	state := ManagerAccess.Data.EB.Managers.CoreGrpc.State
	var strs []string
	strs = append(strs, fmt.Sprintln(" GRPC Succeeded:", state.GrpcSucc.Load()))
	strs = append(strs, fmt.Sprintln(" GRPC Failed:", state.GrpcFail.Load()))
	strs = append(strs, fmt.Sprintln(" GRPC Status:", computils.CompStatus(state.HealthStatus.Load())))
	strs = append(strs, fmt.Sprintln(" GRPC Last Error:", state.Err))

	return strs
}

// GetGrpcClientVersion returns the current version of the gRPC client
func (coregrpc *API) GetGrpcClientVersion() int64 {
	return ManagerAccess.Data.EB.Managers.CoreGrpc.Client.Version()
}
