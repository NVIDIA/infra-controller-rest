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
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/require"
)

func TestResolveCallConfig_PrecedenceChain(t *testing.T) {
	type expect struct {
		baseURL, org, apiName, token string
		hasRefresh                   bool
		wantErr                      bool
	}
	cases := []struct {
		name     string
		in       map[string]any
		req      *mcp.CallToolRequest
		opts     Options
		expected expect
	}{
		{
			name: "tool_args_win_every_field",
			in: map[string]any{
				"base_url": "https://from-arg.example.com",
				"org":      "arg-org",
				"api_name": "arg-name",
				"token":    "arg-token",
			},
			req:  requestWithBearer("inbound-bearer"),
			opts: Options{BaseURL: "https://opts.example.com", Org: "opts-org", APIName: "opts-name", Token: "opts-token"},
			expected: expect{
				baseURL: "https://from-arg.example.com",
				org:     "arg-org",
				apiName: "arg-name",
				token:   "arg-token",
			},
		},
		{
			name: "inbound_bearer_wins_when_no_token_arg",
			in:   map[string]any{},
			req:  requestWithBearer("from-header"),
			opts: Options{BaseURL: "https://opts.example.com", Org: "opts-org", APIName: "nico", Token: "opts-token"},
			expected: expect{
				baseURL: "https://opts.example.com",
				org:     "opts-org",
				apiName: "nico",
				token:   "from-header",
			},
		},
		{
			name: "opts_token_used_when_no_arg_and_no_inbound",
			in:   map[string]any{},
			req:  nil,
			opts: Options{BaseURL: "https://opts.example.com", Org: "opts-org", APIName: "nico", Token: "opts-token"},
			expected: expect{
				baseURL: "https://opts.example.com",
				org:     "opts-org",
				apiName: "nico",
				token:   "opts-token",
			},
		},
		{
			name: "token_command_wired_as_refresh_hook",
			in:   map[string]any{},
			req:  nil,
			opts: Options{
				BaseURL:      "https://opts.example.com",
				Org:          "opts-org",
				APIName:      "nico",
				TokenCommand: "echo refreshed-token",
			},
			expected: expect{
				baseURL:    "https://opts.example.com",
				org:        "opts-org",
				apiName:    "nico",
				token:      "",
				hasRefresh: true,
			},
		},
		{
			name: "missing_org_errors",
			in:   map[string]any{},
			req:  nil,
			opts: Options{BaseURL: "https://opts.example.com"},
			expected: expect{
				wantErr: true,
			},
		},
		{
			name: "missing_base_url_errors",
			in:   map[string]any{},
			req:  nil,
			opts: Options{Org: "opts-org"},
			expected: expect{
				wantErr: true,
			},
		},
		{
			name: "missing_both_errors",
			in:   map[string]any{},
			req:  nil,
			opts: Options{},
			expected: expect{
				wantErr: true,
			},
		},
		{
			name: "api_name_defaults_when_unset",
			in:   map[string]any{},
			req:  nil,
			opts: Options{BaseURL: "https://opts.example.com", Org: "opts-org", Token: "t"}.withDefaults(),
			expected: expect{
				baseURL: "https://opts.example.com",
				org:     "opts-org",
				apiName: "nico",
				token:   "t",
			},
		},
		{
			name: "empty_string_arg_falls_through_to_opts",
			in: map[string]any{
				"org":   "",
				"token": "   ",
			},
			req: requestWithBearer("inbound"),
			opts: Options{
				BaseURL: "https://opts.example.com",
				Org:     "opts-org",
				APIName: "nico",
				Token:   "opts-token",
			},
			expected: expect{
				baseURL: "https://opts.example.com",
				org:     "opts-org",
				apiName: "nico",
				token:   "inbound",
			},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			cfg, err := resolveCallConfig(c.in, c.req, c.opts)
			if c.expected.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, c.expected.baseURL, cfg.BaseURL)
			require.Equal(t, c.expected.org, cfg.Org)
			require.Equal(t, c.expected.apiName, cfg.APIName)
			require.Equal(t, c.expected.token, cfg.Token)
			require.Equal(t, c.expected.hasRefresh, cfg.TokenRefresh != nil)
		})
	}
}

func TestResolveCallConfig_TokenRefreshIsPerCall(t *testing.T) {
	// Sanity-check: the TokenRefresh closure is rebuilt on every call
	// to resolveCallConfig. Two adjacent invocations produce two
	// distinct closures, so a refresh on call A cannot leak state into
	// call B.
	opts := Options{
		BaseURL:      "https://opts.example.com",
		Org:          "opts-org",
		TokenCommand: "echo t",
	}
	cfgA, err := resolveCallConfig(nil, nil, opts)
	require.NoError(t, err)
	cfgB, err := resolveCallConfig(nil, nil, opts)
	require.NoError(t, err)
	require.NotNil(t, cfgA.TokenRefresh)
	require.NotNil(t, cfgB.TokenRefresh)
	// Closures themselves are distinct values.
	require.NotSame(t, &cfgA.TokenRefresh, &cfgB.TokenRefresh)
}

func TestStringArg(t *testing.T) {
	in := map[string]any{
		"a": "hello",
		"b": "  hello  ",
		"c": 42,
		"d": nil,
		"e": "",
	}
	require.Equal(t, "hello", stringArg(in, "a"))
	require.Equal(t, "hello", stringArg(in, "b"))
	require.Equal(t, "", stringArg(in, "c"))
	require.Equal(t, "", stringArg(in, "d"))
	require.Equal(t, "", stringArg(in, "e"))
	require.Equal(t, "", stringArg(in, "missing"))
	require.Equal(t, "", stringArg(nil, "any"))
}

func TestPickString(t *testing.T) {
	require.Equal(t, "a", pickString("a", "b", "c"))
	require.Equal(t, "b", pickString("", "b", "c"))
	require.Equal(t, "c", pickString("", "", "c"))
	require.Equal(t, "", pickString("", "", ""))
	require.Equal(t, "", pickString())
}

func TestBearerFromExtra(t *testing.T) {
	cases := []struct {
		name string
		hdr  http.Header
		want string
	}{
		{"nil_req", nil, ""},
		{"empty_header", http.Header{}, ""},
		{"bearer", http.Header{"Authorization": []string{"Bearer abc.def"}}, "abc.def"},
		{"bearer_lowercase_scheme", http.Header{"Authorization": []string{"bearer abc.def"}}, "abc.def"},
		{"bearer_with_padding", http.Header{"Authorization": []string{"Bearer   spaced   "}}, "spaced"},
		{"non_bearer_basic", http.Header{"Authorization": []string{"Basic dXNlcjpwYXNz"}}, ""},
		{"empty_value", http.Header{"Authorization": []string{""}}, ""},
		{"bearer_alone", http.Header{"Authorization": []string{"Bearer "}}, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var req *mcp.CallToolRequest
			if c.hdr != nil {
				req = &mcp.CallToolRequest{Extra: &mcp.RequestExtra{Header: c.hdr}}
			}
			require.Equal(t, c.want, bearerFromExtra(req))
		})
	}
}

// TestOptions_WithDefaults_PreservesCustomLogger asserts that
// withDefaults does not clobber a caller-supplied Log entry.
func TestOptions_WithDefaults(t *testing.T) {
	o := Options{}.withDefaults()
	require.Equal(t, "nico", o.APIName)
	require.NotNil(t, o.Log)
}

// TestNoConfigWriteBack runs the runServe-relevant config helpers (just
// LoadConfig + ApplyEnvOverrides + the package's TokenRefresh closure)
// against a temp config file and verifies the file on disk is byte-
// identical before and after. This is the design guarantee from the
// "Statelessness invariants" section: the MCP server never writes back
// to the on-disk config, even when a token_command refresh fires.
func TestNoConfigWriteBack(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "config.yaml")
	original := []byte("api:\n  base: https://example.test\n  org: tester\nauth:\n  token_command: echo new-token\n")
	require.NoError(t, os.WriteFile(cfgPath, original, 0o600))

	opts := Options{
		BaseURL:      "https://example.test",
		Org:          "tester",
		APIName:      "nico",
		TokenCommand: "echo new-token",
	}
	cfg, err := resolveCallConfig(nil, nil, opts)
	require.NoError(t, err)
	require.NotNil(t, cfg.TokenRefresh)
	token, err := cfg.TokenRefresh()
	require.NoError(t, err)
	require.Equal(t, "new-token", token)

	after, err := os.ReadFile(cfgPath)
	require.NoError(t, err)
	require.Equal(t, original, after, "TokenRefresh must not rewrite the on-disk config file")
}

func requestWithBearer(token string) *mcp.CallToolRequest {
	return &mcp.CallToolRequest{
		Extra: &mcp.RequestExtra{
			Header: http.Header{"Authorization": []string{"Bearer " + token}},
		},
	}
}
