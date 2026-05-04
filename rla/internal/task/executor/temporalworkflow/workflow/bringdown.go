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

package workflow

import (
	"time"

	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/workflow"

	taskcommon "github.com/NVIDIA/ncx-infra-controller-rest/rla/internal/task/common"
	"github.com/NVIDIA/ncx-infra-controller-rest/rla/internal/task/operations"
	"github.com/NVIDIA/ncx-infra-controller-rest/rla/internal/task/task"
)

// init registers the BringDown workflow descriptor with the package registry.
//
// BringDown is a separate workflow from BringUp (rather than another op code
// under TaskTypeBringUp) because the two operations are expected to diverge:
// bring-down will eventually need to retract expectations, remove instance
// records, and potentially delete on-site state — none of which belong on
// the bring-up code path. Sharing only the rule-driven executor lets each
// workflow evolve its own pre/post processing without entangling the other.
func init() {
	registerTaskWorkflow[operations.BringDownTaskInfo](
		taskcommon.TaskTypeBringDown, "BringDown", bringDown,
	)
}

// bringDownActivityOptions are the default activity options for bring-down
// workflows. Mirrors bringUpActivityOptions; kept separate so the two can
// evolve independently.
var bringDownActivityOptions = workflow.ActivityOptions{
	StartToCloseTimeout: 20 * time.Minute,
	RetryPolicy: &temporal.RetryPolicy{
		MaximumAttempts:    3,
		InitialInterval:    5 * time.Second,
		MaximumInterval:    1 * time.Minute,
		BackoffCoefficient: 2,
	},
}

// bringDown orchestrates the rack bring-down sequence using operation rules.
// The execution sequence is driven by the RuleDefinition attached to the
// task, falling back to a hardcoded default when no custom rule exists.
func bringDown(
	ctx workflow.Context,
	reqInfo task.ExecutionInfo,
	info *operations.BringDownTaskInfo,
) error {
	// Components and operation info are validated by executeWorkflow before
	// this function is invoked — no need to re-validate here.
	ctx = workflow.WithActivityOptions(ctx, bringDownActivityOptions)

	if err := updateRunningTaskStatus(ctx, reqInfo.TaskID); err != nil {
		return err
	}

	typeToTargets := buildTargets(&reqInfo)

	err := executeRuleBasedOperation(
		ctx,
		typeToTargets,
		info,
		reqInfo.RuleDefinition,
	)

	return updateFinishedTaskStatus(ctx, reqInfo.TaskID, err)
}
