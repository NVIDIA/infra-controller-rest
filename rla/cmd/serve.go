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

package cmd

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
	"go.temporal.io/sdk/worker"

	cdb "github.com/NVIDIA/ncx-infra-controller-rest/db/pkg/db"
	"github.com/NVIDIA/ncx-infra-controller-rest/rla/internal/nicoapi"
	"github.com/NVIDIA/ncx-infra-controller-rest/rla/internal/config"
	svc "github.com/NVIDIA/ncx-infra-controller-rest/rla/internal/service"
	"github.com/NVIDIA/ncx-infra-controller-rest/rla/internal/task/componentmanager"
	computenico "github.com/NVIDIA/ncx-infra-controller-rest/rla/internal/task/componentmanager/compute/nico"
	"github.com/NVIDIA/ncx-infra-controller-rest/rla/internal/task/componentmanager/mock"
	nvlswitchnico "github.com/NVIDIA/ncx-infra-controller-rest/rla/internal/task/componentmanager/nvlswitch/nico"
	nvlswitchnsm "github.com/NVIDIA/ncx-infra-controller-rest/rla/internal/task/componentmanager/nvlswitch/nvswitchmanager"
	powershelfnico "github.com/NVIDIA/ncx-infra-controller-rest/rla/internal/task/componentmanager/powershelf/nico"
	powershelfpsm "github.com/NVIDIA/ncx-infra-controller-rest/rla/internal/task/componentmanager/powershelf/psm"
	"github.com/NVIDIA/ncx-infra-controller-rest/rla/internal/task/componentmanager/providers/nico"
	"github.com/NVIDIA/ncx-infra-controller-rest/rla/internal/task/componentmanager/providers/nvswitchmanager"
	"github.com/NVIDIA/ncx-infra-controller-rest/rla/internal/task/componentmanager/providers/psm"
	temporalmanager "github.com/NVIDIA/ncx-infra-controller-rest/rla/internal/task/executor/temporalworkflow/manager"
	pkgcerts "github.com/NVIDIA/ncx-infra-controller-rest/rla/pkg/certs"
)

const (
	defaultServicePort    = 50051
	componentMgrCfgEnvVar = "COMPONENT_MANAGER_CONFIG"
)

var (
	port               int
	componentMgrConfig string
	devMode            bool

	// clientOnlyFlags are the global persistent flags that apply only to
	// client commands. They are hidden from serve's help and rejected if set.
	clientOnlyFlags = []string{flagHost, flagPort}

	// serveCmd represents the serve command
	serveCmd = &cobra.Command{
		Use:   "serve",
		Short: "Start the RLA gPRC server",
		Long:  `Start the gRPC server to allow other services to manage the racks`,
		PreRunE: func(cmd *cobra.Command, args []string) error {
			for _, name := range clientOnlyFlags {
				if cmd.Root().PersistentFlags().Changed(name) {
					return fmt.Errorf("--%s is not applicable to 'rla serve'", name)
				}
			}
			return nil
		},
		Run: func(cmd *cobra.Command, args []string) {
			doServe()
		},
	}
)

func init() {
	rootCmd.AddCommand(serveCmd)

	// Hide client-only persistent flags from serve's help output.
	for _, name := range clientOnlyFlags {
		_ = serveCmd.InheritedFlags().MarkHidden(name)
	}

	serveCmd.Flags().IntVarP(&port, "listen-port", "p", defaultServicePort, "Port for the gRPC server") //nolint:lll
	// Component manager config: priority is CLI flag > env var > default prod config
	serveCmd.Flags().StringVarP(&componentMgrConfig, "component-config", "c", "", "Path to component manager config file (YAML)")               //nolint:lll
	serveCmd.Flags().BoolVar(&devMode, "dev-mode", false, "Enable developer options (gRPC reflection, debug logging). Not for production use.") //nolint:lll
}

// initProviderRegistry creates and initializes the provider registry based on configuration.
func initProviderRegistry(config componentmanager.Config) (*componentmanager.ProviderRegistry, error) {
	providerRegistry := componentmanager.NewProviderRegistry()

	// Initialize NICo provider if configured
	if config.Providers.NICo != nil {
		nicoProvider, err := nico.New(*config.Providers.NICo)
		if err != nil {
			log.Warn().Err(err).Msg("Unable to create NICo GRPC client (power control may not work)")
		} else {
			providerRegistry.Register(nicoProvider)
			log.Info().
				Dur("timeout", config.Providers.NICo.Timeout).
				Msg("Initialized NICo provider")
		}
	}

	// Initialize PSM provider if configured
	if config.Providers.PSM != nil {
		psmProvider, err := psm.New(*config.Providers.PSM)
		if err != nil {
			log.Warn().Err(err).Msg("Unable to create PSM client (powershelf operations may not work)")
		} else {
			providerRegistry.Register(psmProvider)
			log.Info().
				Dur("timeout", config.Providers.PSM.Timeout).
				Msg("Initialized PSM provider")
		}
	}

	// Initialize NV-Switch Manager provider if configured
	if config.Providers.NVSwitchManager != nil {
		nsmProvider, err := nvswitchmanager.New(*config.Providers.NVSwitchManager)
		if err != nil {
			log.Warn().Err(err).Msg("Unable to create NV-Switch Manager client (NVLSwitch operations may not work)")
		} else {
			providerRegistry.Register(nsmProvider)
			log.Info().
				Dur("timeout", config.Providers.NVSwitchManager.Timeout).
				Msg("Initialized NV-Switch Manager provider")
		}
	}

	// Log all registered providers
	registeredProviders := providerRegistry.List()
	log.Info().
		Strs("providers", registeredProviders).
		Msg("Provider registry initialized")

	return providerRegistry, nil
}

// initComponentManagerRegistry creates and initializes the component manager registry.
func initComponentManagerRegistry(config componentmanager.Config, providerRegistry *componentmanager.ProviderRegistry) (*componentmanager.Registry, error) {
	registry := componentmanager.NewRegistry()

	// Register all available component manager factories
	var computePowerDelay time.Duration
	if config.Providers.NICo != nil {
		computePowerDelay = config.Providers.NICo.ComputePowerDelay
	}
	computenico.Register(registry, computePowerDelay)
	nvlswitchnico.Register(registry)
	nvlswitchnsm.Register(registry)
	powershelfnico.Register(registry)
	powershelfpsm.Register(registry)
	mock.RegisterAll(registry)

	// Initialize registry with the config and providers
	if err := registry.Initialize(config, providerRegistry); err != nil {
		return nil, fmt.Errorf("failed to initialize component managers: %w", err)
	}

	// Log registered implementations
	impls := registry.ListRegisteredImplementations()
	for compType, names := range impls {
		log.Debug().
			Str("component_type", compType.String()).
			Strs("implementations", names).
			Msg("Registered component manager implementations")
	}

	return registry, nil
}

// loadComponentManagerConfig loads the component manager configuration with the following priority:
//
//  1. CLI flag: --component-config / -c <path>
//     Example: ./rla serve -c /etc/rla/custom.yaml
//
//  2. Environment variable: COMPONENT_MANAGER_CONFIG=<path>
//     Example: COMPONENT_MANAGER_CONFIG=/etc/rla/componentmanager.yaml
//
//  3. Embedded default: componentmanager.DefaultProdConfig()
//     Used when no config file is provided. The primary production path.
//     Uses real implementations (NICo for compute/nvlswitch, PSM for powershelf).
//
// The config specifies:
//   - Which component manager implementations to use (nico, psm, mock)
//   - Provider settings (timeouts, endpoints)
func loadComponentManagerConfig() (componentmanager.Config, error) {
	// Priority 1: CLI flag
	configPath := componentMgrConfig

	// Priority 2: Environment variable
	if configPath == "" {
		configPath = os.Getenv(componentMgrCfgEnvVar)
	}

	// Load from file if a path was specified
	if configPath != "" {
		log.Info().Str("config_path", configPath).Msg("Loading component manager config from file")
		return componentmanager.LoadConfig(configPath)
	}

	// Priority 3: Embedded production config
	log.Info().Msg("Using embedded production config (nico + psm)")
	return componentmanager.DefaultProdConfig(), nil
}

// doServe is the main entry point for the serve subcommand. It loads all
// configuration, initialises provider and component manager registries, builds
// the service, and blocks until a termination signal is received.
func doServe() {
	if devMode {
		zerolog.SetGlobalLevel(zerolog.DebugLevel)
	} else {
		zerolog.SetGlobalLevel(zerolog.InfoLevel)
	}

	if os.Getenv(svc.EnvVarName) == "" {
		log.Warn().Msgf("%s not set, defaulting to %q for local development", svc.EnvVarName, "development")
		os.Setenv(svc.EnvVarName, "development") //nolint:errcheck
	}

	rlaEnv, err := svc.GetDeploymentEnv()
	if err != nil {
		log.Fatal().Err(err).Msg("Invalid deployment environment")
	}

	log.Info().Str(svc.EnvVarName, rlaEnv).Msg("Deployment environment")

	rlaConfig := config.ReadConfig()

	dbConf, err := cdb.ConfigFromEnv()
	if err != nil {
		log.Fatal().Msgf("failed to retrieve DB conn information: %v", err)
	}

	temporalConf, err := svc.BuildTemporalConfigFromEnv()
	if err != nil {
		log.Fatal().Msgf("failed to retrieve Temporal conn information: %v", err)
	}

	// Load component manager configuration
	cmConfig, err := loadComponentManagerConfig()
	if err != nil {
		log.Fatal().Msgf("failed to load component manager config: %v", err)
	}

	// Initialize provider registry (creates API clients based on config)
	providerRegistry, err := initProviderRegistry(cmConfig)
	if err != nil {
		log.Fatal().Msgf("failed to initialize provider registry: %v", err)
	}

	// Initialize component manager registry
	cmRegistry, err := initComponentManagerRegistry(cmConfig, providerRegistry)
	if err != nil {
		log.Fatal().Msgf("failed to initialize component manager registry: %v", err)
	}

	temporalManagerConf := temporalmanager.Config{
		ClientConf: *temporalConf,
		WorkerOptions: map[string]worker.Options{
			temporalmanager.WorkflowQueue: {},
		},
		ComponentManagerRegistry: cmRegistry,
	}

	ctx := context.Background()

	if os.Getenv("REPORT_NICO_API_VERSION") != "" {
		// Do some basic nico-core-api requests, mainly for early testing; this code can be removed when we're doing actual communication
		go func() {
			client, err := nicoapi.NewClient(time.Minute)
			if err != nil {
				log.Fatal().Msgf("Unable to create GRPC client: %v", err)
			}
			for {
				time.Sleep(time.Second * 10)
				if version, err := client.Version(ctx); err != nil {
					log.Error().Msgf("Unable to retrieve version from nico-core-api: %v", err)
					continue
				} else {
					log.Info().Msgf("nico-core-api version: %s", version)
					break
				}
			}
			for {
				time.Sleep(time.Second * 10)
				if machines, err := client.GetMachines(ctx); err != nil {
					log.Error().Msgf("Unable to retrieve machines from nico-core-api: %v", err)
					continue
				} else {
					log.Info().Msgf("nico-core-api machines: %v", machines)
					break
				}
			}
		}()
	}

	service, err := svc.New(
		ctx,
		svc.Config{
			Port:             port,
			DBConf:           dbConf,
			ExecutorConf:     &temporalManagerConf,
			RLAConfig:        rlaConfig,
			CMConfig:         cmConfig,
			ProviderRegistry: providerRegistry,
			DevMode:          devMode,
			CertConfig: pkgcerts.Config{
				CACert:  globalCACert,
				TLSCert: globalTLSCert,
				TLSKey:  globalTLSKey,
			},
		},
	)

	if err != nil {
		log.Fatal().Msgf("failed to create the new gRPC server: %v", err)
	}

	log.Info().Msg("New RLA service is created\n")
	log.Info().Msgf("DB config: %+v", dbConf)
	log.Info().Msgf("Temporal config: %+v", temporalManagerConf)

	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigs // Block execution until signal from terminal gets triggered here.
		service.Stop(ctx)
	}()

	if err := service.Start(ctx); err != nil {
		log.Fatal().Msgf("failed to start the service: %v\n", err)
	}
}
