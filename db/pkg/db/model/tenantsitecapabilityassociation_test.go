// SPDX-FileCopyrightText: Copyright (c) 2026 NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package model

import (
	"context"
	"fmt"
	"testing"

	"github.com/NVIDIA/infra-controller-rest/common/pkg/roles"
	"github.com/NVIDIA/infra-controller-rest/db/pkg/db"
	"github.com/NVIDIA/infra-controller-rest/db/pkg/db/paginator"
	stracer "github.com/NVIDIA/infra-controller-rest/db/pkg/tracer"
	"github.com/NVIDIA/infra-controller-rest/db/pkg/util"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	otrace "go.opentelemetry.io/otel/trace"
)

func TestNewTenantSiteCapabilityAssociationDAO(t *testing.T) {
	dbSession := &db.Session{}

	type args struct {
		dbSession *db.Session
	}
	tests := []struct {
		name string
		args args
		want TenantSiteCapabilityAssociationDAO
	}{
		{
			name: "test Tenant Site Capability Association DAO initialization",
			args: args{
				dbSession: dbSession,
			},
			want: &TenantSiteCapabilityAssociationSQLDAO{
				dbSession:  dbSession,
				tracerSpan: stracer.NewTracerSpan(),
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NewTenantSiteCapabilityAssociationDAO(tt.args.dbSession)
			assert.Equal(t, got, tt.want)
		})
	}
}

func TestTenantSiteCapabilityAssociationSQLDAO_GetByID(t *testing.T) {
	ctx := context.Background()
	dbSession := util.TestInitDB(t)
	defer dbSession.Close()

	TestSetupSchema(t, dbSession)

	// Create initial data
	ipOrg := "test-provider-org"
	ipRoles := []string{roles.ProviderAdminRole}
	tnOrg := "test-tenant-org"
	tnRoles := []string{roles.TenantAdminRole}
	ipu := TestBuildUser(t, dbSession, uuid.NewString(), ipOrg, ipRoles)
	ip := TestBuildInfrastructureProvider(t, dbSession, "Test Provider", ipOrg, ipu)

	tnu := TestBuildUser(t, dbSession, uuid.NewString(), tnOrg, tnRoles)
	tn := TestBuildTenant(t, dbSession, "Test Tenant", tnOrg, tnu)

	site := TestBuildSite(t, dbSession, ip, "Test Site 1", ipu)
	tsca := TestBuildTenantSiteCapabilityAssociation(t, dbSession, tn, site, true, tnu)

	type fields struct {
		dbSession *db.Session
	}
	type args struct {
		ctx              context.Context
		tx               *db.Tx
		id               uuid.UUID
		includeRelations []string
	}

	// OTEL Spanner configuration
	_, _, ctx = testCommonTraceProviderSetup(t, ctx)

	tests := []struct {
		name               string
		fields             fields
		args               args
		want               *TenantSiteCapabilityAssociation
		wantErr            bool
		verifyChildSpanner bool
	}{
		{
			name: "test get tenant site capability association by ID",
			fields: fields{
				dbSession: dbSession,
			},
			args: args{
				ctx: ctx,
				tx:  nil,
				id:  tsca.ID,
			},
			want:               tsca,
			verifyChildSpanner: true,
		},
		{
			name: "test get tenant site capability association by ID with relations",
			fields: fields{
				dbSession: dbSession,
			},
			args: args{
				ctx: ctx,
				tx:  nil,
				id:  tsca.ID,
				includeRelations: []string{
					TenantRelationName,
					SiteRelationName,
					InfrastructureProviderRelationName,
				},
			},
			want: tsca,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tscad := TenantSiteCapabilityAssociationSQLDAO{
				dbSession: tt.fields.dbSession,
			}
			got, err := tscad.GetByID(tt.args.ctx, tt.args.tx, tt.args.id, tt.args.includeRelations)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}

			assert.NoError(t, err)

			assert.Equal(t, tt.want.ID, got.ID)
			assert.Equal(t, tt.want.TenantID, got.TenantID)
			assert.Equal(t, tt.want.SiteID, got.SiteID)
			assert.Equal(t, tt.want.InfrastructureProviderID, got.InfrastructureProviderID)
			assert.Equal(t, tt.want.TargetedInstanceCreation, got.TargetedInstanceCreation)

			if tt.args.includeRelations != nil {
				assert.NotNil(t, got.Tenant)
				assert.NotNil(t, got.Site)
				assert.NotNil(t, got.InfrastructureProvider)
			}

			if tt.verifyChildSpanner {
				span := otrace.SpanFromContext(ctx)
				assert.True(t, span.SpanContext().IsValid())
				_, ok := ctx.Value(stracer.TracerKey).(otrace.Tracer)
				assert.True(t, ok)
			}
		})
	}
}

func TestTenantSiteCapabilityAssociationSQLDAO_GetByTenantIDAndSiteID(t *testing.T) {
	ctx := context.Background()
	dbSession := util.TestInitDB(t)
	defer dbSession.Close()

	TestSetupSchema(t, dbSession)

	// Create initial data
	ipOrg := "test-provider-org"
	ipRoles := []string{roles.ProviderAdminRole}
	tnOrg := "test-tenant-org"
	tnRoles := []string{roles.TenantAdminRole}
	ipu := TestBuildUser(t, dbSession, uuid.NewString(), ipOrg, ipRoles)
	ip := TestBuildInfrastructureProvider(t, dbSession, "Test Provider", ipOrg, ipu)

	tnu := TestBuildUser(t, dbSession, uuid.NewString(), tnOrg, tnRoles)
	tn := TestBuildTenant(t, dbSession, "Test Tenant", tnOrg, tnu)

	site := TestBuildSite(t, dbSession, ip, "Test Site 1", ipu)
	tsca := TestBuildTenantSiteCapabilityAssociation(t, dbSession, tn, site, true, tnu)

	type fields struct {
		dbSession *db.Session
	}
	type args struct {
		ctx              context.Context
		tx               *db.Tx
		tenantID         uuid.UUID
		siteID           uuid.UUID
		includeRelations []string
	}

	// OTEL Spanner configuration
	_, _, ctx = testCommonTraceProviderSetup(t, ctx)

	tests := []struct {
		name               string
		fields             fields
		args               args
		want               *TenantSiteCapabilityAssociation
		wantErr            bool
		verifyChildSpanner bool
	}{
		{
			name: "test get TenantSiteCapabilityAssociation by Tenant ID and Site ID",
			fields: fields{
				dbSession: dbSession,
			},
			args: args{
				ctx:      ctx,
				tx:       nil,
				tenantID: tn.ID,
				siteID:   site.ID,
			},
			want:               tsca,
			verifyChildSpanner: true,
		},
		{
			name: "test get TenantSiteCapabilityAssociation by Tenant ID and Site ID with relations",
			fields: fields{
				dbSession: dbSession,
			},
			args: args{
				ctx:      ctx,
				tx:       nil,
				tenantID: tn.ID,
				siteID:   site.ID,
				includeRelations: []string{
					TenantRelationName,
					SiteRelationName,
					InfrastructureProviderRelationName,
				},
			},
			want: tsca,
		},
		{
			name: "test get TenantSiteCapabilityAssociation that does not exist",
			fields: fields{
				dbSession: dbSession,
			},
			args: args{
				ctx:      ctx,
				tx:       nil,
				tenantID: tn.ID,
				siteID:   uuid.New(),
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tscad := TenantSiteCapabilityAssociationSQLDAO{
				dbSession: tt.fields.dbSession,
			}
			got, err := tscad.GetByTenantIDAndSiteID(tt.args.ctx, tt.args.tx, tt.args.tenantID, tt.args.siteID, tt.args.includeRelations)
			if tt.wantErr {
				assert.ErrorIs(t, err, db.ErrDoesNotExist)
				return
			}

			assert.NoError(t, err)

			assert.Equal(t, tt.want.ID, got.ID)
			assert.Equal(t, tt.want.TenantID, got.TenantID)
			assert.Equal(t, tt.want.SiteID, got.SiteID)

			if tt.args.includeRelations != nil {
				assert.NotNil(t, got.Tenant)
				assert.NotNil(t, got.Site)
				assert.NotNil(t, got.InfrastructureProvider)
			}

			if tt.verifyChildSpanner {
				span := otrace.SpanFromContext(ctx)
				assert.True(t, span.SpanContext().IsValid())
				_, ok := ctx.Value(stracer.TracerKey).(otrace.Tracer)
				assert.True(t, ok)
			}
		})
	}
}

func TestTenantSiteCapabilityAssociationSQLDAO_GetAll(t *testing.T) {
	ctx := context.Background()
	dbSession := util.TestInitDB(t)
	defer dbSession.Close()

	TestSetupSchema(t, dbSession)

	// Create initial data
	ipOrg := "test-provider-org"
	ipRoles := []string{roles.ProviderAdminRole}
	tnOrg1 := "test-tenant-org-1"
	tnOrg2 := "test-tenant-org-2"
	tnRoles := []string{roles.TenantAdminRole}
	ipu := TestBuildUser(t, dbSession, uuid.NewString(), ipOrg, ipRoles)
	ip := TestBuildInfrastructureProvider(t, dbSession, "Test Provider", ipOrg, ipu)

	tnu1 := TestBuildUser(t, dbSession, uuid.NewString(), tnOrg1, tnRoles)
	tn1 := TestBuildTenant(t, dbSession, "Test Tenant", tnOrg1, tnu1)

	tnu2 := TestBuildUser(t, dbSession, uuid.NewString(), tnOrg2, tnRoles)
	tn2 := TestBuildTenant(t, dbSession, "Test Tenant 2", tnOrg2, tnu2)

	sites := []*Site{}
	siteCount := 30
	for i := 0; i < siteCount; i++ {
		site := TestBuildSite(t, dbSession, ip, fmt.Sprintf("test-site-%d", i), ipu)
		sites = append(sites, site)
		if i%2 == 0 {
			TestBuildTenantSiteCapabilityAssociation(t, dbSession, tn1, site, true, tnu1)
		} else {
			TestBuildTenantSiteCapabilityAssociation(t, dbSession, tn2, site, false, tnu2)
		}
	}

	type fields struct {
		dbSession *db.Session
	}
	type args struct {
		tenantIDs                 []uuid.UUID
		siteIDs                   []uuid.UUID
		infrastructureProviderIDs []uuid.UUID
		includeRelations          []string
		offset                    *int
		limit                     *int
		orderBy                   *paginator.OrderBy
	}

	// OTEL Spanner configuration
	_, _, ctx = testCommonTraceProviderSetup(t, ctx)

	tests := []struct {
		name               string
		fields             fields
		args               args
		wantCount          int
		wantTotalCount     int
		verifyChildSpanner bool
	}{
		{
			name: "test get all, no filter",
			fields: fields{
				dbSession: dbSession,
			},
			args:               args{},
			wantCount:          paginator.DefaultLimit,
			wantTotalCount:     siteCount,
			verifyChildSpanner: true,
		},
		{
			name: "test get all, filter by tenant ID",
			fields: fields{
				dbSession: dbSession,
			},
			args: args{
				tenantIDs: []uuid.UUID{tn1.ID},
			},
			wantCount:      siteCount / 2,
			wantTotalCount: siteCount / 2,
		},
		{
			name: "test get all, filter by multiple tenant IDs",
			fields: fields{
				dbSession: dbSession,
			},
			args: args{
				tenantIDs: []uuid.UUID{tn1.ID, tn2.ID},
			},
			wantCount:      paginator.DefaultLimit,
			wantTotalCount: siteCount,
		},
		{
			name: "test get all, filter by site ID",
			fields: fields{
				dbSession: dbSession,
			},
			args: args{
				siteIDs: []uuid.UUID{sites[0].ID},
			},
			wantCount:      1,
			wantTotalCount: 1,
		},
		{
			name: "test get all, filter by multiple site IDs",
			fields: fields{
				dbSession: dbSession,
			},
			args: args{
				siteIDs: []uuid.UUID{sites[0].ID, sites[1].ID},
			},
			wantCount:      2,
			wantTotalCount: 2,
		},
		{
			name: "test get all, filter by infrastructure provider ID",
			fields: fields{
				dbSession: dbSession,
			},
			args: args{
				infrastructureProviderIDs: []uuid.UUID{ip.ID},
			},
			wantCount:      paginator.DefaultLimit,
			wantTotalCount: siteCount,
		},
		{
			name: "test get all, with limit",
			fields: fields{
				dbSession: dbSession,
			},
			args: args{
				limit: db.GetIntPtr(10),
			},
			wantCount:      10,
			wantTotalCount: siteCount,
		},
		{
			name: "test get all, with offset",
			fields: fields{
				dbSession: dbSession,
			},
			args: args{
				offset: db.GetIntPtr(10),
			},
			wantCount:      paginator.DefaultLimit,
			wantTotalCount: siteCount,
		},
		{
			name: "test get all, with order by",
			fields: fields{
				dbSession: dbSession,
			},
			args: args{
				orderBy: &paginator.OrderBy{
					Field: "created",
					Order: paginator.OrderDescending,
				},
			},
			wantCount:      paginator.DefaultLimit,
			wantTotalCount: siteCount,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tscad := TenantSiteCapabilityAssociationSQLDAO{
				dbSession: tt.fields.dbSession,
			}
			filter := TenantSiteCapabilityAssociationFilterInput{
				TenantIDs:                 tt.args.tenantIDs,
				SiteIDs:                   tt.args.siteIDs,
				InfrastructureProviderIDs: tt.args.infrastructureProviderIDs,
			}
			page := paginator.PageInput{
				Limit:   tt.args.limit,
				Offset:  tt.args.offset,
				OrderBy: tt.args.orderBy,
			}
			got, count, err := tscad.GetAll(ctx, nil, filter, page, tt.args.includeRelations)

			assert.NoError(t, err)
			assert.Equal(t, tt.wantCount, len(got))
			assert.Equal(t, tt.wantTotalCount, count)

			if tt.args.orderBy != nil {
				assert.Equal(t, sites[siteCount-1].ID, got[0].SiteID)
			}

			if tt.verifyChildSpanner {
				span := otrace.SpanFromContext(ctx)
				assert.True(t, span.SpanContext().IsValid())
				_, ok := ctx.Value(stracer.TracerKey).(otrace.Tracer)
				assert.True(t, ok)
			}
		})
	}
}

func TestTenantSiteCapabilityAssociationSQLDAO_Create(t *testing.T) {
	ctx := context.Background()
	dbSession := util.TestInitDB(t)
	defer dbSession.Close()

	TestSetupSchema(t, dbSession)

	// Create initial data
	ipOrg := "test-provider-org"
	ipRoles := []string{roles.ProviderAdminRole}
	tnOrg := "test-tenant-org"
	tnRoles := []string{roles.TenantAdminRole}
	ipu := TestBuildUser(t, dbSession, uuid.NewString(), ipOrg, ipRoles)
	ip := TestBuildInfrastructureProvider(t, dbSession, "Test Provider", ipOrg, ipu)

	tnu := TestBuildUser(t, dbSession, uuid.NewString(), tnOrg, tnRoles)
	tn := TestBuildTenant(t, dbSession, "Test Tenant", tnOrg, tnu)

	site := TestBuildSite(t, dbSession, ip, "Test Site 1", ipu)

	type fields struct {
		dbSession *db.Session
	}
	type args struct {
		tenantID                 uuid.UUID
		siteID                   uuid.UUID
		infrastructureProviderID uuid.UUID
		targetedInstanceCreation bool
		createdBy                uuid.UUID
	}

	// OTEL Spanner configuration
	_, _, ctx = testCommonTraceProviderSetup(t, ctx)

	tests := []struct {
		name               string
		fields             fields
		args               args
		want               *TenantSiteCapabilityAssociation
		verifyChildSpanner bool
	}{
		{
			name: "test create tenant site capability association",
			fields: fields{
				dbSession: dbSession,
			},
			args: args{
				tenantID:                 tn.ID,
				siteID:                   site.ID,
				infrastructureProviderID: ip.ID,
				targetedInstanceCreation: true,
				createdBy:                tnu.ID,
			},
			want: &TenantSiteCapabilityAssociation{
				TenantID:                 tn.ID,
				SiteID:                   site.ID,
				InfrastructureProviderID: ip.ID,
				TargetedInstanceCreation: true,
				CreatedBy:                tnu.ID,
			},
			verifyChildSpanner: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tscad := TenantSiteCapabilityAssociationSQLDAO{
				dbSession: tt.fields.dbSession,
			}
			input := TenantSiteCapabilityAssociationCreateInput{
				TenantID:                 tt.args.tenantID,
				SiteID:                   tt.args.siteID,
				InfrastructureProviderID: tt.args.infrastructureProviderID,
				TargetedInstanceCreation: tt.args.targetedInstanceCreation,
				CreatedBy:                tt.args.createdBy,
			}
			got, err := tscad.Create(ctx, nil, input)
			assert.NoError(t, err)

			assert.NotNil(t, got.ID)
			assert.Equal(t, tt.want.TenantID, got.TenantID)
			assert.Equal(t, tt.want.SiteID, got.SiteID)
			assert.Equal(t, tt.want.InfrastructureProviderID, got.InfrastructureProviderID)
			assert.Equal(t, tt.want.TargetedInstanceCreation, got.TargetedInstanceCreation)
			assert.Equal(t, tt.want.CreatedBy, got.CreatedBy)

			if tt.verifyChildSpanner {
				span := otrace.SpanFromContext(ctx)
				assert.True(t, span.SpanContext().IsValid())
				_, ok := ctx.Value(stracer.TracerKey).(otrace.Tracer)
				assert.True(t, ok)
			}
		})
	}
}

func TestTenantSiteCapabilityAssociationSQLDAO_Update(t *testing.T) {
	ctx := context.Background()
	dbSession := util.TestInitDB(t)
	defer dbSession.Close()

	TestSetupSchema(t, dbSession)

	// Create initial data
	ipOrg := "test-provider-org"
	ipRoles := []string{roles.ProviderAdminRole}
	tnOrg := "test-tenant-org"
	tnRoles := []string{roles.TenantAdminRole}
	ipu := TestBuildUser(t, dbSession, uuid.NewString(), ipOrg, ipRoles)
	ip := TestBuildInfrastructureProvider(t, dbSession, "Test Provider", ipOrg, ipu)

	tnu := TestBuildUser(t, dbSession, uuid.NewString(), tnOrg, tnRoles)
	tn := TestBuildTenant(t, dbSession, "Test Tenant", tnOrg, tnu)

	site := TestBuildSite(t, dbSession, ip, "Test Site 1", ipu)
	tsca := TestBuildTenantSiteCapabilityAssociation(t, dbSession, tn, site, true, tnu)

	type fields struct {
		dbSession *db.Session
	}
	type args struct {
		id                       uuid.UUID
		targetedInstanceCreation *bool
	}

	// OTEL Spanner configuration
	_, _, ctx = testCommonTraceProviderSetup(t, ctx)

	tests := []struct {
		name               string
		fields             fields
		args               args
		want               *TenantSiteCapabilityAssociation
		verifyChildSpanner bool
	}{
		{
			name: "test update tenant site capability association, disable capability",
			fields: fields{
				dbSession: dbSession,
			},
			args: args{
				id:                       tsca.ID,
				targetedInstanceCreation: db.GetBoolPtr(false),
			},
			want: &TenantSiteCapabilityAssociation{
				ID:                       tsca.ID,
				TargetedInstanceCreation: false,
			},
			verifyChildSpanner: true,
		},
		{
			name: "test update tenant site capability association, enable capability",
			fields: fields{
				dbSession: dbSession,
			},
			args: args{
				id:                       tsca.ID,
				targetedInstanceCreation: db.GetBoolPtr(true),
			},
			want: &TenantSiteCapabilityAssociation{
				ID:                       tsca.ID,
				TargetedInstanceCreation: true,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tscad := TenantSiteCapabilityAssociationSQLDAO{
				dbSession: tt.fields.dbSession,
			}
			input := TenantSiteCapabilityAssociationUpdateInput{
				TenantSiteCapabilityAssociationID: tt.args.id,
				TargetedInstanceCreation:          tt.args.targetedInstanceCreation,
			}
			got, err := tscad.Update(ctx, nil, input)
			assert.NoError(t, err)

			if tt.args.targetedInstanceCreation != nil {
				assert.Equal(t, tt.want.TargetedInstanceCreation, got.TargetedInstanceCreation)
			}

			if tt.verifyChildSpanner {
				span := otrace.SpanFromContext(ctx)
				assert.True(t, span.SpanContext().IsValid())
				_, ok := ctx.Value(stracer.TracerKey).(otrace.Tracer)
				assert.True(t, ok)
			}
		})
	}
}

func TestTenantSiteCapabilityAssociationSQLDAO_Delete(t *testing.T) {
	ctx := context.Background()
	dbSession := util.TestInitDB(t)
	defer dbSession.Close()

	TestSetupSchema(t, dbSession)

	// Create initial data
	ipOrg := "test-provider-org"
	ipRoles := []string{roles.ProviderAdminRole}
	tnOrg := "test-tenant-org"
	tnRoles := []string{roles.TenantAdminRole}
	ipu := TestBuildUser(t, dbSession, uuid.NewString(), ipOrg, ipRoles)
	ip := TestBuildInfrastructureProvider(t, dbSession, "Test Provider", ipOrg, ipu)

	tnu := TestBuildUser(t, dbSession, uuid.NewString(), tnOrg, tnRoles)
	tn := TestBuildTenant(t, dbSession, "Test Tenant", tnOrg, tnu)

	site := TestBuildSite(t, dbSession, ip, "Test Site 1", ipu)
	tsca := TestBuildTenantSiteCapabilityAssociation(t, dbSession, tn, site, true, tnu)

	type fields struct {
		dbSession *db.Session
	}
	type args struct {
		id uuid.UUID
	}

	// OTEL Spanner configuration
	_, _, ctx = testCommonTraceProviderSetup(t, ctx)

	tests := []struct {
		name               string
		fields             fields
		args               args
		verifyChildSpanner bool
	}{
		{
			name: "test delete tenant site capability association",
			fields: fields{
				dbSession: dbSession,
			},
			args: args{
				id: tsca.ID,
			},
			verifyChildSpanner: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tscad := TenantSiteCapabilityAssociationSQLDAO{
				dbSession: tt.fields.dbSession,
			}
			err := tscad.Delete(ctx, nil, tt.args.id)
			assert.NoError(t, err)

			// Check if the tenant site capability association is deleted
			_, err = tscad.GetByID(ctx, nil, tt.args.id, nil)
			assert.ErrorIs(t, err, db.ErrDoesNotExist)

			if tt.verifyChildSpanner {
				span := otrace.SpanFromContext(ctx)
				assert.True(t, span.SpanContext().IsValid())
				_, ok := ctx.Value(stracer.TracerKey).(otrace.Tracer)
				assert.True(t, ok)
			}
		})
	}
}
