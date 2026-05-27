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
	"testing"

	"github.com/stretchr/testify/require"

	appcli "github.com/NVIDIA/infra-controller-rest/cli/pkg"
)

func TestBuildInputSchema_OnlyConfigFields(t *testing.T) {
	schema := buildInputSchema(appcli.PathItem{}, &appcli.Operation{OperationID: "get-metadata"})
	require.Equal(t, "object", schema.Type)
	for _, c := range commonConfigDescriptions {
		require.Contains(t, schema.Properties, c.Name, "missing common config field %s", c.Name)
		require.Equal(t, "string", schema.Properties[c.Name].Type)
	}
	require.Empty(t, schema.Required, "common config fields should never be required")
}

func TestBuildInputSchema_PathAndQuery(t *testing.T) {
	item := appcli.PathItem{
		Parameters: []appcli.Parameter{
			{Name: "org", In: "path", Required: true, Schema: &appcli.Schema{Type: "string"}},
			{Name: "siteId", In: "path", Required: true, Schema: &appcli.Schema{Type: "string"}},
		},
	}
	op := &appcli.Operation{
		OperationID: "get-site-status-history",
		Parameters: []appcli.Parameter{
			{Name: "pageNumber", In: "query", Schema: &appcli.Schema{Type: "integer"}},
			{Name: "pageSize", In: "query", Schema: &appcli.Schema{Type: "integer"}},
			{Name: "status", In: "query", Schema: &appcli.Schema{Type: "string", Enum: []string{"ACTIVE", "INACTIVE"}}, Required: true},
		},
	}

	schema := buildInputSchema(item, op)

	require.Equal(t, "object", schema.Type)
	// org as a path parameter is dropped because Client.Do fills the
	// {org} segment from cfg.Org. It is then re-added by the common
	// config layer as an optional override, so the schema does carry
	// "org" -- but it must NOT be required and must carry the config-
	// override description, not the OpenAPI path-param description.
	require.Contains(t, schema.Properties, "org")
	require.NotContains(t, schema.Required, "org")
	require.Equal(t, commonConfigDescriptions[0].Desc, schema.Properties["org"].Description)
	require.Contains(t, schema.Properties, "siteId")
	require.Equal(t, "string", schema.Properties["siteId"].Type)
	require.Contains(t, schema.Properties, "pageNumber")
	require.Equal(t, "integer", schema.Properties["pageNumber"].Type)
	require.Contains(t, schema.Properties, "pageSize")
	require.Equal(t, "integer", schema.Properties["pageSize"].Type)
	require.Contains(t, schema.Properties, "status")
	require.Equal(t, []any{"ACTIVE", "INACTIVE"}, schema.Properties["status"].Enum)

	// Path params + Required:true query params are required; pure
	// query params are not.
	require.ElementsMatch(t, []string{"siteId", "status"}, schema.Required)

	// Config-layer fields still merged in.
	for _, c := range commonConfigDescriptions {
		require.Contains(t, schema.Properties, c.Name)
	}
}

func TestBuildInputSchema_ConfigArgDoesNotOverrideOpenAPIParam(t *testing.T) {
	// If an OpenAPI spec accidentally declares a query param named
	// "token", the OpenAPI definition wins -- we never overwrite a
	// real parameter with the common-config placeholder.
	op := &appcli.Operation{
		OperationID: "get-foo",
		Parameters: []appcli.Parameter{
			{Name: "token", In: "query", Description: "API-specific token query param", Schema: &appcli.Schema{Type: "string"}},
		},
	}
	schema := buildInputSchema(appcli.PathItem{}, op)
	require.Contains(t, schema.Properties, "token")
	require.Equal(t, "API-specific token query param", schema.Properties["token"].Description)
}

func TestParamToJSONSchema_TypeMapping(t *testing.T) {
	cases := []struct {
		openapiType string
		want        string
	}{
		{"string", "string"},
		{"integer", "integer"},
		{"number", "number"},
		{"boolean", "boolean"},
		{"unknown", "string"},
		{"", "string"},
	}
	for _, c := range cases {
		t.Run(c.openapiType, func(t *testing.T) {
			s := paramToJSONSchema(appcli.Parameter{Name: "x", Schema: &appcli.Schema{Type: appcli.SchemaType(c.openapiType)}})
			require.Equal(t, c.want, s.Type)
		})
	}
}

func TestParamToJSONSchema_NoSchemaDefaultsToString(t *testing.T) {
	s := paramToJSONSchema(appcli.Parameter{Name: "x", Description: "no schema"})
	require.Equal(t, "string", s.Type)
	require.Equal(t, "no schema", s.Description)
}
