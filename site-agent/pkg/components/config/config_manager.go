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

package config

import (
	"flag"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/NVIDIA/ncx-infra-controller-rest/site-agent/pkg/conftypes"
	"github.com/NVIDIA/ncx-infra-controller-rest/site-workflow/pkg/grpc/client"
	"github.com/google/uuid"
	"github.com/joho/godotenv"
	"github.com/rs/zerolog/log"
)

const (
	DefaultNICoClientCAPath   = "/etc/nico/ca.crt"
	DefaultNICoClientCertPath = "/etc/nico/tls.crt"
	DefaultNICoClientKeyPath  = "/etc/nico/tls.key"

	// RLA uses the same SPIFFE trust domain (nico.local) and vault-nico-issuer as NICo,
	// so we can reuse the NICo certificates for mTLS with RLA.
	DefaultRLAClientCAPath   = "/etc/nico/ca.crt"
	DefaultRLAClientCertPath = "/etc/nico/tls.crt"
	DefaultRLAClientKeyPath  = "/etc/nico/tls.key"
)

// NewElektraConfig reads configurations from env variables and returns
func NewElektraConfig(utMode bool) *conftypes.Config {
	log.Info().Msg("Config Manager: Processing Config")
	conf := conftypes.NewConfType()

	var enableDebug string
	var devmode string
	var enableTLS string
	var disableBootstrap string
	var watcherInterval string
	var podName string
	var skipServerAuth string

	// Determine environment in which app is running.
	conf.RunningIn = determineEnvironment()
	conf.UtMode = utMode

	// NICo config
	// For each env var, try the new NICO_* name first then fall back to the legacy CARBIDE_* name.
	// TODO: remove CARBIDE_* fallbacks once deployment config repo is fully updated to NICO_* vars.
	nicoAddress := os.Getenv("NICO_ADDRESS")
	if nicoAddress == "" {
		nicoAddress = os.Getenv("CARBIDE_ADDRESS")
	}
	flag.StringVar(&conf.NICo.Address, "nicoAddress", nicoAddress, "NICo Address")
	if conf.NICo.Address == "" {
		conf.NICo.Address = "nico-api.nico-system.svc.cluster.local:1079"
	}
	nicoSecOpt := os.Getenv("NICO_SEC_OPT")
	if nicoSecOpt == "" {
		nicoSecOpt = os.Getenv("CARBIDE_SEC_OPT") // TODO: remove once deployment config repo is updated
	}
	cSecOpt, err := strconv.Atoi(nicoSecOpt)
	if err != nil {
		log.Info().Msg(err.Error())
		cSecOpt = int(client.ServerTLS)
	}
	if cSecOpt < int(client.InsecuregRPC) && cSecOpt > int(client.MutualTLS) {
		cSecOpt = int(client.ServerTLS)
	}
	sOpt := 0
	flag.IntVar(&sOpt, "nicoSecureOptions", cSecOpt, "NICo security option")
	conf.NICo.Secure = client.SecureOptions(sOpt)
	nicoCAPath := os.Getenv("NICO_CA_CERT_PATH")
	if nicoCAPath == "" {
		nicoCAPath = os.Getenv("CARBIDE_CA_CERT_PATH") // TODO: remove once deployment config repo is updated
	}
	flag.StringVar(&conf.NICo.ServerCAPath, "nicoCertPath", nicoCAPath, "NICo Cert Path")
	if conf.NICo.ServerCAPath == "" {
		conf.NICo.ServerCAPath = DefaultNICoClientCAPath
	}
	nicoClientCert := os.Getenv("NICO_CLIENT_CERT_PATH")
	if nicoClientCert == "" {
		nicoClientCert = os.Getenv("CARBIDE_CLIENT_CERT_PATH") // TODO: remove once deployment config repo is updated
	}
	flag.StringVar(&conf.NICo.ClientCertPath, "nicoClientCertPath", nicoClientCert, "NICo client Cert Path")
	if conf.NICo.ClientCertPath == "" {
		conf.NICo.ClientCertPath = DefaultNICoClientCertPath
	}
	nicoClientKey := os.Getenv("NICO_CLIENT_KEY_PATH")
	if nicoClientKey == "" {
		nicoClientKey = os.Getenv("CARBIDE_CLIENT_KEY_PATH") // TODO: remove once deployment config repo is updated
	}
	flag.StringVar(&conf.NICo.ClientKeyPath, "nicoClientKeyPath", nicoClientKey, "NICo client Cert Path")
	if conf.NICo.ClientKeyPath == "" {
		conf.NICo.ClientKeyPath = DefaultNICoClientKeyPath
	}

	log.Info().Msg(conf.NICo.Address)
	log.Info().Msg(strconv.Itoa(int(conf.NICo.Secure)))

	log.Info().Msg("CA Path:" + conf.NICo.ServerCAPath)
	log.Info().Msg("client Cert:" + conf.NICo.ClientCertPath)
	log.Info().Msg("client Key:" + conf.NICo.ClientKeyPath)

	// RLA config
	flag.StringVar(&conf.RLA.Address, "rlaAddress", os.Getenv("RLA_ADDRESS"), "RLA Address")
	if conf.RLA.Address == "" {
		conf.RLA.Address = "rla.rla.svc.cluster.local:50051"
	}
	rlaSecOpt, err := strconv.Atoi(os.Getenv("RLA_SEC_OPT"))
	if err != nil {
		log.Info().Msg("Invalid RLA security option, using default")
		rlaSecOpt = int(client.RlaServerTLS)
	}
	if rlaSecOpt < int(client.RlaInsecureGrpc) || rlaSecOpt > int(client.RlaMutualTLS) {
		rlaSecOpt = int(client.RlaServerTLS)
	}
	rlaOpt := 0
	flag.IntVar(&rlaOpt, "rlaSecureOptions", rlaSecOpt, "RLA security option")
	conf.RLA.Secure = client.RlaClientSecureOptions(rlaOpt)
	flag.StringVar(&conf.RLA.ServerCAPath, "rlaCertPath", os.Getenv("RLA_CA_CERT_PATH"), "RLA CA Cert Path")
	if conf.RLA.ServerCAPath == "" {
		conf.RLA.ServerCAPath = DefaultRLAClientCAPath
	}
	flag.StringVar(&conf.RLA.ClientCertPath, "rlaClientCertPath", os.Getenv("RLA_CLIENT_CERT_PATH"), "RLA client Cert Path")
	if conf.RLA.ClientCertPath == "" {
		conf.RLA.ClientCertPath = DefaultRLAClientCertPath
	}
	flag.StringVar(&conf.RLA.ClientKeyPath, "rlaClientKeyPath", os.Getenv("RLA_CLIENT_KEY_PATH"), "RLA client Key Path")
	if conf.RLA.ClientKeyPath == "" {
		conf.RLA.ClientKeyPath = DefaultRLAClientKeyPath
	}

	log.Info().Msg("RLA Address:" + conf.RLA.Address)
	log.Info().Msg("RLA CA Path:" + conf.RLA.ServerCAPath)
	log.Info().Msg("RLA client Cert:" + conf.RLA.ClientCertPath)
	log.Info().Msg("RLA client Key:" + conf.RLA.ClientKeyPath)

	// General config
	flag.StringVar(&conf.MetricsPort, "metricsPort", os.Getenv("METRICS_PORT"), "Metrics port number")
	flag.StringVar(&conf.Temporal.Host, "temporalHost", os.Getenv("TEMPORAL_HOST"), "Temporal hostname/IP")
	flag.StringVar(&conf.Temporal.Port, "temporalPort", os.Getenv("TEMPORAL_PORT"), "Temporal port")
	flag.StringVar(&enableDebug, "EnableDebug", os.Getenv("ENABLE_DEBUG"), "Debug log level setting")
	flag.StringVar(&devmode, "DevMode", os.Getenv("DEV_MODE"), "Local development")
	flag.StringVar(&enableTLS, "EnableTLS", os.Getenv("ENABLE_TLS"), "Elable TLS based auth")
	flag.StringVar(&disableBootstrap, "DisableBootstrap", os.Getenv("DISABLE_BOOTSTRAP"), "Disable secret based bootstrap")
	flag.StringVar(&conf.BootstrapSecret, "bootstrapSecret", os.Getenv("BOOTSTRAP_SECRET"), "Bootstrap secret")
	flag.StringVar(&watcherInterval, "watcherInterval", os.Getenv("WATCHER_INTERVAL"), "Watcher Interval")
	flag.StringVar(&podName, "podName", os.Getenv("POD_NAME"), "POD Name")
	flag.StringVar(&conf.PodNamespace, "podNamespace", os.Getenv("POD_NAMESPACE"), "POD Namespace")
	flag.StringVar(&conf.TemporalSecret, "temporalSecret", os.Getenv("TEMPORAL_CERT"), "Temporal cert secret")
	flag.StringVar(&conf.CloudVersion, "cloudVersion", os.Getenv("CLOUD_WORKFLOW_VERSION"), "Cloud Workflow Proto version")
	flag.StringVar(&conf.SiteVersion, "siteVersion", os.Getenv("SITE_WORKFLOW_VERSION"), "Site Workflow Proto version")
	flag.StringVar(&skipServerAuth, "nicoSkipServerAuth", os.Getenv("SKIP_GRPC_SERVER_AUTH"), "Skip gRPC server auth in TLS")

	var skipRlaServerAuth string
	flag.StringVar(&skipRlaServerAuth, "rlaSkipServerAuth", os.Getenv("SKIP_RLA_GRPC_SERVER_AUTH"), "Skip RLA gRPC server auth in TLS")

	var rlaEnabled string
	flag.StringVar(&rlaEnabled, "rlaEnabled", os.Getenv("RLA_ENABLED"), "Enable RLA")

	if conf.MetricsPort == "" {
		log.Fatal().Msg("error loading config, invalid metrics port")
	}
	if conf.Temporal.Host == "" {
		log.Fatal().Msg("error loading config, Temporal host must be specified")
	}
	if conf.Temporal.Port == "" {
		log.Fatal().Msg("error loading config, invalid Temporal port")
	}
	if podName == "" {
		log.Fatal().Msg("error loading config, empty Pod Name")
	} else {
		conf.IsMasterPod = false
		parts := regexp.MustCompile(`(.*)-(\d+)$`).FindStringSubmatch(podName)
		if len(parts) == 3 {
			id, err := strconv.Atoi(parts[2])
			if err != nil {
				log.Fatal().Msgf("error loading config, invalid Pod Name %v %v", podName, err.Error())
			}
			if id == 0 {
				conf.IsMasterPod = true
			}
		} else {
			log.Fatal().Msgf("error loading config, invalid Pod Name %v", podName)
		}
	}
	if conf.PodNamespace == "" {
		log.Fatal().Msg("error loading config, empty Pod Namespace")
	}

	conf.EnableDebug = strings.ToLower(enableDebug) == "true"
	conf.DevMode = strings.ToLower(devmode) == "true"
	conf.EnableTLS = strings.ToLower(enableTLS) == "true"
	conf.DisableBootstrap = strings.ToLower(disableBootstrap) == "true"
	conf.NICo.SkipServerAuth = strings.ToLower(skipServerAuth) == "true"
	conf.RLA.SkipServerAuth = strings.ToLower(skipRlaServerAuth) == "true"
	conf.RLA.Enabled = strings.ToLower(rlaEnabled) == "true"

	// Initialize the WatcherInterval to default if not defined
	if watcherInterval == "" {
		watcherInterval = "10"
	}
	wi, err := strconv.Atoi(watcherInterval)
	if err != nil {
		log.Fatal().Msg(fmt.Sprint("invalid watcher interval", err))
	}
	// convert watcherInterval to Minutes
	conf.WatcherInterval = time.Duration(wi) * time.Minute

	if conf.BootstrapSecret == "" {
		conf.BootstrapSecret = "/etc/sitereg/"
	}

	// Site ID
	// TODO: Rename CLUSTER_ID to SITE_ID
	clusterID := ""
	if csi := os.Getenv("CLUSTER_ID"); csi != "" {
		clusterID = csi
	}
	_, err = uuid.Parse(clusterID)
	if err != nil {
		log.Fatal().Msg("error loading config, specified Cluster ID is not a UUID")
	}

	// Load the Temporal configuration from env vars
	var temporalPublishQueue string
	if mcq := os.Getenv("TEMPORAL_PUBLISH_QUEUE"); mcq != "" {
		temporalPublishQueue = mcq
	}

	var temporalSubscribeQueue string
	if msq := os.Getenv("TEMPORAL_SUBSCRIBE_QUEUE"); msq != "" {
		temporalSubscribeQueue = msq
	}

	var temporalPublishNamespace string
	if mcq := os.Getenv("TEMPORAL_PUBLISH_NAMESPACE"); mcq != "" {
		temporalPublishNamespace = mcq
	}

	temporalSubscribeNamespace := clusterID
	if msq := os.Getenv("TEMPORAL_SUBSCRIBE_NAMESPACE"); msq != "" {
		temporalSubscribeNamespace = msq
	}

	temporalCertPath := ""
	if msf := os.Getenv("TEMPORAL_CERT_PATH"); msf != "" {
		temporalCertPath = msf
	}

	flag.StringVar(&conf.Temporal.TemporalPublishQueue, "TemporalPublishQueue", temporalPublishQueue, "Temporal Publish queue")
	flag.StringVar(&conf.Temporal.TemporalSubscribeQueue, "TemporalSubscribeQueue", temporalSubscribeQueue, "Temporal Subscribe queue")
	flag.StringVar(&conf.Temporal.TemporalPublishNamespace, "TemporalPublishNamespace", temporalPublishNamespace, "Temporal Publish Namespace")
	flag.StringVar(&conf.Temporal.TemporalSubscribeNamespace, "TemporalSubscribeNamespace", temporalSubscribeNamespace, "Temporal Subscribe Namespace")
	flag.StringVar(&conf.Temporal.ClusterID, "ClusterID", clusterID, "NICo Site cluster ID")
	flag.StringVar(&conf.Temporal.TemporalCertPath, "TemporalCertPath", temporalCertPath, "Temporal cert path")
	flag.StringVar(&conf.Temporal.TemporalServer, "TemporalServer", os.Getenv("TEMPORAL_SERVER"), "Temporal server")
	flag.StringVar(&conf.Temporal.TemporalInventorySchedule, "TemporalInventorySchedule", os.Getenv("TEMPORAL_INVENTORY_SCHEDULE"), "Temporal Inventory schedule")

	if conf.Temporal.TemporalPublishQueue == "" {
		log.Fatal().Msg("error loading config, Temporal publish queue must be specified")
	}

	if conf.Temporal.TemporalSubscribeQueue == "" {
		log.Fatal().Msg("error loading config, Temporal subscribe queue must be specified")
	}

	log.Info().Interface("config", conf).Msg("Config Manager: Config loaded")
	flag.Parse()
	return conf
}

func determineEnvironment() conftypes.RunInEnvironment {
	// Check for env file presence at explicit location.
	_, err := os.Stat("../../config.env")
	if err != nil {
		log.Info().Msg("Config Manager: Could not find .env file, assuming Kubernetes environment")
		return conftypes.RunningInK8s
	}

	log.Info().Msg("Config Manager: Found .env file, assuming Docker environment")
	err = godotenv.Load("../../config.env")
	if err != nil {
		log.Info().Str("err", err.Error()).Msg("Config Manager: Failed to load .env file")
	} else {
		log.Info().Msg("Config Manager: Successfully loaded .env file")
	}

	return conftypes.RunningInDocker
}
