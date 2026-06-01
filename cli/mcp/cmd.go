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
	opts := buildServeOptions(c)

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

// buildServeOptions resolves the MCP server's start-up defaults from the
// process flags (each of which also reads its NICO_* environment
// variable). Unlike the dynamically-generated commands, mcp serve does
// NOT read ~/.nico/config.yaml: the server is stateless and entirely
// parameter-driven, so every connection detail is supplied per tool call
// via resolveCallConfig, with these flag values as the only fallback.
// This lets "nicocli mcp serve" start cleanly with no config file present.
func buildServeOptions(c *urfave.Context) Options {
	log := logrus.NewEntry(logrus.StandardLogger())
	if c.Bool("debug") {
		log.Logger.SetLevel(logrus.DebugLevel)
	}

	return Options{
		BaseURL:      c.String("base-url"),
		Org:          c.String("org"),
		APIName:      c.String("api-name"),
		Token:        c.String("token"),
		TokenCommand: c.String("token-command"),
		Debug:        c.Bool("debug"),
		Log:          log,
	}
}
