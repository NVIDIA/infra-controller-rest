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

	wflows "github.com/NVIDIA/ncx-infra-controller-rest/workflow-schema/schema/site-agent/workflows/v1"
)

// Errors
var (
	ErrNICoClientInvalidAddress    = errors.New("NICoClient: invalid address")
	ErrNICoClientInvalidDialOpts   = errors.New("NICoClient: invalid dial options")
	ErrNICoClientInvalidSecureOpts = errors.New("NICoClient: invalid secure options")
	ErrNICoClientInvalidServerCA   = errors.New("NICoClient: invalid server CA")
	ErrNICoClientInvalidClientCA   = errors.New("NICoClient: invalid client CA")
	ErrNICoClientInvalidClientKey  = errors.New("NICoClient: invalid client key")
	ErrNICoClientInvalidClientCert = errors.New("NICoClient: invalid client cert")
	ErrClientNotConnected             = errors.New("gRPC client is not connected to the server")
)

// SecureOptions is the enum for the secure options
type SecureOptions int

const (
	// InsecuregRPC is the insecure dial option
	InsecuregRPC SecureOptions = iota
	// ServerTLS is the secure dial option for server tls
	ServerTLS
	// MutualTLS for mutual tls
	MutualTLS
	// defaultCheckCertificateIntervalSeconds is the default interval to check for certificate changes
	defaultCheckCertificateIntervalSeconds = 15 * 60 // 15 minutes in seconds
)

// NICoClientConfig is the data structure for the client configuration
type NICoClientConfig struct {
	// The address of the server <host>:<port>
	Address string
	// Secure flag
	Secure SecureOptions
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

// NewNICoClient creates a new NICoClient
func NewNICoClient(config *NICoClientConfig) (client *NICoClient, err error) {
	// Validate the config
	if config.Address == "" {
		log.Error().Err(ErrNICoClientInvalidAddress).Msg("NICoClient: no address provided")
		return nil, ErrNICoClientInvalidAddress
	}
	client = &NICoClient{}

	switch config.Secure {
	case InsecuregRPC:
		// No secure options
		// Default option
		// connect with plain TCP
		log.Debug().Msg("NICoClient: insecure gRPC")
		client.dialOpts = append(client.dialOpts, grpc.WithTransportCredentials(insecure.NewCredentials()))
	case ServerTLS:
		log.Debug().Msg("NICoClient: server TLS")
		// Validate the config contains server ca path
		if config.ServerCAPath == "" {
			log.Error().Err(ErrNICoClientInvalidServerCA).Msg("NICoClient: no server ca path provided")
			return nil, ErrNICoClientInvalidServerCA
		}
		if config.SkipServerAuth {
			// Server TLS
			// connect with TLS but not mutual TLS
			log.Info().Msg("NICoClient: skipping server auth in TLS ( Warn: This shouldn't be used in Prod)")
			tlsConfig := &tls.Config{
				InsecureSkipVerify: true,
			}
			// Load the server ca
			_, err := credentials.NewClientTLSFromFile(config.ServerCAPath, "")
			if err != nil {
				log.Error().Err(err).Msg("NICoClient: failed to load server ca")
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
				log.Error().Err(err).Msg("NICoClient: failed to load server ca")
				return nil, err
			}
			// Append the dial option
			client.dialOpts = append(client.dialOpts, grpc.WithTransportCredentials(creds))
		}
	case MutualTLS:
		// Mutual TLS
		// connect with mutual TLS
		log.Debug().Msg("NICoClient: mutual TLS")
		// 1. Load the client certificates
		clientCert, err := tls.LoadX509KeyPair(config.ClientCertPath, config.ClientKeyPath)
		if err != nil {
			log.Error().Err(err).Msg("NICoClient: failed to load client certificates")
			return nil, err
		}
		// 2. Load the Trust chain, root ca
		cabytes, err := os.ReadFile(config.ServerCAPath)
		if err != nil {
			log.Error().Err(err).Msg("NICoClient: failed to load Root CA certificates")

			return nil, err
		}
		capool := x509.NewCertPool()
		if !capool.AppendCertsFromPEM(cabytes) {
			return nil, fmt.Errorf("NICoClient: failed to append ca certificates to ca pool")
		}
		mutualTLSConfig := &tls.Config{
			Certificates: []tls.Certificate{clientCert},
			RootCAs:      capool,
		}
		creds := credentials.NewTLS(mutualTLSConfig)

		// Append to the dial option
		client.dialOpts = append(client.dialOpts, grpc.WithTransportCredentials(creds))

	default:
		log.Error().Err(ErrNICoClientInvalidSecureOpts).Msg("NICoClient: invalid dial options")
		return nil, ErrNICoClientInvalidSecureOpts
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
		log.Error().Err(err).Msg("NICoClient: failed to initialize gRPC client")
		return nil, err
	}
	log.Info().Msg("NICoClient: gRPC client initialized")

	// Create nico client
	client.nico = wflows.NewNICoClient(client.conn)
	log.Info().Msg("NICoClient: client created")

	// Check the version of the server
	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(time.Duration(5000)*time.Millisecond))
	defer cancel()
	_, err = client.nico.Version(ctx, &wflows.VersionRequest{})
	if err != nil {
		log.Error().Err(err).Msg("NICoClient: failed to get version from server")
		return nil, fmt.Errorf("NICoClient: failed to get version from server: %w", err)
	}

	log.Info().Msg("NICoClient: successfully connected to server")

	return client, nil
}

// NICoClient is the data structure for the client
type NICoClient struct {
	// The client connection
	conn *grpc.ClientConn
	// gRPC dial options
	dialOpts []grpc.DialOption
	// nico client interface
	nico wflows.NICoClient
}

// Close gracefully shuts down the client's gRPC connection.
func (cc *NICoClient) Close() error {
	if cc.conn != nil {
		// Close the grpc.ClientConn.
		return cc.conn.Close()
	}
	return nil
}

// NICo client getter
func (client *NICoClient) NICo() wflows.NICoClient {
	return client.nico
}

// NICoAtomicClient is an atomic wrapper around the NICoClient
type NICoAtomicClient struct {
	Config  *NICoClientConfig
	value   *atomic.Value
	version atomic.Int64
}

// Version returns the current version of the NICoClient
func (cac *NICoAtomicClient) Version() int64 {
	return cac.version.Load()
}

// SwapClient atomically replaces the current NICoClient with a new one,
// returning the old client for the caller to manage.
func (cac *NICoAtomicClient) SwapClient(newClient *NICoClient) *NICoClient {

	// Atomically replace the current client with the new one and return the old client.
	oldClientInterface := cac.value.Swap(newClient)

	// Type assert the returned value to *NICoClient.
	// This should always succeed if the correct type was stored initially.
	oldClient, ok := oldClientInterface.(*NICoClient)
	if !ok {
		log.Error().Msg("SwapClient: Type assertion failed for the old client")
		return nil
	}

	// Increment the version number
	cac.version.Add(1)

	return oldClient
}

// GetClient returns the current version of NICo client from the atomic value.
// Returns nil if the client has not been initialized yet.
func (cac *NICoAtomicClient) GetClient() *NICoClient {
	v := cac.value.Load()
	if v == nil {
		return nil
	}
	client, _ := v.(*NICoClient)

	return client
}

// CheckAndReloadCerts continuously monitors the TLS certificates for changes.
// If a change is detected, it reinitializes the NICoClient with the new certificates to ensure secure communication.
func (cac *NICoAtomicClient) CheckAndReloadCerts(initialClientCertMD5, initialServerCAMD5 []byte) {
	// Initialize contextual logger
	logger := log.With().Str("Component", "NICo").Str("Operation", "CheckAndReloadCerts").Logger()

	ticker := time.NewTicker(getCertificateCheckInterval())
	defer ticker.Stop()

	lastClientCertMD5, lastServerCAMD5 := initialClientCertMD5, initialServerCAMD5

	for range ticker.C {
		changed, newClientMD5, newServerMD5, err := cac.CheckCertificates(lastClientCertMD5, lastServerCAMD5)
		if err != nil {
			logger.Error().Err(err).Msg("Error checking certificates for changes")
			continue
		}

		if changed {
			newClient, err := NewNICoClient(cac.Config)
			if err != nil {
				logger.Error().Err(err).Msg("Failed to reinitialize gRPC client with new certificates")
				continue
			}

			// Atomically update the client instance and get the old one.
			oldClient := cac.SwapClient(newClient)

			// Delayed closure of the old client.
			go func(clientToClose *NICoClient) {
				// Delay the closure to allow ongoing client requests to complete.
				time.Sleep(10 * time.Second) // Adjust the delay as needed.

				// Ensure the client exists and has a connection to close.
				if clientToClose != nil {
					if err := clientToClose.Close(); err != nil {
						log.Error().Err(err).Msg("Error closing old NICoClient connection")
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
func (cac *NICoAtomicClient) GetInitialCertMD5() (clientCertMD5, serverCAMD5 []byte, err error) {
	// Load and hash the client certificate
	clientCertBytes, err := os.ReadFile(cac.Config.ClientCertPath)
	if err != nil {
		return nil, nil, err
	}
	clientCertMD5Hash := md5.Sum(clientCertBytes)
	clientCertMD5 = clientCertMD5Hash[:]

	// Load and hash the server CA certificate using os.ReadFile
	serverCABytes, err := os.ReadFile(cac.Config.ServerCAPath)
	if err != nil {
		return nil, nil, err
	}
	serverCAMD5Hash := md5.Sum(serverCABytes)
	serverCAMD5 = serverCAMD5Hash[:]

	return clientCertMD5, serverCAMD5, nil
}

// CheckCertificates checks if the client and server CA certificates have changed
func (cac *NICoAtomicClient) CheckCertificates(lastClientCertMD5, lastServerCAMD5 []byte) (bool, []byte, []byte, error) {
	// Load and hash the client certificate using os.ReadFile
	clientCertBytes, err := os.ReadFile(cac.Config.ClientCertPath)
	if err != nil {
		return false, lastClientCertMD5, lastServerCAMD5, err
	}
	clientCertMD5 := md5.Sum(clientCertBytes)

	// Load and hash the server CA certificate using os.ReadFile
	serverCABytes, err := os.ReadFile(cac.Config.ServerCAPath)
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

// NewNICoAtomicClient creates a new NICoAtomicClient
func NewNICoAtomicClient(config *NICoClientConfig) *NICoAtomicClient {
	// Create the atomic value
	atomicClient := &NICoAtomicClient{
		Config:  config,
		value:   &atomic.Value{},
		version: atomic.Int64{},
	}

	return atomicClient
}

func getCertificateCheckInterval() time.Duration {
	var err error
	if value, ok := os.LookupEnv("NICO_CERT_CHECK_INTERVAL"); ok {
		if interval, err := strconv.Atoi(value); err == nil {
			return time.Duration(interval) * time.Second
		}
		log.Error().Err(err).Msg("Invalid NICO_CERT_CHECK_INTERVAL value; using default.")
	}
	return defaultCheckCertificateIntervalSeconds * time.Second
}

func equalMD5(a, b []byte) bool {
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func SliceToChunks[T any](slice []T, chunkSize int) (chunks [][]T) {
	chunks = [][]T{}
	for {
		if len(slice) == 0 {
			break
		}

		if len(slice) < chunkSize {
			chunkSize = len(slice)
		}

		chunks = append(chunks, slice[0:chunkSize])
		slice = slice[chunkSize:]
	}

	return chunks
}
