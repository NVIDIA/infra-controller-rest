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

package migrations

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/uptrace/bun"
)

func instanceAutoNetworkRenameUpMigration(ctx context.Context, db *bun.DB) error {
	// Start transaction
	tx, terr := db.BeginTx(ctx, &sql.TxOptions{})
	if terr != nil {
		handlePanic(terr, "failed to begin transaction")
	}

	// Align the column with the renamed `AutoNetwork` model field. Two
	// starting states are possible:
	//   - Existing deployments carry only `network_auto`            -> rename it.
	//   - Fresh DBs build `instance` from the current model (which
	//     already has `auto_network`); the preceding ADD-COLUMN
	//     migration then adds a redundant `network_auto`            -> drop it.
	res, err := tx.Exec("SELECT column_name FROM information_schema.columns WHERE table_name = 'instance' AND column_name = 'network_auto'")
	handleError(tx, err)
	networkAutoRowsAffected, err := res.RowsAffected()
	handleError(tx, err)
	res, err = tx.Exec("SELECT column_name FROM information_schema.columns WHERE table_name = 'instance' AND column_name = 'auto_network'")
	handleError(tx, err)
	autoNetworkRowsAffected, err := res.RowsAffected()
	handleError(tx, err)

	if networkAutoRowsAffected > 0 && autoNetworkRowsAffected == 0 {
		_, err := tx.Exec("ALTER TABLE instance RENAME COLUMN network_auto TO auto_network")
		handleError(tx, err)
	} else if networkAutoRowsAffected > 0 && autoNetworkRowsAffected > 0 {
		_, err := tx.Exec("ALTER TABLE instance DROP COLUMN network_auto")
		handleError(tx, err)
	} else {
		fmt.Println("network_auto rename to auto_network: Migration skipped. Either the column does not exist or already renamed")
	}

	terr = tx.Commit()
	if terr != nil {
		handlePanic(terr, "failed to commit transaction")
	}

	fmt.Print(" [up migration] ")
	return nil
}

func init() {
	Migrations.MustRegister(instanceAutoNetworkRenameUpMigration, func(ctx context.Context, db *bun.DB) error {
		fmt.Print(" [down migration] ")
		return nil
	})
}
