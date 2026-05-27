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
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	appcli "github.com/NVIDIA/infra-controller-rest/cli/pkg"
)

func TestToolName(t *testing.T) {
	cases := []struct {
		operationID string
		want        string
	}{
		{"get-metadata", "nico_get_metadata"},
		{"get-all-site", "nico_get_all_site"},
		{"get-site-status-history", "nico_get_site_status_history"},
		{"get-current-tenant", "nico_get_current_tenant"},
		{"validate-rack", "nico_validate_rack"},
		{"validate-trays", "nico_validate_trays"},
	}
	for _, c := range cases {
		t.Run(c.operationID, func(t *testing.T) {
			require.Equal(t, c.want, toolName(c.operationID))
		})
	}
}

func TestToolDescription(t *testing.T) {
	t.Run("summary and description", func(t *testing.T) {
		op := &appcli.Operation{
			OperationID: "get-foo",
			Summary:     "Retrieve Foo",
			Description: "More detail on Foo.",
		}
		require.Equal(t, "Retrieve Foo\n\nMore detail on Foo.", toolDescription(op))
	})
	t.Run("summary only", func(t *testing.T) {
		op := &appcli.Operation{OperationID: "get-foo", Summary: "Retrieve Foo"}
		require.Equal(t, "Retrieve Foo", toolDescription(op))
	})
	t.Run("operationID fallback", func(t *testing.T) {
		op := &appcli.Operation{OperationID: "get-foo"}
		require.Equal(t, "get-foo", toolDescription(op))
	})
}

func TestSplitArgs(t *testing.T) {
	params := []appcli.Parameter{
		{Name: "org", In: "path"},
		{Name: "siteId", In: "path"},
		{Name: "pageNumber", In: "query"},
		{Name: "pageSize", In: "query"},
	}
	in := map[string]any{
		"siteId":     "abc-123",
		"pageNumber": float64(5),
		"pageSize":   float64(50),
		"org":        "should-not-appear-here",
		"token":      "should-be-ignored-by-splitArgs",
		"base_url":   "should-be-ignored",
		"unknown":    "ignored",
	}
	pathParams, queryParams := splitArgs(in, params)
	require.Equal(t, map[string]string{"siteId": "abc-123"}, pathParams)
	require.Equal(t, map[string]string{"pageNumber": "5", "pageSize": "50"}, queryParams)
}

func TestCoerceToString(t *testing.T) {
	cases := []struct {
		name string
		in   any
		want string
	}{
		{"string", "foo", "foo"},
		{"empty_string", "", ""},
		{"int_as_float64", float64(42), "42"},
		{"negative_int_as_float64", float64(-3), "-3"},
		{"float_with_fraction", float64(3.14), "3.14"},
		{"bool_true", true, "true"},
		{"bool_false", false, "false"},
		{"int", 7, "7"},
		{"int64", int64(99), "99"},
		{"json_number", json.Number("12345"), "12345"},
		{"nil", nil, ""},
		{"unsupported_slice", []int{1, 2}, ""},
		{"unsupported_map", map[string]any{"a": 1}, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			require.Equal(t, c.want, coerceToString(c.in))
		})
	}
}

func TestSortedPaths(t *testing.T) {
	spec := &appcli.Spec{
		Paths: map[string]appcli.PathItem{
			"/z": {}, "/a": {}, "/m": {},
		},
	}
	require.Equal(t, []string{"/a", "/m", "/z"}, sortedPaths(spec))
}

// TestBuildServer_SyntheticSpec exercises BuildServer end-to-end on a
// hand-crafted YAML spec to assert tool registration does not panic and
// no error escapes for any combination of GET, POST, and parameterless
// path items. The downstream tool list is verified at the HTTP layer
// in transport_test.go.
func TestBuildServer_SyntheticSpec(t *testing.T) {
	specYAML := []byte(`
openapi: 3.0.0
info:
  title: Test
  version: 0.0.1
paths:
  /v2/org/{org}/nico/foo:
    parameters:
      - name: org
        in: path
        required: true
        schema: {type: string}
    get:
      operationId: get-all-foo
      summary: List foos
      parameters:
        - name: pageSize
          in: query
          schema: {type: integer}
  /v2/org/{org}/nico/foo/{fooId}:
    parameters:
      - name: org
        in: path
        required: true
        schema: {type: string}
      - name: fooId
        in: path
        required: true
        schema: {type: string}
    get:
      operationId: get-foo
      summary: Retrieve a foo
  /v2/org/{org}/nico/foo/{fooId}/status-history:
    parameters:
      - name: org
        in: path
        required: true
        schema: {type: string}
      - name: fooId
        in: path
        required: true
        schema: {type: string}
    get:
      operationId: get-foo-status-history
      summary: Foo status history
  /v2/org/{org}/nico/skip:
    parameters:
      - name: org
        in: path
        required: true
    post:
      operationId: create-skip
      summary: Excluded mutation
`)

	server, err := BuildServer(specYAML, Options{BaseURL: "http://example.test", Org: "demo"})
	require.NoError(t, err)
	require.NotNil(t, server)
}

func TestBuildServer_RejectsInvalidSpec(t *testing.T) {
	_, err := BuildServer([]byte("not: valid: yaml: ::"), Options{})
	require.Error(t, err)
}
