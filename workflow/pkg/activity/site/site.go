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

package site

import (
	"context"
	"encoding/base64"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
	tOperatorv1 "go.temporal.io/api/operatorservice/v1"
	tWorkflowv1 "go.temporal.io/api/workflowservice/v1"
	"go.temporal.io/sdk/client"

	cloudutils "github.com/NVIDIA/infra-controller-rest/common/pkg/util"
	cdb "github.com/NVIDIA/infra-controller-rest/db/pkg/db"
	cdbm "github.com/NVIDIA/infra-controller-rest/db/pkg/db/model"
	cdbp "github.com/NVIDIA/infra-controller-rest/db/pkg/db/paginator"
	csm "github.com/NVIDIA/infra-controller-rest/site-manager/pkg/sitemgr"

	"github.com/NVIDIA/infra-controller-rest/workflow/internal/config"
	sc "github.com/NVIDIA/infra-controller-rest/workflow/pkg/client/site"
	"github.com/NVIDIA/infra-controller-rest/workflow/pkg/queue"
	"github.com/NVIDIA/infra-controller-rest/workflow/pkg/util"
)

const (
	// SiteInventoryReceiptThreshold is the period since last Site inventory receipt before error is logged
	SiteInventoryReceiptThreshold = 15 * 60 * time.Second // 10 minutes

	// Number of days before cert expiration when rotation should be triggered
	rotationBufferDays = 10
)

// ManageSite is an activity wrapper for managing Site lifecycle that allows
// injecting DB access
type ManageSite struct {
	dbSession      *cdb.Session
	siteClientPool *sc.ClientPool
	tc             client.Client
	cfg            *config.Config
}

// Activity functions

// DeleteSiteComponentsFromDB is a Temporal activity that initiate delete for instancetype/machine/operatingsystem/instance/subnet/vpc
func (mst ManageSite) DeleteSiteComponentsFromDB(ctx context.Context, siteID uuid.UUID, infrastructureProviderID uuid.UUID, purgeMachines bool) error {
	logger := log.With().Str("Activity", "DeleteSiteComponentsFromDB").Str("Site ID", siteID.String()).
		Str("InfrastructureProvider ID", infrastructureProviderID.String()).Bool("Purge Machines", purgeMachines).Logger()

	logger.Info().Msg("starting activity")

	// Check if site exists
	siteDAO := cdbm.NewSiteDAO(mst.dbSession)
	_, err := siteDAO.GetByID(ctx, nil, siteID, nil, false)
	if err != nil {
		if err != cdb.ErrDoesNotExist {
			logger.Error().Err(err).Msg("failed to retrieve Site from DB by ID")
			return err
		}
	}

	itDAO := cdbm.NewInstanceTypeDAO(mst.dbSession)
	ipbDAO := cdbm.NewIPBlockDAO(mst.dbSession)
	vpcDAO := cdbm.NewVpcDAO(mst.dbSession)
	subnetDAO := cdbm.NewSubnetDAO(mst.dbSession)
	ibpDAO := cdbm.NewInfiniBandPartitionDAO(mst.dbSession)
	mitDAO := cdbm.NewMachineInstanceTypeDAO(mst.dbSession)
	mDAO := cdbm.NewMachineDAO(mst.dbSession)
	mcDAO := cdbm.NewMachineCapabilityDAO(mst.dbSession)
	miDAO := cdbm.NewMachineInterfaceDAO(mst.dbSession)
	instanceDAO := cdbm.NewInstanceDAO(mst.dbSession)

	// Delete Instance Types
	// Check for Instance Types associated with Site
	its, _, err := itDAO.GetAll(ctx, nil, cdbm.InstanceTypeFilterInput{SiteIDs: []uuid.UUID{siteID}}, nil, nil, cdb.GetIntPtr(cdbp.TotalLimit), nil)
	if err != nil {
		logger.Error().Err(err).Msg("error retrieving Instance Types for Site from DB")
		return err
	}

	for _, it := range its {
		// Delete Machine/Instance Type associations
		err = mitDAO.DeleteAllByInstanceTypeID(ctx, nil, it.ID, purgeMachines)
		if err != nil && err != cdb.ErrDoesNotExist {
			logger.Error().Err(err).Str("Instance Type ID", it.ID.String()).Msg("error deleting Machine/Instance Type associations for Instance Type in DB")
			return err
		}

		// Delete Instance Type
		err = itDAO.DeleteByID(ctx, nil, it.ID)
		if err != nil && err != cdb.ErrDoesNotExist {
			logger.Error().Err(err).Str("Instance Type ID", it.ID.String()).Msg("error deleting Instance Type record in DB")
			return err
		}
	}

	// Delete IP Blocks
	// Check for IP Blocks associated with Site
	ipbs, _, err := ipbDAO.GetAll(
		ctx,
		nil,
		cdbm.IPBlockFilterInput{
			SiteIDs:        []uuid.UUID{siteID},
			ExcludeDerived: true,
		},
		cdbp.PageInput{Limit: cdb.GetIntPtr(cdbp.TotalLimit)},
		nil,
	)
	if err != nil {
		logger.Error().Err(err).Msg("error retrieving IP Blocks for Site from DB")
		return err
	}

	for _, ipb := range ipbs {
		// Delete IPBlock in DB
		err = ipbDAO.Delete(ctx, nil, ipb.ID)
		if err != nil && err != cdb.ErrDoesNotExist {
			logger.Error().Err(err).Str("IP Block ID", ipb.ID.String()).Msg("error deleting IP Block in db")
			return err
		}
	}

	// Delete Instances
	// Check that Instance exists
	instances, _, err := instanceDAO.GetAll(ctx, nil, cdbm.InstanceFilterInput{SiteIDs: []uuid.UUID{siteID}}, cdbp.PageInput{Limit: cdb.GetIntPtr(cdbp.TotalLimit)}, nil)
	if err != nil {
		logger.Error().Err(err).Msg("failed to retrieve Instances from DB by Site ID")
		return err
	}

	for _, instance := range instances {
		// Remove Machine reference if purgeMachines is true
		if purgeMachines {
			_, serr := instanceDAO.Clear(ctx, nil, cdbm.InstanceClearInput{InstanceID: instance.ID, MachineID: true})
			if serr != nil && serr != cdb.ErrDoesNotExist {
				logger.Error().Err(serr).Str("Instance ID", instance.ID.String()).Msg("error clearing Machine ID for Instance in DB")
				return err
			}
		}

		// Delete Instance
		err = instanceDAO.Delete(ctx, nil, instance.ID)
		if err != nil && err != cdb.ErrDoesNotExist {
			logger.Error().Err(err).Str("Instance ID", instance.ID.String()).Msg("error deleting Instance record in DB")
			return err
		}
	}

	// Delete Machines
	// Check if Machines exist
	mcs, _, err := mDAO.GetAll(ctx, nil, cdbm.MachineFilterInput{SiteID: &siteID}, cdbp.PageInput{Limit: cdb.GetIntPtr(cdbp.TotalLimit)}, nil)
	if err != nil {
		logger.Error().Err(err).Msg("error retrieving Machine for Site from DB")
		return err
	}

	for _, mc := range mcs {
		// Get MachineInterfaces by Machine
		mits, _, serr := miDAO.GetAll(
			ctx,
			nil,
			cdbm.MachineInterfaceFilterInput{
				MachineIDs: []string{mc.ID},
			},
			cdbp.PageInput{Limit: cdb.GetIntPtr(cdbp.TotalLimit)},
			nil,
		)
		if serr != nil {
			logger.Error().Err(serr).Str("Machine ID", mc.ID).Msg("error retrieving Machine Interfaces for Machine from DB")
			return serr
		}

		// Delete Machine Interfaces
		for _, mit := range mits {
			sserr := miDAO.Delete(ctx, nil, mit.ID, purgeMachines)
			if sserr != nil && sserr != cdb.ErrDoesNotExist {
				logger.Error().Err(sserr).Str("Machine Interface ID", mit.ID.String()).Msg("error deleting Machine Interface record in db")
				return sserr
			}
		}

		// Get Machine Capability records from the db
		mcbs, _, serr := mcDAO.GetAll(ctx, nil, []string{mc.ID}, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, cdb.GetIntPtr(cdbp.TotalLimit), nil)
		if serr != nil {
			logger.Error().Err(serr).Msg("error retrieving MachineCapabilities for Site's machine from DB")
			return serr
		}

		// Delete Machine Capabilities
		for _, mcb := range mcbs {
			sserr := mcDAO.DeleteByID(ctx, nil, mcb.ID, purgeMachines)
			if sserr != nil && sserr != cdb.ErrDoesNotExist {
				logger.Error().Err(sserr).Str("Machine Capability ID", mcb.ID.String()).Msg("error deleting Machine Capability record in db")
				return sserr
			}
		}

		serr = mDAO.Delete(ctx, nil, mc.ID, purgeMachines)
		if serr != nil && serr != cdb.ErrDoesNotExist {
			logger.Error().Err(serr).Msg("error deleting Machine record in db")
			return serr
		}
	}

	// Delete Subnets
	// Check if Subnets exist
	subnets, _, err := subnetDAO.GetAll(ctx, nil, cdbm.SubnetFilterInput{SiteIDs: []uuid.UUID{siteID}}, cdbp.PageInput{Limit: cdb.GetIntPtr(cdbp.TotalLimit)}, []string{})
	if err != nil {
		logger.Error().Err(err).Msg("failed to retrieve Subnets from DB by Site ID")
		return err
	}

	for _, sb := range subnets {
		// Delete Subnet
		serr := subnetDAO.Delete(ctx, nil, sb.ID)
		if serr != nil && serr != cdb.ErrDoesNotExist {
			logger.Error().Err(serr).Str("Subnet ID", sb.ID.String()).Msg("error deleting Subnet record in DB")
			return serr
		}
	}

	// Delete VPCs
	// Check if VPCs exist
	vpcs, _, err := vpcDAO.GetAll(ctx, nil, cdbm.VpcFilterInput{SiteIDs: []uuid.UUID{siteID}}, cdbp.PageInput{Limit: cdb.GetIntPtr(cdbp.TotalLimit)}, nil)
	if err != nil {
		logger.Error().Err(err).Msg("failed to retrieve VPCs from DB by Site ID")
		return err
	}

	for _, vpc := range vpcs {
		// Delete VPC
		serr := vpcDAO.DeleteByID(ctx, nil, vpc.ID)
		if serr != nil && serr != cdb.ErrDoesNotExist {
			logger.Error().Err(serr).Str("VPC ID", vpc.ID.String()).Msg("error deleting VPC record in DB")
			return serr
		}
	}

	// Delete IB Partitions
	ibps, _, err := ibpDAO.GetAll(
		ctx,
		nil,
		cdbm.InfiniBandPartitionFilterInput{
			SiteIDs: []uuid.UUID{siteID},
		},
		cdbp.PageInput{Limit: cdb.GetIntPtr(cdbp.TotalLimit)},
		nil,
	)
	if err != nil {
		logger.Error().Err(err).Msg("failed to retrieve IB Partitions from DB by Site ID")
		return err
	}

	for _, ibp := range ibps {
		// Delete IB Partition
		serr := ibpDAO.Delete(ctx, nil, ibp.ID)
		if serr != nil && serr != cdb.ErrDoesNotExist {
			logger.Error().Err(serr).Str("IB Partition ID", ibp.ID.String()).Msg("error deleting IB Partition record in DB")
			return serr
		}
	}

	// Delete Site if exists
	err = siteDAO.Delete(ctx, nil, siteID)
	if err != nil && err != cdb.ErrDoesNotExist {
		logger.Error().Err(err).Msg("failed to delete Site record in DB")
		return err
	}

	logger.Info().Msg("successfully completed activity")

	return nil
}

// MonitorInventoryReceiptForAllSites loops through all Sites and checks when the last inventory was received
func (mst ManageSite) MonitorInventoryReceiptForAllSites(ctx context.Context) error {
	logger := log.With().Str("activity", "MonitorInventoryReceiptForAllSites").Logger()

	logger.Info().Msg("starting activity")

	// Get all Sites
	siteDAO := cdbm.NewSiteDAO(mst.dbSession)

	sites, _, err := siteDAO.GetAll(ctx, nil, cdbm.SiteFilterInput{Statuses: []string{string(cdbm.SiteStatusRegistered)}}, cdbp.PageInput{Limit: cdb.GetIntPtr(cdbp.TotalLimit)}, nil)
	if err != nil {
		logger.Error().Err(err).Msg("failed to retrieve Sites from DB")
		return err
	}

	// Loop through Sites
	for _, site := range sites {
		// Get Site's last inventory receipt
		if site.InventoryReceived == nil {
			logger.Warn().Str("Site ID", site.ID.String()).Msg("Site has Registered status but hasn't received inventory yet")
			continue
		}

		// Check if last inventory receipt is older than timeout
		if time.Since(*site.InventoryReceived) > SiteInventoryReceiptThreshold {
			logger.Error().Str("Site ID", site.ID.String()).Msg("Site hasn't received inventory for longer than threshold period")

			if mst.cfg.GetNotificationsSlackEnabled() {
				// Send Slack notification
				sc := util.NewSlackClient(mst.cfg.GetNotificationsSlackWebhookURL())
				sm := util.SlackMessage{
					Text: fmt.Sprintf(":rotating_light: *Site Disconnection Detected*\n\nSite: `%s` hasn't received Machine inventory for longer than threshold period of: %v minutes", site.Name, SiteInventoryReceiptThreshold.Minutes()),
				}
				err := sc.SendSlackNotification(sm)
				if err != nil {
					logger.Error().Err(err).Msg("failed to send Slack notification for Site down event")
				}
			}

			if mst.cfg.GetNotificationsPagerDutyEnabled() {
				// Send PagerDuty notification
				pc := util.NewPagerDutyClient(mst.cfg.GetNotificationsPagerDutyIntegrationKey())
				customDetails := map[string]string{
					"site_id":             site.ID.String(),
					"site_name":           site.Name,
					"threshold_minutes":   fmt.Sprintf("%.0f", SiteInventoryReceiptThreshold.Minutes()),
					"last_inventory_time": site.InventoryReceived.Format(time.RFC3339),
					"time_since_last":     time.Since(*site.InventoryReceived).String(),
					"description":         fmt.Sprintf("Site hasn't received Machine inventory for longer than threshold period of: %v minutes", SiteInventoryReceiptThreshold.Minutes()),
				}
				err := pc.SendPagerDutyAlertWithDedupeKey(
					ctx,
					fmt.Sprintf("Site Disconnection Detected: %s", site.Name),
					"cloud-workflow-monitor",
					fmt.Sprintf("site-disconnection-%s", site.ID.String()),
					customDetails,
				)
				if err != nil {
					logger.Error().Err(err).Msg("failed to send PagerDuty notification for Site down event")
				}
			}

			// Set Site status to error
			errMsg := fmt.Sprintf("Site hasn't received inventory for longer than threshold period of: %v minutes", SiteInventoryReceiptThreshold.Minutes())
			serr := mst.updateSiteStatusInDB(ctx, nil, site.ID, cdb.GetStrPtr(cdbm.SiteStatusError), &errMsg)
			if serr != nil {
				logger.Error().Err(serr).Msg("error updating Site status in DB")
				return serr
			}
		}
	}

	logger.Info().Msg("successfully completed activity")

	return nil
}

// GetAllSites returns all sites
func (mst ManageSite) GetAllSiteIDs(ctx context.Context) ([]uuid.UUID, error) {
	logger := log.With().Str("activity", "GetAllSites").Logger()

	logger.Info().Msg("starting activity")

	// Get all Sites
	siteDAO := cdbm.NewSiteDAO(mst.dbSession)

	sites, _, err := siteDAO.GetAll(ctx, nil, cdbm.SiteFilterInput{}, cdbp.PageInput{Limit: cdb.GetIntPtr(cdbp.TotalLimit)}, nil)
	if err != nil {
		logger.Error().Err(err).Msg("failed to retrieve Sites from DB")
		return nil, err
	}

	var siteIDs []uuid.UUID
	for _, site := range sites {
		siteIDs = append(siteIDs, site.ID)
	}

	logger.Info().Msg("successfully completed activity")

	return siteIDs, nil
}

// updateSiteStatusInDB is helper function to write Site status updates to DB
func (mst ManageSite) updateSiteStatusInDB(ctx context.Context, tx *cdb.Tx, siteID uuid.UUID, status *string, statusMessage *string) error {
	logger := log.With().Str("activity", "updateSiteStatusInDB").Logger()
	logger.Info().Msg(fmt.Sprintf("status value: %v, siteID: %v", status, siteID))
	if status != nil {
		siteDAO := cdbm.NewSiteDAO(mst.dbSession)

		_, err := siteDAO.Update(ctx, tx, cdbm.SiteUpdateInput{SiteID: siteID, Status: status})
		if err != nil {
			return err
		}

		statusDetailDAO := cdbm.NewStatusDetailDAO(mst.dbSession)
		_, err = statusDetailDAO.CreateFromParams(ctx, tx, siteID.String(), *status, statusMessage)
		if err != nil {
			return err
		}
	}
	return nil
}

// CheckOTPExpirationAndRenewForAllSites periodically checks all sites and rotates OTPs if necessary
func (mst ManageSite) CheckOTPExpirationAndRenewForAllSites(ctx context.Context) error {
	logger := log.With().Str("Activity", "CheckOTPExpirationAndRenewForAllSites").Logger()

	logger.Info().Msg("starting activity")

	stDAO := cdbm.NewSiteDAO(mst.dbSession)
	sites, _, err := stDAO.GetAll(ctx, nil, cdbm.SiteFilterInput{Statuses: []string{cdbm.SiteStatusRegistered}}, cdbp.PageInput{Limit: cdb.GetIntPtr(cdbp.TotalLimit)}, nil)
	if err != nil {
		logger.Error().Err(err).Msg("Error retrieving Site from DB")
		return err
	}

	siteMgrURL := mst.cfg.GetSiteManagerEndpoint()
	for _, site := range sites {

		// Assume we need to rotate immediately
		daysToExpiration := float64(0)

		// If the site data _has_ expiration info, then do he work
		// to figure out if we _really_ need to rotate. For brand new
		// sites, AgentCertExpiry might be nil, which means our worst
		// case is that a new site gets a rotation immediately after
		// being registered.
		if site.AgentCertExpiry != nil {
			daysToExpiration = time.Until(*site.AgentCertExpiry).Hours() / 24
		}

		// Check if certificates are close to expiry
		if daysToExpiration <= rotationBufferDays {
			logger.Info().Str("siteUUID", site.ID.String()).Msg("Certificates are close to expiry, rotating OTPs")

			err = csm.RollSite(ctx, logger, site.ID.String(), site.Name, siteMgrURL)
			if err != nil {
				logger.Error().Err(err).Str("siteUUID", site.ID.String()).Msg("Failed to rotate OTPs")
				continue
			}

			newOTP, _, err := csm.GetSiteOTP(ctx, logger, site.ID.String(), siteMgrURL)
			if err != nil {
				logger.Error().Err(err).Str("siteUUID", site.ID.String()).Msg("Failed to retrieve new OTP after rotation")
				continue
			}

			// Encrypt the new OTP with the siteID
			encryptedOTP := cloudutils.EncryptData([]byte(*newOTP), site.ID.String())
			// Base64 encode the encrypted OTP
			base64EncodedEncryptedOTP := base64.StdEncoding.EncodeToString(encryptedOTP)

			// Retrieve Temporal client for the site
			tc, err := mst.siteClientPool.GetClientByID(site.ID)
			if err != nil {
				logger.Error().Err(err).Str("siteUUID", site.ID.String()).Msg("Failed to retrieve Temporal client for Site")
				continue
			}

			// Start the Temporal workflow with the base64 encoded OTP
			workflowOptions := client.StartWorkflowOptions{
				ID:        "site-otp-rotation-" + site.ID.String(),
				TaskQueue: queue.SiteTaskQueue,
			}

			we, err := tc.ExecuteWorkflow(ctx, workflowOptions, "RotateTemporalCertAccessOTP", base64EncodedEncryptedOTP)
			if err != nil {
				logger.Error().Err(err).Str("siteUUID", site.ID.String()).Msg("Failed to start Temporal workflow for OTP processing")
			} else {
				logger.Info().Str("Workflow ID", we.GetID()).Str("siteUUID", site.ID.String()).Msg("Successfully started Temporal workflow for OTP processing")
			}
		}
	}

	logger.Info().Msg("successfully completed activity")

	return nil
}

// UpdateAgentCertExpiry updates the AgentCertExpiry field for a site
func (mst ManageSite) UpdateAgentCertExpiry(ctx context.Context, siteID uuid.UUID, certExpiry time.Time) error {
	logger := log.With().Str("Activity", "UpdateAgentCertExpiry").Str("SiteID", siteID.String()).Logger()

	logger.Info().Msg("starting activity")

	// Update the AgentCertExpiry field in the database
	siteDAO := cdbm.NewSiteDAO(mst.dbSession)
	input := cdbm.SiteUpdateInput{
		SiteID:          siteID,
		AgentCertExpiry: &certExpiry,
	}

	_, err := siteDAO.Update(ctx, nil, input)
	if err != nil {
		logger.Error().Err(err).Msg("Failed to update AgentCertExpiry in the database")
		return err
	}

	logger.Info().Msg("successfully completed activity")

	return nil
}

// DeleteOrphanedSiteTemporalNamespaces finds and deletes orphaned Temporal namespaces for sites
func (mst ManageSite) DeleteOrphanedSiteTemporalNamespaces(ctx context.Context) error {
	logger := log.With().Str("activity", "DeleteOrphanedSiteTemporalNamespaces").Logger()
	logger.Info().Msg("Starting activity")

	tosc := mst.tc.WorkflowService()

	// Get existing namespaces from Temporal
	page := 1
	resp, err := tosc.ListNamespaces(ctx, &tWorkflowv1.ListNamespacesRequest{
		PageSize: 100,
	})
	if err != nil {
		logger.Error().Err(err).Msg("Failed to list Temporal namespaces")
		return fmt.Errorf("failed to list Temporal namespaces: %w", err)
	}

	var namespaces []string

	for resp.NextPageToken != nil {
		for _, ns := range resp.Namespaces {
			namespaces = append(namespaces, ns.NamespaceInfo.Name)
		}

		page += 1

		logger.Info().Int("Page", page).Msg("Listing Temporal namespaces for page")

		resp, err = tosc.ListNamespaces(ctx, &tWorkflowv1.ListNamespacesRequest{
			PageSize:      100,
			NextPageToken: resp.NextPageToken,
		})
		if err != nil {
			logger.Error().Err(err).Int("Page", page).Msg("Failed to list Temporal namespaces for page")
			return fmt.Errorf("failed to list Temporal namespaces: %w", err)
		}
	}

	for _, ns := range resp.Namespaces {
		namespaces = append(namespaces, ns.NamespaceInfo.Name)
	}

	logger.Info().Int("Total Namespaces", len(namespaces)).Msg("Retrieved Temporal namespaces")

	// Get existing Site IDs
	stDAO := cdbm.NewSiteDAO(mst.dbSession)
	sites, count, err := stDAO.GetAll(ctx, nil, cdbm.SiteFilterInput{}, cdbp.PageInput{Limit: cdb.GetIntPtr(cdbp.TotalLimit)}, nil)
	if err != nil {
		logger.Error().Err(err).Msg("Failed to retrieve Sites from DB")
		return fmt.Errorf("failed to get sites from DB: %w", err)
	}

	logger.Info().Int("Total Sites", count).Msg("Retrieved Sites from DB")

	existingSiteMap := map[string]bool{}
	for _, site := range sites {
		existingSiteMap[site.ID.String()] = true
	}

	// Check if namespace is orphaned
	for _, namespace := range namespaces {
		// Skip if namespace is not a UUID (not a site namespace)
		siteID, err := uuid.Parse(namespace)
		if err != nil {
			// Not a valid UUID, like cloud, site or temporal-system namespace
			continue
		}

		if existingSiteMap[namespace] {
			continue
		}

		// Check that namespace refers to a deleted Site
		_, err = stDAO.GetByID(ctx, nil, siteID, nil, true)
		if err != nil {
			logger.Error().Err(err).Str("Namespace", namespace).Msg("Failed to retrieve deleted Site from DB, skipping namespace deletion")
			continue
		}

		logger.Info().Str("Namespace", namespace).Msg("Deleting orphaned Temporal namespace")

		tosc := mst.tc.OperatorService()
		_, err = tosc.DeleteNamespace(ctx, &tOperatorv1.DeleteNamespaceRequest{
			Namespace: namespace,
		})
		if err != nil {
			logger.Error().Err(err).Str("Namespace", namespace).Msg("Failed to delete temporal namespace")
			return err
		}
	}

	logger.Info().Msg("successfully completed activity")

	return nil
}

// NewManageSite returns a new ManageSite activity
func NewManageSite(dbSession *cdb.Session, siteClientPool *sc.ClientPool, tc client.Client, cfg *config.Config) ManageSite {
	return ManageSite{
		dbSession:      dbSession,
		siteClientPool: siteClientPool,
		tc:             tc,
		cfg:            cfg,
	}
}
