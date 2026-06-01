// SPDX-FileCopyrightText: Copyright (c) 2026 NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package model

import (
	"context"
	"database/sql"
	"time"

	"github.com/NVIDIA/infra-controller-rest/db/pkg/db"
	"github.com/NVIDIA/infra-controller-rest/db/pkg/db/paginator"
	stracer "github.com/NVIDIA/infra-controller-rest/db/pkg/tracer"
	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

const (
	// TenantSiteCapabilityAssociationOrderByDefault default field to be used for ordering when none specified
	TenantSiteCapabilityAssociationOrderByDefault = "created"
)

var (
	// TenantSiteCapabilityAssociationOrderByFields is a list of valid order by fields for the TenantSiteCapabilityAssociation model
	TenantSiteCapabilityAssociationOrderByFields = []string{"created", "updated"}
	// TenantSiteCapabilityAssociationRelatedEntities is a list of valid relation by fields for the TenantSiteCapabilityAssociation model
	TenantSiteCapabilityAssociationRelatedEntities = map[string]bool{
		TenantRelationName:                 true,
		SiteRelationName:                   true,
		InfrastructureProviderRelationName: true,
	}
)

// TenantSiteCapabilityAssociation stores per-site capability flags for a Tenant.
// One row per (tenant_id, site_id); PATCH handlers create or update rows when the
// Tenant enables or disables a capability for a resolved set of Sites. See the
// Enhancing-Tenant-Capabilities HLD.
//
// InfrastructureProviderID is denormalized from Site (each Site has exactly one
// Infrastructure Provider) so bulk queries scoped to a Provider don't have to
// join through site; it must stay consistent with the Site's provider.
type TenantSiteCapabilityAssociation struct {
	bun.BaseModel `bun:"table:tenant_site_capability_association,alias:tsca"`

	ID                       uuid.UUID               `bun:"type:uuid,pk"`
	TenantID                 uuid.UUID               `bun:"tenant_id,type:uuid,notnull"`
	Tenant                   *Tenant                 `bun:"rel:belongs-to,join:tenant_id=id"`
	SiteID                   uuid.UUID               `bun:"site_id,type:uuid,notnull"`
	Site                     *Site                   `bun:"rel:belongs-to,join:site_id=id"`
	InfrastructureProviderID uuid.UUID               `bun:"infrastructure_provider_id,type:uuid,notnull"`
	InfrastructureProvider   *InfrastructureProvider `bun:"rel:belongs-to,join:infrastructure_provider_id=id"`
	TargetedInstanceCreation bool                    `bun:"targeted_instance_creation,notnull"`
	Created                  time.Time               `bun:"created,nullzero,notnull,default:current_timestamp"`
	Updated                  time.Time               `bun:"updated,nullzero,notnull,default:current_timestamp"`
	Deleted                  *time.Time              `bun:"deleted,soft_delete"`
	CreatedBy                uuid.UUID               `bun:"type:uuid,notnull"`
}

// TenantSiteCapabilityAssociationCreateInput input parameters for Create method
type TenantSiteCapabilityAssociationCreateInput struct {
	TenantID                 uuid.UUID
	SiteID                   uuid.UUID
	InfrastructureProviderID uuid.UUID
	TargetedInstanceCreation bool
	CreatedBy                uuid.UUID
}

// TenantSiteCapabilityAssociationUpdateInput input parameters for Update method
type TenantSiteCapabilityAssociationUpdateInput struct {
	TenantSiteCapabilityAssociationID uuid.UUID
	TargetedInstanceCreation          *bool
	InfrastructureProviderID          *uuid.UUID
}

// TenantSiteCapabilityAssociationFilterInput filtering options for GetAll method
type TenantSiteCapabilityAssociationFilterInput struct {
	TenantIDs                 []uuid.UUID
	SiteIDs                   []uuid.UUID
	InfrastructureProviderIDs []uuid.UUID
}

var _ bun.BeforeAppendModelHook = (*TenantSiteCapabilityAssociation)(nil)

// BeforeAppendModel is a hook that is called before the model is appended to the query
func (tsca *TenantSiteCapabilityAssociation) BeforeAppendModel(_ context.Context, query bun.Query) error {
	switch query.(type) {
	case *bun.InsertQuery:
		tsca.Created = db.GetCurTime()
		tsca.Updated = db.GetCurTime()
	case *bun.UpdateQuery:
		tsca.Updated = db.GetCurTime()
	}
	return nil
}

var _ bun.BeforeCreateTableHook = (*TenantSiteCapabilityAssociation)(nil)

// BeforeCreateTable is a hook that is called before the table is created
func (tsca *TenantSiteCapabilityAssociation) BeforeCreateTable(_ context.Context, query *bun.CreateTableQuery) error {
	query.ForeignKey(`("tenant_id") REFERENCES "tenant" ("id")`).
		ForeignKey(`("site_id") REFERENCES "site" ("id")`).
		ForeignKey(`("infrastructure_provider_id") REFERENCES "infrastructure_provider" ("id")`).
		ForeignKey(`("created_by") REFERENCES "user" ("id")`)
	return nil
}

// TenantSiteCapabilityAssociationDAO is an interface for interacting with the TenantSiteCapabilityAssociation model
type TenantSiteCapabilityAssociationDAO interface {
	//
	GetByID(ctx context.Context, tx *db.Tx, id uuid.UUID, includeRelations []string) (*TenantSiteCapabilityAssociation, error)
	//
	GetByTenantIDAndSiteID(ctx context.Context, tx *db.Tx, tenantID uuid.UUID, siteID uuid.UUID, includeRelations []string) (*TenantSiteCapabilityAssociation, error)
	//
	GetAll(ctx context.Context, tx *db.Tx, filter TenantSiteCapabilityAssociationFilterInput, page paginator.PageInput, includeRelations []string) ([]TenantSiteCapabilityAssociation, int, error)
	//
	Create(ctx context.Context, tx *db.Tx, input TenantSiteCapabilityAssociationCreateInput) (*TenantSiteCapabilityAssociation, error)
	//
	Update(ctx context.Context, tx *db.Tx, input TenantSiteCapabilityAssociationUpdateInput) (*TenantSiteCapabilityAssociation, error)
	//
	Delete(ctx context.Context, tx *db.Tx, id uuid.UUID) error
}

// TenantSiteCapabilityAssociationSQLDAO is an implementation of the TenantSiteCapabilityAssociationDAO interface
type TenantSiteCapabilityAssociationSQLDAO struct {
	dbSession  *db.Session
	tracerSpan *stracer.TracerSpan
}

// GetByID returns a TenantSiteCapabilityAssociation by ID
func (tscad TenantSiteCapabilityAssociationSQLDAO) GetByID(ctx context.Context, tx *db.Tx, id uuid.UUID, includeRelations []string) (*TenantSiteCapabilityAssociation, error) {
	// Create a child span and set the attributes for current request
	ctx, tscaDAOSpan := tscad.tracerSpan.CreateChildInCurrentContext(ctx, "TenantSiteCapabilityAssociationDAO.GetByID")
	if tscaDAOSpan != nil {
		defer tscaDAOSpan.End()

		tscad.tracerSpan.SetAttribute(tscaDAOSpan, "id", id.String())
	}

	tsca := &TenantSiteCapabilityAssociation{}

	query := db.GetIDB(tx, tscad.dbSession).NewSelect().Model(tsca).Where("tsca.id = ?", id)

	for _, relation := range includeRelations {
		query = query.Relation(relation)
	}

	err := query.Scan(ctx)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, db.ErrDoesNotExist
		}
		return nil, err
	}

	return tsca, nil
}

// GetByTenantIDAndSiteID returns a TenantSiteCapabilityAssociation by Tenant ID and Site ID.
// Returns db.ErrDoesNotExist when no (non-deleted) row exists for the pair.
func (tscad TenantSiteCapabilityAssociationSQLDAO) GetByTenantIDAndSiteID(ctx context.Context, tx *db.Tx, tenantID uuid.UUID, siteID uuid.UUID, includeRelations []string) (*TenantSiteCapabilityAssociation, error) {
	// Create a child span and set the attributes for current request
	ctx, tscaDAOSpan := tscad.tracerSpan.CreateChildInCurrentContext(ctx, "TenantSiteCapabilityAssociationDAO.GetByTenantIDAndSiteID")
	if tscaDAOSpan != nil {
		defer tscaDAOSpan.End()

		tscad.tracerSpan.SetAttribute(tscaDAOSpan, "tenant_id", tenantID.String())
		tscad.tracerSpan.SetAttribute(tscaDAOSpan, "site_id", siteID.String())
	}

	tsca := &TenantSiteCapabilityAssociation{}

	query := db.GetIDB(tx, tscad.dbSession).NewSelect().Model(tsca).
		Where("tsca.tenant_id = ?", tenantID).
		Where("tsca.site_id = ?", siteID)

	for _, relation := range includeRelations {
		query = query.Relation(relation)
	}

	err := query.Scan(ctx)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, db.ErrDoesNotExist
		}
		return nil, err
	}

	return tsca, nil
}

// GetAll returns a list of TenantSiteCapabilityAssociations filtered by tenantIDs, siteIDs, infrastructureProviderIDs, offset, limit and orderBy
// if orderBy is nil, then records are ordered by column specified in TenantSiteCapabilityAssociationOrderByDefault in ascending order
func (tscad TenantSiteCapabilityAssociationSQLDAO) GetAll(ctx context.Context, tx *db.Tx, filter TenantSiteCapabilityAssociationFilterInput, page paginator.PageInput, includeRelations []string) ([]TenantSiteCapabilityAssociation, int, error) {
	// Create a child span and set the attributes for current request
	ctx, tscaDAOSpan := tscad.tracerSpan.CreateChildInCurrentContext(ctx, "TenantSiteCapabilityAssociationDAO.GetAll")
	if tscaDAOSpan != nil {
		defer tscaDAOSpan.End()
	}

	tscas := []TenantSiteCapabilityAssociation{}

	query := db.GetIDB(tx, tscad.dbSession).NewSelect().Model(&tscas)

	if filter.TenantIDs != nil {
		query = query.Where("tsca.tenant_id IN (?)", bun.In(filter.TenantIDs))
		tscad.tracerSpan.SetAttribute(tscaDAOSpan, "tenant_id", filter.TenantIDs)
	}

	if filter.SiteIDs != nil {
		query = query.Where("tsca.site_id IN (?)", bun.In(filter.SiteIDs))
		tscad.tracerSpan.SetAttribute(tscaDAOSpan, "site_id", filter.SiteIDs)
	}

	if filter.InfrastructureProviderIDs != nil {
		query = query.Where("tsca.infrastructure_provider_id IN (?)", bun.In(filter.InfrastructureProviderIDs))
		tscad.tracerSpan.SetAttribute(tscaDAOSpan, "infrastructure_provider_id", filter.InfrastructureProviderIDs)
	}

	for _, relation := range includeRelations {
		query = query.Relation(relation)
	}

	// if no order is passed, set default to make sure objects return always in the same order and pagination works properly
	if page.OrderBy == nil {
		page.OrderBy = paginator.NewDefaultOrderBy(TenantSiteCapabilityAssociationOrderByDefault)
	}

	paginator, err := paginator.NewPaginator(ctx, query, page.Offset, page.Limit, page.OrderBy, TenantSiteCapabilityAssociationOrderByFields)
	if err != nil {
		return nil, 0, err
	}

	err = paginator.Query.Limit(paginator.Limit).Offset(paginator.Offset).Scan(ctx)
	if err != nil {
		return nil, 0, err
	}

	return tscas, paginator.Total, nil
}

// Create creates a new TenantSiteCapabilityAssociation from the given parameters
func (tscad TenantSiteCapabilityAssociationSQLDAO) Create(ctx context.Context, tx *db.Tx, input TenantSiteCapabilityAssociationCreateInput) (*TenantSiteCapabilityAssociation, error) {
	// Create a child span and set the attributes for current request
	ctx, tscaDAOSpan := tscad.tracerSpan.CreateChildInCurrentContext(ctx, "TenantSiteCapabilityAssociationDAO.Create")
	if tscaDAOSpan != nil {
		defer tscaDAOSpan.End()

		tscad.tracerSpan.SetAttribute(tscaDAOSpan, "tenant_id", input.TenantID.String())
		tscad.tracerSpan.SetAttribute(tscaDAOSpan, "site_id", input.SiteID.String())
	}

	tsca := &TenantSiteCapabilityAssociation{
		ID:                       uuid.New(),
		TenantID:                 input.TenantID,
		SiteID:                   input.SiteID,
		InfrastructureProviderID: input.InfrastructureProviderID,
		TargetedInstanceCreation: input.TargetedInstanceCreation,
		CreatedBy:                input.CreatedBy,
	}

	_, err := db.GetIDB(tx, tscad.dbSession).NewInsert().Model(tsca).Exec(ctx)
	if err != nil {
		return nil, err
	}

	ntsca, err := tscad.GetByID(ctx, tx, tsca.ID, nil)
	if err != nil {
		return nil, err
	}

	return ntsca, nil
}

// Update updates an existing TenantSiteCapabilityAssociation from the given parameters
func (tscad TenantSiteCapabilityAssociationSQLDAO) Update(ctx context.Context, tx *db.Tx, input TenantSiteCapabilityAssociationUpdateInput) (*TenantSiteCapabilityAssociation, error) {
	// Create a child span and set the attributes for current request
	ctx, tscaDAOSpan := tscad.tracerSpan.CreateChildInCurrentContext(ctx, "TenantSiteCapabilityAssociationDAO.Update")
	if tscaDAOSpan != nil {
		defer tscaDAOSpan.End()

		tscad.tracerSpan.SetAttribute(tscaDAOSpan, "id", input.TenantSiteCapabilityAssociationID.String())
	}

	tsca := &TenantSiteCapabilityAssociation{
		ID: input.TenantSiteCapabilityAssociationID,
	}

	updatedFields := []string{}

	if input.TargetedInstanceCreation != nil {
		tsca.TargetedInstanceCreation = *input.TargetedInstanceCreation
		updatedFields = append(updatedFields, "targeted_instance_creation")
		tscad.tracerSpan.SetAttribute(tscaDAOSpan, "targeted_instance_creation", *input.TargetedInstanceCreation)
	}

	if input.InfrastructureProviderID != nil {
		tsca.InfrastructureProviderID = *input.InfrastructureProviderID
		updatedFields = append(updatedFields, "infrastructure_provider_id")
		tscad.tracerSpan.SetAttribute(tscaDAOSpan, "infrastructure_provider_id", input.InfrastructureProviderID.String())
	}

	if len(updatedFields) > 0 {
		updatedFields = append(updatedFields, "updated")

		_, err := db.GetIDB(tx, tscad.dbSession).NewUpdate().Model(tsca).Column(updatedFields...).Where("id = ?", input.TenantSiteCapabilityAssociationID).Exec(ctx)
		if err != nil {
			return nil, err
		}
	}

	utsca, err := tscad.GetByID(ctx, tx, input.TenantSiteCapabilityAssociationID, nil)
	if err != nil {
		return nil, err
	}

	return utsca, nil
}

// Delete deletes a TenantSiteCapabilityAssociation by ID
func (tscad TenantSiteCapabilityAssociationSQLDAO) Delete(ctx context.Context, tx *db.Tx, id uuid.UUID) error {
	// Create a child span and set the attributes for current request
	ctx, tscaDAOSpan := tscad.tracerSpan.CreateChildInCurrentContext(ctx, "TenantSiteCapabilityAssociationDAO.Delete")
	if tscaDAOSpan != nil {
		defer tscaDAOSpan.End()

		tscad.tracerSpan.SetAttribute(tscaDAOSpan, "id", id.String())
	}

	tsca := &TenantSiteCapabilityAssociation{
		ID: id,
	}

	_, err := db.GetIDB(tx, tscad.dbSession).NewDelete().Model(tsca).Where("id = ?", id).Exec(ctx)
	if err != nil {
		return err
	}

	return nil
}

// NewTenantSiteCapabilityAssociationDAO creates a new TenantSiteCapabilityAssociationDAO
func NewTenantSiteCapabilityAssociationDAO(dbSession *db.Session) TenantSiteCapabilityAssociationDAO {
	return &TenantSiteCapabilityAssociationSQLDAO{
		dbSession:  dbSession,
		tracerSpan: stracer.NewTracerSpan(),
	}
}
