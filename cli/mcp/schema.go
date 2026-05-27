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
	"sort"

	"github.com/google/jsonschema-go/jsonschema"

	appcli "github.com/NVIDIA/infra-controller-rest/cli/pkg"
)

// commonConfigDescriptions documents the four per-call config overrides
// that are merged into every tool's input schema. Kept as a slice (not
// a map) so the schema render order is stable.
var commonConfigDescriptions = []struct {
	Name string
	Desc string
}{
	{"org", "Override the server's default org (api.org) for this call. If unset, the inbound bearer's claims plus server-side config decide the org."},
	{"base_url", "Override the server's default base URL (api.base) for this call. Useful when one MCP server fronts multiple NICo REST deployments."},
	{"api_name", "Override the API path segment used in /v2/org/<org>/<name>/... (api.name; default \"nico\")."},
	{"token", "Bearer token for this call. Overrides the inbound Authorization header. Omit in production behind agentgateway; the gateway-injected JWT is passed through automatically."},
}

// buildInputSchema produces a JSON Schema describing a tool's input:
// OpenAPI path and query parameters merged with the four common config
// override fields (org, base_url, api_name, token). Path parameters are
// marked required; OpenAPI-required query parameters are marked
// required; the config overrides are always optional.
func buildInputSchema(item appcli.PathItem, op *appcli.Operation) *jsonschema.Schema {
	props := map[string]*jsonschema.Schema{}
	var required []string

	allParams := append([]appcli.Parameter{}, item.Parameters...)
	allParams = append(allParams, op.Parameters...)
	for _, p := range allParams {
		if p.Name == "org" {
			// Resolved from the config layer (org override or server
			// default). The OpenAPI {org} segment is filled in by
			// appcli.Client.Do.
			continue
		}
		if p.In != "path" && p.In != "query" {
			continue
		}
		props[p.Name] = paramToJSONSchema(p)
		if p.In == "path" || p.Required {
			required = append(required, p.Name)
		}
	}

	for _, c := range commonConfigDescriptions {
		if _, exists := props[c.Name]; exists {
			continue
		}
		props[c.Name] = &jsonschema.Schema{
			Type:        "string",
			Description: c.Desc,
		}
	}

	sort.Strings(required)
	return &jsonschema.Schema{
		Type:       "object",
		Properties: props,
		Required:   required,
	}
}

// paramToJSONSchema converts a single OpenAPI parameter to a JSON
// schema fragment. Types are normalised to integer/boolean/number/
// string; everything else falls back to string. Enums are preserved
// where present.
func paramToJSONSchema(p appcli.Parameter) *jsonschema.Schema {
	s := &jsonschema.Schema{Description: p.Description}
	if p.Schema == nil {
		s.Type = "string"
		return s
	}
	switch p.Schema.Type {
	case "integer":
		s.Type = "integer"
	case "boolean":
		s.Type = "boolean"
	case "number":
		s.Type = "number"
	default:
		s.Type = "string"
	}
	if len(p.Schema.Enum) > 0 {
		s.Enum = make([]any, 0, len(p.Schema.Enum))
		for _, e := range p.Schema.Enum {
			s.Enum = append(s.Enum, e)
		}
	}
	return s
}
