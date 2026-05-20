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

package manager

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	taskcommon "github.com/NVIDIA/infra-controller-rest/flow/internal/task/common"
	"github.com/NVIDIA/infra-controller-rest/flow/internal/task/componentmanager"
	"github.com/NVIDIA/infra-controller-rest/flow/internal/task/componentmanager/capability"
	cmcatalog "github.com/NVIDIA/infra-controller-rest/flow/internal/task/componentmanager/catalog"
	cmconfig "github.com/NVIDIA/infra-controller-rest/flow/internal/task/componentmanager/config"
	"github.com/NVIDIA/infra-controller-rest/flow/internal/task/componentmanager/providerapi"
	"github.com/NVIDIA/infra-controller-rest/flow/internal/task/executor/temporalworkflow/common"
	"github.com/NVIDIA/infra-controller-rest/flow/internal/task/operationrules"
	"github.com/NVIDIA/infra-controller-rest/flow/internal/task/operations"
	"github.com/NVIDIA/infra-controller-rest/flow/internal/task/task"
	"github.com/NVIDIA/infra-controller-rest/flow/pkg/common/devicetypes"
)

func TestValidateRequiredCapabilitiesRejectsUnsupportedCapability(t *testing.T) {
	registry := newPreflightCapabilityRegistry(
		t,
		capability.CapabilityPowerStatus,
	)

	err := validateRequiredCapabilities(
		powerControlExecutionInfo(t),
		registry,
	)

	require.Error(t, err)
	require.True(t, errors.Is(err, componentmanager.ErrUnsupportedCapability))

	var capabilityErr componentmanager.UnsupportedCapabilityError
	require.True(t, errors.As(err, &capabilityErr))
	require.Equal(t, devicetypes.ComponentTypeCompute, capabilityErr.ComponentType)
	require.Equal(t, capability.CapabilityPowerControl, capabilityErr.Capability)
}

func TestExecuteValidatesCapabilitiesBeforeDispatch(t *testing.T) {
	registry := newPreflightCapabilityRegistry(
		t,
		capability.CapabilityPowerStatus,
	)
	manager := &Manager{
		conf: Config{
			ComponentManagerRegistry: registry,
		},
	}

	resp, err := manager.Execute(
		context.Background(),
		&task.ExecutionRequest{
			Info:  powerControlExecutionInfo(t),
			Async: true,
		},
	)

	require.Nil(t, resp)
	require.Error(t, err)
	require.True(t, errors.Is(err, componentmanager.ErrUnsupportedCapability))
}

func TestExecutionComponentTypesDeduplicatesAndSorts(t *testing.T) {
	got := executionComponentTypes([]task.WorkflowComponent{
		{Type: devicetypes.ComponentTypePowerShelf},
		{Type: devicetypes.ComponentTypeCompute},
		{Type: devicetypes.ComponentTypePowerShelf},
		{Type: devicetypes.ComponentTypeNVSwitch},
		{Type: devicetypes.ComponentTypeCompute},
	})

	require.Equal(
		t,
		[]devicetypes.ComponentType{
			devicetypes.ComponentTypeCompute,
			devicetypes.ComponentTypeNVSwitch,
			devicetypes.ComponentTypePowerShelf,
		},
		got,
	)
}

func powerControlExecutionInfo(t *testing.T) task.ExecutionInfo {
	t.Helper()

	data, err := json.Marshal(operations.PowerControlTaskInfo{
		Operation: operations.PowerOperationPowerOn,
	})
	require.NoError(t, err)

	return task.ExecutionInfo{
		TaskID: uuid.New(),
		Components: []task.WorkflowComponent{
			{Type: devicetypes.ComponentTypeCompute, ComponentID: "machine-1"},
		},
		RuleDefinition: &operationrules.RuleDefinition{
			Steps: []operationrules.SequenceStep{
				{
					ComponentType: devicetypes.ComponentTypeCompute,
					MainOperation: operationrules.ActionConfig{
						Name: operationrules.ActionPowerControl,
					},
				},
			},
		},
		OperationType: taskcommon.TaskTypePowerControl,
		OperationInfo: data,
	}
}

type preflightCapabilityManager struct {
	descriptor cmcatalog.Descriptor
}

func (m preflightCapabilityManager) Descriptor() cmcatalog.Descriptor {
	return m.descriptor
}

func (m preflightCapabilityManager) InjectExpectation(
	context.Context,
	common.Target,
	operations.InjectExpectationTaskInfo,
) error {
	return nil
}

func (m preflightCapabilityManager) PowerControl(
	context.Context,
	common.Target,
	operations.PowerControlTaskInfo,
) error {
	return nil
}

func (m preflightCapabilityManager) GetPowerStatus(
	context.Context,
	common.Target,
) (map[string]operations.PowerStatus, error) {
	return nil, nil
}

func (m preflightCapabilityManager) FirmwareControl(
	context.Context,
	common.Target,
	operations.FirmwareControlTaskInfo,
) error {
	return nil
}

func (m preflightCapabilityManager) GetFirmwareStatus(
	context.Context,
	common.Target,
) (map[string]operations.FirmwareUpdateStatus, error) {
	return nil, nil
}

func newPreflightCapabilityRegistry(
	t *testing.T,
	capabilities ...capability.Capability,
) *componentmanager.Registry {
	t.Helper()

	descriptor := cmcatalog.Descriptor{
		Type:           devicetypes.ComponentTypeCompute,
		Implementation: "limited",
		Capabilities:   capability.CapabilitySet(capabilities),
	}

	registry, err := componentmanager.NewRegistry(
		[]componentmanager.FactorySpec{
			{
				Descriptor: descriptor,
				Factory: func(*providerapi.ProviderRegistry) (componentmanager.ComponentManager, error) {
					return preflightCapabilityManager{descriptor: descriptor}, nil
				},
			},
		},
		cmconfig.Config{
			ComponentManagers: map[devicetypes.ComponentType]string{
				devicetypes.ComponentTypeCompute: "limited",
			},
		},
		providerapi.NewProviderRegistry(),
	)
	require.NoError(t, err)

	return registry
}
