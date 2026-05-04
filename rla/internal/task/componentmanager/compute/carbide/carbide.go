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

package carbide

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/rs/zerolog/log"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/NVIDIA/ncx-infra-controller-rest/rla/internal/carbideapi"
	pb "github.com/NVIDIA/ncx-infra-controller-rest/rla/internal/carbideapi/gen"
	"github.com/NVIDIA/ncx-infra-controller-rest/rla/internal/task/componentmanager"
	carbideprovider "github.com/NVIDIA/ncx-infra-controller-rest/rla/internal/task/componentmanager/providers/carbide"
	"github.com/NVIDIA/ncx-infra-controller-rest/rla/internal/task/executor/temporalworkflow/common"
	"github.com/NVIDIA/ncx-infra-controller-rest/rla/internal/task/operations"
	"github.com/NVIDIA/ncx-infra-controller-rest/rla/pkg/common/devicetypes"
)

const (
	// ImplementationName is the name used to identify this implementation.
	ImplementationName = "carbide"

	// healthOverrideSource is the source tag written into health-report
	// overrides so they can be matched on removal.
	healthOverrideSource = "rla-power-control"

	// bringDownOverrideSource is the source tag for the long-lived
	// maintenance override placed by EnterMaintenance during bring-down.
	// Distinct from healthOverrideSource so that the transient
	// power-control override cleanup does not remove it.
	bringDownOverrideSource = "rla-bring-down"
)

// Manager manages compute node components via the Carbide API.
type Manager struct {
	carbideClient carbideapi.Client
	// powerDelay is inserted between sequential power control calls to
	// avoid overwhelming the power delivery system when commanding
	// multiple compute trays. 0 means no delay.
	powerDelay time.Duration
}

// New creates a new Carbide-based compute Manager instance.
func New(carbideClient carbideapi.Client, powerDelay time.Duration) *Manager {
	return &Manager{
		carbideClient: carbideClient,
		powerDelay:    powerDelay,
	}
}

// Register registers the Carbide compute manager factory with the given registry.
// powerDelay is the inter-component stagger for power control calls.
func Register(registry *componentmanager.Registry, powerDelay time.Duration) {
	factory := func(
		providerRegistry *componentmanager.ProviderRegistry,
	) (componentmanager.ComponentManager, error) {
		provider, err := componentmanager.GetTyped[*carbideprovider.Provider](
			providerRegistry,
			carbideprovider.ProviderName,
		)
		if err != nil {
			return nil, fmt.Errorf(
				"compute/carbide requires carbide provider: %w", err,
			)
		}
		return New(provider.Client(), powerDelay), nil
	}
	registry.RegisterFactory(devicetypes.ComponentTypeCompute, ImplementationName, factory)
}

// Type returns the component type this manager handles.
func (m *Manager) Type() devicetypes.ComponentType {
	return devicetypes.ComponentTypeCompute
}

// InjectExpectation registers an expected machine with Carbide via AddExpectedMachine.
// The Info field should contain a JSON-encoded carbideapi.AddExpectedMachineRequest.
func (m *Manager) InjectExpectation(
	ctx context.Context,
	target common.Target,
	info operations.InjectExpectationTaskInfo,
) error {
	var req carbideapi.AddExpectedMachineRequest
	if err := json.Unmarshal(info.Info, &req); err != nil {
		return fmt.Errorf("failed to unmarshal AddExpectedMachineRequest: %w", err)
	}

	if m.carbideClient == nil {
		return fmt.Errorf("carbide client is not configured")
	}

	if err := m.carbideClient.AddExpectedMachine(ctx, req); err != nil {
		return fmt.Errorf("failed to add expected machine: %w", err)
	}

	log.Info().
		Str("bmc_mac", req.BMCMACAddress).
		Str("chassis_serial", req.ChassisSerialNumber).
		Msg("Successfully registered expected machine with Carbide")

	return nil
}

// PowerControl performs power operations on a compute node via Carbide API.
func (m *Manager) PowerControl(
	ctx context.Context,
	target common.Target,
	info operations.PowerControlTaskInfo,
) error {
	log.Debug().Msgf(
		"compute power control %s op %s activity received",
		target.String(),
		info.Operation.String(),
	)

	if m.carbideClient == nil {
		return fmt.Errorf("carbide client is not configured")
	}

	if err := target.Validate(); err != nil {
		return fmt.Errorf("target is invalid: %w", err)
	}

	// Map common.PowerOperation to carbideapi.SystemPowerControl
	var action carbideapi.SystemPowerControl
	var desiredPowerState carbideapi.PowerState
	switch info.Operation {
	// Power On
	case operations.PowerOperationPowerOn:
		action = carbideapi.PowerControlOn
		desiredPowerState = carbideapi.PowerStateOn
	case operations.PowerOperationForcePowerOn:
		action = carbideapi.PowerControlForceOn
		desiredPowerState = carbideapi.PowerStateOn
	// Power Off
	case operations.PowerOperationPowerOff:
		action = carbideapi.PowerControlGracefulShutdown
		desiredPowerState = carbideapi.PowerStateOff
	case operations.PowerOperationForcePowerOff:
		action = carbideapi.PowerControlForceOff
		desiredPowerState = carbideapi.PowerStateOff
	// Restart (OS level)
	case operations.PowerOperationRestart:
		action = carbideapi.PowerControlGracefulRestart
		desiredPowerState = carbideapi.PowerStateOn
	case operations.PowerOperationForceRestart:
		action = carbideapi.PowerControlForceRestart
		desiredPowerState = carbideapi.PowerStateOn
	// Reset (hardware level)
	case operations.PowerOperationWarmReset:
		action = carbideapi.PowerControlWarmReset
		desiredPowerState = carbideapi.PowerStateOn
	case operations.PowerOperationColdReset:
		action = carbideapi.PowerControlColdReset
		desiredPowerState = carbideapi.PowerStateOn
	default:
		return fmt.Errorf("unknown power operation: %v", info.Operation)
	}

	// Track which components had health-report overrides inserted so we
	// can clean them up after the power operation completes.
	var overrideInserted []string

	defer func() {
		// Use a detached context so cleanup is not blocked by the
		// parent context being canceled or timed-out. Each individual
		// gRPC call inside RemoveHealthReportOverride already applies
		// the configured per-call timeout.
		cleanupCtx := context.WithoutCancel(ctx)

		for _, componentID := range overrideInserted {
			if err := m.carbideClient.RemoveHealthReportOverride(
				cleanupCtx, componentID, healthOverrideSource,
			); err != nil {
				log.Warn().Err(err).Str("component", componentID).
					Msg("Failed to remove health report override after power control")
			}
		}
	}()

	for i, componentID := range target.ComponentIDs {
		// Place a health-report override so Carbide marks the machine as
		// under maintenance for the duration of the power operation.
		if err := m.carbideClient.InsertHealthReportOverride(
			ctx, componentID, healthOverrideSource,
		); err != nil {
			log.Warn().Err(err).Str("component", componentID).
				Msg("Failed to insert health report override, proceeding anyway")
		} else {
			overrideInserted = append(overrideInserted, componentID)
		}

		// Set Carbide's power-on gate (desired power state) before issuing the
		// actual power control command so the power manager doesn't conflict.
		if err := m.carbideClient.UpdatePowerOption(
			ctx, componentID, desiredPowerState,
		); err != nil {
			if isAlreadyInDesiredStateError(err) {
				log.Debug().Str("component", componentID).
					Int("desired_state", int(desiredPowerState)).
					Msg("Power option already in desired state, skipping")
			} else {
				return fmt.Errorf(
					"failed to update power option for %s: %w", componentID, err,
				)
			}
		}

		if err := m.carbideClient.AdminPowerControl(ctx, componentID, action); err != nil {
			return fmt.Errorf(
				"failed to perform power control on %s: %w", componentID, err,
			)
		}

		// Stagger calls to avoid overwhelming the power delivery system
		if m.powerDelay > 0 && i < len(target.ComponentIDs)-1 {
			time.Sleep(m.powerDelay)
		}
	}

	log.Info().Msgf("power control %s on %s completed",
		info.Operation.String(), target.String())

	return nil
}

// GetPowerStatus retrieves the power status of compute nodes via Carbide API.
func (m *Manager) GetPowerStatus(
	ctx context.Context,
	target common.Target,
) (map[string]operations.PowerStatus, error) {
	log.Debug().Msgf(
		"compute get power status %s activity received",
		target.String(),
	)

	if m.carbideClient == nil {
		return nil, fmt.Errorf("carbide client is not configured")
	}

	if err := target.Validate(); err != nil {
		return nil, fmt.Errorf("target is invalid: %w", err)
	}

	powerStates, err := m.carbideClient.GetPowerStates(ctx, target.ComponentIDs)
	if err != nil {
		return nil, fmt.Errorf("failed to get power states: %w", err)
	}

	result := make(map[string]operations.PowerStatus, len(powerStates))
	for _, state := range powerStates {
		result[state.MachineID] = carbidePowerStateToOperationsPowerStatus(state.PowerState)
	}

	log.Info().Msgf("get power status for %s completed, got %d results",
		target.String(), len(result))

	return result, nil
}

// carbidePowerStateToOperationsPowerStatus converts carbide PowerState to operations PowerStatus.
func carbidePowerStateToOperationsPowerStatus(state carbideapi.PowerState) operations.PowerStatus {
	switch state {
	case carbideapi.PowerStateOn:
		return operations.PowerStatusOn
	case carbideapi.PowerStateOff, carbideapi.PowerStateDisabled:
		return operations.PowerStatusOff
	default:
		return operations.PowerStatusUnknown
	}
}

// FirmwareControl schedules a firmware update via Carbide's SetFirmwareUpdateTimeWindow API.
//
// Before scheduling, it performs:
//  1. Fail-fast: if targetVersion is provided, it must match one of the
//     desired firmware entries configured in Core.
//  2. Idempotent: queries the actual firmware versions from explored
//     endpoints and compares them against the target (or desired) versions.
//     If all machines are already at the desired firmware, returns early.
func (m *Manager) FirmwareControl(ctx context.Context, target common.Target, info operations.FirmwareControlTaskInfo) error {
	log.Debug().
		Str("components", target.String()).
		Str("target_version", info.TargetVersion).
		Msg("Scheduling firmware update for compute via Carbide")

	if m.carbideClient == nil {
		return fmt.Errorf("carbide client is not configured")
	}

	if err := target.Validate(); err != nil {
		return fmt.Errorf("target is invalid: %w", err)
	}

	desiredEntries, err := m.carbideClient.GetDesiredFirmwareVersions(ctx)
	if err != nil {
		return fmt.Errorf("failed to query desired firmware versions: %w", err)
	}

	var targetFirmware map[string]string
	if info.TargetVersion != "" {
		parsedTarget, err := parseTargetVersion(info.TargetVersion)
		if err != nil {
			return fmt.Errorf("invalid TargetVersion: %w", err)
		}
		if !isTargetVersionInDesired(parsedTarget, desiredEntries) {
			return fmt.Errorf(
				"target version %q does not match any desired firmware entry in Core; update cannot succeed",
				info.TargetVersion,
			)
		}
		targetFirmware = parsedTarget
	}

	machinesByID := make(map[string]carbideapi.MachineDetail)
	machines, err := m.carbideClient.FindMachinesByIds(ctx, target.ComponentIDs)
	if err != nil {
		log.Warn().Err(err).Msg("Failed to check machines, proceeding with schedule")
	} else {
		for _, machine := range machines {
			machinesByID[machine.MachineID] = machine
		}

		actualFirmware := m.getActualFirmwareVersions(ctx, machines)

		log.Debug().
			Int("machines_found", len(machines)).
			Int("actual_firmware_count", len(actualFirmware)).
			Int("desired_entries_count", len(desiredEntries)).
			Interface("target_firmware", targetFirmware).
			Msg("Idempotent check summary")
		for id, fv := range actualFirmware {
			log.Debug().Str("machine_id", id).Interface("actual_fw", fv).Msg("Idempotent check: actual firmware")
		}
		for i, de := range desiredEntries {
			log.Debug().Int("idx", i).
				Str("vendor", de.GetVendor()).Str("model", de.GetModel()).
				Interface("desired_fw", de.GetComponentVersions()).
				Msg("Idempotent check: desired entry")
		}

		if allFirmwareUpToDate(target.ComponentIDs, actualFirmware, targetFirmware, desiredEntries) {
			log.Info().
				Str("components", target.String()).
				Msg("All firmware already at desired version, skipping schedule")
			return nil
		}
	}

	var startTime, endTime time.Time
	switch {
	case info.StartTime == 0 && info.EndTime == 0:
		startTime = time.Now()
		endTime = startTime.Add(24 * time.Hour)
	case info.StartTime == 0 || info.EndTime == 0:
		return fmt.Errorf("firmware window requires both start_time and end_time, got start=%d end=%d",
			info.StartTime, info.EndTime)
	default:
		startTime = time.Unix(info.StartTime, 0)
		endTime = time.Unix(info.EndTime, 0)
		if !startTime.Before(endTime) {
			return fmt.Errorf("firmware window start_time (%s) must be before end_time (%s)",
				startTime.Format(time.RFC3339), endTime.Format(time.RFC3339))
		}
	}

	var autoUpdateErrors []string
	for _, machineID := range target.ComponentIDs {
		if machine, ok := machinesByID[machineID]; ok && machine.FirmwareAutoupdate != nil && *machine.FirmwareAutoupdate {
			log.Debug().Str("machine_id", machineID).Msg("firmware_autoupdate already enabled, skipping SetMachineAutoUpdate")
			continue
		}
		if err := m.carbideClient.SetMachineAutoUpdate(ctx, machineID, true); err != nil {
			log.Error().Err(err).Str("machine_id", machineID).Msg("Failed to enable auto-update, will continue with remaining machines")
			autoUpdateErrors = append(autoUpdateErrors, machineID)
			continue
		}
		log.Debug().Str("machine_id", machineID).Msg("Enabled firmware_autoupdate on machine")
	}
	if len(autoUpdateErrors) > 0 {
		return fmt.Errorf("failed to enable auto-update for %d machine(s): %v", len(autoUpdateErrors), autoUpdateErrors)
	}

	if err := m.carbideClient.SetFirmwareUpdateTimeWindow(ctx, target.ComponentIDs, startTime, endTime); err != nil {
		return fmt.Errorf("failed to schedule firmware update for compute: %w", err)
	}

	log.Info().
		Str("components", target.String()).
		Time("start_time", startTime).
		Time("end_time", endTime).
		Msg("Firmware update scheduled for compute")

	return nil
}

// parseTargetVersion parses a targetVersion JSON string into a component
// version map. targetVersion must be a JSON object with the same schema as
// desired_firmware.versions->>'Versions', e.g. {"bmc":"7.10.30.00","uefi":"2.22.1"}.
func parseTargetVersion(targetVersion string) (map[string]string, error) {
	var target map[string]string
	if err := json.Unmarshal([]byte(targetVersion), &target); err != nil {
		return nil, fmt.Errorf("target_version must be a JSON object: %w", err)
	}
	return target, nil
}

// isTargetVersionInDesired checks whether a pre-parsed component version map
// matches the component_versions of any desired firmware entry.
func isTargetVersionInDesired(target map[string]string, entries []*pb.DesiredFirmwareVersionEntry) bool {
	for _, entry := range entries {
		if versionsEqual(target, entry.GetComponentVersions()) {
			return true
		}
	}
	return false
}

func versionsEqual(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}

// getActualFirmwareVersions queries explored endpoints via BMC IPs and
// returns a map of machineID → firmware_versions (map[string]string).
func (m *Manager) getActualFirmwareVersions(
	ctx context.Context,
	machines []carbideapi.MachineDetail,
) map[string]map[string]string {
	bmcIPs := make([]string, 0, len(machines))
	machineByBmcIP := make(map[string]string, len(machines))
	for _, machine := range machines {
		if machine.BmcIP != "" {
			bmcIPs = append(bmcIPs, machine.BmcIP)
			machineByBmcIP[machine.BmcIP] = machine.MachineID
		} else {
			log.Warn().
				Str("machine_id", machine.MachineID).
				Str("machine_state", machine.State).
				Msg("getActualFirmwareVersions: machine has empty BmcIP, skipping")
		}
	}

	if len(bmcIPs) == 0 {
		log.Warn().Msg("getActualFirmwareVersions: no BMC IPs found")
		return nil
	}

	endpoints, err := m.carbideClient.FindExploredEndpointsByIds(ctx, bmcIPs)
	if err != nil {
		log.Warn().Err(err).Msg("Failed to query explored endpoints for firmware versions")
		return nil
	}

	result := make(map[string]map[string]string, len(endpoints))
	for _, ep := range endpoints {
		fwVersions := ep.GetReport().GetFirmwareVersions()
		machineID, idOk := machineByBmcIP[ep.GetAddress()]
		log.Info().
			Str("address", ep.GetAddress()).
			Bool("matched_machine", idOk).
			Str("machine_id", machineID).
			Int("fw_versions_count", len(fwVersions)).
			Interface("fw_versions", fwVersions).
			Msg("getActualFirmwareVersions: endpoint data")
		if len(fwVersions) == 0 {
			continue
		}
		if idOk {
			result[machineID] = fwVersions
		}
	}

	returnedAddrs := make(map[string]bool, len(endpoints))
	for _, ep := range endpoints {
		returnedAddrs[ep.GetAddress()] = true
	}
	for _, ip := range bmcIPs {
		if !returnedAddrs[ip] {
			log.Warn().
				Str("bmc_ip", ip).
				Str("machine_id", machineByBmcIP[ip]).
				Msg("BMC IP had no explored endpoint returned")
		}
	}
	return result
}

// allFirmwareUpToDate returns true when every component's actual firmware
// matches the target (if provided) or any desired firmware entry.
func allFirmwareUpToDate(
	componentIDs []string,
	actualFirmware map[string]map[string]string,
	targetFirmware map[string]string,
	desiredEntries []*pb.DesiredFirmwareVersionEntry,
) bool {
	if len(actualFirmware) == 0 {
		return false
	}
	for _, id := range componentIDs {
		actual, ok := actualFirmware[id]
		if !ok || len(actual) == 0 {
			return false
		}
		if targetFirmware != nil {
			if !firmwareVersionsMatch(targetFirmware, actual) {
				return false
			}
		} else {
			if !matchesAnyDesired(actual, desiredEntries) {
				return false
			}
		}
	}
	return true
}

// firmwareVersionsMatch returns true when every key in desired exists in
// actual with the same value (desired is a subset of actual).
func firmwareVersionsMatch(desired, actual map[string]string) bool {
	if len(desired) == 0 {
		return false
	}
	for k, v := range desired {
		if actual[k] != v {
			return false
		}
	}
	return true
}

// matchesAnyDesired returns true when the actual firmware versions satisfy
// at least one desired firmware entry.
func matchesAnyDesired(actual map[string]string, entries []*pb.DesiredFirmwareVersionEntry) bool {
	for _, entry := range entries {
		if firmwareVersionsMatch(entry.GetComponentVersions(), actual) {
			return true
		}
	}
	return false
}

// GetFirmwareStatus returns the current status of firmware updates for the
// target components. It compares the actual firmware versions (from explored
// endpoints) against Core's desired firmware entries. If the actual versions
// match the desired versions, the component is considered Completed.
// Falls back to Machine.UpdateComplete / Machine.State when version data
// is unavailable.
func (m *Manager) GetFirmwareStatus(ctx context.Context, target common.Target) (map[string]operations.FirmwareUpdateStatus, error) {
	log.Debug().
		Str("components", target.String()).
		Msg("GetFirmwareStatus called for compute")

	if m.carbideClient == nil {
		return nil, fmt.Errorf("carbide client is not configured")
	}

	machines, err := m.carbideClient.FindMachinesByIds(ctx, target.ComponentIDs)
	if err != nil {
		return nil, fmt.Errorf("failed to query machines: %w", err)
	}

	actualFirmware := m.getActualFirmwareVersions(ctx, machines)

	var desiredEntries []*pb.DesiredFirmwareVersionEntry
	if de, err := m.carbideClient.GetDesiredFirmwareVersions(ctx); err != nil {
		log.Warn().Err(err).Msg("Failed to get desired firmware versions, falling back to state-based check")
	} else {
		desiredEntries = de
	}

	log.Info().
		Str("components", target.String()).
		Int("desired_entries_count", len(desiredEntries)).
		Interface("desired_entries", desiredEntries).
		Int("actual_machines_count", len(actualFirmware)).
		Interface("actual_firmware_by_machine", actualFirmware).
		Msg("GetFirmwareStatus inputs")

	machineByID := make(map[string]carbideapi.MachineDetail, len(machines))
	for _, machine := range machines {
		machineByID[machine.MachineID] = machine
	}

	result := make(map[string]operations.FirmwareUpdateStatus, len(target.ComponentIDs))
	for _, id := range target.ComponentIDs {
		machine, ok := machineByID[id]
		if !ok {
			log.Warn().Str("machine_id", id).Msg("machine not found in Carbide")
			result[id] = operations.FirmwareUpdateStatus{
				ComponentID: id,
				State:       operations.FirmwareUpdateStateUnknown,
				Error:       "machine not found in Carbide",
			}
			continue
		}

		fwStatus := operations.FirmwareUpdateStatus{ComponentID: id}

		actual, hasActual := actualFirmware[id]
		log.Info().
			Str("machine_id", id).
			Str("machine_state", machine.State).
			Bool("machine_update_complete", machine.UpdateComplete).
			Str("bmc_ip", machine.BmcIP).
			Str("machine_type", machine.MachineType).
			Str("firmware_version", machine.FirmwareVersion).
			Bool("has_actual", hasActual).
			Interface("actual_versions", actual).
			Int("desired_entries_count", len(desiredEntries)).
			Msg("per-machine firmware status inputs")

		if hasActual && len(desiredEntries) > 0 {
			matched := false
			for idx, entry := range desiredEntries {
				entryMatch := firmwareVersionsMatch(entry.GetComponentVersions(), actual)
				log.Info().
					Str("machine_id", id).
					Int("desired_idx", idx).
					Str("desired_vendor", entry.GetVendor()).
					Str("desired_model", entry.GetModel()).
					Interface("desired_versions", entry.GetComponentVersions()).
					Bool("matches", entryMatch).
					Msg("comparing actual vs desired entry")
				if entryMatch {
					matched = true
				}
			}
			if matched {
				if strings.Contains(machine.State, "HostReprovision") {
					fwStatus.State = operations.FirmwareUpdateStateVerifying
					log.Info().
						Str("machine_id", id).
						Str("machine_state", machine.State).
						Msg("Actual firmware matches desired but machine still in HostReprovision state, marking Verifying")
				} else {
					fwStatus.State = operations.FirmwareUpdateStateCompleted
					log.Info().
						Str("machine_id", id).
						Str("completion_reason", "desired_equals_actual").
						Msg("Firmware update completed: actual firmware matches desired versions")
				}
				result[id] = fwStatus
				continue
			}
		}

		fallthroughReason := ""
		switch {
		case machine.UpdateComplete:
			fwStatus.State = operations.FirmwareUpdateStateCompleted
			fallthroughReason = "core_update_complete"
		case strings.Contains(machine.State, "HostReprovision") && strings.Contains(machine.State, "FailedFirmwareUpgrade"):
			fwStatus.State = operations.FirmwareUpdateStateFailed
			fallthroughReason = "host_reprovision_failed"
		case strings.Contains(machine.State, "HostReprovision"):
			fwStatus.State = operations.FirmwareUpdateStateVerifying
			fallthroughReason = "host_reprovision_in_progress"
		default:
			fwStatus.State = operations.FirmwareUpdateStateQueued
			fallthroughReason = "default_queued"
		}

		log.Info().
			Str("machine_id", id).
			Str("machine_state", machine.State).
			Bool("update_complete", machine.UpdateComplete).
			Bool("has_actual", hasActual).
			Int("desired_entries_count", len(desiredEntries)).
			Str("firmware_status", fwStatus.State.String()).
			Str("fallthrough_reason", fallthroughReason).
			Msg("final firmware status decision")

		result[id] = fwStatus
	}

	return result, nil
}

// VerifyNoInstance returns an error if any target machine still has an
// allocated instance on top of it. Used as a precondition for bring-down so
// that we never power off a machine that is still serving workload.
//
// On the first machine that reports an instance the call short-circuits with
// an error naming that machine — there is no value in continuing to query
// when we already know the operation cannot proceed.
func (m *Manager) VerifyNoInstance(
	ctx context.Context,
	target common.Target,
) error {
	if m.carbideClient == nil {
		return fmt.Errorf("carbide client is not configured")
	}
	if err := target.Validate(); err != nil {
		return fmt.Errorf("target is invalid: %w", err)
	}

	for _, componentID := range target.ComponentIDs {
		hasInstance, err := m.carbideClient.MachineHasInstance(ctx, componentID)
		if err != nil {
			return fmt.Errorf(
				"failed to query instances for %s: %w", componentID, err,
			)
		}
		if hasInstance {
			return fmt.Errorf(
				"machine %s still has an allocated instance; refuse to proceed",
				componentID,
			)
		}
	}
	return nil
}

// EnterMaintenance places each target machine under maintenance by inserting
// a long-lived health-report override with the bring-down source tag. The
// override is intentionally not removed here — bring-up (or a dedicated
// cleanup operation) is responsible for clearing it.
//
// Idempotent: re-inserting under the same source replaces the existing entry
// (Carbide's InsertHealthReportOverride is OverrideMode_Replace).
func (m *Manager) EnterMaintenance(
	ctx context.Context,
	target common.Target,
) error {
	if m.carbideClient == nil {
		return fmt.Errorf("carbide client is not configured")
	}
	if err := target.Validate(); err != nil {
		return fmt.Errorf("target is invalid: %w", err)
	}

	for _, componentID := range target.ComponentIDs {
		if err := m.carbideClient.InsertHealthReportOverride(
			ctx, componentID, bringDownOverrideSource,
		); err != nil {
			return fmt.Errorf(
				"failed to enter maintenance for %s: %w", componentID, err,
			)
		}
		log.Info().
			Str("component_id", componentID).
			Str("source", bringDownOverrideSource).
			Msg("EnterMaintenance succeeded")
	}
	return nil
}

// PausePowerOnGate moves the per-machine desired power state to
// PowerManagerDisabled, taking the machine out of Carbide's power-manager
// control. Once paused, Carbide's reconcile loop will not attempt to bring
// the machine back up until the gate is re-enabled (typically by setting
// the desired state back to On via UpdatePowerOption).
//
// Idempotent: if the desired state is already Disabled, Carbide returns
// "already set as ..." which we recognise via isAlreadyInDesiredStateError
// and skip without erroring.
func (m *Manager) PausePowerOnGate(
	ctx context.Context,
	target common.Target,
) error {
	log.Debug().
		Str("components", target.String()).
		Msg("PausePowerOnGate for compute")

	if m.carbideClient == nil {
		return fmt.Errorf("carbide client is not configured")
	}

	if err := target.Validate(); err != nil {
		return fmt.Errorf("target is invalid: %w", err)
	}

	for _, componentID := range target.ComponentIDs {
		if err := m.carbideClient.UpdatePowerOption(
			ctx, componentID, carbideapi.PowerStateDisabled,
		); err != nil {
			if isAlreadyInDesiredStateError(err) {
				log.Debug().Str("component", componentID).
					Msg("Power option already disabled, skipping")
				continue
			}
			return fmt.Errorf(
				"failed to pause power-on gate for %s: %w", componentID, err,
			)
		}
		log.Info().
			Str("component_id", componentID).
			Msg("PausePowerOnGate succeeded")
	}

	return nil
}

// BringUpControl opens the Carbide power-on gate for
// each compute component, allowing bring-up and power on.
func (m *Manager) BringUpControl(
	ctx context.Context,
	target common.Target,
) error {
	log.Debug().
		Str("components", target.String()).
		Msg("BringUpControl for compute")

	if m.carbideClient == nil {
		return fmt.Errorf("carbide client is not configured")
	}

	if err := target.Validate(); err != nil {
		return fmt.Errorf("target is invalid: %w", err)
	}

	for _, componentID := range target.ComponentIDs {
		if err := m.carbideClient.AllowIngestionAndPowerOn(
			ctx, componentID, "",
		); err != nil {
			return fmt.Errorf(
				"BringUpControl failed for %s: %w",
				componentID, err,
			)
		}
		log.Info().
			Str("component_id", componentID).
			Msg("BringUpControl succeeded")
	}

	return nil
}

// GetBringUpStatus returns the bring-up state for each
// compute component via Carbide.
func (m *Manager) GetBringUpStatus(
	ctx context.Context,
	target common.Target,
) (map[string]operations.MachineBringUpState, error) {
	log.Debug().
		Str("components", target.String()).
		Msg("GetBringUpStatus for compute")

	if m.carbideClient == nil {
		return nil, fmt.Errorf("carbide client is not configured")
	}

	if err := target.Validate(); err != nil {
		return nil, fmt.Errorf("target is invalid: %w", err)
	}

	result := make(
		map[string]operations.MachineBringUpState,
		len(target.ComponentIDs),
	)
	for _, componentID := range target.ComponentIDs {
		state, err := m.carbideClient.DetermineMachineIngestionState(
			ctx, componentID, "",
		)
		if err != nil {
			return nil, fmt.Errorf(
				"GetBringUpStatus failed for %s: %w",
				componentID, err,
			)
		}
		result[componentID] = carbideToBringUpState(state)
	}

	return result, nil
}

func carbideToBringUpState(
	s carbideapi.BringUpState,
) operations.MachineBringUpState {
	switch s {
	case carbideapi.BringUpStateWaitingForIngestion:
		return operations.MachineBringUpStateWaitingForIngestion
	case carbideapi.BringUpStateMachineNotCreated:
		return operations.MachineBringUpStateMachineNotCreated
	case carbideapi.BringUpStateMachineCreated:
		return operations.MachineBringUpStateMachineCreated
	default:
		return operations.MachineBringUpStateNotDiscovered
	}
}

// isAlreadyInDesiredStateError returns true when Carbide reports that the
// power option is already set to the requested state (idempotent no-op).
func isAlreadyInDesiredStateError(err error) bool {
	if s, ok := status.FromError(err); ok && s.Code() == codes.InvalidArgument {
		return strings.Contains(s.Message(), "already set as")
	}
	return false
}
