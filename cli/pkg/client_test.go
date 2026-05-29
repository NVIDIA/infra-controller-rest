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

package cli

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestClientDoRefreshesTokenOnUnauthorizedAndRetries(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if requests == 1 {
			http.Error(w, `{"message":"expired"}`, http.StatusUnauthorized)
			return
		}
		if got := r.Header.Get("Authorization"); got != "Bearer refreshed-token" {
			require.Equal(t, "Bearer refreshed-token", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()

	refreshes := 0
	client := NewClient(server.URL, "test-org", "stale-token", nil, false)
	client.TokenRefresh = func() (string, error) {
		refreshes++
		return "refreshed-token", nil
	}

	body, _, err := client.Do("GET", "/v2/org/{org}/carbide/test", nil, nil, nil)
	require.NoError(t, err)
	require.Equal(t, `{"ok":true}`, string(body))
	require.Equal(t, 2, requests)
	require.Equal(t, 1, refreshes)
}

func TestClientDoRetriesUnauthorizedAtMostThreeTimes(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests++
		http.Error(w, `{"message":"expired"}`, http.StatusUnauthorized)
	}))
	defer server.Close()

	var events []AuthRetryEvent
	refreshes := 0
	client := NewClient(server.URL, "test-org", "stale-token", nil, false)
	client.TokenRefresh = func() (string, error) {
		refreshes++
		return "still-invalid-token", nil
	}
	client.AuthRetryNotify = func(event AuthRetryEvent) {
		events = append(events, event)
	}

	_, _, err := client.Do("GET", "/v2/org/{org}/carbide/test", nil, nil, nil)
	apiErr, ok := err.(*APIError)
	require.True(t, ok, "err = %T, want *APIError", err)
	require.Equal(t, http.StatusUnauthorized, apiErr.StatusCode)
	require.Equal(t, 4, requests)
	require.Equal(t, 3, refreshes)
	require.Len(t, events, 6)
	for i := 0; i < 3; i++ {
		login := events[i*2]
		retry := events[i*2+1]
		require.Equal(t, AuthRetryActionLogin, login.Action)
		require.Equal(t, AuthRetryActionRetry, retry.Action)
		require.Equal(t, i+1, login.Attempt)
		require.Equal(t, i+1, retry.Attempt)
		require.Equal(t, 3, login.MaxAttempts)
		require.Equal(t, 3, retry.MaxAttempts)
	}
}

func TestClientDoDoesNotReplayNonIdempotentRequestAfterUnauthorized(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests++
		http.Error(w, `{"message":"expired"}`, http.StatusUnauthorized)
	}))
	defer server.Close()

	var events []AuthRetryEvent
	refreshes := 0
	client := NewClient(server.URL, "test-org", "stale-token", nil, false)
	client.TokenRefresh = func() (string, error) {
		refreshes++
		return "new-token", nil
	}
	client.AuthRetryNotify = func(event AuthRetryEvent) {
		events = append(events, event)
	}

	_, _, err := client.Do("POST", "/v2/org/{org}/carbide/test", nil, nil, []byte(`{"name":"x"}`))
	apiErr, ok := err.(*APIError)
	require.True(t, ok, "err = %T, want *APIError", err)
	require.Equal(t, http.StatusUnauthorized, apiErr.StatusCode)
	require.Equal(t, 1, requests)
	require.Equal(t, 0, refreshes)
	require.Len(t, events, 1)
	require.Equal(t, AuthRetryActionSkip, events[0].Action)
	require.Equal(t, http.MethodPost, events[0].Method)
}

func TestClientDoReturnsRefreshErrorWithoutRetrying(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests++
		http.Error(w, `{"message":"expired"}`, http.StatusUnauthorized)
	}))
	defer server.Close()

	refreshes := 0
	client := NewClient(server.URL, "test-org", "stale-token", nil, false)
	client.TokenRefresh = func() (string, error) {
		refreshes++
		return "", errors.New("refresh failed")
	}

	_, _, err := client.Do("GET", "/v2/org/{org}/carbide/test", nil, nil, nil)
	require.Error(t, err)
	require.True(t, strings.Contains(err.Error(), "refresh failed"), "err = %v", err)
	require.Equal(t, 1, requests)
	require.Equal(t, 1, refreshes)
}

func TestClientDoReturnsEmptyTokenErrorWithoutRetrying(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests++
		http.Error(w, `{"message":"expired"}`, http.StatusUnauthorized)
	}))
	defer server.Close()

	refreshes := 0
	client := NewClient(server.URL, "test-org", "stale-token", nil, false)
	client.TokenRefresh = func() (string, error) {
		refreshes++
		return "", nil
	}

	_, _, err := client.Do("GET", "/v2/org/{org}/carbide/test", nil, nil, nil)
	require.Error(t, err)
	require.True(t, strings.Contains(err.Error(), "no token returned"), "err = %v", err)
	require.Equal(t, 1, requests)
	require.Equal(t, 1, refreshes)
}

func TestClientDoReturnsUnauthorizedWhenNoRefreshFunc(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, `{"message":"expired"}`, http.StatusUnauthorized)
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-org", "stale-token", nil, false)
	_, _, err := client.Do("GET", "/v2/org/{org}/carbide/test", nil, nil, nil)
	apiErr, ok := err.(*APIError)
	require.True(t, ok, "err = %T, want *APIError", err)
	require.Equal(t, http.StatusUnauthorized, apiErr.StatusCode)
}

func TestClientDoDoesNotRefreshOnForbidden(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, `{"message":"forbidden"}`, http.StatusForbidden)
	}))
	defer server.Close()

	refreshes := 0
	client := NewClient(server.URL, "test-org", "token", nil, false)
	client.TokenRefresh = func() (string, error) {
		refreshes++
		return "new-token", nil
	}

	_, _, err := client.Do("GET", "/v2/org/{org}/carbide/test", nil, nil, nil)
	apiErr, ok := err.(*APIError)
	require.True(t, ok, "err = %T, want *APIError", err)
	require.Equal(t, http.StatusForbidden, apiErr.StatusCode)
	require.Equal(t, 0, refreshes)
}

func TestAPINameMismatchHint(t *testing.T) {
	cases := []struct {
		name       string
		clientName string
		statusCode int
		source     string
		wantHas    string
		wantEmpty  bool
	}{
		{
			name:       "404 with mismatched source suggests server value",
			clientName: "nico",
			statusCode: http.StatusNotFound,
			source:     "carbide",
			wantHas:    `set api.name to "carbide"`,
		},
		{
			name:       "404 with forge source suggests forge",
			clientName: "nico",
			statusCode: http.StatusNotFound,
			source:     "forge",
			wantHas:    `set api.name to "forge"`,
		},
		{
			name:       "404 with matching source returns empty hint",
			clientName: "nico",
			statusCode: http.StatusNotFound,
			source:     "nico",
			wantEmpty:  true,
		},
		{
			name:       "404 with empty source returns empty hint",
			clientName: "nico",
			statusCode: http.StatusNotFound,
			source:     "",
			wantEmpty:  true,
		},
		{
			name:       "non-404 status returns empty hint",
			clientName: "nico",
			statusCode: http.StatusInternalServerError,
			source:     "carbide",
			wantEmpty:  true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := &Client{APIName: tc.clientName}
			got := c.apiNameMismatchHint(tc.statusCode, tc.source)
			if tc.wantEmpty {
				require.Empty(t, got)
				return
			}
			require.Contains(t, got, tc.wantHas)
			require.Contains(t, got, tc.clientName)
		})
	}
}

func TestClientDoAddsAPINameHintOn404SourceMismatch(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"source":"carbide","message":"The requested path could not be found"}`))
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-org", "token", nil, false)
	_, _, err := client.Do("GET", "/v2/org/{org}/nico/site", nil, nil, nil)
	apiErr, ok := err.(*APIError)
	require.True(t, ok, "err = %T, want *APIError", err)
	require.Equal(t, http.StatusNotFound, apiErr.StatusCode)
	require.Contains(t, apiErr.Hint, `set api.name to "carbide"`)
	require.Contains(t, apiErr.Error(), "Hint:")
}

func TestClientDoOmitsAPINameHintWhenSourceMatches(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"source":"nico","message":"site not found"}`))
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-org", "token", nil, false)
	_, _, err := client.Do("GET", "/v2/org/{org}/nico/site/missing", nil, nil, nil)
	apiErr, ok := err.(*APIError)
	require.True(t, ok, "err = %T, want *APIError", err)
	require.Equal(t, http.StatusNotFound, apiErr.StatusCode)
	require.Empty(t, apiErr.Hint)
	require.NotContains(t, apiErr.Error(), "Hint:")
}

func TestAPIErrorErrorIncludesHintAndDetails(t *testing.T) {
	e := &APIError{
		StatusCode: 404,
		Message:    "not found",
		Data:       map[string]string{"path": "/x"},
		Hint:       "try foo",
	}
	got := e.Error()
	require.Contains(t, got, "API error 404")
	require.Contains(t, got, "Details:")
	require.Contains(t, got, "Hint: try foo")
}

func TestAPIErrorErrorOmitsHintWhenEmpty(t *testing.T) {
	e := &APIError{StatusCode: 500, Message: "boom"}
	got := e.Error()
	require.NotContains(t, got, "Hint:")
}
