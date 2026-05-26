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

package activity

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/NVIDIA/infra-controller-rest/flow/internal/task/executor/temporalworkflow/common"
	"github.com/NVIDIA/infra-controller-rest/flow/internal/task/operations"
	"github.com/NVIDIA/infra-controller-rest/flow/internal/task/task"
)

// Canonical Temporal activity names. These constants are the single source of
// truth: used in All() for worker registration and when scheduling via
// workflow.ExecuteActivity.
const (
	NameInjectExpectation         = "InjectExpectation"
	NamePowerControl              = "PowerControl"
	NameGetPowerStatus            = "GetPowerStatus"
	NameUpdateTaskStatus          = "UpdateTaskStatus"
	NameFirmwareControl           = "FirmwareControl"
	NameGetFirmwareStatus         = "GetFirmwareStatus"
	NameBringUpControl            = "BringUpControl"
	NameGetBringUpStatus          = "GetBringUpStatus"
	NameVerifyFirmwareConsistency = "VerifyFirmwareConsistency"
)

// InjectExpectation is a Temporal activity that registers expected component
// configurations with the appropriate component manager service.
func (a *Activities) InjectExpectation(
	ctx context.Context,
	target common.Target,
	info operations.InjectExpectationTaskInfo,
) error {
	injector, err := requireExpectationInjector(a.registry, target)
	if err != nil {
		return err
	}

	return injector.InjectExpectation(ctx, target, info)
}

// PowerControl is a Temporal activity that applies a power state transition
// to the target components via the appropriate component manager.
func (a *Activities) PowerControl(
	ctx context.Context,
	target common.Target,
	info operations.PowerControlTaskInfo,
) error {
	controller, err := requirePowerController(a.registry, target)
	if err != nil {
		return err
	}

	return controller.PowerControl(ctx, target, info)
}

// GetPowerStatus is a Temporal activity that queries current power states for
// all components in the target. Returns a map of component ID to PowerStatus.
func (a *Activities) GetPowerStatus(
	ctx context.Context,
	target common.Target,
) (map[string]operations.PowerStatus, error) {
	reader, err := requirePowerStatusReader(a.registry, target)
	if err != nil {
		return nil, err
	}

	return reader.GetPowerStatus(ctx, target)
}

// UpdateTaskStatus is a Temporal activity that updates task status by ID.
func (a *Activities) UpdateTaskStatus(
	ctx context.Context,
	arg *task.TaskStatusUpdate,
) error {
	if a.updater == nil {
		return fmt.Errorf("task status updater is not configured")
	}

	if arg == nil || arg.ID == uuid.Nil {
		return fmt.Errorf("invalid task identifier")
	}

	return a.updater.UpdateTaskStatus(ctx, arg)
}

// FirmwareControl initiates firmware update without waiting for completion.
// This activity returns immediately after the update request is accepted.
func (a *Activities) FirmwareControl(
	ctx context.Context,
	target common.Target,
	info operations.FirmwareControlTaskInfo,
) error {
	controller, err := requireFirmwareController(a.registry, target)
	if err != nil {
		return err
	}

	return controller.FirmwareControl(ctx, target, info)
}

// GetFirmwareStatusResult is the result of GetFirmwareStatus activity.
type GetFirmwareStatusResult struct {
	// Statuses maps each component ID to its current firmware update state.
	Statuses map[string]operations.FirmwareUpdateStatus
}

// GetFirmwareStatus returns the current status of firmware updates.
// This activity is designed to be called repeatedly in a polling loop.
func (a *Activities) GetFirmwareStatus(
	ctx context.Context,
	target common.Target,
) (*GetFirmwareStatusResult, error) {
	reader, err := requireFirmwareStatusReader(a.registry, target)
	if err != nil {
		return nil, err
	}

	statuses, err := reader.GetFirmwareStatus(ctx, target)
	if err != nil {
		return nil, err
	}

	return &GetFirmwareStatusResult{Statuses: statuses}, nil
}

// BringUpControl opens the power-on gate for the target components.
func (a *Activities) BringUpControl(
	ctx context.Context,
	target common.Target,
) error {
	controller, err := requireBringUpController(a.registry, target)
	if err != nil {
		return err
	}

	return controller.BringUpControl(ctx, target)
}

// GetBringUpStatusResult is the result of GetBringUpStatus activity.
type GetBringUpStatusResult struct {
	// States maps each component ID to its current bring-up state.
	States map[string]operations.MachineBringUpState
}

// GetBringUpStatus returns the bring-up state for target components.
func (a *Activities) GetBringUpStatus(
	ctx context.Context,
	target common.Target,
) (*GetBringUpStatusResult, error) {
	reader, err := requireBringUpStatusReader(a.registry, target)
	if err != nil {
		return nil, err
	}

	states, err := reader.GetBringUpStatus(ctx, target)
	if err != nil {
		return nil, err
	}

	return &GetBringUpStatusResult{States: states}, nil
}

// VerifyFirmwareConsistency checks that all target components report the
// same firmware version set. Only supported by component managers that
// implement FirmwareConsistencyChecker.
func (a *Activities) VerifyFirmwareConsistency(
	ctx context.Context,
	target common.Target,
) error {
	checker, err := requireFirmwareConsistencyChecker(a.registry, target)
	if err != nil {
		return err
	}

	return checker.VerifyFirmwareConsistency(ctx, target)
}
