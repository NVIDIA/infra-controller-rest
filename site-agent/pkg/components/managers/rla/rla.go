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

package rla

import (
	"fmt"

	computils "github.com/NVIDIA/infra-controller-rest/site-agent/pkg/components/utils"
	"github.com/prometheus/client_golang/prometheus"
)

const (
	// MetricRLAStatus - Metric RLA Status
	MetricRLAStatus = "rla_health_status"
)

// Init - initialize RLA manager
func (rla *API) Init() {
	// Check if RLA is enabled via environment variable
	if !ManagerAccess.Conf.EB.RLA.Enabled {
		ManagerAccess.Data.EB.Log.Info().Msg("RLA: RLA is disabled, skipping initialization")
		return
	}

	ManagerAccess.Data.EB.Log.Info().Msg("RLA: Initializing RLA manager")

	prometheus.MustRegister(
		prometheus.NewGaugeFunc(prometheus.GaugeOpts{
			Namespace: "elektra_site_agent",
			Name:      MetricRLAStatus,
			Help:      "RLA gRPC health status",
		},
			func() float64 {
				return float64(ManagerAccess.Data.EB.Managers.RLA.State.HealthStatus.Load())
			}))
	ManagerAccess.Data.EB.Managers.RLA.State.HealthStatus.Store(uint64(computils.CompNotKnown))

	// initialize workflow metrics
	ManagerAccess.Data.EB.Managers.RLA.State.WflowMetrics = newWorkflowMetrics()
}

// Start - Start RLA manager
func (rla *API) Start() {
	ManagerAccess.Data.EB.Log.Info().Msg("RLA: Starting RLA manager")

	// Check if RLA is enabled via environment variable
	if !ManagerAccess.Conf.EB.RLA.Enabled {
		ManagerAccess.Data.EB.Log.Info().Msg("RLA: RLA is disabled, skipping gRPC client initialization")
		return
	}

	// Create the client here
	// Each workflow will check and reinitialize the client if needed
	if err := rla.CreateGRPCClient(); err != nil {
		ManagerAccess.Data.EB.Log.Error().Msgf("RLA: failed to create GRPC client: %v", err)
	}
}

// GetState Machine
func (rla *API) GetState() []string {
	state := ManagerAccess.Data.EB.Managers.RLA.State
	var strs []string
	strs = append(strs, fmt.Sprintln(" GRPC Succeeded:", state.GrpcSucc.Load()))
	strs = append(strs, fmt.Sprintln(" GRPC Failed:", state.GrpcFail.Load()))
	strs = append(strs, fmt.Sprintln(" RLA Status:", computils.CompStatus(state.HealthStatus.Load())))
	strs = append(strs, fmt.Sprintln(" RLA Last Error:", state.Err))

	return strs
}

// GetGrpcClientVersion returns the current version of the GRPC client
func (rla *API) GetGRPCClientVersion() int64 {
	return ManagerAccess.Data.EB.Managers.RLA.Client.Version()
}
