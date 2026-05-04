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

package client

import (
	"context"
	"crypto/md5"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"os"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/rs/zerolog/log"

	grpcmw "github.com/grpc-ecosystem/go-grpc-middleware"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"

	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"go.opentelemetry.io/otel"

	rlav1 "github.com/NVIDIA/infra-controller-rest/workflow-schema/rla/protobuf/v1"
)

// Errors
var (
	ErrRlaClientInvalidAddress    = errors.New("RlaClient: invalid address")
	ErrRlaClientInvalidDialOpts   = errors.New("RlaClient: invalid dial options")
	ErrRlaClientInvalidSecureOpts = errors.New("RlaClient: invalid secure options")
	ErrRlaClientInvalidServerCA   = errors.New("RlaClient: invalid server CA")
	ErrRlaClientInvalidClientCA   = errors.New("RlaClient: invalid client CA")
	ErrRlaClientInvalidClientKey  = errors.New("RlaClient: invalid client key")
	ErrRlaClientInvalidClientCert = errors.New("RlaClient: invalid client cert")
)

// SecureOptions is the enum for the secure options
type RlaClientSecureOptions int

const (
	// RlaInsecureGrpc is the insecure dial option
	RlaInsecureGrpc RlaClientSecureOptions = iota
	// RlaServerTLS is the secure dial option for server tls
	RlaServerTLS
	// RlaMutualTLS for mutual tls
	RlaMutualTLS

	// defaultCheckCertificateIntervalSeconds is the default interval to check for certificate changes
	defaultCheckRlaCertificateIntervalSeconds = 15 * 60 // 15 minutes in seconds
)

// RlaClientConfig is the data structure for the client configuration
type RlaClientConfig struct {
	// The address of the server <host>:<port>
	Address string
	// Secure flag
	Secure RlaClientSecureOptions
	// Skip Server Auth
	SkipServerAuth bool
	// The TLS certificate for the server
	ServerCAPath string
	// The TLS certificate for the client
	ClientCertPath string
	// The TLS key for the client
	ClientKeyPath string
	// client metrics interface
	ClientMetrics Metrics
}

// NewRlaClient creates a new RlaClient
func NewRlaClient(config *RlaClientConfig) (client *RlaClient, err error) {
	// Validate the config
	if config.Address == "" {
		log.Error().Err(ErrRlaClientInvalidAddress).Msg("RlaClient: no address provided")
		return nil, ErrRlaClientInvalidAddress
	}
	client = &RlaClient{}

	switch config.Secure {
	case RlaInsecureGrpc:
		// No secure options
		// Default option
		// connect with plain TCP
		log.Debug().Msg("RlaClient: insecure gRPC")
		client.dialOpts = append(client.dialOpts, grpc.WithTransportCredentials(insecure.NewCredentials()))
	case RlaServerTLS:
		log.Debug().Msg("RlaClient: server TLS")
		// Validate the config contains server ca path
		if config.ServerCAPath == "" {
			log.Error().Err(ErrRlaClientInvalidServerCA).Msg("RlaClient: no server ca path provided")
			return nil, ErrRlaClientInvalidServerCA
		}
		if config.SkipServerAuth {
			// Server TLS
			// connect with TLS but not mutual TLS
			log.Info().Msg("RlaClient: skipping server auth in TLS ( Warn: This shouldn't be used in Prod)")
			tlsConfig := &tls.Config{
				InsecureSkipVerify: true,
			}
			// Load the server ca
			_, err := credentials.NewClientTLSFromFile(config.ServerCAPath, "")
			if err != nil {
				log.Error().Err(err).Msg("RlaClient: failed to load server ca")
				return nil, err
			}

			// Create client dial option
			// Append the dial option
			client.dialOpts = append(client.dialOpts, grpc.WithTransportCredentials(credentials.NewTLS(tlsConfig)))

		} else {
			// Server TLS
			// connect with TLS but not mutual TLS
			// Load the server ca
			creds, err := credentials.NewClientTLSFromFile(config.ServerCAPath, "")
			if err != nil {
				log.Error().Err(err).Msg("RlaClient: failed to load server ca")
				return nil, err
			}
			// Append the dial option
			client.dialOpts = append(client.dialOpts, grpc.WithTransportCredentials(creds))
		}
	case RlaMutualTLS:
		// Mutual TLS
		// connect with mutual TLS
		log.Debug().Msg("RlaClient: mutual TLS")
		// 1. Load the client certificates
		clientCert, err := tls.LoadX509KeyPair(config.ClientCertPath, config.ClientKeyPath)
		if err != nil {
			log.Error().Err(err).Msg("RlaClient: failed to load client certificates")
			return nil, err
		}
		// 2. Load the Trust chain, root ca
		cabytes, err := os.ReadFile(config.ServerCAPath)
		if err != nil {
			log.Error().Err(err).Msg("RlaClient: failed to load Root CA certificates")

			return nil, err
		}
		capool := x509.NewCertPool()
		if !capool.AppendCertsFromPEM(cabytes) {
			return nil, fmt.Errorf("RlaClient: failed to append ca certificates to ca pool")
		}
		mutualTLSConfig := &tls.Config{
			Certificates: []tls.Certificate{clientCert},
			RootCAs:      capool,
		}
		creds := credentials.NewTLS(mutualTLSConfig)

		// Append to the dial option
		client.dialOpts = append(client.dialOpts, grpc.WithTransportCredentials(creds))

	default:
		log.Error().Err(ErrRlaClientInvalidSecureOpts).Msg("RlaClient: invalid dial options")
		return nil, ErrRlaClientInvalidSecureOpts
	}

	// configure interceptors
	var unaryInterceptors []grpc.UnaryClientInterceptor
	if config.ClientMetrics != nil {
		unaryInterceptors = append(unaryInterceptors, newGrpcUnaryMetricsInterceptor(config.ClientMetrics))
	}
	var streamInterceptors []grpc.StreamClientInterceptor
	if config.ClientMetrics != nil {
		streamInterceptors = append(streamInterceptors, newGrpcStreamMetricsInterceptor(config.ClientMetrics))
	}
	if os.Getenv("LS_SERVICE_NAME") != "" {
		handler := otelgrpc.NewClientHandler(otelgrpc.WithPropagators(otel.GetTextMapPropagator()))
		client.dialOpts = append(client.dialOpts, grpc.WithStatsHandler(handler))
	}
	if len(unaryInterceptors) > 0 {
		client.dialOpts = append(client.dialOpts, grpc.WithUnaryInterceptor(grpcmw.ChainUnaryClient(unaryInterceptors...)))
	}
	if len(streamInterceptors) > 0 {
		client.dialOpts = append(client.dialOpts, grpc.WithStreamInterceptor(grpcmw.ChainStreamClient(streamInterceptors...)))
	}

	// Create the client connection
	client.conn, err = grpc.NewClient(config.Address, client.dialOpts...)
	if err != nil {
		log.Error().Err(err).Msg("RlaClient: failed to initialize gRPC client")
		return nil, err
	}
	log.Info().Msg("RlaClient: gRPC client initialized")

	// Create RLA client
	client.rla = rlav1.NewRLAClient(client.conn)
	log.Info().Msg("RlaClient: client created")

	// Check the version of the server
	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(time.Duration(5000)*time.Millisecond))
	defer cancel()
	_, err = client.rla.Version(ctx, &rlav1.VersionRequest{})
	if err != nil {
		log.Error().Err(err).Msg("RlaClient: failed to get version from server")
		return nil, fmt.Errorf("RlaClient: failed to get version from server: %w", err)
	}

	log.Info().Msg("RlaClient: successfully connected to server")

	return client, nil
}

// RlaClient is the data structure for the client
type RlaClient struct {
	// The client connection
	conn *grpc.ClientConn
	// gRPC dial options
	dialOpts []grpc.DialOption
	// rla client interface
	rla rlav1.RLAClient
}

// Close gracefully shuts down the client's gRPC connection.
func (cc *RlaClient) Close() error {
	if cc.conn != nil {
		// Close the grpc.ClientConn.
		return cc.conn.Close()
	}
	return nil
}

// Rla client getter
func (client *RlaClient) Rla() rlav1.RLAClient {
	return client.rla
}

// RlaAtomicClient is an atomic wrapper around the RlaClient
type RlaAtomicClient struct {
	Config  *RlaClientConfig
	value   *atomic.Value
	version atomic.Int64
}

// Version returns the current version of the RlaClient
func (rac *RlaAtomicClient) Version() int64 {
	return rac.version.Load()
}

// SwapClient atomically replaces the current RlaClient with a new one,
// returning the old client for the caller to manage.
func (rac *RlaAtomicClient) SwapClient(newClient *RlaClient) *RlaClient {

	// Atomically replace the current client with the new one and return the old client.
	oldClientInterface := rac.value.Swap(newClient)

	// Type assert the returned value to *RlaClient.
	// This should always succeed if the correct type was stored initially.
	oldClient, ok := oldClientInterface.(*RlaClient)
	if !ok {
		log.Error().Msg("SwapClient: Type assertion failed for the old client")
		return nil
	}

	// Increment the version number
	rac.version.Add(1)

	return oldClient
}

// GetClient returns the current version of Rla client from the atomic value.
// Returns nil if the client has not been initialized yet.
func (rac *RlaAtomicClient) GetClient() *RlaClient {
	v := rac.value.Load()
	if v == nil {
		return nil
	}
	client, _ := v.(*RlaClient)

	return client
}

// GetRLAClient returns the underlying RLA gRPC client. Returns ErrClientNotConnected
// if the client has not been initialized or is not currently connected.
// Prefer this over GetClient() + manual nil-check + .Rla() at call sites.
func (rac *RlaAtomicClient) GetRLAClient() (rlav1.RLAClient, error) {
	client := rac.GetClient()
	if client == nil {
		return nil, ErrClientNotConnected
	}
	// It's true that NewRlaClient always populates the inner rla field, BUT,
	// guard against zero-value RlaClient instances slipping in via direct
	// construction. Without this, a misconstructed wrapper would yield (nil,
	// nil) and break things.
	rla := client.Rla()
	if rla == nil {
		return nil, ErrClientNotConnected
	}
	return rla, nil
}

// CheckAndReloadCerts continuously monitors the TLS certificates for changes.
// If a change is detected, it reinitializes the RlaClient with the new certificates to ensure secure communication.
func (rac *RlaAtomicClient) CheckAndReloadCerts(initialClientCertMD5, initialServerCAMD5 []byte) {
	// Initialize contextual logger
	logger := log.With().Str("Component", "Rla").Str("Operation", "CheckAndReloadCerts").Logger()

	ticker := time.NewTicker(getRlaCertificateCheckInterval())
	defer ticker.Stop()

	lastClientCertMD5, lastServerCAMD5 := initialClientCertMD5, initialServerCAMD5

	for range ticker.C {
		changed, newClientMD5, newServerMD5, err := rac.CheckCertificates(lastClientCertMD5, lastServerCAMD5)
		if err != nil {
			logger.Error().Err(err).Msg("Error checking certificates for changes")
			continue
		}

		if changed {
			newClient, err := NewRlaClient(rac.Config)
			if err != nil {
				logger.Error().Err(err).Msg("Failed to reinitialize gRPC client with new certificates")
				continue
			}

			// Atomically update the client instance and get the old one.
			oldClient := rac.SwapClient(newClient)

			// Delayed closure of the old client.
			go func(clientToClose *RlaClient) {
				// Delay the closure to allow ongoing client requests to complete.
				time.Sleep(10 * time.Second) // Adjust the delay as needed.

				// Ensure the client exists and has a connection to close.
				if clientToClose != nil {
					if err := clientToClose.Close(); err != nil {
						log.Error().Err(err).Msg("Error closing old RlaClient connection")
					}
				}
			}(oldClient)

			logger.Info().Msg("gRPC client successfully reinitialized with new certificates")

			// Update the stored MD5 hashes with the new ones for the next comparison.
			lastClientCertMD5, lastServerCAMD5 = newClientMD5, newServerMD5
		}
	}
}

// GetInitialCertMD5 retrieves the MD5 hash of the initial set of certificate that the client is Using
func (rac *RlaAtomicClient) GetInitialCertMD5() (clientCertMD5, serverCAMD5 []byte, err error) {
	// Load and hash the client certificate
	clientCertBytes, err := os.ReadFile(rac.Config.ClientCertPath)
	if err != nil {
		return nil, nil, err
	}
	clientCertMD5Hash := md5.Sum(clientCertBytes)
	clientCertMD5 = clientCertMD5Hash[:]

	// Load and hash the server CA certificate using os.ReadFile
	serverCABytes, err := os.ReadFile(rac.Config.ServerCAPath)
	if err != nil {
		return nil, nil, err
	}
	serverCAMD5Hash := md5.Sum(serverCABytes)
	serverCAMD5 = serverCAMD5Hash[:]

	return clientCertMD5, serverCAMD5, nil
}

// CheckCertificates checks if the client and server CA certificates have changed
func (rac *RlaAtomicClient) CheckCertificates(lastClientCertMD5, lastServerCAMD5 []byte) (bool, []byte, []byte, error) {
	// Load and hash the client certificate using os.ReadFile
	clientCertBytes, err := os.ReadFile(rac.Config.ClientCertPath)
	if err != nil {
		return false, lastClientCertMD5, lastServerCAMD5, err
	}
	clientCertMD5 := md5.Sum(clientCertBytes)

	// Load and hash the server CA certificate using os.ReadFile
	serverCABytes, err := os.ReadFile(rac.Config.ServerCAPath)
	if err != nil {
		return false, lastClientCertMD5, lastServerCAMD5, err
	}
	serverCAMD5 := md5.Sum(serverCABytes)

	// Check if either certificate has changed
	if !equalMD5(lastClientCertMD5, clientCertMD5[:]) || !equalMD5(lastServerCAMD5, serverCAMD5[:]) {
		return true, clientCertMD5[:], serverCAMD5[:], nil
	}

	return false, lastClientCertMD5, lastServerCAMD5, nil
}

// NewRlaAtomicClient creates a new RlaAtomicClient
func NewRlaAtomicClient(config *RlaClientConfig) *RlaAtomicClient {
	// Create the atomic value
	atomicClient := &RlaAtomicClient{
		Config:  config,
		value:   &atomic.Value{},
		version: atomic.Int64{},
	}

	return atomicClient
}

func getRlaCertificateCheckInterval() time.Duration {
	var err error
	if value, ok := os.LookupEnv("RLA_CERT_CHECK_INTERVAL"); ok {
		if interval, err := strconv.Atoi(value); err == nil {
			return time.Duration(interval) * time.Second
		}
		log.Error().Err(err).Msg("Invalid RLA_CERT_CHECK_INTERVAL value; using default.")
	}
	return defaultCheckRlaCertificateIntervalSeconds * time.Second
}
