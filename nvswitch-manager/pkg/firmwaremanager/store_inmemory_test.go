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

package firmwaremanager_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/NVIDIA/infra-controller-rest/nvswitch-manager/pkg/firmwaremanager"
	"github.com/NVIDIA/infra-controller-rest/nvswitch-manager/pkg/objects/nvswitch"
)

func makeUpdate(switchUUID uuid.UUID, component nvswitch.Component) *firmwaremanager.FirmwareUpdate {
	return firmwaremanager.NewFirmwareUpdate(switchUUID, component, "1.0.0", firmwaremanager.StrategyRedfish, "1.0.1")
}

func TestInMemory_SaveAll_Empty(t *testing.T) {
	store := firmwaremanager.NewInMemoryUpdateStore()
	ctx := context.Background()

	require.NoError(t, store.SaveAll(ctx, nil))
	require.NoError(t, store.SaveAll(ctx, []*firmwaremanager.FirmwareUpdate{}))
}

func TestInMemory_SaveAll_PersistsAll(t *testing.T) {
	store := firmwaremanager.NewInMemoryUpdateStore()
	ctx := context.Background()
	switchUUID := uuid.New()

	update1 := makeUpdate(switchUUID, nvswitch.BMC)
	update2 := makeUpdate(switchUUID, nvswitch.BIOS)

	require.NoError(t, store.SaveAll(ctx, []*firmwaremanager.FirmwareUpdate{update1, update2}))

	_, err := store.Get(ctx, update1.ID)
	require.NoError(t, err)
	_, err = store.Get(ctx, update2.ID)
	require.NoError(t, err)
}

func TestInMemory_SaveAll_Upsert(t *testing.T) {
	store := firmwaremanager.NewInMemoryUpdateStore()
	ctx := context.Background()
	update := makeUpdate(uuid.New(), nvswitch.BMC)

	require.NoError(t, store.SaveAll(ctx, []*firmwaremanager.FirmwareUpdate{update}))

	update.SetState(firmwaremanager.StateCompleted)
	require.NoError(t, store.SaveAll(ctx, []*firmwaremanager.FirmwareUpdate{update}))

	got, err := store.Get(ctx, update.ID)
	require.NoError(t, err)
	assert.Equal(t, firmwaremanager.StateCompleted, got.State)
}

func TestInMemory_GetPendingUpdates(t *testing.T) {
	ctx := context.Background()

	t.Run("queued_no_predecessor_returned", func(t *testing.T) {
		store := firmwaremanager.NewInMemoryUpdateStore()
		update := makeUpdate(uuid.New(), nvswitch.BMC)
		require.NoError(t, store.Save(ctx, update))

		pending, err := store.GetPendingUpdates(ctx, 10)
		require.NoError(t, err)
		require.Len(t, pending, 1)
		assert.Equal(t, update.ID, pending[0].ID)
	})

	t.Run("queued_after_pred_finishes", func(t *testing.T) {
		store := firmwaremanager.NewInMemoryUpdateStore()
		switchUUID := uuid.New()

		pred := makeUpdate(switchUUID, nvswitch.BMC)
		pred.SetState(firmwaremanager.StateCompleted)
		require.NoError(t, store.Save(ctx, pred))

		next := makeUpdate(switchUUID, nvswitch.BIOS)
		next.WithSequencing(nil, 2, &pred.ID)
		require.NoError(t, store.Save(ctx, next))

		pending, err := store.GetPendingUpdates(ctx, 10)
		require.NoError(t, err)
		ids := make([]uuid.UUID, len(pending))
		for i, p := range pending {
			ids[i] = p.ID
		}
		assert.Contains(t, ids, next.ID)
	})

	t.Run("queued_while_pred_in_progress", func(t *testing.T) {
		store := firmwaremanager.NewInMemoryUpdateStore()
		switchUUID := uuid.New()

		pred := makeUpdate(switchUUID, nvswitch.BMC) // still QUEUED
		require.NoError(t, store.Save(ctx, pred))

		next := makeUpdate(switchUUID, nvswitch.BIOS)
		next.WithSequencing(nil, 2, &pred.ID)
		require.NoError(t, store.Save(ctx, next))

		pending, err := store.GetPendingUpdates(ctx, 10)
		require.NoError(t, err)
		require.Len(t, pending, 1)
		assert.Equal(t, pred.ID, pending[0].ID)
	})

	t.Run("active_update_returned", func(t *testing.T) {
		store := firmwaremanager.NewInMemoryUpdateStore()
		update := makeUpdate(uuid.New(), nvswitch.BMC)
		update.SetState(firmwaremanager.StateInstall)
		require.NoError(t, store.Save(ctx, update))

		pending, err := store.GetPendingUpdates(ctx, 10)
		require.NoError(t, err)
		require.Len(t, pending, 1)
		assert.Equal(t, update.ID, pending[0].ID)
	})

	t.Run("terminal_update_not_returned", func(t *testing.T) {
		store := firmwaremanager.NewInMemoryUpdateStore()
		for _, state := range []firmwaremanager.UpdateState{
			firmwaremanager.StateCompleted,
			firmwaremanager.StateFailed,
			firmwaremanager.StateCancelled,
		} {
			u := makeUpdate(uuid.New(), nvswitch.BMC)
			u.SetState(state)
			require.NoError(t, store.Save(ctx, u))
		}

		pending, err := store.GetPendingUpdates(ctx, 10)
		require.NoError(t, err)
		assert.Empty(t, pending)
	})

	t.Run("respects_limit", func(t *testing.T) {
		store := firmwaremanager.NewInMemoryUpdateStore()
		switchUUID := uuid.New()
		for _, comp := range []nvswitch.Component{nvswitch.BMC, nvswitch.BIOS, nvswitch.CPLD} {
			require.NoError(t, store.Save(ctx, makeUpdate(switchUUID, comp)))
		}

		pending, err := store.GetPendingUpdates(ctx, 2)
		require.NoError(t, err)
		assert.Len(t, pending, 2)
	})
}

func TestInMemory_GetLatestBundleBySwitch(t *testing.T) {
	ctx := context.Background()

	t.Run("returns_only_latest_bundle_not_older_ones", func(t *testing.T) {
		store := firmwaremanager.NewInMemoryUpdateStore()
		switchUUID := uuid.New()

		oldBundleID := uuid.New()
		old1 := makeUpdate(switchUUID, nvswitch.BMC)
		old1.BundleUpdateID = &oldBundleID
		old1.CreatedAt = time.Now().Add(-time.Second)
		require.NoError(t, store.Save(ctx, old1))

		newBundleID := uuid.New()
		new1 := makeUpdate(switchUUID, nvswitch.BMC)
		new2 := makeUpdate(switchUUID, nvswitch.BIOS)
		new1.WithSequencing(&newBundleID, 1, nil)
		new2.WithSequencing(&newBundleID, 2, &new1.ID)
		require.NoError(t, store.SaveAll(ctx, []*firmwaremanager.FirmwareUpdate{new1, new2}))

		got, err := store.GetLatestBundleBySwitch(ctx, switchUUID)
		require.NoError(t, err)
		require.Len(t, got, 2)
		ids := []uuid.UUID{got[0].ID, got[1].ID}
		assert.Contains(t, ids, new1.ID)
		assert.Contains(t, ids, new2.ID)
	})

	t.Run("single_component_returns_only_latest", func(t *testing.T) {
		store := firmwaremanager.NewInMemoryUpdateStore()
		switchUUID := uuid.New()

		old := makeUpdate(switchUUID, nvswitch.BMC)
		old.CreatedAt = time.Now().Add(-time.Second)
		require.NoError(t, store.Save(ctx, old))

		newer := makeUpdate(switchUUID, nvswitch.BMC)
		require.NoError(t, store.Save(ctx, newer))

		got, err := store.GetLatestBundleBySwitch(ctx, switchUUID)
		require.NoError(t, err)
		require.Len(t, got, 1)
		assert.Equal(t, newer.ID, got[0].ID)
	})
}

func TestInMemory_GetActive(t *testing.T) {
	ctx := context.Background()

	t.Run("returns_non_terminal_update", func(t *testing.T) {
		store := firmwaremanager.NewInMemoryUpdateStore()
		switchUUID := uuid.New()
		update := makeUpdate(switchUUID, nvswitch.BMC)
		require.NoError(t, store.Save(ctx, update))

		got, err := store.GetActive(ctx, switchUUID, nvswitch.BMC)
		require.NoError(t, err)
		require.NotNil(t, got)
		assert.Equal(t, update.ID, got.ID)
	})

	t.Run("returns_nil_for_terminal_update", func(t *testing.T) {
		store := firmwaremanager.NewInMemoryUpdateStore()
		switchUUID := uuid.New()
		update := makeUpdate(switchUUID, nvswitch.BMC)
		update.SetState(firmwaremanager.StateCompleted)
		require.NoError(t, store.Save(ctx, update))

		got, err := store.GetActive(ctx, switchUUID, nvswitch.BMC)
		require.NoError(t, err)
		assert.Nil(t, got)
	})

	t.Run("does_not_return_different_component", func(t *testing.T) {
		store := firmwaremanager.NewInMemoryUpdateStore()
		switchUUID := uuid.New()
		update := makeUpdate(switchUUID, nvswitch.BMC)
		require.NoError(t, store.Save(ctx, update))

		got, err := store.GetActive(ctx, switchUUID, nvswitch.BIOS)
		require.NoError(t, err)
		assert.Nil(t, got)
	})
}

func TestInMemory_GetAnyActiveForSwitch(t *testing.T) {
	ctx := context.Background()

	t.Run("returns_active_update_for_switch", func(t *testing.T) {
		store := firmwaremanager.NewInMemoryUpdateStore()
		switchUUID := uuid.New()
		update := makeUpdate(switchUUID, nvswitch.BMC)
		require.NoError(t, store.Save(ctx, update))

		got, err := store.GetAnyActiveForSwitch(ctx, switchUUID)
		require.NoError(t, err)
		require.NotNil(t, got)
		assert.Equal(t, switchUUID, got.SwitchUUID)
	})

	t.Run("returns_nil_when_all_terminal", func(t *testing.T) {
		store := firmwaremanager.NewInMemoryUpdateStore()
		switchUUID := uuid.New()
		update := makeUpdate(switchUUID, nvswitch.BMC)
		update.SetState(firmwaremanager.StateFailed)
		require.NoError(t, store.Save(ctx, update))

		got, err := store.GetAnyActiveForSwitch(ctx, switchUUID)
		require.NoError(t, err)
		assert.Nil(t, got)
	})

	t.Run("does_not_return_different_switch", func(t *testing.T) {
		store := firmwaremanager.NewInMemoryUpdateStore()
		update := makeUpdate(uuid.New(), nvswitch.BMC)
		require.NoError(t, store.Save(ctx, update))

		got, err := store.GetAnyActiveForSwitch(ctx, uuid.New())
		require.NoError(t, err)
		assert.Nil(t, got)
	})
}

func TestInMemory_CancelRemainingInBundle(t *testing.T) {
	ctx := context.Background()

	t.Run("cancels_queued_updates_after_sequence", func(t *testing.T) {
		store := firmwaremanager.NewInMemoryUpdateStore()
		switchUUID := uuid.New()
		bundleID := uuid.New()

		u1 := makeUpdate(switchUUID, nvswitch.BMC)
		u1.WithSequencing(&bundleID, 1, nil)
		u2 := makeUpdate(switchUUID, nvswitch.BIOS)
		u2.WithSequencing(&bundleID, 2, &u1.ID)
		u3 := makeUpdate(switchUUID, nvswitch.CPLD)
		u3.WithSequencing(&bundleID, 3, &u2.ID)
		require.NoError(t, store.SaveAll(ctx, []*firmwaremanager.FirmwareUpdate{u1, u2, u3}))

		cancelled, err := store.CancelRemainingInBundle(ctx, bundleID, 1, nvswitch.BMC)
		require.NoError(t, err)
		assert.Equal(t, 2, cancelled)

		got2, err := store.Get(ctx, u2.ID)
		require.NoError(t, err)
		assert.Equal(t, firmwaremanager.StateCancelled, got2.State)

		got3, err := store.Get(ctx, u3.ID)
		require.NoError(t, err)
		assert.Equal(t, firmwaremanager.StateCancelled, got3.State)
	})

	t.Run("does_not_cancel_non_queued_updates", func(t *testing.T) {
		store := firmwaremanager.NewInMemoryUpdateStore()
		switchUUID := uuid.New()
		bundleID := uuid.New()

		u1 := makeUpdate(switchUUID, nvswitch.BMC)
		u1.WithSequencing(&bundleID, 1, nil)
		u2 := makeUpdate(switchUUID, nvswitch.BIOS)
		u2.WithSequencing(&bundleID, 2, &u1.ID)
		u2.SetState(firmwaremanager.StateInstall) // already active, not QUEUED
		require.NoError(t, store.SaveAll(ctx, []*firmwaremanager.FirmwareUpdate{u1, u2}))

		cancelled, err := store.CancelRemainingInBundle(ctx, bundleID, 1, nvswitch.BMC)
		require.NoError(t, err)
		assert.Equal(t, 0, cancelled)

		got2, err := store.Get(ctx, u2.ID)
		require.NoError(t, err)
		assert.Equal(t, firmwaremanager.StateInstall, got2.State) // unchanged
	})

	t.Run("does_not_affect_other_bundles", func(t *testing.T) {
		store := firmwaremanager.NewInMemoryUpdateStore()
		switchUUID := uuid.New()
		bundleA, bundleB := uuid.New(), uuid.New()

		uA := makeUpdate(switchUUID, nvswitch.BMC)
		uA.WithSequencing(&bundleA, 1, nil)
		uB := makeUpdate(switchUUID, nvswitch.BIOS)
		uB.WithSequencing(&bundleB, 1, nil)
		require.NoError(t, store.SaveAll(ctx, []*firmwaremanager.FirmwareUpdate{uA, uB}))

		_, err := store.CancelRemainingInBundle(ctx, bundleA, 0, nvswitch.BMC)
		require.NoError(t, err)

		gotB, err := store.Get(ctx, uB.ID)
		require.NoError(t, err)
		assert.Equal(t, firmwaremanager.StateQueued, gotB.State) // bundleB unaffected
	})
}

func TestInMemory_Delete(t *testing.T) {
	ctx := context.Background()

	t.Run("deletes_existing_record", func(t *testing.T) {
		store := firmwaremanager.NewInMemoryUpdateStore()
		update := makeUpdate(uuid.New(), nvswitch.BMC)
		require.NoError(t, store.Save(ctx, update))

		require.NoError(t, store.Delete(ctx, update.ID))

		_, err := store.Get(ctx, update.ID)
		require.ErrorIs(t, err, firmwaremanager.ErrUpdateNotFound)
	})

	t.Run("errors_on_not_found", func(t *testing.T) {
		store := firmwaremanager.NewInMemoryUpdateStore()
		err := store.Delete(ctx, uuid.New())
		require.ErrorIs(t, err, firmwaremanager.ErrUpdateNotFound)
	})
}
