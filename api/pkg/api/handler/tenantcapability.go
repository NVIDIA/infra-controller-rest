// SPDX-FileCopyrightText: Copyright (c) 2026 NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package handler

import (
	"context"
	"fmt"
	"net/http"

	mapset "github.com/deckarep/golang-set/v2"
	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"github.com/rs/zerolog"
	"go.opentelemetry.io/otel/attribute"
	temporalClient "go.temporal.io/sdk/client"

	"github.com/NVIDIA/infra-controller-rest/api/internal/config"
	common "github.com/NVIDIA/infra-controller-rest/api/pkg/api/handler/util/common"
	"github.com/NVIDIA/infra-controller-rest/api/pkg/api/model"
	auth "github.com/NVIDIA/infra-controller-rest/auth/pkg/authorization"
	cutil "github.com/NVIDIA/infra-controller-rest/common/pkg/util"
	cdb "github.com/NVIDIA/infra-controller-rest/db/pkg/db"
	cdbm "github.com/NVIDIA/infra-controller-rest/db/pkg/db/model"
	cdbp "github.com/NVIDIA/infra-controller-rest/db/pkg/db/paginator"
)

// ~~~~~ Update Handler ~~~~~ //

// UpdateTenantCapabilityHandler is the API Handler for enabling or disabling a scoped
// tenant capability (e.g. TargetedInstanceCreation) for a resolved set of Sites.
type UpdateTenantCapabilityHandler struct {
	dbSession  *cdb.Session
	tc         temporalClient.Client
	cfg        *config.Config
	tracerSpan *cutil.TracerSpan
}

// NewUpdateTenantCapabilityHandler initializes and returns a new handler for updating tenant capabilities
func NewUpdateTenantCapabilityHandler(dbSession *cdb.Session, tc temporalClient.Client, cfg *config.Config) UpdateTenantCapabilityHandler {
	return UpdateTenantCapabilityHandler{
		dbSession:  dbSession,
		tc:         tc,
		cfg:        cfg,
		tracerSpan: cutil.NewTracerSpan(),
	}
}

// Handle godoc
// @Summary Update scoped tenant capabilities
// @Description Enable or disable a scoped capability (e.g. TargetedInstanceCreation) for a resolved set of Sites
// @Tags tenant
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Param org path string true "Name of NGC organization"
// @Param tenantId path string true "ID of Tenant"
// @Param message body model.APITenantCapabilityUpdateRequest true "Tenant capability update request"
// @Success 200 {object} model.APITenantCapability
// @Router /v2/org/{org}/nico/tenant/{tenantId}/capabilities [patch]
func (utch UpdateTenantCapabilityHandler) Handle(c echo.Context) error {
	org, dbUser, ctx, logger, handlerSpan := common.SetupHandler("TenantCapability", "Update", c, utch.tracerSpan)
	if handlerSpan != nil {
		defer handlerSpan.End()
	}
	if dbUser == nil {
		return cutil.NewAPIErrorResponse(c, http.StatusInternalServerError, "Failed to retrieve current user", nil)
	}

	// Validate org
	ok, err := auth.ValidateOrgMembership(dbUser, org)
	if !ok {
		if err != nil {
			logger.Error().Err(err).Msg("error validating org membership for User in request")
		} else {
			logger.Warn().Msg("could not validate org membership for user, access denied")
		}
		return cutil.NewAPIErrorResponse(c, http.StatusForbidden, fmt.Sprintf("Failed to validate membership for org: %s", org), nil)
	}

	// Validate role, only Tenant Admins may configure scoped capabilities for their tenant
	ok = auth.ValidateUserRoles(dbUser, org, nil, auth.TenantAdminRole)
	if !ok {
		logger.Warn().Msg("user does not have Tenant Admin role with org, access denied")
		return cutil.NewAPIErrorResponse(c, http.StatusForbidden, "User does not have Tenant Admin role with org", nil)
	}

	// Get tenant ID from URL param
	tenantStrID := c.Param("tenantId")

	utch.tracerSpan.SetAttribute(handlerSpan, attribute.String("tenant_id", tenantStrID), logger)

	tenantID, err := uuid.Parse(tenantStrID)
	if err != nil {
		logger.Warn().Err(err).Msg("error parsing tenantId in url into uuid")
		return cutil.NewAPIErrorResponse(c, http.StatusBadRequest, "Invalid Tenant ID in URL", nil)
	}

	// Validate request
	// Bind request data to API model
	apiRequest := model.APITenantCapabilityUpdateRequest{}
	err = c.Bind(&apiRequest)
	if err != nil {
		logger.Warn().Err(err).Msg("error binding request data into API model")
		return cutil.NewAPIErrorResponse(c, http.StatusBadRequest, "Failed to parse request data, potentially invalid structure", nil)
	}

	// Validate request attributes
	verr := apiRequest.Validate()
	if verr != nil {
		logger.Warn().Err(verr).Msg("error validating Tenant capability update request data")
		return cutil.NewAPIErrorResponse(c, http.StatusBadRequest, "Error validating Tenant capability update request data", verr)
	}

	enabled := *apiRequest.Enabled

	// Resolve the tenant for this org and ensure it matches the tenant in the URL. A Tenant
	// Admin may only configure capabilities for their own org's tenant.
	tenant, err := common.GetTenantForOrg(ctx, nil, utch.dbSession, org)
	if err != nil {
		logger.Warn().Err(err).Msg("tenant does not exist for org")
		return cutil.NewAPIErrorResponse(c, http.StatusNotFound, "Org does not have a tenant", nil)
	}
	if tenant.ID != tenantID {
		logger.Warn().Msg("tenant in URL does not match tenant for org")
		return cutil.NewAPIErrorResponse(c, http.StatusForbidden, "Tenant in URL does not belong to org", nil)
	}

	// Ceiling check: enabling a scoped capability requires the tenant-level entitlement.
	// Disabling is always allowed (a site can be turned off while the ceiling stays true).
	if enabled && !common.TenantHasTargetedInstanceCreation(tenant) {
		logger.Warn().Msg("tenant does not have the targeted instance creation ceiling, cannot enable capability")
		return cutil.NewAPIErrorResponse(c, http.StatusForbidden, "Tenant must have the TargetedInstanceCreation capability enabled before scoping it to sites", nil)
	}

	// Optional provider filter
	var providerFilter *uuid.UUID
	if apiRequest.InfrastructureProviderID != nil {
		pid, perr := uuid.Parse(*apiRequest.InfrastructureProviderID)
		if perr != nil {
			return cutil.NewAPIErrorResponse(c, http.StatusBadRequest, "Invalid Infrastructure Provider ID in request", nil)
		}
		providerFilter = &pid
	}

	// Resolve the eligible Site set for the tenant: the union of TenantSite memberships and
	// Sites reachable via Ready TenantAccounts. This mirrors the Site listing behavior.
	eligibleSites, err := utch.resolveEligibleSites(ctx, logger, tenant.ID)
	if err != nil {
		return cutil.NewAPIErrorResponse(c, http.StatusInternalServerError, "Failed to resolve eligible Sites for Tenant", nil)
	}

	// Determine the target Sites from the request.
	targetSites, aerr := resolveTargetSites(apiRequest.Sites, providerFilter, eligibleSites)
	if aerr != nil {
		logger.Warn().Err(aerr).Msg("error resolving target sites for capability update")
		return cutil.NewAPIErrorResponse(c, http.StatusBadRequest, aerr.Error(), nil)
	}

	tscaDAO := cdbm.NewTenantSiteCapabilityAssociationDAO(utch.dbSession)

	appliedSiteIDs := []string{}

	err = cdb.WithTx(ctx, utch.dbSession, func(tx *cdb.Tx) error {
		// Serialize concurrent capability updates for the same tenant so the read-then-write
		// per site sees a consistent snapshot.
		derr := tx.TryAcquireAdvisoryLock(ctx, cdb.GetAdvisoryLockIDFromString(fmt.Sprintf("tsca-%s", tenant.ID.String())), nil)
		if derr != nil {
			logger.Error().Err(derr).Msg("error acquiring advisory lock for tenant capability update")
			return cutil.NewAPIError(http.StatusInternalServerError, "Failed to acquire lock for capability update", nil)
		}

		for _, site := range targetSites {
			existing, gerr := tscaDAO.GetByTenantIDAndSiteID(ctx, tx, tenant.ID, site.ID, nil)
			if gerr != nil && gerr != cdb.ErrDoesNotExist {
				logger.Error().Err(gerr).Msg("error retrieving existing capability association")
				return cutil.NewAPIError(http.StatusInternalServerError, "Failed to retrieve capability association", nil)
			}

			if gerr == cdb.ErrDoesNotExist {
				// No association yet. Disabling is a no-op (absent ⇒ effective false);
				// only materialize a row when enabling.
				if !enabled {
					continue
				}
				_, cerr := tscaDAO.Create(ctx, tx, cdbm.TenantSiteCapabilityAssociationCreateInput{
					TenantID:                 tenant.ID,
					SiteID:                   site.ID,
					InfrastructureProviderID: site.InfrastructureProviderID,
					TargetedInstanceCreation: enabled,
					CreatedBy:                dbUser.ID,
				})
				if cerr != nil {
					logger.Error().Err(cerr).Msg("error creating capability association")
					return cutil.NewAPIError(http.StatusInternalServerError, "Failed to create capability association", nil)
				}
				appliedSiteIDs = append(appliedSiteIDs, site.ID.String())
				continue
			}

			// Association exists; update the flag in place.
			_, uerr := tscaDAO.Update(ctx, tx, cdbm.TenantSiteCapabilityAssociationUpdateInput{
				TenantSiteCapabilityAssociationID: existing.ID,
				TargetedInstanceCreation:          cdb.GetBoolPtr(enabled),
			})
			if uerr != nil {
				logger.Error().Err(uerr).Msg("error updating capability association")
				return cutil.NewAPIError(http.StatusInternalServerError, "Failed to update capability association", nil)
			}
			appliedSiteIDs = append(appliedSiteIDs, site.ID.String())
		}

		return nil
	})
	if err != nil {
		return common.HandleTxError(c, logger, err, "Failed to update Tenant capabilities, DB transaction error")
	}

	apiResponse := model.APITenantCapability{
		CapabilityName: apiRequest.CapabilityName,
		Enabled:        enabled,
		SiteIDs:        appliedSiteIDs,
	}

	logger.Info().Msg("finishing API handler")

	return c.JSON(http.StatusOK, apiResponse)
}

// resolveEligibleSites returns the Sites a tenant may scope capabilities to: the union of its
// TenantSite memberships and the Sites of providers it has a Ready TenantAccount with. The
// returned map is keyed by Site ID so callers can look up each Site's provider.
func (utch UpdateTenantCapabilityHandler) resolveEligibleSites(ctx context.Context, logger zerolog.Logger, tenantID uuid.UUID) (map[uuid.UUID]*cdbm.Site, error) {
	stDAO := cdbm.NewSiteDAO(utch.dbSession)
	tsDAO := cdbm.NewTenantSiteDAO(utch.dbSession)
	taDAO := cdbm.NewTenantAccountDAO(utch.dbSession)

	eligibleSiteIDs := mapset.NewSet[uuid.UUID]()

	tss, _, serr := tsDAO.GetAll(ctx, nil, cdbm.TenantSiteFilterInput{TenantIDs: []uuid.UUID{tenantID}}, cdbp.PageInput{Limit: cdb.GetIntPtr(cdbp.TotalLimit)}, nil)
	if serr != nil {
		logger.Error().Err(serr).Msg("error retrieving Tenant Site associations from DB by Tenant ID")
		return nil, serr
	}
	for _, ts := range tss {
		eligibleSiteIDs.Add(ts.SiteID)
	}

	tas, _, serr := taDAO.GetAll(ctx, nil, cdbm.TenantAccountFilterInput{
		TenantIDs: []uuid.UUID{tenantID},
		Statuses:  []string{cdbm.TenantAccountStatusReady},
	}, cdbp.PageInput{Limit: cdb.GetIntPtr(cdbp.TotalLimit)}, nil)
	if serr != nil {
		logger.Error().Err(serr).Msg("error retrieving Tenant Accounts for Tenant")
		return nil, serr
	}
	if len(tas) > 0 {
		providerIDs := make([]uuid.UUID, 0, len(tas))
		for _, ta := range tas {
			providerIDs = append(providerIDs, ta.InfrastructureProviderID)
		}
		providerSites, _, serr := stDAO.GetAll(ctx, nil, cdbm.SiteFilterInput{InfrastructureProviderIDs: providerIDs}, cdbp.PageInput{Limit: cdb.GetIntPtr(cdbp.TotalLimit)}, nil)
		if serr != nil {
			logger.Error().Err(serr).Msg("error retrieving Sites for Providers from Tenant Accounts")
			return nil, serr
		}
		for _, site := range providerSites {
			eligibleSiteIDs.Add(site.ID)
		}
	}

	eligible := map[uuid.UUID]*cdbm.Site{}
	if eligibleSiteIDs.Cardinality() == 0 {
		return eligible, nil
	}

	// Fetch the resolved Sites in one pass to get each Site's provider id uniformly.
	sites, _, serr := stDAO.GetAll(ctx, nil, cdbm.SiteFilterInput{SiteIDs: eligibleSiteIDs.ToSlice()}, cdbp.PageInput{Limit: cdb.GetIntPtr(cdbp.TotalLimit)}, nil)
	if serr != nil {
		logger.Error().Err(serr).Msg("error retrieving eligible Sites from DB")
		return nil, serr
	}
	for i := range sites {
		site := sites[i]
		eligible[site.ID] = &site
	}

	return eligible, nil
}

// resolveTargetSites maps the request's site selection onto the eligible Site set, applying the
// optional provider filter. An empty requestedSites means "all eligible sites".
func resolveTargetSites(requestedSites []string, providerFilter *uuid.UUID, eligible map[uuid.UUID]*cdbm.Site) ([]*cdbm.Site, error) {
	targets := []*cdbm.Site{}

	if len(requestedSites) == 0 {
		for _, site := range eligible {
			if providerFilter != nil && site.InfrastructureProviderID != *providerFilter {
				continue
			}
			targets = append(targets, site)
		}
		return targets, nil
	}

	for _, raw := range requestedSites {
		siteID, perr := uuid.Parse(raw)
		if perr != nil {
			return nil, fmt.Errorf("invalid Site ID in request: %s", raw)
		}
		site, ok := eligible[siteID]
		if !ok {
			return nil, fmt.Errorf("Site %s is not eligible for this Tenant", raw)
		}
		if providerFilter != nil && site.InfrastructureProviderID != *providerFilter {
			return nil, fmt.Errorf("Site %s does not belong to the specified Infrastructure Provider", raw)
		}
		targets = append(targets, site)
	}

	return targets, nil
}
