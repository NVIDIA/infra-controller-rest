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
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
)

// synthSpec is a minimal three-operation OpenAPI document that drives
// the integration tests. The org path param is required because that's
// the real shape of NICo REST routes (/v2/org/{org}/...).
const synthSpec = `
openapi: 3.0.0
info:
  title: SynthNICo
  version: 0.0.1
paths:
  /v2/org/{org}/nico/foo:
    parameters:
      - {name: org, in: path, required: true, schema: {type: string}}
    get:
      operationId: get-all-foo
      summary: List foos
      parameters:
        - {name: pageSize, in: query, schema: {type: integer}}
  /v2/org/{org}/nico/foo/{fooId}:
    parameters:
      - {name: org, in: path, required: true, schema: {type: string}}
      - {name: fooId, in: path, required: true, schema: {type: string}}
    get:
      operationId: get-foo
      summary: Retrieve a foo
  /v2/org/{org}/nico/foo/{fooId}/status-history:
    parameters:
      - {name: org, in: path, required: true, schema: {type: string}}
      - {name: fooId, in: path, required: true, schema: {type: string}}
    get:
      operationId: get-foo-status-history
      summary: Foo status history
`

func TestHandler_RejectsLongPollGET(t *testing.T) {
	server, err := BuildServer([]byte(synthSpec), Options{BaseURL: "http://example.test", Org: "x"})
	require.NoError(t, err)
	ts := httptest.NewServer(NewHandler(server))
	defer ts.Close()

	req, err := http.NewRequest(http.MethodGet, ts.URL, nil)
	require.NoError(t, err)
	req.Header.Set("Accept", "text/event-stream")
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	// In Stateless mode the SDK rejects long-poll GETs because there is
	// no session for the server to push notifications onto. The exact
	// status code is an SDK choice; we assert it is a non-success
	// response so a future SDK change is caught.
	require.GreaterOrEqual(t, resp.StatusCode, http.StatusBadRequest,
		"long-poll GET on /mcp must be rejected in stateless mode (got %d)", resp.StatusCode)
}

func TestHandler_ToolsListAndJSONResponse(t *testing.T) {
	server, err := BuildServer([]byte(synthSpec), Options{BaseURL: "http://example.test", Org: "x"})
	require.NoError(t, err)
	ts := httptest.NewServer(NewHandler(server))
	defer ts.Close()

	resp := mcpPost(t, ts.URL, "", jsonrpcRequest(1, "tools/list", map[string]any{}))
	defer resp.Body.Close()

	require.Equal(t, http.StatusOK, resp.StatusCode)
	ctype := resp.Header.Get("Content-Type")
	require.True(t, strings.HasPrefix(ctype, "application/json"),
		"response Content-Type must be application/json, never text/event-stream (got %q)", ctype)

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	tools := decodeToolList(t, body)

	wantNames := []string{
		"nico_get_all_foo",
		"nico_get_foo",
		"nico_get_foo_status_history",
	}
	gotNames := make([]string, 0, len(tools))
	for _, tool := range tools {
		gotNames = append(gotNames, tool.Name)
	}
	require.ElementsMatch(t, wantNames, gotNames)
}

func TestHandler_ToolsCall_BearerPassthrough(t *testing.T) {
	var (
		recordedAuth atomic.Value
		recordedPath atomic.Value
	)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		recordedAuth.Store(r.Header.Get("Authorization"))
		recordedPath.Store(r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"foo-1","name":"Foo One"}`))
	}))
	defer upstream.Close()

	server, err := BuildServer([]byte(synthSpec), Options{
		BaseURL: upstream.URL,
		Org:     "tester",
		APIName: "nico",
	})
	require.NoError(t, err)
	ts := httptest.NewServer(NewHandler(server))
	defer ts.Close()

	resp := mcpPost(t, ts.URL, "Bearer caller-jwt-xyz", jsonrpcRequest(2, "tools/call", map[string]any{
		"name":      "nico_get_foo",
		"arguments": map[string]any{"fooId": "foo-1"},
	}))
	defer resp.Body.Close()

	require.Equal(t, http.StatusOK, resp.StatusCode)
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	result := decodeToolCallResult(t, body)
	require.False(t, result.IsError, "tool call should succeed: %s", body)
	require.Equal(t, `{"id":"foo-1","name":"Foo One"}`, firstText(result))

	require.Equal(t, "Bearer caller-jwt-xyz", recordedAuth.Load())
	require.Equal(t, "/v2/org/tester/nico/foo/foo-1", recordedPath.Load())
}

func TestHandler_ToolsCall_TokenArgWins(t *testing.T) {
	var recordedAuth atomic.Value
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		recordedAuth.Store(r.Header.Get("Authorization"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[]`))
	}))
	defer upstream.Close()

	server, err := BuildServer([]byte(synthSpec), Options{
		BaseURL: upstream.URL,
		Org:     "tester",
		APIName: "nico",
	})
	require.NoError(t, err)
	ts := httptest.NewServer(NewHandler(server))
	defer ts.Close()

	resp := mcpPost(t, ts.URL, "Bearer inbound-bearer", jsonrpcRequest(3, "tools/call", map[string]any{
		"name": "nico_get_all_foo",
		"arguments": map[string]any{
			"token": "explicit-tool-arg-token",
		},
	}))
	defer resp.Body.Close()

	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Equal(t, "Bearer explicit-tool-arg-token", recordedAuth.Load())
}

// TestHandler_ConcurrentCallersDoNotBleedTokens proves the
// statelessness invariant: two callers hitting the same handler with
// different bearers each get their own bearer forwarded to NICo REST.
// Run in parallel to also stress for shared-state races.
func TestHandler_ConcurrentCallersDoNotBleedTokens(t *testing.T) {
	t.Parallel()

	var (
		mu      sync.Mutex
		perPath = map[string]string{}
	)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		perPath[r.URL.Path] = r.Header.Get("Authorization")
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{}`))
	}))
	defer upstream.Close()

	server, err := BuildServer([]byte(synthSpec), Options{
		BaseURL: upstream.URL,
		Org:     "tester",
		APIName: "nico",
	})
	require.NoError(t, err)
	ts := httptest.NewServer(NewHandler(server))
	defer ts.Close()

	const callers = 8
	wg := sync.WaitGroup{}
	for i := 0; i < callers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			id := i + 100
			resp := mcpPost(t, ts.URL,
				"Bearer caller-"+itoa(i),
				jsonrpcRequest(id, "tools/call", map[string]any{
					"name": "nico_get_foo",
					"arguments": map[string]any{
						"fooId": "foo-" + itoa(i),
					},
				}))
			defer resp.Body.Close()
			require.Equal(t, http.StatusOK, resp.StatusCode)
		}(i)
	}
	wg.Wait()

	mu.Lock()
	defer mu.Unlock()
	require.Len(t, perPath, callers, "each caller should have hit a distinct path")
	for path, auth := range perPath {
		// path looks like /v2/org/tester/nico/foo/foo-<i>; auth must
		// match its caller's bearer.
		idx := strings.TrimPrefix(path, "/v2/org/tester/nico/foo/foo-")
		require.Equal(t, "Bearer caller-"+idx, auth, "bearer leaked between callers on %s", path)
	}
}

// TestHandler_TokenCommandRefreshIsPerCall drives two unrelated tool
// calls through a server configured with a token_command but no static
// token. The upstream returns 401 whenever no Authorization header is
// present, so each call's 401 fires Client.TokenRefresh exactly once.
// The shell counter file proves token_command ran twice -- once per
// tool call -- with no cross-call reuse.
func TestHandler_TokenCommandRefreshIsPerCall(t *testing.T) {
	t.Parallel()

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") == "" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer upstream.Close()

	tmpDir := t.TempDir()
	counterPath := filepath.Join(tmpDir, "counter")
	// printf >> appends one line per invocation; we read line count
	// after the test to learn how many times the shell ran.
	tokenCmd := "printf 'x\\n' >> " + shellQuote(counterPath) + " && printf '%s' refreshed-token"

	server, err := BuildServer([]byte(synthSpec), Options{
		BaseURL:      upstream.URL,
		Org:          "tester",
		APIName:      "nico",
		TokenCommand: tokenCmd,
	})
	require.NoError(t, err)
	ts := httptest.NewServer(NewHandler(server))
	defer ts.Close()

	for i := 0; i < 2; i++ {
		resp := mcpPost(t, ts.URL, "", jsonrpcRequest(i+1, "tools/call", map[string]any{
			"name":      "nico_get_foo",
			"arguments": map[string]any{"fooId": "foo-" + itoa(i)},
		}))
		_ = resp.Body.Close()
	}

	count := countLines(t, counterPath)
	require.GreaterOrEqual(t, count, 2,
		"token_command must run at least once per tool call (got %d invocations)", count)
}

// --- helpers below ---

func mcpPost(t *testing.T, base string, authorization string, body []byte) *http.Response {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, base, bytes.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	if authorization != "" {
		req.Header.Set("Authorization", authorization)
	}
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	return resp
}

func jsonrpcRequest(id int, method string, params map[string]any) []byte {
	type wrapper struct {
		JSONRPC string         `json:"jsonrpc"`
		ID      int            `json:"id"`
		Method  string         `json:"method"`
		Params  map[string]any `json:"params"`
	}
	b, err := json.Marshal(wrapper{
		JSONRPC: "2.0",
		ID:      id,
		Method:  method,
		Params:  params,
	})
	if err != nil {
		panic(err)
	}
	return b
}

type rpcTool struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

func decodeToolList(t *testing.T, body []byte) []rpcTool {
	t.Helper()
	var env struct {
		Result struct {
			Tools []rpcTool `json:"tools"`
		} `json:"result"`
	}
	require.NoError(t, json.Unmarshal(body, &env))
	return env.Result.Tools
}

type rpcContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type rpcToolCallResult struct {
	IsError bool         `json:"isError"`
	Content []rpcContent `json:"content"`
}

func decodeToolCallResult(t *testing.T, body []byte) rpcToolCallResult {
	t.Helper()
	var env struct {
		Result rpcToolCallResult `json:"result"`
	}
	require.NoError(t, json.Unmarshal(body, &env))
	return env.Result
}

func firstText(r rpcToolCallResult) string {
	for _, c := range r.Content {
		if c.Type == "text" {
			return c.Text
		}
	}
	return ""
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	var b strings.Builder
	if i < 0 {
		b.WriteByte('-')
		i = -i
	}
	digits := []byte{}
	for i > 0 {
		digits = append([]byte{byte('0' + i%10)}, digits...)
		i /= 10
	}
	b.Write(digits)
	return b.String()
}

// shellQuote returns a single-quoted shell string. Inputs from t.TempDir
// are safe paths, but quoting keeps the test resilient to environments
// that may sneak whitespace into tmp paths.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

func countLines(t *testing.T, path string) int {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return 0
		}
		require.NoError(t, err)
	}
	if len(data) == 0 {
		return 0
	}
	return strings.Count(string(data), "\n")
}
