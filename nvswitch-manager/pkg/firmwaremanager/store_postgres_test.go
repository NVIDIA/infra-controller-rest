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
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/NVIDIA/infra-controller-rest/nvswitch-manager/pkg/db"
	"github.com/NVIDIA/infra-controller-rest/nvswitch-manager/pkg/db/migrations"
	"github.com/NVIDIA/infra-controller-rest/nvswitch-manager/pkg/db/postgres"
	"github.com/NVIDIA/infra-controller-rest/nvswitch-manager/pkg/db/testutil"
	"github.com/NVIDIA/infra-controller-rest/nvswitch-manager/pkg/firmwaremanager"
	"github.com/NVIDIA/infra-controller-rest/nvswitch-manager/pkg/objects/nvswitch"
)

func skipIfNoDatabase(t *testing.T) {
	t.Helper()
	if os.Getenv("DB_PORT") == "" {
		t.Skip("DB_PORT not set - skipping integration test")
	}
}

func setupPostgresStore(t *testing.T) (*firmwaremanager.PostgresUpdateStore, *postgres.Postgres, func()) {
	t.Helper()
	ctx := context.Background()

	dbConf, err := db.BuildDBConfigFromEnv()
	require.NoError(t, err)

	pg, err := testutil.CreateTestDB(ctx, t, dbConf)
	require.NoError(t, err)

	require.NoError(t, migrations.Migrate(ctx, pg))

	store := firmwaremanager.NewPostgresUpdateStore(pg.DB())
	return store, pg, func() { pg.Close(ctx) }
}

// insertTestNVSwitch inserts a minimal nvswitch row to satisfy the FK constraint on firmware_update.
func insertTestNVSwitch(t *testing.T, pg *postgres.Postgres, bmcMAC, nvosMAC, bmcIP, nvosIP string) uuid.UUID {
	t.Helper()
	id := uuid.New()

	_, err := pg.DB().NewRaw(`
		INSERT INTO nvswitch (uuid, vendor, bmc_mac_address, bmc_ip_address, nvos_mac_address, nvos_ip_address)
		VALUES (?, ?, ?, ?, ?, ?)
	`, id, 1, bmcMAC, bmcIP, nvosMAC, nvosIP).Exec(context.Background())

	require.NoError(t, err)

	return id
}

func TestIntegration_SaveAll_PersistsAll(t *testing.T) {
	skipIfNoDatabase(t)

	ctx := context.Background()
	store, pg, cleanup := setupPostgresStore(t)
	defer cleanup()

	switchUUID := insertTestNVSwitch(t, pg, "aa:bb:cc:dd:ee:01", "aa:bb:cc:dd:ee:02", "10.0.0.1", "10.0.0.2")
	update1 := makeUpdate(switchUUID, nvswitch.BMC)
	update2 := makeUpdate(switchUUID, nvswitch.BIOS)

	require.NoError(t, store.SaveAll(ctx, []*firmwaremanager.FirmwareUpdate{update1, update2}))

	_, err := store.Get(ctx, update1.ID)
	require.NoError(t, err)

	_, err = store.Get(ctx, update2.ID)
	require.NoError(t, err)
}

func TestIntegration_SaveAll_Upsert(t *testing.T) {
	skipIfNoDatabase(t)

	ctx := context.Background()
	store, pg, cleanup := setupPostgresStore(t)
	defer cleanup()

	switchUUID := insertTestNVSwitch(t, pg, "aa:bb:cc:dd:ee:01", "aa:bb:cc:dd:ee:02", "10.0.0.1", "10.0.0.2")
	update1 := makeUpdate(switchUUID, nvswitch.BMC)
	update2 := makeUpdate(switchUUID, nvswitch.BIOS)

	require.NoError(t, store.SaveAll(ctx, []*firmwaremanager.FirmwareUpdate{update1, update2}))

	update1.SetState(firmwaremanager.StateCompleted)
	update2.SetState(firmwaremanager.StateFailed)
	require.NoError(t, store.SaveAll(ctx, []*firmwaremanager.FirmwareUpdate{update1, update2}))

	got1, err := store.Get(ctx, update1.ID)
	require.NoError(t, err)
	assert.Equal(t, firmwaremanager.StateCompleted, got1.State)
	assert.Equal(t, nvswitch.BMC, got1.Component)

	got2, err := store.Get(ctx, update2.ID)
	require.NoError(t, err)
	assert.Equal(t, firmwaremanager.StateFailed, got2.State)
	assert.Equal(t, nvswitch.BIOS, got2.Component)
}

// TestIntegration_SaveAll_RollbackOnFailure verifies that SaveAll is atomic: if any
// record in the batch fails to persist, the entire batch is rolled back.
//
// The FK constraint on firmware_update.switch_uuid → nvswitch.uuid is used to force
// a failure on the second record (orphanUUID has no nvswitch row). The test then
// confirms that the first record was also not persisted.
func TestIntegration_SaveAll_RollbackOnFailure(t *testing.T) {
	skipIfNoDatabase(t)

	ctx := context.Background()
	store, pg, cleanup := setupPostgresStore(t)
	defer cleanup()

	validUUID := insertTestNVSwitch(t, pg, "aa:bb:cc:dd:ee:01", "aa:bb:cc:dd:ee:02", "10.0.0.1", "10.0.0.2")
	orphanUUID := uuid.New() // no nvswitch row → triggers FK violation

	update1 := makeUpdate(validUUID, nvswitch.BMC)
	update2 := makeUpdate(orphanUUID, nvswitch.BMC)

	err := store.SaveAll(ctx, []*firmwaremanager.FirmwareUpdate{update1, update2})
	require.Error(t, err, "SaveAll should fail due to FK violation on update2")

	_, err = store.Get(ctx, update1.ID)
	require.ErrorIs(t, err, firmwaremanager.ErrUpdateNotFound, "update1 must not be persisted after rollback")
}

func TestIntegration_GetPendingUpdates(t *testing.T) {
	skipIfNoDatabase(t)

	ctx := context.Background()

	t.Run("queued_no_predecessor_returned", func(t *testing.T) {
		store, pg, cleanup := setupPostgresStore(t)
		defer cleanup()
		switchUUID := insertTestNVSwitch(t, pg, "aa:bb:cc:dd:ee:01", "aa:bb:cc:dd:ee:02", "10.0.0.1", "10.0.0.2")
		update := makeUpdate(switchUUID, nvswitch.BMC)
		require.NoError(t, store.Save(ctx, update))

		pending, err := store.GetPendingUpdates(ctx, 10)
		require.NoError(t, err)
		require.Len(t, pending, 1)
		assert.Equal(t, update.ID, pending[0].ID)
	})

	t.Run("queued_after_pred_finishes", func(t *testing.T) {
		store, pg, cleanup := setupPostgresStore(t)
		defer cleanup()
		switchUUID := insertTestNVSwitch(t, pg, "aa:bb:cc:dd:ee:01", "aa:bb:cc:dd:ee:02", "10.0.0.1", "10.0.0.2")

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
		store, pg, cleanup := setupPostgresStore(t)
		defer cleanup()
		switchUUID := insertTestNVSwitch(t, pg, "aa:bb:cc:dd:ee:01", "aa:bb:cc:dd:ee:02", "10.0.0.1", "10.0.0.2")

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
		store, pg, cleanup := setupPostgresStore(t)
		defer cleanup()
		switchUUID := insertTestNVSwitch(t, pg, "aa:bb:cc:dd:ee:01", "aa:bb:cc:dd:ee:02", "10.0.0.1", "10.0.0.2")
		update := makeUpdate(switchUUID, nvswitch.BMC)
		update.SetState(firmwaremanager.StateInstall)
		require.NoError(t, store.Save(ctx, update))

		pending, err := store.GetPendingUpdates(ctx, 10)
		require.NoError(t, err)
		require.Len(t, pending, 1)
		assert.Equal(t, update.ID, pending[0].ID)
	})

	t.Run("terminal_update_not_returned", func(t *testing.T) {
		store, pg, cleanup := setupPostgresStore(t)
		defer cleanup()
		switchUUID := insertTestNVSwitch(t, pg, "aa:bb:cc:dd:ee:01", "aa:bb:cc:dd:ee:02", "10.0.0.1", "10.0.0.2")

		for _, state := range []firmwaremanager.UpdateState{
			firmwaremanager.StateCompleted,
			firmwaremanager.StateFailed,
			firmwaremanager.StateCancelled,
		} {
			u := makeUpdate(switchUUID, nvswitch.BMC)
			u.SetState(state)
			require.NoError(t, store.Save(ctx, u))
		}

		pending, err := store.GetPendingUpdates(ctx, 10)
		require.NoError(t, err)
		assert.Empty(t, pending)
	})

	t.Run("respects_limit", func(t *testing.T) {
		store, pg, cleanup := setupPostgresStore(t)
		defer cleanup()
		switchUUID := insertTestNVSwitch(t, pg, "aa:bb:cc:dd:ee:01", "aa:bb:cc:dd:ee:02", "10.0.0.1", "10.0.0.2")

		for _, comp := range []nvswitch.Component{nvswitch.BMC, nvswitch.BIOS, nvswitch.CPLD} {
			require.NoError(t, store.Save(ctx, makeUpdate(switchUUID, comp)))
		}

		pending, err := store.GetPendingUpdates(ctx, 2)
		require.NoError(t, err)
		assert.Len(t, pending, 2)
	})
}

func TestIntegration_GetLatestBundleBySwitch(t *testing.T) {
	skipIfNoDatabase(t)

	ctx := context.Background()

	t.Run("returns_only_latest_bundle_not_older_ones", func(t *testing.T) {
		store, pg, cleanup := setupPostgresStore(t)
		defer cleanup()
		switchUUID := insertTestNVSwitch(t, pg, "aa:bb:cc:dd:ee:01", "aa:bb:cc:dd:ee:02", "10.0.0.1", "10.0.0.2")

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
		store, pg, cleanup := setupPostgresStore(t)
		defer cleanup()
		switchUUID := insertTestNVSwitch(t, pg, "aa:bb:cc:dd:ee:01", "aa:bb:cc:dd:ee:02", "10.0.0.1", "10.0.0.2")

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

func TestIntegration_GetActive(t *testing.T) {
	skipIfNoDatabase(t)

	ctx := context.Background()

	t.Run("returns_non_terminal_update", func(t *testing.T) {
		store, pg, cleanup := setupPostgresStore(t)
		defer cleanup()
		switchUUID := insertTestNVSwitch(t, pg, "aa:bb:cc:dd:ee:01", "aa:bb:cc:dd:ee:02", "10.0.0.1", "10.0.0.2")
		update := makeUpdate(switchUUID, nvswitch.BMC)
		require.NoError(t, store.Save(ctx, update))

		got, err := store.GetActive(ctx, switchUUID, nvswitch.BMC)
		require.NoError(t, err)
		require.NotNil(t, got)
		assert.Equal(t, update.ID, got.ID)
	})

	t.Run("returns_nil_for_terminal_update", func(t *testing.T) {
		store, pg, cleanup := setupPostgresStore(t)
		defer cleanup()
		switchUUID := insertTestNVSwitch(t, pg, "aa:bb:cc:dd:ee:01", "aa:bb:cc:dd:ee:02", "10.0.0.1", "10.0.0.2")
		update := makeUpdate(switchUUID, nvswitch.BMC)
		update.SetState(firmwaremanager.StateCompleted)
		require.NoError(t, store.Save(ctx, update))

		got, err := store.GetActive(ctx, switchUUID, nvswitch.BMC)
		require.NoError(t, err)
		assert.Nil(t, got)
	})

	t.Run("does_not_return_different_component", func(t *testing.T) {
		store, pg, cleanup := setupPostgresStore(t)
		defer cleanup()
		switchUUID := insertTestNVSwitch(t, pg, "aa:bb:cc:dd:ee:01", "aa:bb:cc:dd:ee:02", "10.0.0.1", "10.0.0.2")
		update := makeUpdate(switchUUID, nvswitch.BMC)
		require.NoError(t, store.Save(ctx, update))

		got, err := store.GetActive(ctx, switchUUID, nvswitch.BIOS)
		require.NoError(t, err)
		assert.Nil(t, got)
	})
}

func TestIntegration_GetAnyActiveForSwitch(t *testing.T) {
	skipIfNoDatabase(t)

	ctx := context.Background()

	t.Run("returns_active_update_for_switch", func(t *testing.T) {
		store, pg, cleanup := setupPostgresStore(t)
		defer cleanup()
		switchUUID := insertTestNVSwitch(t, pg, "aa:bb:cc:dd:ee:01", "aa:bb:cc:dd:ee:02", "10.0.0.1", "10.0.0.2")
		update := makeUpdate(switchUUID, nvswitch.BMC)
		require.NoError(t, store.Save(ctx, update))

		got, err := store.GetAnyActiveForSwitch(ctx, switchUUID)
		require.NoError(t, err)
		require.NotNil(t, got)
		assert.Equal(t, switchUUID, got.SwitchUUID)
	})

	t.Run("returns_nil_when_all_terminal", func(t *testing.T) {
		store, pg, cleanup := setupPostgresStore(t)
		defer cleanup()
		switchUUID := insertTestNVSwitch(t, pg, "aa:bb:cc:dd:ee:01", "aa:bb:cc:dd:ee:02", "10.0.0.1", "10.0.0.2")
		update := makeUpdate(switchUUID, nvswitch.BMC)
		update.SetState(firmwaremanager.StateFailed)
		require.NoError(t, store.Save(ctx, update))

		got, err := store.GetAnyActiveForSwitch(ctx, switchUUID)
		require.NoError(t, err)
		assert.Nil(t, got)
	})

	t.Run("does_not_return_different_switch", func(t *testing.T) {
		store, pg, cleanup := setupPostgresStore(t)
		defer cleanup()
		switchUUID := insertTestNVSwitch(t, pg, "aa:bb:cc:dd:ee:01", "aa:bb:cc:dd:ee:02", "10.0.0.1", "10.0.0.2")
		update := makeUpdate(switchUUID, nvswitch.BMC)
		require.NoError(t, store.Save(ctx, update))

		otherSwitch := insertTestNVSwitch(t, pg, "bb:cc:dd:ee:ff:01", "bb:cc:dd:ee:ff:02", "10.0.1.1", "10.0.1.2")
		got, err := store.GetAnyActiveForSwitch(ctx, otherSwitch)
		require.NoError(t, err)
		assert.Nil(t, got)
	})
}

func TestIntegration_CancelRemainingInBundle(t *testing.T) {
	skipIfNoDatabase(t)

	ctx := context.Background()

	t.Run("cancels_queued_updates_after_sequence", func(t *testing.T) {
		store, pg, cleanup := setupPostgresStore(t)
		defer cleanup()
		switchUUID := insertTestNVSwitch(t, pg, "aa:bb:cc:dd:ee:01", "aa:bb:cc:dd:ee:02", "10.0.0.1", "10.0.0.2")
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
		store, pg, cleanup := setupPostgresStore(t)
		defer cleanup()
		switchUUID := insertTestNVSwitch(t, pg, "aa:bb:cc:dd:ee:01", "aa:bb:cc:dd:ee:02", "10.0.0.1", "10.0.0.2")
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
		store, pg, cleanup := setupPostgresStore(t)
		defer cleanup()
		switchUUID := insertTestNVSwitch(t, pg, "aa:bb:cc:dd:ee:01", "aa:bb:cc:dd:ee:02", "10.0.0.1", "10.0.0.2")
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

func TestIntegration_Delete(t *testing.T) {
	skipIfNoDatabase(t)

	ctx := context.Background()

	t.Run("deletes_existing_record", func(t *testing.T) {
		store, pg, cleanup := setupPostgresStore(t)
		defer cleanup()
		switchUUID := insertTestNVSwitch(t, pg, "aa:bb:cc:dd:ee:01", "aa:bb:cc:dd:ee:02", "10.0.0.1", "10.0.0.2")
		update := makeUpdate(switchUUID, nvswitch.BMC)
		require.NoError(t, store.Save(ctx, update))

		require.NoError(t, store.Delete(ctx, update.ID))

		_, err := store.Get(ctx, update.ID)
		require.ErrorIs(t, err, firmwaremanager.ErrUpdateNotFound)
	})

	// Unlike InMemoryUpdateStore which returns ErrUpdateNotFound, the postgres
	// implementation silently succeeds when deleting a non-existent record
	// because it does not check RowsAffected.
	t.Run("no_error_on_not_found", func(t *testing.T) {
		store, _, cleanup := setupPostgresStore(t)
		defer cleanup()

		err := store.Delete(ctx, uuid.New())
		require.NoError(t, err)
	})
}
