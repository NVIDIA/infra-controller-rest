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

// Package mcp serves the NICo REST read surface as MCP tools over
// streamable-HTTP. Tools are projected 1:1 from the embedded OpenAPI
// spec's GET operations. The server is stateless and never emits SSE:
// every tool/call returns a single application/json body.
package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	appcli "github.com/NVIDIA/infra-controller-rest/cli/pkg"
)

// BuildServer constructs an *mcp.Server with one tool registered for
// every GET operation in the supplied OpenAPI spec. Tool names follow
// the SDD: nico_<snake_case(operationId)>. Each tool handler builds a
// fresh appcli.Client per call from resolveCallConfig and forwards the
// bearer token from the inbound MCP request to NICo REST unchanged.
//
// BuildServer does not start a listener; callers wrap the result with
// NewHandler to get an *http.Handler ready for ListenAndServe.
func BuildServer(specData []byte, opts Options) (*mcp.Server, error) {
	spec, err := appcli.ParseSpec(specData)
	if err != nil {
		return nil, fmt.Errorf("parsing spec: %w", err)
	}
	opts = opts.withDefaults()

	server := mcp.NewServer(&mcp.Implementation{
		Name:    "nico-mcp",
		Title:   "NVIDIA Infrastructure Controller (NICo) MCP",
		Version: spec.Info.Version,
	}, nil)

	for _, path := range sortedPaths(spec) {
		item := spec.Paths[path]
		if item.Get == nil || item.Get.OperationID == "" {
			continue
		}
		registerGET(server, spec, path, item, opts)
	}
	return server, nil
}

// NewHandler wraps an *mcp.Server in a streamable-HTTP handler
// configured for stateless, request/response-only operation:
//
//   - Stateless: true   -- the SDK does not validate Mcp-Session-Id
//     and rejects server->client requests. initialize is a no-op.
//   - JSONResponse: true -- every tool/call response uses
//     Content-Type: application/json; the SDK never opens an SSE
//     stream. The data-hall deployment behind the Latinum Agent
//     Gateway Shard Proxy (NATS) requires this.
func NewHandler(server *mcp.Server) http.Handler {
	return mcp.NewStreamableHTTPHandler(
		func(*http.Request) *mcp.Server { return server },
		&mcp.StreamableHTTPOptions{
			Stateless:    true,
			JSONResponse: true,
		},
	)
}

// sortedPaths returns spec.Paths keys in deterministic order so the
// resulting tool list is stable across server restarts.
func sortedPaths(spec *appcli.Spec) []string {
	keys := make([]string, 0, len(spec.Paths))
	for k := range spec.Paths {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func registerGET(server *mcp.Server, spec *appcli.Spec, path string, item appcli.PathItem, opts Options) {
	op := item.Get
	allParams := mergeParameters(item, op)

	tool := &mcp.Tool{
		Name:        toolName(op.OperationID),
		Description: toolDescription(op),
		InputSchema: buildInputSchema(item, op),
		Annotations: &mcp.ToolAnnotations{
			ReadOnlyHint: true,
			Title:        op.Summary,
		},
	}

	mcp.AddTool(server, tool, func(ctx context.Context, req *mcp.CallToolRequest, in map[string]any) (*mcp.CallToolResult, any, error) {
		cfg, err := resolveCallConfig(in, req, opts)
		if err != nil {
			return errorResult(err), nil, nil
		}
		client := appcli.NewClient(cfg.BaseURL, cfg.Org, cfg.Token, opts.Log, opts.Debug)
		client.APIName = cfg.APIName
		if cfg.TokenRefresh != nil {
			client.TokenRefresh = cfg.TokenRefresh
		}

		pathParams, queryParams, err := splitArgs(in, allParams)
		if err != nil {
			return errorResult(err), nil, nil
		}
		body, _, err := client.Do(http.MethodGet, path, pathParams, queryParams, nil)
		if err != nil {
			return errorResult(err), nil, nil
		}
		return jsonResult(body), nil, nil
	})
}

// toolName converts an operationId like get-all-site to the SDD's
// canonical MCP tool name nico_get_all_site by replacing hyphens with
// underscores and prefixing with nico_.
func toolName(operationID string) string {
	return "nico_" + strings.ReplaceAll(operationID, "-", "_")
}

func toolDescription(op *appcli.Operation) string {
	parts := make([]string, 0, 2)
	if s := strings.TrimSpace(op.Summary); s != "" {
		parts = append(parts, s)
	}
	if s := strings.TrimSpace(op.Description); s != "" {
		parts = append(parts, s)
	}
	if len(parts) == 0 {
		return op.OperationID
	}
	return strings.Join(parts, "\n\n")
}

// splitArgs maps the tool input map onto path and query parameters
// using the OpenAPI parameter definitions. Common config keys (org,
// base_url, api_name, token) and any unrecognised keys are dropped:
// they are consumed by resolveCallConfig, not the URL builder. The
// "org" path parameter is intentionally skipped here because
// appcli.Client.Do substitutes {org} from Client.Org, which
// resolveCallConfig sets from the per-call override or config layer.
//
// TODO: full OpenAPI style/explode serialization for array and object
// parameters is intentionally deferred; unsupported shapes fail fast.
func splitArgs(in map[string]any, params []appcli.Parameter) (pathParams, queryParams map[string]string, err error) {
	pathParams = map[string]string{}
	queryParams = map[string]string{}
	for _, p := range params {
		if p.Name == "org" {
			continue
		}
		raw, ok := in[p.Name]
		if !ok {
			continue
		}
		s, ok := coerceToString(raw)
		if !ok {
			return nil, nil, fmt.Errorf("unsupported argument type for %q: %T", p.Name, raw)
		}
		if s == "" {
			continue
		}
		switch p.In {
		case "path":
			pathParams[p.Name] = s
		case "query":
			queryParams[p.Name] = s
		}
	}
	return pathParams, queryParams, nil
}

func coerceToString(v any) (string, bool) {
	switch t := v.(type) {
	case nil:
		return "", true
	case string:
		return t, true
	case bool:
		if t {
			return "true", true
		}
		return "false", true
	case float64:
		// JSON numbers decode to float64; format integers without the
		// decimal point so they round-trip through query strings.
		if t == float64(int64(t)) {
			return fmt.Sprintf("%d", int64(t)), true
		}
		return fmt.Sprintf("%g", t), true
	case int:
		return fmt.Sprintf("%d", t), true
	case int64:
		return fmt.Sprintf("%d", t), true
	case json.Number:
		return t.String(), true
	default:
		return "", false
	}
}

func errorResult(err error) *mcp.CallToolResult {
	return &mcp.CallToolResult{
		IsError: true,
		Content: []mcp.Content{&mcp.TextContent{Text: err.Error()}},
	}
}

func jsonResult(body []byte) *mcp.CallToolResult {
	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: string(body)}},
	}
}
