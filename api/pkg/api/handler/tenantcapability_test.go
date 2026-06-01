// SPDX-FileCopyrightText: Copyright (c) 2026 NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/NVIDIA/infra-controller-rest/api/pkg/api/handler/util/common"
	"github.com/NVIDIA/infra-controller-rest/api/pkg/api/model"
	authz "github.com/NVIDIA/infra-controller-rest/auth/pkg/authorization"
	"github.com/NVIDIA/infra-controller-rest/common/pkg/otelecho"
	cdb "github.com/NVIDIA/infra-controller-rest/db/pkg/db"
	cdbm "github.com/NVIDIA/infra-controller-rest/db/pkg/db/model"
	cdbu "github.com/NVIDIA/infra-controller-rest/db/pkg/util"
	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/uptrace/bun/extra/bundebug"
	oteltrace "go.opentelemetry.io/otel/trace"
	tmocks "go.temporal.io/sdk/mocks"
)

func testTenantCapabilityInitDB(t *testing.T) *cdb.Session {
	dbSession := cdbu.GetTestDBSession(t, false)
	dbSession.DB.AddQueryHook(bundebug.NewQueryHook(
		bundebug.WithEnabled(false),
		bundebug.FromEnv("BUNDEBUG"),
	))
	return dbSession
}

// testTenantCapabilitySetupSchema resets the tables needed for tenant capability tests.
// Referenced tables are created before the tables that hold foreign keys to them.
func testTenantCapabilitySetupSchema(t *testing.T, dbSession *cdb.Session) {
	models := []interface{}{
		(*cdbm.User)(nil),
		(*cdbm.InfrastructureProvider)(nil),
		(*cdbm.Tenant)(nil),
		(*cdbm.Site)(nil),
		(*cdbm.TenantSite)(nil),
		(*cdbm.TenantAccount)(nil),
		(*cdbm.TenantSiteCapabilityAssociation)(nil),
	}
	for _, m := range models {
		err := dbSession.DB.ResetModel(context.Background(), m)
		require.NoError(t, err)
	}
}

func testTenantCapabilityBuildTenant(t *testing.T, dbSession *cdb.Session, org string, user *cdbm.User, targetedInstanceCreation bool) *cdbm.Tenant {
	tnDAO := cdbm.NewTenantDAO(dbSession)
	tn, err := tnDAO.CreateFromParams(context.Background(), nil, org, cdb.GetStrPtr("Test Tenant"), org, cdb.GetStrPtr(org),
		&cdbm.TenantConfig{TargetedInstanceCreation: targetedInstanceCreation}, user)
	require.NoError(t, err)
	return tn
}

func testTenantCapabilityBuildTenantAccount(t *testing.T, dbSession *cdb.Session, ip *cdbm.InfrastructureProvider, tn *cdbm.Tenant, status string, createdBy uuid.UUID) *cdbm.TenantAccount {
	taDAO := cdbm.NewTenantAccountDAO(dbSession)
	ta, err := taDAO.Create(context.Background(), nil, cdbm.TenantAccountCreateInput{
		AccountNumber:             uuid.New().String(),
		TenantID:                  &tn.ID,
		TenantOrg:                 tn.Org,
		InfrastructureProviderID:  ip.ID,
		InfrastructureProviderOrg: ip.Org,
		Status:                    status,
		CreatedBy:                 createdBy,
	})
	require.NoError(t, err)
	return ta
}

func mustMarshal(t *testing.T, v interface{}) string {
	b, err := json.Marshal(v)
	require.NoError(t, err)
	return string(b)
}

func TestTenantCapabilityHandler_Update(t *testing.T) {
	ctx := context.Background()
	dbSession := testTenantCapabilityInitDB(t)
	defer dbSession.Close()

	testTenantCapabilitySetupSchema(t, dbSession)

	ipOrg := "test-ip-org"
	tnOrg := "test-tn-org"
	tnOrgNoCeiling := "test-tn-org-no-ceiling"
	tnOrgNoTenant := "test-tn-org-no-tenant"

	ipUser := cdbm.TestBuildUser(t, dbSession, uuid.NewString(), ipOrg, []string{authz.ProviderAdminRole})
	tnUser := cdbm.TestBuildUser(t, dbSession, uuid.NewString(), tnOrg, []string{authz.TenantAdminRole})
	tnUserNoCeiling := cdbm.TestBuildUser(t, dbSession, uuid.NewString(), tnOrgNoCeiling, []string{authz.TenantAdminRole})
	// A user who is a member of the tenant org but lacks the Tenant Admin role.
	tnNonAdminUser := cdbm.TestBuildUser(t, dbSession, uuid.NewString(), tnOrg, []string{authz.ProviderAdminRole})
	// A user who is a member of an org that has no tenant entity.
	noTenantUser := cdbm.TestBuildUser(t, dbSession, uuid.NewString(), tnOrgNoTenant, []string{authz.TenantAdminRole})

	ip := cdbm.TestBuildInfrastructureProvider(t, dbSession, "Test Provider", ipOrg, ipUser)
	ip2 := cdbm.TestBuildInfrastructureProvider(t, dbSession, "Test Provider 2", ipOrg+"-2", ipUser)

	// Privileged tenant (ceiling enabled) and a non-privileged tenant (ceiling disabled).
	tn := testTenantCapabilityBuildTenant(t, dbSession, tnOrg, tnUser, true)
	tnNoCeiling := testTenantCapabilityBuildTenant(t, dbSession, tnOrgNoCeiling, tnUserNoCeiling, false)

	// Sites: site1 + site2 under ip (eligible via TenantSite + Ready TenantAccount),
	// site3 under ip2 (not eligible for tn).
	site1 := cdbm.TestBuildSite(t, dbSession, ip, "Test Site 1", ipUser)
	// site2 is reachable via the Ready TenantAccount with ip; only needs to exist in the DB.
	cdbm.TestBuildSite(t, dbSession, ip, "Test Site 2", ipUser)
	site3 := cdbm.TestBuildSite(t, dbSession, ip2, "Test Site 3", ipUser)

	// tn is a member of site1 and has a Ready account with ip (reaching site1 + site2).
	cdbm.TestBuildTenantSite(t, dbSession, tn, site1, map[string]interface{}{}, tnUser)
	testTenantCapabilityBuildTenantAccount(t, dbSession, ip, tn, cdbm.TenantAccountStatusReady, ipUser.ID)

	enableAllBody := mustMarshal(t, model.APITenantCapabilityUpdateRequest{
		CapabilityName: model.CapabilityNameTargetedInstanceCreation,
		Sites:          []string{},
		Enabled:        cdb.GetBoolPtr(true),
	})
	enableSite1Body := mustMarshal(t, model.APITenantCapabilityUpdateRequest{
		CapabilityName: model.CapabilityNameTargetedInstanceCreation,
		Sites:          []string{site1.ID.String()},
		Enabled:        cdb.GetBoolPtr(true),
	})
	disableSite1Body := mustMarshal(t, model.APITenantCapabilityUpdateRequest{
		CapabilityName: model.CapabilityNameTargetedInstanceCreation,
		Sites:          []string{site1.ID.String()},
		Enabled:        cdb.GetBoolPtr(false),
	})
	enableIneligibleSiteBody := mustMarshal(t, model.APITenantCapabilityUpdateRequest{
		CapabilityName: model.CapabilityNameTargetedInstanceCreation,
		Sites:          []string{site3.ID.String()},
		Enabled:        cdb.GetBoolPtr(true),
	})
	providerMismatchBody := mustMarshal(t, model.APITenantCapabilityUpdateRequest{
		CapabilityName:           model.CapabilityNameTargetedInstanceCreation,
		Sites:                    []string{site1.ID.String()},
		InfrastructureProviderID: cdb.GetStrPtr(ip2.ID.String()),
		Enabled:                  cdb.GetBoolPtr(true),
	})
	unknownCapabilityBody := mustMarshal(t, model.APITenantCapabilityUpdateRequest{
		CapabilityName: "NotARealCapability",
		Enabled:        cdb.GetBoolPtr(true),
	})
	missingEnabledBody := mustMarshal(t, model.APITenantCapabilityUpdateRequest{
		CapabilityName: model.CapabilityNameTargetedInstanceCreation,
	})

	cfg := common.GetTestConfig()
	tempClient := &tmocks.Client{}

	// OTEL Spanner configuration
	tracer, _, ctx := common.TestCommonTraceProviderSetup(t, ctx)

	tests := []struct {
		name               string
		reqOrgName         string
		tenantID           string
		reqBody            string
		user               *cdbm.User
		expectedStatus     int
		expectedEnabled    bool
		expectedSiteCount  int
		verifyChildSpanner bool
	}{
		{
			name:           "error when user not found in request context",
			reqOrgName:     tnOrg,
			tenantID:       tn.ID.String(),
			reqBody:        enableAllBody,
			user:           nil,
			expectedStatus: http.StatusInternalServerError,
		},
		{
			name:           "error when user is not a tenant admin",
			reqOrgName:     tnOrg,
			tenantID:       tn.ID.String(),
			reqBody:        enableAllBody,
			user:           tnNonAdminUser,
			expectedStatus: http.StatusForbidden,
		},
		{
			name:           "error when tenantId in URL is not a valid uuid",
			reqOrgName:     tnOrg,
			tenantID:       "not-a-uuid",
			reqBody:        enableAllBody,
			user:           tnUser,
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "error when request body does not bind",
			reqOrgName:     tnOrg,
			tenantID:       tn.ID.String(),
			reqBody:        "not-json",
			user:           tnUser,
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "error when enabled is missing",
			reqOrgName:     tnOrg,
			tenantID:       tn.ID.String(),
			reqBody:        missingEnabledBody,
			user:           tnUser,
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "error when capabilityName is unknown",
			reqOrgName:     tnOrg,
			tenantID:       tn.ID.String(),
			reqBody:        unknownCapabilityBody,
			user:           tnUser,
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "error when org has no tenant",
			reqOrgName:     tnOrgNoTenant,
			tenantID:       tn.ID.String(),
			reqBody:        enableAllBody,
			user:           noTenantUser,
			expectedStatus: http.StatusNotFound,
		},
		{
			name:           "error when tenant in URL does not match org tenant",
			reqOrgName:     tnOrg,
			tenantID:       uuid.New().String(),
			reqBody:        enableAllBody,
			user:           tnUser,
			expectedStatus: http.StatusForbidden,
		},
		{
			name:           "error when ceiling is not set and enabling",
			reqOrgName:     tnOrgNoCeiling,
			tenantID:       tnNoCeiling.ID.String(),
			reqBody:        enableAllBody,
			user:           tnUserNoCeiling,
			expectedStatus: http.StatusForbidden,
		},
		{
			name:           "error when listed site is not eligible",
			reqOrgName:     tnOrg,
			tenantID:       tn.ID.String(),
			reqBody:        enableIneligibleSiteBody,
			user:           tnUser,
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "error when listed site does not match provider filter",
			reqOrgName:     tnOrg,
			tenantID:       tn.ID.String(),
			reqBody:        providerMismatchBody,
			user:           tnUser,
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:              "success enabling specific site",
			reqOrgName:        tnOrg,
			tenantID:          tn.ID.String(),
			reqBody:           enableSite1Body,
			user:              tnUser,
			expectedStatus:    http.StatusOK,
			expectedEnabled:   true,
			expectedSiteCount: 1,
		},
		{
			name:              "success disabling specific site",
			reqOrgName:        tnOrg,
			tenantID:          tn.ID.String(),
			reqBody:           disableSite1Body,
			user:              tnUser,
			expectedStatus:    http.StatusOK,
			expectedEnabled:   false,
			expectedSiteCount: 1,
		},
		{
			name:               "success enabling all eligible sites",
			reqOrgName:         tnOrg,
			tenantID:           tn.ID.String(),
			reqBody:            enableAllBody,
			user:               tnUser,
			expectedStatus:     http.StatusOK,
			expectedEnabled:    true,
			expectedSiteCount:  2,
			verifyChildSpanner: true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup echo server/context
			e := echo.New()
			req := httptest.NewRequest(http.MethodPatch, "/", strings.NewReader(tc.reqBody))
			req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
			rec := httptest.NewRecorder()

			ec := e.NewContext(req, rec)
			ec.SetParamNames("orgName", "tenantId")
			ec.SetParamValues(tc.reqOrgName, tc.tenantID)
			if tc.user != nil {
				ec.Set("user", tc.user)
			}

			ctx = context.WithValue(ctx, otelecho.TracerKey, tracer)
			ec.SetRequest(ec.Request().WithContext(ctx))

			h := UpdateTenantCapabilityHandler{
				dbSession: dbSession,
				tc:        tempClient,
				cfg:       cfg,
			}
			err := h.Handle(ec)
			assert.Nil(t, err)

			if tc.expectedStatus != rec.Code {
				t.Errorf("response: %v\n", rec.Body.String())
			}
			require.Equal(t, tc.expectedStatus, rec.Code)

			if tc.expectedStatus == http.StatusOK {
				rsp := &model.APITenantCapability{}
				uerr := json.Unmarshal(rec.Body.Bytes(), rsp)
				assert.Nil(t, uerr)
				assert.Equal(t, model.CapabilityNameTargetedInstanceCreation, rsp.CapabilityName)
				assert.Equal(t, tc.expectedEnabled, rsp.Enabled)
				assert.Equal(t, tc.expectedSiteCount, len(rsp.SiteIDs))
			}

			if tc.verifyChildSpanner {
				span := oteltrace.SpanFromContext(ec.Request().Context())
				assert.True(t, span.SpanContext().IsValid())
			}
		})
	}
}
