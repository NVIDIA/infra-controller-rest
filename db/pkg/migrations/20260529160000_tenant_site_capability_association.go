// SPDX-FileCopyrightText: Copyright (c) 2026 NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package migrations

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/NVIDIA/infra-controller-rest/db/pkg/db/model"
	"github.com/uptrace/bun"
)

// tenantSiteCapabilityAssociationUpMigration creates the
// tenant_site_capability_association table with its indexes and a conservative
// backfill so that tenants that already have the tenant-global
// TargetedInstanceCreation capability keep their current effective behavior.
//
// Backfill (conservative strategy from the HLD): for every Tenant whose
// config.targetedInstanceCreation is true, enable the capability for all of the
// Sites it could implicitly reach today, i.e. the union of:
//   - Sites it is explicitly associated with via tenant_site, and
//   - Sites belonging to any Infrastructure Provider it has a Ready tenant_account with.
func tenantSiteCapabilityAssociationUpMigration(ctx context.Context, db *bun.DB) error {
	// Start transaction
	tx, terr := db.BeginTx(ctx, &sql.TxOptions{})
	if terr != nil {
		handlePanic(terr, "failed to begin transaction")
	}

	// Create tenant_site_capability_association table
	_, err := tx.NewCreateTable().Model((*model.TenantSiteCapabilityAssociation)(nil)).IfNotExists().Exec(ctx)
	handleError(tx, err)
	fmt.Print(" [up migration] Created tenant_site_capability_association table successfully.")

	// At most one active row per (tenant, site). Partial so that a soft-deleted
	// row does not block re-creating the association for the same pair.
	_, err = tx.Exec("CREATE UNIQUE INDEX IF NOT EXISTS tsca_tenant_site_uniq ON tenant_site_capability_association (tenant_id, site_id) WHERE deleted IS NULL")
	handleError(tx, err)
	fmt.Print(" [up migration] Created unique index tsca_tenant_site_uniq successfully.")

	// List/filter capabilities by tenant
	_, err = tx.Exec("CREATE INDEX IF NOT EXISTS tsca_tenant_id_idx ON tenant_site_capability_association (tenant_id)")
	handleError(tx, err)

	// Bulk updates and queries "all sites under provider P" for a tenant
	_, err = tx.Exec("CREATE INDEX IF NOT EXISTS tsca_infrastructure_provider_id_idx ON tenant_site_capability_association (infrastructure_provider_id)")
	handleError(tx, err)

	// Resolve effective capability when handling instance/machine APIs keyed by site
	_, err = tx.Exec("CREATE INDEX IF NOT EXISTS tsca_site_id_idx ON tenant_site_capability_association (site_id)")
	handleError(tx, err)
	fmt.Print(" [up migration] Created supporting indexes on tenant_site_capability_association successfully.")

	// Conservative backfill: preserve existing tenant-global behavior by enabling
	// the capability on every Site each privileged Tenant could implicitly reach today.
	_, err = tx.ExecContext(ctx, `
		INSERT INTO tenant_site_capability_association
			(id, tenant_id, site_id, infrastructure_provider_id, targeted_instance_creation, created, updated, created_by)
		SELECT gen_random_uuid(), x.tenant_id, x.site_id, x.infrastructure_provider_id, true, now(), now(), x.created_by
		FROM (
			-- Sites the Tenant is explicitly associated with
			SELECT t.id AS tenant_id, s.id AS site_id, s.infrastructure_provider_id AS infrastructure_provider_id, t.created_by AS created_by
			FROM tenant t
			JOIN tenant_site ts ON ts.tenant_id = t.id AND ts.deleted IS NULL
			JOIN site s ON s.id = ts.site_id AND s.deleted IS NULL
			WHERE t.deleted IS NULL
				AND COALESCE((t.config->>'targetedInstanceCreation')::boolean, false) = true
			UNION
			-- Sites reachable via a Ready tenant_account for the Site's provider
			SELECT t.id, s.id, s.infrastructure_provider_id, t.created_by
			FROM tenant t
			JOIN tenant_account ta ON ta.tenant_id = t.id AND ta.deleted IS NULL AND ta.status = ?
			JOIN site s ON s.infrastructure_provider_id = ta.infrastructure_provider_id AND s.deleted IS NULL
			WHERE t.deleted IS NULL
				AND COALESCE((t.config->>'targetedInstanceCreation')::boolean, false) = true
		) x
		ON CONFLICT (tenant_id, site_id) WHERE deleted IS NULL DO NOTHING
	`, model.TenantAccountStatusReady)
	handleError(tx, err)
	fmt.Print(" [up migration] Backfilled tenant_site_capability_association rows for privileged tenants successfully.")

	terr = tx.Commit()
	if terr != nil {
		handlePanic(terr, "failed to commit transaction")
	}

	return nil
}

func init() {
	Migrations.MustRegister(tenantSiteCapabilityAssociationUpMigration, func(ctx context.Context, db *bun.DB) error {
		_, err := db.NewDropTable().Model((*model.TenantSiteCapabilityAssociation)(nil)).IfExists().Exec(ctx)
		if err != nil {
			return err
		}
		fmt.Print(" [down migration] Dropped tenant_site_capability_association table.")
		return nil
	})
}
