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

package message

import (
	"strings"

	taskcommon "github.com/NVIDIA/infra-controller-rest/flow/internal/task/common"
)

const maxLen = 512

// ForStatus returns the default message for a status transition. Callers may
// override Waiting / Terminated with more specific text before persisting.
func ForStatus(status taskcommon.TaskStatus) string {
	switch status {
	case taskcommon.TaskStatusWaiting:
		return "Queued: waiting for rack to become available"
	case taskcommon.TaskStatusPending:
		return "Pending"
	case taskcommon.TaskStatusRunning:
		return "Running"
	case taskcommon.TaskStatusCompleted:
		return "Succeeded"
	case taskcommon.TaskStatusFailed:
		return "Failed"
	case taskcommon.TaskStatusTerminated:
		return "Terminated"
	default:
		return ""
	}
}

// ForFailure returns a one-line failure summary suitable for the message field.
func ForFailure(err error) string {
	if err == nil {
		return ForStatus(taskcommon.TaskStatusFailed)
	}
	msg := strings.TrimSpace(err.Error())
	if msg == "" {
		return ForStatus(taskcommon.TaskStatusFailed)
	}
	idx := strings.IndexByte(msg, '\n')
	if idx >= 0 {
		msg = strings.TrimSpace(msg[:idx])
	}
	return truncate(msg)
}

func truncate(msg string) string {
	if len(msg) <= maxLen {
		return msg
	}
	return msg[:maxLen]
}
