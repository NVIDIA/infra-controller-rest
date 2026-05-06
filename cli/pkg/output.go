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
	"encoding/json"
	"fmt"
	"os"
	"text/tabwriter"

	"gopkg.in/yaml.v3"
)

func FormatOutput(data []byte, format string) error {
	switch format {
	case "yaml":
		return formatYAML(data)
	case "table":
		return formatTable(data)
	default:
		return formatJSON(data)
	}
}

func formatJSON(data []byte) error {
	var v interface{}
	if err := json.Unmarshal(data, &v); err != nil {
		_, err = os.Stdout.Write(data)
		return err
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

func formatYAML(data []byte) error {
	var v interface{}
	if err := json.Unmarshal(data, &v); err != nil {
		_, err = os.Stdout.Write(data)
		return err
	}
	return yaml.NewEncoder(os.Stdout).Encode(v)
}

var tableFields = []string{"id", "name", "status", "created", "updated"}

func formatTable(data []byte) error {
	var raw interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		_, err = os.Stdout.Write(data)
		return err
	}

	var items []map[string]interface{}
	switch v := raw.(type) {
	case []interface{}:
		for _, item := range v {
			if m, ok := item.(map[string]interface{}); ok {
				items = append(items, m)
			}
		}
	case map[string]interface{}:
		items = append(items, v)
	default:
		return formatJSON(data)
	}

	if len(items) == 0 {
		fmt.Println("(no results)")
		return nil
	}

	var cols []string
	for _, f := range tableFields {
		if _, ok := items[0][f]; ok {
			cols = append(cols, f)
		}
	}
	if len(cols) == 0 {
		return formatJSON(data)
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	for i, c := range cols {
		if i > 0 {
			fmt.Fprint(w, "\t")
		}
		fmt.Fprint(w, c)
	}
	fmt.Fprintln(w)

	for _, item := range items {
		for i, c := range cols {
			if i > 0 {
				fmt.Fprint(w, "\t")
			}
			fmt.Fprintf(w, "%v", item[c])
		}
		fmt.Fprintln(w)
	}

	return w.Flush()
}
