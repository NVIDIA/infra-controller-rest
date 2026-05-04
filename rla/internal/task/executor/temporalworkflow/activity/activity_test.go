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
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/NVIDIA/ncx-infra-controller-rest/rla/internal/task/componentmanager"
	"github.com/NVIDIA/ncx-infra-controller-rest/rla/internal/task/executor/temporalworkflow/common"
	"github.com/NVIDIA/ncx-infra-controller-rest/rla/internal/task/operations"
	"github.com/NVIDIA/ncx-infra-controller-rest/rla/pkg/common/devicetypes"
)

func TestActivitiesReturnErrorWhenComponentManagerRegistryIsMissing(t *testing.T) {
	acts := New(nil, nil)

	for name, call := range activityCallsForMissingManagerTest(t, acts) {
		t.Run(name, func(t *testing.T) {
			err := call()
			require.Error(t, err)
			require.True(t, errors.Is(err, componentmanager.ErrRegistryNotConfigured))
		})
	}
}

func TestActivitiesReturnErrorWhenComponentManagerIsMissing(t *testing.T) {
	acts := New(nil, componentmanager.NewRegistry())

	for name, call := range activityCallsForMissingManagerTest(t, acts) {
		t.Run(name, func(t *testing.T) {
			err := call()
			require.Error(t, err)
			require.True(t, errors.Is(err, componentmanager.ErrManagerNotConfigured))
		})
	}
}

func activityCallsForMissingManagerTest(
	t *testing.T,
	acts *Activities,
) map[string]func() error {
	t.Helper()

	ctx := context.Background()
	target := newActivityTestTarget()

	return map[string]func() error{
		"InjectExpectation": func() error {
			return acts.InjectExpectation(
				ctx,
				target,
				operations.InjectExpectationTaskInfo{},
			)
		},
		"PowerControl": func() error {
			return acts.PowerControl(
				ctx,
				target,
				operations.PowerControlTaskInfo{
					Operation: operations.PowerOperationPowerOn,
				},
			)
		},
		"GetPowerStatus": func() error {
			_, err := acts.GetPowerStatus(ctx, target)
			return err
		},
		"VerifyFirmwareConsistency": func() error {
			return acts.VerifyFirmwareConsistency(ctx, target)
		},
		"BringUpControl": func() error {
			return acts.BringUpControl(ctx, target)
		},
		"GetBringUpStatus": func() error {
			_, err := acts.GetBringUpStatus(ctx, target)
			return err
		},
		"FirmwareControl": func() error {
			return acts.FirmwareControl(
				ctx,
				target,
				operations.FirmwareControlTaskInfo{
					Operation: operations.FirmwareOperationUpgrade,
				},
			)
		},
		"GetFirmwareStatus": func() error {
			_, err := acts.GetFirmwareStatus(ctx, target)
			return err
		},
	}
}

func newActivityTestTarget() common.Target {
	return common.Target{
		Type:         devicetypes.ComponentTypeCompute,
		ComponentIDs: []string{"machine-1"},
	}
}
