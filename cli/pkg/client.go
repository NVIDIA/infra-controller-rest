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
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
)

type Client struct {
	BaseURL    string
	Org        string
	Token      string
	APIName    string
	HTTPClient *http.Client
	Debug      bool
	Log        *logrus.Entry
}

type APIError struct {
	StatusCode int
	Status     string
	Body       string
	Message    string
	Data       interface{}
}

func (e *APIError) Error() string {
	msg := e.Message
	if msg == "" {
		msg = e.Body
	}
	if e.Data != nil {
		dataJSON, err := json.Marshal(e.Data)
		if err == nil && string(dataJSON) != "null" {
			return fmt.Sprintf("API error %d: %s\nDetails: %s", e.StatusCode, msg, string(dataJSON))
		}
	}
	return fmt.Sprintf("API error %d: %s", e.StatusCode, msg)
}

func NewClient(baseURL, org, token string, log *logrus.Entry, debug bool) *Client {
	return &Client{
		BaseURL: strings.TrimRight(baseURL, "/"),
		Org:     org,
		Token:   token,
		APIName: "nico",
		HTTPClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		Debug: debug,
		Log:   log,
	}
}

var orgScopedAPIPathPattern = regexp.MustCompile(`(/v[0-9]+/org/[^/]+/)([^/]+)`)

// rewriteAPIName replaces the API path segment after /org/{org}/ with the
// configured API name. This decouples the CLI from the hardcoded path in the spec.
func (c *Client) rewriteAPIName(path string) string {
	if c.APIName == "" || c.APIName == "nico" {
		return path
	}
	return orgScopedAPIPathPattern.ReplaceAllString(path, "${1}"+c.APIName)
}

// Do executes an HTTP request against the API.
func (c *Client) Do(method, pathTemplate string, pathParams, queryParams map[string]string, body []byte) ([]byte, http.Header, error) {
	path := pathTemplate
	path = strings.ReplaceAll(path, "{org}", url.PathEscape(c.Org))
	for k, v := range pathParams {
		path = strings.ReplaceAll(path, "{"+k+"}", url.PathEscape(v))
	}

	path = c.rewriteAPIName(path)
	reqURL := c.BaseURL + path
	if len(queryParams) > 0 {
		q := url.Values{}
		for k, v := range queryParams {
			q.Set(k, v)
		}
		reqURL += "?" + q.Encode()
	}

	var bodyReader io.Reader
	if body != nil {
		bodyReader = bytes.NewReader(body)
	}

	if c.Debug {
		c.Log.Debugf("%s %s", method, reqURL)
		if body != nil {
			c.Log.Debugf("Body: %s", string(body))
		}
	}

	req, err := http.NewRequest(method, reqURL, bodyReader)
	if err != nil {
		return nil, nil, fmt.Errorf("creating request: %w", err)
	}

	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Accept", "application/json")
	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, nil, fmt.Errorf("executing request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, nil, fmt.Errorf("reading response: %w", err)
	}

	if c.Debug {
		c.Log.Debugf("Response %d: %s", resp.StatusCode, string(respBody))
	}

	if resp.StatusCode >= 400 {
		apiErr := &APIError{
			StatusCode: resp.StatusCode,
			Status:     resp.Status,
			Body:       string(respBody),
		}
		var errResp struct {
			Source  string      `json:"source"`
			Message string      `json:"message"`
			Error   string      `json:"error"`
			Data    interface{} `json:"data"`
		}
		if json.Unmarshal(respBody, &errResp) == nil {
			if errResp.Message != "" {
				apiErr.Message = errResp.Message
			} else if errResp.Error != "" {
				apiErr.Message = errResp.Error
			}
			apiErr.Data = errResp.Data
		}
		return nil, nil, apiErr
	}

	return respBody, resp.Header, nil
}

// ResolveToken returns the token or executes the token command.
func ResolveToken(token, tokenCommand string) (string, error) {
	if token != "" {
		return token, nil
	}
	if tokenCommand != "" {
		out, err := exec.Command("sh", "-c", tokenCommand).Output()
		if err != nil {
			return "", fmt.Errorf("executing token command: %w", err)
		}
		return strings.TrimSpace(string(out)), nil
	}
	return "", nil
}

// ReadBodyInput reads request body from --data flag or --data-file flag.
// Use "--data-file -" to read from stdin.
func ReadBodyInput(data, dataFile string) ([]byte, error) {
	if data != "" && dataFile != "" {
		return nil, fmt.Errorf("specify either --data or --data-file, not both")
	}
	if data != "" {
		return []byte(data), nil
	}
	if dataFile != "" {
		if dataFile == "-" {
			b, err := io.ReadAll(os.Stdin)
			if err != nil {
				return nil, fmt.Errorf("reading stdin: %w", err)
			}
			return b, nil
		}
		b, err := os.ReadFile(dataFile)
		if err != nil {
			return nil, fmt.Errorf("reading data file: %w", err)
		}
		return b, nil
	}
	return nil, nil
}
