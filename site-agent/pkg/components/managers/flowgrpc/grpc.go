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
	"sync"

	computils "github.com/NVIDIA/infra-controller-rest/site-agent/pkg/components/utils"
	"github.com/NVIDIA/infra-controller-rest/site-workflow/pkg/grpc/client"
	"github.com/gogo/status"
	"github.com/rs/zerolog/log"
	"google.golang.org/grpc/codes"
)

// checkCertsOnce is a local variable to ensure the go routine for checking if the certificate has changed only gets
// kicked off once even if creategRPC gets called multiple times
var checkCertsOnce sync.Once

func createGrpcClient() (conn *client.FlowGrpcClient, err error) {
	// Initialize contextual logger
	logger := log.With().Str("Method", "FlowClient.createRlaGrpcRPC").Logger()
	logger.Info().Msg("GRPC: Starting GRPC client")

	// Initialize the GRPC client configuration
	ManagerAccess.Data.EB.Managers.FlowGrpc.Client.Config = &client.FlowGrpcClientConfig{
		Address:        ManagerAccess.Conf.EB.FlowGrpc.Address,
		Secure:         ManagerAccess.Conf.EB.FlowGrpc.Secure,
		ServerCAPath:   ManagerAccess.Conf.EB.FlowGrpc.ServerCAPath,
		SkipServerAuth: ManagerAccess.Conf.EB.FlowGrpc.SkipServerAuth,
		ClientCertPath: ManagerAccess.Conf.EB.FlowGrpc.ClientCertPath,
		ClientKeyPath:  ManagerAccess.Conf.EB.FlowGrpc.ClientKeyPath,
		ClientMetrics:  makeGrpcClientMetrics(),
	}
	logger.Info().Interface("GRPCConfig", ManagerAccess.Data.EB.Managers.FlowGrpc.Client.Config).Msg("Initializing GRPC client")

	// Get initial certificate MD5 hashes
	initialClientMD5, initialServerMD5, err := ManagerAccess.Data.EB.Managers.FlowGrpc.Client.GetInitialCertMD5()
	if err != nil {
		logger.Error().Err(err).Msg("Failed to get initial certificate MD5 hashes")
		return nil, err
	}
	newClient, err := client.NewFlowGrpcClient(ManagerAccess.Data.EB.Managers.FlowGrpc.Client.Config)
	if err != nil {
		logger.Error().Err(err).Msg("Failed to initialize GRPC client")
		return nil, err
	}

	// Since this is initial creation, there's no old client to manage. SwapClient still used for consistency.
	_ = ManagerAccess.Data.EB.Managers.FlowGrpc.Client.SwapClient(newClient)
	logger.Info().Msg("Successfully initialized GRPC client")

	// Start the certificate check and reload routine in a background goroutine
	checkCertsOnce.Do(func() {
		go ManagerAccess.Data.EB.Managers.FlowGrpc.Client.CheckAndReloadCerts(initialClientMD5, initialServerMD5)
		logger.Info().Msg("Started certificate reload routine")
	})

	return ManagerAccess.Data.EB.Managers.FlowGrpc.GetClient(), nil
}

// CreateGrpcClient - creates the grpc connection handle
func (flowgrpc *API) CreateGrpcClient() error {
	// Initialize the GRPC client
	// We can handle advanced features later
	_, err := createGrpcClient()
	if err != nil {
		ManagerAccess.Data.EB.Managers.FlowGrpc.State.HealthStatus.Store(uint64(computils.CompUnhealthy))
	} else {
		ManagerAccess.Data.EB.Managers.FlowGrpc.State.HealthStatus.Store(uint64(computils.CompNotKnown))
	}

	return err
}

// GetGRPCClient - gets the grpc connection handle
func (flowgrpc *API) GetGrpcClient() *client.FlowGrpcClient {
	return ManagerAccess.Data.EB.Managers.FlowGrpc.GetClient()
}

// isGrpcUp Is grpc connection functional
func isGrpcUp(c codes.Code) bool {
	switch c {
	case codes.Unavailable, codes.Unauthenticated:
		return false
	}
	return true
}

// UpdateGrpcClientState - updates Flow state
func (flowgrpc *API) UpdateGrpcClientState(err error) {
	defer computils.UpdateState(ManagerAccess.Data.EB)
	if err == nil {
		ManagerAccess.Data.EB.Managers.FlowGrpc.State.GrpcSucc.Inc()
		ManagerAccess.Data.EB.Managers.FlowGrpc.State.HealthStatus.Store(uint64(computils.CompHealthy))
		return
	}
	ManagerAccess.Data.EB.Managers.FlowGrpc.State.GrpcFail.Inc()
	ManagerAccess.Data.EB.Managers.FlowGrpc.State.Err = err.Error()
	log.Error().Err(err).Msg("GRPC: Failed to send request to GRPC server")
	st, ok := status.FromError(err)
	if ok {
		if !isGrpcUp(st.Code()) {
			ManagerAccess.Data.EB.Managers.FlowGrpc.State.HealthStatus.Store(uint64(computils.CompUnhealthy))
			log.Error().Err(err).Msg("GRPC: connection down")
		} else {
			log.Info().Msgf("GRPC application error %v", st.Code())
		}
	}
}
