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

package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"github.com/rs/zerolog"
	"go.opentelemetry.io/otel/attribute"
	temporalEnums "go.temporal.io/api/enums/v1"
	tClient "go.temporal.io/sdk/client"
	tp "go.temporal.io/sdk/temporal"

	"github.com/NVIDIA/infra-controller-rest/api/internal/config"
	"github.com/NVIDIA/infra-controller-rest/api/pkg/api/handler/util/common"
	"github.com/NVIDIA/infra-controller-rest/api/pkg/api/model"
	"github.com/NVIDIA/infra-controller-rest/api/pkg/api/pagination"
	sc "github.com/NVIDIA/infra-controller-rest/api/pkg/client/site"
	auth "github.com/NVIDIA/infra-controller-rest/auth/pkg/authorization"
	cutil "github.com/NVIDIA/infra-controller-rest/common/pkg/util"
	cdb "github.com/NVIDIA/infra-controller-rest/db/pkg/db"
	cdbm "github.com/NVIDIA/infra-controller-rest/db/pkg/db/model"
	flowv1 "github.com/NVIDIA/infra-controller-rest/workflow-schema/flow/protobuf/v1"
	"github.com/NVIDIA/infra-controller-rest/workflow/pkg/queue"
)

// ~~~~~ Get Task Handler ~~~~~ //

// GetTaskHandler is the API Handler for getting a Task by ID
type GetTaskHandler struct {
	dbSession  *cdb.Session
	tc         tClient.Client
	scp        *sc.ClientPool
	cfg        *config.Config
	tracerSpan *cutil.TracerSpan
}

// NewGetTaskHandler initializes and returns a new handler for getting a Task
func NewGetTaskHandler(dbSession *cdb.Session, tc tClient.Client, scp *sc.ClientPool, cfg *config.Config) GetTaskHandler {
	return GetTaskHandler{
		dbSession:  dbSession,
		tc:         tc,
		scp:        scp,
		cfg:        cfg,
		tracerSpan: cutil.NewTracerSpan(),
	}
}

// Handle godoc
// @Summary Get a Task
// @Description Get a Task by UUID
// @Tags rack
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Param org path string true "Name of NGC organization"
// @Param id path string true "UUID of the Task"
// @Param siteId query string true "ID of the Site"
// @Success 200 {object} model.APIRackTask
// @Router /v2/org/{org}/nico/rack/task/{id} [get]
func (gth GetTaskHandler) Handle(c echo.Context) error {
	org, dbUser, ctx, logger, handlerSpan := common.SetupHandler("Task", "Get", c, gth.tracerSpan)
	if handlerSpan != nil {
		defer handlerSpan.End()
	}

	var apiRequest model.APIGetTaskRequest
	if err := common.ValidateKnownQueryParams(c.QueryParams(), apiRequest); err != nil {
		return cutil.NewAPIErrorResponse(c, http.StatusBadRequest, err.Error(), nil)
	}
	if err := c.Bind(&apiRequest); err != nil {
		return cutil.NewAPIErrorResponse(c, http.StatusBadRequest, "Failed to parse request data", nil)
	}
	if err := apiRequest.Validate(); err != nil {
		return cutil.NewAPIErrorResponse(c, http.StatusBadRequest, err.Error(), nil)
	}

	if dbUser == nil {
		logger.Error().Msg("invalid User object found in request context")
		return cutil.NewAPIErrorResponse(c, http.StatusInternalServerError, "Failed to retrieve current user", nil)
	}

	ok, err := auth.ValidateOrgMembership(dbUser, org)
	if !ok {
		if err != nil {
			logger.Error().Err(err).Msg("error validating org membership for User in request")
		} else {
			logger.Warn().Msg("could not validate org membership for user, access denied")
		}
		return cutil.NewAPIErrorResponse(c, http.StatusForbidden, fmt.Sprintf("Failed to validate membership for org: %s", org), nil)
	}

	ok = auth.ValidateUserRoles(dbUser, org, nil, auth.ProviderAdminRole)
	if !ok {
		logger.Warn().Msg("user does not have Provider Admin role, access denied")
		return cutil.NewAPIErrorResponse(c, http.StatusForbidden, "User does not have Provider Admin role with org", nil)
	}

	infrastructureProvider, err := common.GetInfrastructureProviderForOrg(ctx, nil, gth.dbSession, org)
	if err != nil {
		logger.Warn().Err(err).Msg("error getting infrastructure provider for org")
		return cutil.NewAPIErrorResponse(c, http.StatusBadRequest, "Failed to retrieve Infrastructure Provider for org", nil)
	}

	taskID := c.Param("id")
	if _, err := uuid.Parse(taskID); err != nil {
		return cutil.NewAPIErrorResponse(c, http.StatusBadRequest, "Invalid Task ID specified in URL", nil)
	}

	site, err := common.GetSiteFromIDString(ctx, nil, apiRequest.SiteID, gth.dbSession)
	if err != nil {
		if errors.Is(err, common.ErrInvalidID) {
			return cutil.NewAPIErrorResponse(c, http.StatusBadRequest, "Failed to validate Site specified in request: invalid ID", nil)
		}
		if errors.Is(err, cdb.ErrDoesNotExist) {
			return cutil.NewAPIErrorResponse(c, http.StatusBadRequest, "Site specified in request does not exist", nil)
		}
		logger.Error().Err(err).Msg("error retrieving Site from DB")
		return cutil.NewAPIErrorResponse(c, http.StatusInternalServerError, "Failed to retrieve Site specified in request due to DB error", nil)
	}

	if site.InfrastructureProviderID != infrastructureProvider.ID {
		return cutil.NewAPIErrorResponse(c, http.StatusForbidden, "Site specified in request doesn't belong to current org's Provider", nil)
	}

	siteConfig := &cdbm.SiteConfig{}
	if site.Config != nil {
		siteConfig = site.Config
	}

	if !siteConfig.Flow {
		logger.Warn().Msg("site does not have NICo Flow enabled")
		return cutil.NewAPIErrorResponse(c, http.StatusPreconditionFailed, "Site does not have NICo Flow enabled", nil)
	}

	stc, err := gth.scp.GetClientByID(site.ID)
	if err != nil {
		logger.Error().Err(err).Msg("failed to retrieve Temporal client for Site")
		return cutil.NewAPIErrorResponse(c, http.StatusInternalServerError, "Failed to retrieve client for Site", nil)
	}

	flowRequest := &flowv1.GetTasksByIDsRequest{
		TaskIds: []*flowv1.UUID{{Id: taskID}},
	}

	workflowOptions := tClient.StartWorkflowOptions{
		ID:                       fmt.Sprintf("task-get-%s", taskID),
		WorkflowIDReusePolicy:    temporalEnums.WORKFLOW_ID_REUSE_POLICY_ALLOW_DUPLICATE,
		WorkflowIDConflictPolicy: temporalEnums.WORKFLOW_ID_CONFLICT_POLICY_USE_EXISTING,
		WorkflowExecutionTimeout: cutil.WorkflowExecutionTimeout,
		TaskQueue:                queue.SiteTaskQueue,
	}

	ctx, cancel := context.WithTimeout(ctx, cutil.WorkflowContextTimeout)
	defer cancel()

	we, err := stc.ExecuteWorkflow(ctx, workflowOptions, "GetRackTask", flowRequest)
	if err != nil {
		logger.Error().Err(err).Msg("failed to schedule GetRackTask workflow")
		return cutil.NewAPIErrorResponse(c, http.StatusInternalServerError, "Failed to schedule Task retrieval workflow", nil)
	}

	var flowResponse flowv1.GetTasksByIDsResponse
	err = we.Get(ctx, &flowResponse)
	if err != nil {
		var timeoutErr *tp.TimeoutError
		if errors.As(err, &timeoutErr) || err == context.DeadlineExceeded || ctx.Err() != nil {
			return common.TerminateWorkflowOnTimeOut(c, logger, stc, fmt.Sprintf("task-get-%s", taskID), err, "Task", "GetRackTask")
		}
		code, unwrapErr := common.UnwrapWorkflowError(err)
		logger.Error().Err(unwrapErr).Msg("failed to get result from GetRackTask workflow")
		return cutil.NewAPIErrorResponse(c, code, fmt.Sprintf("Failed to execute Task retrieval workflow on Site: %s", unwrapErr), nil)
	}

	tasks := flowResponse.GetTasks()
	if len(tasks) == 0 {
		return cutil.NewAPIErrorResponse(c, http.StatusNotFound, "Task not found", nil)
	}

	apiTask := model.NewAPIRackTask(tasks[0])

	logger.Info().Msg("finishing API handler")

	return c.JSON(http.StatusOK, apiTask)
}

// ~~~~~ Cancel Task Handler ~~~~~ //

// CancelTaskHandler is the API Handler for cancelling a Task by ID.
//
// Cancellation is best-effort and idempotent: tasks in non-terminal states
// (Pending, Running, Waiting) are marked Terminated and any underlying
// Temporal workflow is terminated. Already-Terminated tasks are returned
// unchanged. Tasks that have already finished (Succeeded or Failed) cannot
// be cancelled and yield an error from Flow. The handler returns 202 Accepted
// with the task as last reported by Flow.
type CancelTaskHandler struct {
	dbSession  *cdb.Session
	tc         tClient.Client
	scp        *sc.ClientPool
	cfg        *config.Config
	tracerSpan *cutil.TracerSpan
}

// NewCancelTaskHandler initializes and returns a new handler for cancelling a Task
func NewCancelTaskHandler(dbSession *cdb.Session, tc tClient.Client, scp *sc.ClientPool, cfg *config.Config) CancelTaskHandler {
	return CancelTaskHandler{
		dbSession:  dbSession,
		tc:         tc,
		scp:        scp,
		cfg:        cfg,
		tracerSpan: cutil.NewTracerSpan(),
	}
}

// Handle godoc
// @Summary Cancel a Task
// @Description Cancel a Task by UUID. Best-effort and idempotent.
// @Tags rack
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Param org path string true "Name of NGC organization"
// @Param id path string true "UUID of the Task"
// @Param body body model.APICancelTaskRequest true "Cancel task request"
// @Success 202 {object} model.APIRackTask
// @Router /v2/org/{org}/nico/rack/task/{id}/cancel [post]
func (cth CancelTaskHandler) Handle(c echo.Context) error {
	org, dbUser, ctx, logger, handlerSpan := common.SetupHandler("Task", "Cancel", c, cth.tracerSpan)
	if handlerSpan != nil {
		defer handlerSpan.End()
	}

	// Is DB user missing?
	if dbUser == nil {
		logger.Error().Msg("invalid User object found in request context")
		return cutil.NewAPIErrorResponse(c, http.StatusInternalServerError, "Failed to retrieve current user", nil)
	}

	// Validate org membership
	ok, err := auth.ValidateOrgMembership(dbUser, org)
	if !ok {
		if err != nil {
			logger.Error().Err(err).Msg("error validating org membership for User in request")
		} else {
			logger.Warn().Msg("could not validate org membership for user, access denied")
		}
		return cutil.NewAPIErrorResponse(c, http.StatusForbidden, fmt.Sprintf("Failed to validate membership for org: %s", org), nil)
	}

	// Validate role, only Provider Admins are allowed to cancel a Task
	ok = auth.ValidateUserRoles(dbUser, org, nil, auth.ProviderAdminRole)
	if !ok {
		logger.Warn().Msg("user does not have Provider Admin role, access denied")
		return cutil.NewAPIErrorResponse(c, http.StatusForbidden, "User does not have Provider Admin role with org", nil)
	}

	// Get Infrastructure Provider for org
	infrastructureProvider, err := common.GetInfrastructureProviderForOrg(ctx, nil, cth.dbSession, org)
	if err != nil {
		logger.Warn().Err(err).Msg("error getting infrastructure provider for org")
		return cutil.NewAPIErrorResponse(c, http.StatusBadRequest, "Failed to retrieve Infrastructure Provider for org", nil)
	}

	// Get task ID from URL param
	taskID := c.Param("id")
	cth.tracerSpan.SetAttribute(handlerSpan, attribute.String("task_id", taskID), logger)
	if _, err := uuid.Parse(taskID); err != nil {
		return cutil.NewAPIErrorResponse(c, http.StatusBadRequest, "Invalid Task ID specified in URL", nil)
	}

	// Parse and validate request body
	apiRequest := model.APICancelTaskRequest{}
	if err := c.Bind(&apiRequest); err != nil {
		return cutil.NewAPIErrorResponse(c, http.StatusBadRequest, "Failed to parse request data", nil)
	}
	if verr := apiRequest.Validate(); verr != nil {
		logger.Warn().Err(verr).Msg("error validating cancel task request data")
		return cutil.NewAPIErrorResponse(c, http.StatusBadRequest, "Failed to validate cancel task request data", verr)
	}

	// Validate the site
	site, err := common.GetSiteFromIDString(ctx, nil, apiRequest.SiteID, cth.dbSession)
	if err != nil {
		if errors.Is(err, common.ErrInvalidID) {
			return cutil.NewAPIErrorResponse(c, http.StatusBadRequest, "Failed to validate Site specified in request: invalid ID", nil)
		}
		if errors.Is(err, cdb.ErrDoesNotExist) {
			return cutil.NewAPIErrorResponse(c, http.StatusBadRequest, "Site specified in request does not exist", nil)
		}
		logger.Error().Err(err).Msg("error retrieving Site from DB")
		return cutil.NewAPIErrorResponse(c, http.StatusInternalServerError, "Failed to retrieve Site specified in request due to DB error", nil)
	}

	// Verify site belongs to the org's Infrastructure Provider
	if site.InfrastructureProviderID != infrastructureProvider.ID {
		return cutil.NewAPIErrorResponse(c, http.StatusForbidden, "Site specified in request doesn't belong to current org's Provider", nil)
	}

	// Verify the Site has NICo Flow enabled (Tasks only exist on Flow sites)
	siteConfig := &cdbm.SiteConfig{}
	if site.Config != nil {
		siteConfig = site.Config
	}
	if !siteConfig.Flow {
		logger.Warn().Msg("site does not have NICo Flow enabled")
		return cutil.NewAPIErrorResponse(c, http.StatusPreconditionFailed, "Site does not have NICo Flow enabled", nil)
	}

	// Get the temporal client for the site
	stc, err := cth.scp.GetClientByID(site.ID)
	if err != nil {
		logger.Error().Err(err).Msg("failed to retrieve Temporal client for Site")
		return cutil.NewAPIErrorResponse(c, http.StatusInternalServerError, "Failed to retrieve client for Site", nil)
	}

	flowRequest := &flowv1.CancelTaskRequest{
		TaskId: &flowv1.UUID{Id: taskID},
	}

	workflowID := fmt.Sprintf("task-cancel-%s", taskID)
	workflowOptions := tClient.StartWorkflowOptions{
		ID:                       workflowID,
		WorkflowIDReusePolicy:    temporalEnums.WORKFLOW_ID_REUSE_POLICY_ALLOW_DUPLICATE,
		WorkflowIDConflictPolicy: temporalEnums.WORKFLOW_ID_CONFLICT_POLICY_USE_EXISTING,
		WorkflowExecutionTimeout: cutil.WorkflowExecutionTimeout,
		TaskQueue:                queue.SiteTaskQueue,
	}

	ctx, cancel := context.WithTimeout(ctx, cutil.WorkflowContextTimeout)
	defer cancel()

	we, err := stc.ExecuteWorkflow(ctx, workflowOptions, "CancelRackTask", flowRequest)
	if err != nil {
		logger.Error().Err(err).Msg("failed to schedule CancelRackTask workflow")
		return cutil.NewAPIErrorResponse(c, http.StatusInternalServerError, "Failed to schedule Task cancellation workflow", nil)
	}

	var flowResponse flowv1.CancelTaskResponse
	err = we.Get(ctx, &flowResponse)
	if err != nil {
		var timeoutErr *tp.TimeoutError
		if errors.As(err, &timeoutErr) || err == context.DeadlineExceeded || ctx.Err() != nil {
			return common.TerminateWorkflowOnTimeOut(c, logger, stc, workflowID, err, "Task", "CancelRackTask")
		}
		code, unwrapErr := common.UnwrapWorkflowError(err)
		logger.Error().Err(unwrapErr).Msg("failed to get result from CancelRackTask workflow")
		return cutil.NewAPIErrorResponse(c, code, fmt.Sprintf("Failed to execute Task cancellation workflow on Site: %s", unwrapErr), nil)
	}

	apiTask := model.NewAPIRackTask(flowResponse.GetTask())

	logger.Info().Msg("finishing API handler")
	return c.JSON(http.StatusAccepted, apiTask)
}

// ~~~~~ List Tasks Handlers ~~~~~ //

// listTasksHandlerCommon holds dependencies for GetRackTasksHandler and
// GetTrayTasksHandler. Both handlers authorize the caller, resolve the site,
// apply pagination, and invoke the ListTasks workflow; filtering differs only
// by whether ListTasksRequest carries RackId (rack) or ComponentId (tray).
type listTasksHandlerCommon struct {
	dbSession  *cdb.Session
	tc         tClient.Client
	scp        *sc.ClientPool
	cfg        *config.Config
	tracerSpan *cutil.TracerSpan
}

// listTasksContext holds request-derived state for list handlers after
// authorization and site lookup complete.
type listTasksContext struct {
	apiRequest  model.APIListTasksRequest
	pageRequest pagination.PageRequest
	site        *cdbm.Site
	stc         tClient.Client
}

// prepareListTasks authorizes the request, resolves the site, and binds
// pagination for rack- and tray-scoped task list handlers.
//
// Returns (listTasksContext, nil) on success. On failure it writes the HTTP
// response via cutil.NewAPIErrorResponse and returns (nil, echoResult).
// Callers must abort when lc == nil, not when err != nil: NewAPIErrorResponse
// often returns a nil error after c.JSON succeeds, so err is not a reliable
// signal that the handler should continue.
func (h listTasksHandlerCommon) prepareListTasks(c echo.Context, ctx context.Context, logger zerolog.Logger, org string, dbUser *cdbm.User) (*listTasksContext, error) {
	var apiRequest model.APIListTasksRequest
	if err := common.ValidateKnownQueryParams(c.QueryParams(), apiRequest); err != nil {
		return nil, cutil.NewAPIErrorResponse(c, http.StatusBadRequest, err.Error(), nil)
	}
	if err := c.Bind(&apiRequest); err != nil {
		return nil, cutil.NewAPIErrorResponse(c, http.StatusBadRequest, "Failed to parse request data", nil)
	}
	if err := apiRequest.Validate(); err != nil {
		return nil, cutil.NewAPIErrorResponse(c, http.StatusBadRequest, err.Error(), nil)
	}

	if dbUser == nil {
		logger.Error().Msg("invalid User object found in request context")
		return nil, cutil.NewAPIErrorResponse(c, http.StatusInternalServerError, "Failed to retrieve current user", nil)
	}

	ok, err := auth.ValidateOrgMembership(dbUser, org)
	if !ok {
		if err != nil {
			logger.Error().Err(err).Msg("error validating org membership for User in request")
		} else {
			logger.Warn().Msg("could not validate org membership for user, access denied")
		}
		return nil, cutil.NewAPIErrorResponse(c, http.StatusForbidden, fmt.Sprintf("Failed to validate membership for org: %s", org), nil)
	}

	if !auth.ValidateUserRoles(dbUser, org, nil, auth.ProviderAdminRole) {
		logger.Warn().Msg("user does not have Provider Admin role, access denied")
		return nil, cutil.NewAPIErrorResponse(c, http.StatusForbidden, "User does not have Provider Admin role with org", nil)
	}

	infrastructureProvider, err := common.GetInfrastructureProviderForOrg(ctx, nil, h.dbSession, org)
	if err != nil {
		logger.Warn().Err(err).Msg("error getting infrastructure provider for org")
		return nil, cutil.NewAPIErrorResponse(c, http.StatusBadRequest, "Failed to retrieve Infrastructure Provider for org", nil)
	}

	site, err := common.GetSiteFromIDString(ctx, nil, apiRequest.SiteID, h.dbSession)
	if err != nil {
		if errors.Is(err, common.ErrInvalidID) {
			return nil, cutil.NewAPIErrorResponse(c, http.StatusBadRequest, "Failed to validate Site specified in request: invalid ID", nil)
		}
		if errors.Is(err, cdb.ErrDoesNotExist) {
			return nil, cutil.NewAPIErrorResponse(c, http.StatusBadRequest, "Site specified in request does not exist", nil)
		}
		logger.Error().Err(err).Msg("error retrieving Site from DB")
		return nil, cutil.NewAPIErrorResponse(c, http.StatusInternalServerError, "Failed to retrieve Site specified in request due to DB error", nil)
	}

	if site.InfrastructureProviderID != infrastructureProvider.ID {
		return nil, cutil.NewAPIErrorResponse(c, http.StatusForbidden, "Site specified in request doesn't belong to current org's Provider", nil)
	}

	siteConfig := &cdbm.SiteConfig{}
	if site.Config != nil {
		siteConfig = site.Config
	}
	if !siteConfig.Flow {
		logger.Warn().Msg("site does not have NICo Flow enabled")
		return nil, cutil.NewAPIErrorResponse(c, http.StatusPreconditionFailed, "Site does not have NICo Flow enabled", nil)
	}

	pageRequest := pagination.PageRequest{}
	if err := c.Bind(&pageRequest); err != nil {
		logger.Warn().Err(err).Msg("error binding pagination request data into API model")
		return nil, cutil.NewAPIErrorResponse(c, http.StatusBadRequest, "Failed to parse request pagination data", nil)
	}
	// No orderBy fields are defined for Tasks; reject orderBy while validating
	// page bounds.
	if err := pageRequest.Validate(nil); err != nil {
		logger.Warn().Err(err).Msg("error validating pagination request data")
		return nil, cutil.NewAPIErrorResponse(c, http.StatusBadRequest, "Failed to validate pagination request data", err)
	}

	stc, err := h.scp.GetClientByID(site.ID)
	if err != nil {
		logger.Error().Err(err).Msg("failed to retrieve Temporal client for Site")
		return nil, cutil.NewAPIErrorResponse(c, http.StatusInternalServerError, "Failed to retrieve client for Site", nil)
	}

	return &listTasksContext{
		apiRequest:  apiRequest,
		pageRequest: pageRequest,
		site:        site,
		stc:         stc,
	}, nil
}

// runListTasks invokes the ListTasks workflow on the site and writes the
// response. workflowSuffix is appended to the workflow ID to keep the
// per-rack and per-tray invocations distinct.
func runListTasks(c echo.Context, ctx context.Context, logger zerolog.Logger, lc *listTasksContext, flowRequest *flowv1.ListTasksRequest, workflowSuffix string) error {
	workflowID := fmt.Sprintf("task-list-%s-%s", workflowSuffix, common.QueryParamHash(lc.apiRequest.QueryValues()))
	workflowOptions := tClient.StartWorkflowOptions{
		ID:                       workflowID,
		WorkflowIDReusePolicy:    temporalEnums.WORKFLOW_ID_REUSE_POLICY_ALLOW_DUPLICATE,
		WorkflowIDConflictPolicy: temporalEnums.WORKFLOW_ID_CONFLICT_POLICY_USE_EXISTING,
		WorkflowExecutionTimeout: cutil.WorkflowExecutionTimeout,
		TaskQueue:                queue.SiteTaskQueue,
	}

	ctx, cancel := context.WithTimeout(ctx, cutil.WorkflowContextTimeout)
	defer cancel()

	we, err := lc.stc.ExecuteWorkflow(ctx, workflowOptions, "ListTasks", flowRequest)
	if err != nil {
		logger.Error().Err(err).Msg("failed to schedule ListTasks workflow")
		return cutil.NewAPIErrorResponse(c, http.StatusInternalServerError, "Failed to schedule Task list workflow", nil)
	}

	var flowResponse flowv1.ListTasksResponse
	if err := we.Get(ctx, &flowResponse); err != nil {
		var timeoutErr *tp.TimeoutError
		if errors.As(err, &timeoutErr) || err == context.DeadlineExceeded || ctx.Err() != nil {
			return common.TerminateWorkflowOnTimeOut(c, logger, lc.stc, workflowID, err, "Task", "ListTasks")
		}
		code, unwrapErr := common.UnwrapWorkflowError(err)
		logger.Error().Err(unwrapErr).Msg("failed to get result from ListTasks workflow")
		return cutil.NewAPIErrorResponse(c, code, fmt.Sprintf("Failed to execute Task list workflow on Site: %s", unwrapErr), nil)
	}

	apiTasks := make([]*model.APIRackTask, 0, len(flowResponse.GetTasks()))
	for _, t := range flowResponse.GetTasks() {
		apiTasks = append(apiTasks, model.NewAPIRackTask(t))
	}

	total := int(flowResponse.GetTotal())
	pageResponse := pagination.NewPageResponse(*lc.pageRequest.PageNumber, *lc.pageRequest.PageSize, total, lc.pageRequest.OrderByStr)
	pageHeader, err := json.Marshal(pageResponse)
	if err != nil {
		logger.Error().Err(err).Msg("error marshaling pagination response")
		return cutil.NewAPIErrorResponse(c, http.StatusInternalServerError, "Failed to create pagination response", nil)
	}
	c.Response().Header().Set(pagination.ResponseHeaderName, string(pageHeader))

	logger.Info().Int("Count", len(apiTasks)).Int("Total", total).Msg("finishing API handler")
	return c.JSON(http.StatusOK, apiTasks)
}

// buildListTasksFlowRequest converts the bound REST request into the Flow
// gRPC request, with the caller-supplied filter field (RackId or
// ComponentId) already populated.
func buildListTasksFlowRequest(lc *listTasksContext, mutate func(req *flowv1.ListTasksRequest)) *flowv1.ListTasksRequest {
	req := &flowv1.ListTasksRequest{
		ActiveOnly: lc.apiRequest.ActiveOnly,
	}
	if lc.pageRequest.Offset != nil && lc.pageRequest.Limit != nil {
		req.Pagination = &flowv1.Pagination{
			Offset: int32(*lc.pageRequest.Offset),
			Limit:  int32(*lc.pageRequest.Limit),
		}
	}
	mutate(req)
	return req
}

// GetRackTasksHandler is the API Handler for listing Tasks targeting a
// specific Rack.
type GetRackTasksHandler struct {
	listTasksHandlerCommon
}

// NewGetRackTasksHandler initializes a new GetRackTasksHandler.
func NewGetRackTasksHandler(dbSession *cdb.Session, tc tClient.Client, scp *sc.ClientPool, cfg *config.Config) GetRackTasksHandler {
	return GetRackTasksHandler{
		listTasksHandlerCommon: listTasksHandlerCommon{
			dbSession:  dbSession,
			tc:         tc,
			scp:        scp,
			cfg:        cfg,
			tracerSpan: cutil.NewTracerSpan(),
		},
	}
}

// Handle godoc
// @Summary List Tasks for a Rack
// @Description List Tasks targeting the given Rack, with optional active-only and pagination filters.
// @Tags rack
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Param org path string true "Name of NGC organization"
// @Param id path string true "UUID of the Rack"
// @Param siteId query string true "ID of the Site"
// @Param activeOnly query boolean false "Restrict to non-terminal Tasks"
// @Param pageNumber query integer false "Page number of results returned"
// @Param pageSize query integer false "Number of results per page"
// @Success 200 {array} model.APIRackTask
// @Router /v2/org/{org}/nico/rack/{id}/task [get]
func (h GetRackTasksHandler) Handle(c echo.Context) error {
	org, dbUser, ctx, logger, handlerSpan := common.SetupHandler("RackTasks", "List", c, h.tracerSpan)
	if handlerSpan != nil {
		defer handlerSpan.End()
	}

	rackID := c.Param("id")
	h.tracerSpan.SetAttribute(handlerSpan, attribute.String("rack_id", rackID), logger)
	if _, err := uuid.Parse(rackID); err != nil {
		return cutil.NewAPIErrorResponse(c, http.StatusBadRequest, "Invalid Rack ID specified in URL", nil)
	}

	lc, echoErr := h.prepareListTasks(c, ctx, logger, org, dbUser)
	if lc == nil {
		return echoErr
	}

	flowRequest := buildListTasksFlowRequest(lc, func(req *flowv1.ListTasksRequest) {
		req.RackId = &flowv1.UUID{Id: rackID}
	})

	return runListTasks(c, ctx, logger, lc, flowRequest, "rack-"+rackID)
}

// GetTrayTasksHandler is the API Handler for listing Tasks targeting a
// specific Tray (component).
type GetTrayTasksHandler struct {
	listTasksHandlerCommon
}

// NewGetTrayTasksHandler initializes a new GetTrayTasksHandler.
func NewGetTrayTasksHandler(dbSession *cdb.Session, tc tClient.Client, scp *sc.ClientPool, cfg *config.Config) GetTrayTasksHandler {
	return GetTrayTasksHandler{
		listTasksHandlerCommon: listTasksHandlerCommon{
			dbSession:  dbSession,
			tc:         tc,
			scp:        scp,
			cfg:        cfg,
			tracerSpan: cutil.NewTracerSpan(),
		},
	}
}

// Handle godoc
// @Summary List Tasks for a Tray
// @Description List Tasks targeting the given Tray (matched as a component UUID on Flow), with optional active-only and pagination filters.
// @Tags tray
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Param org path string true "Name of NGC organization"
// @Param id path string true "UUID of the Tray"
// @Param siteId query string true "ID of the Site"
// @Param activeOnly query boolean false "Restrict to non-terminal Tasks"
// @Param pageNumber query integer false "Page number of results returned"
// @Param pageSize query integer false "Number of results per page"
// @Success 200 {array} model.APIRackTask
// @Router /v2/org/{org}/nico/tray/{id}/task [get]
func (h GetTrayTasksHandler) Handle(c echo.Context) error {
	org, dbUser, ctx, logger, handlerSpan := common.SetupHandler("TrayTasks", "List", c, h.tracerSpan)
	if handlerSpan != nil {
		defer handlerSpan.End()
	}

	trayID := c.Param("id")
	h.tracerSpan.SetAttribute(handlerSpan, attribute.String("tray_id", trayID), logger)
	if _, err := uuid.Parse(trayID); err != nil {
		return cutil.NewAPIErrorResponse(c, http.StatusBadRequest, "Invalid Tray ID specified in URL", nil)
	}

	lc, echoErr := h.prepareListTasks(c, ctx, logger, org, dbUser)
	if lc == nil {
		return echoErr
	}

	flowRequest := buildListTasksFlowRequest(lc, func(req *flowv1.ListTasksRequest) {
		req.ComponentId = &flowv1.UUID{Id: trayID}
	})

	return runListTasks(c, ctx, logger, lc, flowRequest, "tray-"+trayID)
}
