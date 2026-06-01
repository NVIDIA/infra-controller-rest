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

package mcp

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/sirupsen/logrus"
	urfave "github.com/urfave/cli/v2"

	appcli "github.com/NVIDIA/infra-controller-rest/cli/pkg"
)

// Command returns the "mcp" urfave/cli command tree for nicocli. Wire
// it into the binary's command list from cmd/cli/main.go alongside the
// dynamically-generated commands the rest of the CLI ships with.
//
// specData is the OpenAPI YAML the rest of the CLI is built from; the
// command's "serve" action passes it to BuildServer so the MCP tool
// catalogue stays in lockstep with every nicocli build.
func Command(specData []byte) *urfave.Command {
	return &urfave.Command{
		Name:  "mcp",
		Usage: "Run an MCP server that exposes the NICo REST read surface over streamable-HTTP",
		Subcommands: []*urfave.Command{
			serveCommand(specData),
		},
	}
}

func serveCommand(specData []byte) *urfave.Command {
	return &urfave.Command{
		Name:  "serve",
		Usage: "Start the streamable-HTTP MCP server",
		Description: "Serves the NICo REST read surface as MCP tools at /mcp on the\n" +
			"configured listen address. The server is stateless and never emits\n" +
			"text/event-stream responses; every tool/call returns a single JSON\n" +
			"body. In production, place an MCP-aware gateway in front and rely on\n" +
			"the inbound Authorization header for per-call authentication.",
		Flags: []urfave.Flag{
			&urfave.StringFlag{
				Name:    "listen",
				Usage:   "address:port to listen on",
				EnvVars: []string{"NICO_MCP_LISTEN"},
				Value:   ":8080",
			},
			&urfave.StringFlag{
				Name:    "path",
				Usage:   "HTTP path prefix the MCP handler is mounted at",
				EnvVars: []string{"NICO_MCP_PATH"},
				Value:   "/mcp",
			},
			&urfave.DurationFlag{
				Name:    "shutdown-timeout",
				Usage:   "graceful shutdown timeout when SIGINT/SIGTERM arrives",
				EnvVars: []string{"NICO_MCP_SHUTDOWN_TIMEOUT"},
				Value:   10 * time.Second,
			},
		},
		Action: func(c *urfave.Context) error {
			return runServe(c, specData)
		},
	}
}

// runServe wires the urfave context into Options, builds the MCP server,
// and runs an http.Server until SIGINT/SIGTERM. It is split out from the
// urfave Action closure so tests can drive it directly.
func runServe(c *urfave.Context, specData []byte) error {
	opts, err := buildServeOptions(c)
	if err != nil {
		return err
	}

	server, err := BuildServer(specData, opts)
	if err != nil {
		return fmt.Errorf("building MCP server: %w", err)
	}

	listen := c.String("listen")
	path := c.String("path")
	if path == "" || path[0] != '/' {
		return fmt.Errorf("invalid --path %q: must be non-empty and start with '/'", path)
	}
	shutdownTimeout := c.Duration("shutdown-timeout")

	mux := http.NewServeMux()
	mux.Handle(path, NewHandler(server))

	httpServer := &http.Server{
		Addr:              listen,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}

	opts.Log.Infof("nico-mcp: listening on %s, MCP at %s (stateless, JSONResponse)", listen, path)

	errCh := make(chan error, 1)
	go func() {
		errCh <- httpServer.ListenAndServe()
	}()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(sigCh)

	select {
	case err := <-errCh:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			return fmt.Errorf("http server: %w", err)
		}
		return nil
	case sig := <-sigCh:
		opts.Log.Infof("nico-mcp: received %s, shutting down", sig)
		ctx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()
		if err := httpServer.Shutdown(ctx); err != nil {
			return fmt.Errorf("graceful shutdown: %w", err)
		}
		return nil
	}
}

// buildServeOptions resolves the urfave context into Options by layering
// app-global flags on top of the loaded config file, mirroring what the
// dynamically-generated commands do via clientFromContext.
func buildServeOptions(c *urfave.Context) (Options, error) {
	cfg, err := appcli.LoadConfig()
	if err != nil {
		return Options{}, fmt.Errorf("loading config: %w", err)
	}
	appcli.ApplyEnvOverrides(cfg)

	baseURL := cfg.API.Base
	if c.IsSet("base-url") {
		baseURL = c.String("base-url")
	}
	if baseURL == "" {
		baseURL = c.String("base-url")
	}

	org := cfg.API.Org
	if c.IsSet("org") {
		org = c.String("org")
	}

	apiName := cfg.API.Name
	if c.IsSet("api-name") {
		apiName = c.String("api-name")
	}
	if apiName == "" {
		apiName = "nico"
	}

	token := ""
	if c.IsSet("token") {
		token = c.String("token")
	} else if cfg.Auth.Token != "" {
		token = cfg.Auth.Token
	}

	tokenCommand := ""
	if c.IsSet("token-command") {
		tokenCommand = c.String("token-command")
	} else if cfg.Auth.TokenCommand != "" {
		tokenCommand = cfg.Auth.TokenCommand
	}

	log := logrus.NewEntry(logrus.StandardLogger())
	if c.Bool("debug") {
		log.Logger.SetLevel(logrus.DebugLevel)
	}

	return Options{
		BaseURL:      baseURL,
		Org:          org,
		APIName:      apiName,
		Token:        token,
		TokenCommand: tokenCommand,
		Debug:        c.Bool("debug"),
		Log:          log,
	}, nil
}
