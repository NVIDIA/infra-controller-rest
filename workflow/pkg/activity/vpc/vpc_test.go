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

package vpc

import (
	"context"
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/NVIDIA/ncx-infra-controller-rest/db/pkg/db"
	cdb "github.com/NVIDIA/ncx-infra-controller-rest/db/pkg/db"
	cdbm "github.com/NVIDIA/ncx-infra-controller-rest/db/pkg/db/model"
	cdbu "github.com/NVIDIA/ncx-infra-controller-rest/db/pkg/util"
	cwssaws "github.com/NVIDIA/ncx-infra-controller-rest/workflow-schema/schema/site-agent/workflows/v1"
	sc "github.com/NVIDIA/ncx-infra-controller-rest/workflow/pkg/client/site"
	"github.com/NVIDIA/ncx-infra-controller-rest/workflow/pkg/queue"
	"github.com/NVIDIA/ncx-infra-controller-rest/workflow/pkg/util"
	cwu "github.com/NVIDIA/ncx-infra-controller-rest/workflow/pkg/util"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/uptrace/bun/extra/bundebug"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/google/uuid"

	"github.com/NVIDIA/ncx-infra-controller-rest/workflow/internal/config"

	"os"

	"go.temporal.io/sdk/client"
	tmocks "go.temporal.io/sdk/mocks"

	"go.temporal.io/sdk/testsuite"

	cwm "github.com/NVIDIA/ncx-infra-controller-rest/workflow/internal/metrics"
	"github.com/prometheus/client_golang/prometheus"

	cwutil "github.com/NVIDIA/ncx-infra-controller-rest/common/pkg/util"
	authz "github.com/NVIDIA/ncx-infra-controller-rest/auth/pkg/authorization"
)

// testTemporalSiteClientPool Building site client pool
func testTemporalSiteClientPool(t *testing.T) *sc.ClientPool {

	keyPath, certPath := config.SetupTestCerts(t)
	defer os.Remove(keyPath)
	defer os.Remove(certPath)

	cfg := config.NewConfig()
	cfg.SetTemporalCertPath(certPath)
	cfg.SetTemporalKeyPath(keyPath)
	cfg.SetTemporalCaPath(certPath)

	tcfg, err := cfg.GetTemporalConfig()
	assert.NoError(t, err)

	tSiteClientPool := sc.NewClientPool(tcfg)
	return tSiteClientPool
}

func testVPCInitDB(t *testing.T) *cdb.Session {
	dbSession := cdbu.GetTestDBSession(t, false)
	dbSession.DB.AddQueryHook(bundebug.NewQueryHook(
		bundebug.WithEnabled(false),
		bundebug.FromEnv("BUNDEBUG"),
	))
	return dbSession
}

func testVPCSetupSchema(t *testing.T, dbSession *cdb.Session) {
	// create Infrastructure Provider table
	err := dbSession.DB.ResetModel(context.Background(), (*cdbm.InfrastructureProvider)(nil))
	assert.Nil(t, err)
	// create Site table
	err = dbSession.DB.ResetModel(context.Background(), (*cdbm.Site)(nil))
	assert.Nil(t, err)
	// create Tenant table
	err = dbSession.DB.ResetModel(context.Background(), (*cdbm.Tenant)(nil))
	assert.Nil(t, err)
	// create User table
	err = dbSession.DB.ResetModel(context.Background(), (*cdbm.User)(nil))
	assert.Nil(t, err)
	// create Allocation table
	err = dbSession.DB.ResetModel(context.Background(), (*cdbm.Allocation)(nil))
	assert.Nil(t, err)
	// create Status Details table
	err = dbSession.DB.ResetModel(context.Background(), (*cdbm.StatusDetail)(nil))
	assert.Nil(t, err)
	// create VPC table
	err = dbSession.DB.ResetModel(context.Background(), (*cdbm.Vpc)(nil))
	assert.Nil(t, err)
}

// testVPCSiteBuildInfrastructureProvider Building Infra Provider in DB
func testVPCSiteBuildInfrastructureProvider(t *testing.T, dbSession *cdb.Session, name string, org string, user *cdbm.User) *cdbm.InfrastructureProvider {
	ipDAO := cdbm.NewInfrastructureProviderDAO(dbSession)

	ip, err := ipDAO.CreateFromParams(context.Background(), nil, name, cdb.GetStrPtr("Test Provider"), org, nil, user)
	assert.Nil(t, err)

	return ip
}

// testVPCBuildSite Building Site in DB
func testVPCBuildSite(t *testing.T, dbSession *cdb.Session, ip *cdbm.InfrastructureProvider, name string, user *cdbm.User) *cdbm.Site {
	stDAO := cdbm.NewSiteDAO(dbSession)

	st, err := stDAO.Create(context.Background(), nil, cdbm.SiteCreateInput{
		Name:                        name,
		DisplayName:                 cdb.GetStrPtr("Test Site"),
		Description:                 cdb.GetStrPtr("Test Site Description"),
		Org:                         ip.Org,
		InfrastructureProviderID:    ip.ID,
		SiteControllerVersion:       cdb.GetStrPtr("1.0.0"),
		SiteAgentVersion:            cdb.GetStrPtr("1.0.0"),
		RegistrationToken:           cdb.GetStrPtr("1234-5678-9012-3456"),
		RegistrationTokenExpiration: cdb.GetTimePtr(cdb.GetCurTime()),
		IsInfinityEnabled:           false,
		IsSerialConsoleEnabled:      false,
		Status:                      cdbm.SiteStatusPending,
		CreatedBy:                   user.ID,
	})
	assert.Nil(t, err)

	return st
}

// testVPCBuildTenant Building Tenant in DB
func testVPCBuildTenant(t *testing.T, dbSession *cdb.Session, name string, org string, user *cdbm.User) *cdbm.Tenant {
	tnDAO := cdbm.NewTenantDAO(dbSession)

	tn, err := tnDAO.CreateFromParams(context.Background(), nil, name, cdb.GetStrPtr("Test Tenant"), org, nil, nil, user)
	assert.Nil(t, err)

	return tn
}

// testVPCBuildUser Building User in DB
func testVPCBuildUser(t *testing.T, dbSession *cdb.Session, starfleetID string, org string, roles []string) *cdbm.User {
	uDAO := cdbm.NewUserDAO(dbSession)

	u, err := uDAO.Create(context.Background(), nil, cdbm.UserCreateInput{
		AuxiliaryID: nil,
		StarfleetID: &starfleetID,
		Email:       cdb.GetStrPtr("jdoe@test.com"),
		FirstName:   cdb.GetStrPtr("John"),
		LastName:    cdb.GetStrPtr("Doe"),
		OrgData: cdbm.OrgData{
			org: cdbm.Org{
				ID:      123,
				Name:    org,
				OrgType: "ENTERPRISE",
				Roles:   roles,
			},
		},
	})
	assert.Nil(t, err)

	return u
}

// testVPCSiteBuildAllocation Building Site Allocation in DB
func testVPCSiteBuildAllocation(t *testing.T, dbSession *cdb.Session, st *cdbm.Site, tn *cdbm.Tenant, name string, user *cdbm.User) *cdbm.Allocation {
	alDAO := cdbm.NewAllocationDAO(dbSession)

	createInput := cdbm.AllocationCreateInput{
		Name:                     name,
		Description:              cdb.GetStrPtr("Test Allocation Description"),
		InfrastructureProviderID: st.InfrastructureProviderID,
		TenantID:                 tn.ID,
		SiteID:                   st.ID,
		Status:                   cdbm.AllocationStatusPending,
		CreatedBy:                user.ID,
	}
	al, err := alDAO.Create(context.Background(), nil, createInput)
	assert.Nil(t, err)

	return al
}

// testVPCBuildVPC Building VPC in DB
func testVPCBuildVPC(t *testing.T, dbSession *cdb.Session, name string, ip *cdbm.InfrastructureProvider, tn *cdbm.Tenant, st *cdbm.Site, networkVirtualizationType *string, ct *uuid.UUID, lb map[string]string, user *cdbm.User, status string) *cdbm.Vpc {
	vpcDAO := cdbm.NewVpcDAO(dbSession)

	input := cdbm.VpcCreateInput{
		Name:                      name,
		Description:               cdb.GetStrPtr("Test VPC"),
		Org:                       st.Org,
		InfrastructureProviderID:  ip.ID,
		TenantID:                  tn.ID,
		SiteID:                    st.ID,
		NetworkVirtualizationType: networkVirtualizationType,
		ControllerVpcID:           ct,
		Labels:                    lb,
		Status:                    status,
		CreatedBy:                 *user,
	}

	vpc, err := vpcDAO.Create(context.Background(), nil, input)
	assert.Nil(t, err)

	return vpc
}

func TestManageVpc_CreateVpcViaSiteAgent(t *testing.T) {
	type fields struct {
		dbSession      *cdb.Session
		siteClientPool *sc.ClientPool
		tc             client.Client
		env            *testsuite.TestWorkflowEnvironment
	}

	type args struct {
		ctx    context.Context
		siteID uuid.UUID
		vpcID  uuid.UUID
	}

	dbSession := testVPCInitDB(t)
	defer dbSession.Close()

	testVPCSetupSchema(t, dbSession)

	org := "test-org"
	orgRoles := []string{authz.ProviderAdminRole}

	vpu := testVPCBuildUser(t, dbSession, "test123", org, orgRoles)
	ip := testVPCSiteBuildInfrastructureProvider(t, dbSession, "Test VPC Site Infrastructure Provider", org, vpu)
	vpt := testVPCBuildTenant(t, dbSession, "test123", org, vpu)
	assert.NotNil(t, vpt)
	vps := testVPCBuildSite(t, dbSession, ip, "test123", vpu)
	assert.NotNil(t, vps)
	vpa := testVPCSiteBuildAllocation(t, dbSession, vps, vpt, "test123", vpu)
	assert.NotNil(t, vpa)
	vpc := testVPCBuildVPC(t, dbSession, "test123", ip, vpt, vps, cdb.GetStrPtr(cdbm.VpcEthernetVirtualizer), nil, map[string]string{"region": "us-west-1", "zone": "west"}, vpu, cdbm.VpcStatusPending)
	assert.NotNil(t, vpc)

	tSiteClientPool := testTemporalSiteClientPool(t)
	assert.NotNil(t, tSiteClientPool)

	tc := &tmocks.Client{}

	temporalsuit := testsuite.WorkflowTestSuite{}
	env := temporalsuit.NewTestWorkflowEnvironment()

	tests := []struct {
		name           string
		vpcID          uuid.UUID
		fields         fields
		args           args
		want           error
		wantErr        bool
		wantLabelCount int
	}{
		{
			name: "test VPC create activity from site agent",
			fields: fields{
				dbSession:      dbSession,
				siteClientPool: tSiteClientPool,
				tc:             tc,
				env:            env,
			},
			args: args{
				ctx:    context.Background(),
				siteID: vps.ID,
				vpcID:  vpc.ID,
			},
			want:           nil,
			wantLabelCount: 2,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mv := ManageVpc{
				dbSession:      tt.fields.dbSession,
				siteClientPool: tSiteClientPool,
			}

			// Mock the "CreateVPC" workflow
			mtc := &tmocks.Client{}
			mv.siteClientPool.IDClientMap[vps.ID.String()] = mtc

			// Vpc metadata info
			metadata := &cwssaws.Metadata{
				Name: vpc.Name,
			}

			// Prepare labels for site controller
			if len(vpc.Labels) > 0 {
				var labels []*cwssaws.Label
				for key, value := range vpc.Labels {
					curVal := value
					localLable := &cwssaws.Label{
						Key:   key,
						Value: &curVal,
					}
					labels = append(labels, localLable)
				}
				metadata.Labels = labels
			}

			createVpcRequest := &cwssaws.CreateVPCRequest{
				VpcId:                &cwssaws.UUID{Value: vpc.ID.String()},
				Name:                 vpc.Name,
				TenantOrganizationId: vpc.Org,
			}

			testWorkflowID := "test-workflowid"
			testRunID := "test-runid"

			mockWorkflowRun := &tmocks.WorkflowRun{}
			mockWorkflowRun.On("GetID").Return(testWorkflowID).Times(4)
			mockWorkflowRun.On("GetRunID").Return(testRunID).Times(4)
			mockWorkflowRun.On("Get", mock.Anything, mock.Anything).Return(nil).Times(2)
			mockWorkflowRun.On("GetWithOptions", mock.Anything, mock.Anything).Return(nil).Times(2)

			mtc.On("ExecuteWorkflow", mock.Anything, mock.Anything, mock.Anything, mock.Anything, createVpcRequest).Return(mockWorkflowRun, nil).Once()

			err := mv.CreateVpcViaSiteAgent(tt.args.ctx, tt.args.siteID, tt.args.vpcID)
			if (err != nil) != tt.wantErr {
				t.Errorf("ManageVpc.CreateVpcViaSiteAgent() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			// Check if the VPC was updated in the DB
			vpcDAO := cdbm.NewVpcDAO(dbSession)
			tvpc, err := vpcDAO.GetByID(context.Background(), nil, tt.args.vpcID, nil)
			assert.Nil(t, err)
			assert.Equal(t, tvpc.Status, cdbm.VpcStatusProvisioning)
		})
	}
}

func TestManageVpc_DeleteVpcViaSiteAgent(t *testing.T) {
	dbSession := testVPCInitDB(t)
	defer dbSession.Close()

	testVPCSetupSchema(t, dbSession)

	org := "test-org"
	orgRoles := []string{authz.ProviderAdminRole}

	vpu := testVPCBuildUser(t, dbSession, "test123", org, orgRoles)
	ip := testVPCSiteBuildInfrastructureProvider(t, dbSession, "Test VPC Site Infrastructure Provider", org, vpu)
	vpt := testVPCBuildTenant(t, dbSession, "test123", org, vpu)
	assert.NotNil(t, vpt)
	vps := testVPCBuildSite(t, dbSession, ip, "test123", vpu)
	assert.NotNil(t, vps)
	vpa := testVPCSiteBuildAllocation(t, dbSession, vps, vpt, "test123", vpu)
	assert.NotNil(t, vpa)
	vpc1 := testVPCBuildVPC(t, dbSession, "test123", ip, vpt, vps, cdb.GetStrPtr(cdbm.VpcEthernetVirtualizer), cdb.GetUUIDPtr(uuid.New()), nil, vpu, cdbm.VpcStatusPending)
	assert.NotNil(t, vpc1)
	vpc2 := testVPCBuildVPC(t, dbSession, "test123", ip, vpt, vps, cdb.GetStrPtr(cdbm.VpcEthernetVirtualizer), nil, nil, vpu, cdbm.VpcStatusPending)
	assert.NotNil(t, vpc2)

	tSiteClientPool := testTemporalSiteClientPool(t)
	assert.NotNil(t, tSiteClientPool)

	temporalsuit := testsuite.WorkflowTestSuite{}
	env := temporalsuit.NewTestWorkflowEnvironment()

	type fields struct {
		dbSession      *cdb.Session
		siteClientPool *sc.ClientPool
		env            *testsuite.TestWorkflowEnvironment
	}

	type args struct {
		ctx    context.Context
		siteID uuid.UUID
		vpc    *cdbm.Vpc
	}

	tests := []struct {
		name    string
		fields  fields
		args    args
		wantErr bool
	}{
		{
			name: "test VPC delete activity from site agent successfully when controller vpc Id set",
			fields: fields{
				dbSession:      dbSession,
				siteClientPool: tSiteClientPool,
				env:            env,
			},
			args: args{
				ctx:    context.Background(),
				siteID: vps.ID,
				vpc:    vpc1,
			},
			wantErr: false,
		},
		{
			name: "test VPC delete activity from returns error when controller vpc Id nil",
			fields: fields{
				dbSession:      dbSession,
				siteClientPool: tSiteClientPool,
				env:            env,
			},
			args: args{
				ctx:    context.Background(),
				siteID: vps.ID,
				vpc:    vpc2,
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mv := ManageVpc{
				dbSession:      tt.fields.dbSession,
				siteClientPool: tSiteClientPool,
			}

			// Mock Temporal client pool
			mtc := &tmocks.Client{}
			mv.siteClientPool.IDClientMap[vps.ID.String()] = mtc

			// Match controller VPC ID for mocking workflow
			controllerVpcID := &cwssaws.UUID{Value: ""}
			if tt.args.vpc.ControllerVpcID != nil {
				controllerVpcID = &cwssaws.UUID{Value: tt.args.vpc.ControllerVpcID.String()}
			}

			deleteVpcRequest := &cwssaws.DeleteVPCRequest{
				Id: controllerVpcID,
			}

			testWorkflowID := "test-workflowid"
			testRunID := "test-runid"

			mockWorkflowRun := &tmocks.WorkflowRun{}
			mockWorkflowRun.On("GetID").Return(testWorkflowID).Times(4)
			mockWorkflowRun.On("GetRunID").Return(testRunID).Times(4)
			mockWorkflowRun.On("Get", mock.Anything, mock.Anything).Return(nil).Times(2)
			mockWorkflowRun.On("GetWithOptions", mock.Anything, mock.Anything).Return(nil).Times(2)

			mtc.On("ExecuteWorkflow", mock.Anything, mock.Anything, mock.Anything, mock.Anything, deleteVpcRequest).Return(mockWorkflowRun, nil).Once()

			err := mv.DeleteVpcViaSiteAgent(tt.args.ctx, tt.args.siteID, tt.args.vpc.ID)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}

			// Check if the VPC was updated or deleted in the DB, we will check for `deleting`
			vpcDAO := cdbm.NewVpcDAO(dbSession)
			tvpc, err := vpcDAO.GetByID(context.Background(), nil, tt.args.vpc.ID, nil)
			assert.Nil(t, err)
			assert.Equal(t, cdbm.VpcStatusDeleting, tvpc.Status)
		})
	}
}

func TestManageVpc_UpdateVpcInDB(t *testing.T) {
	dbSession := testVPCInitDB(t)
	defer dbSession.Close()

	testVPCSetupSchema(t, dbSession)

	ipOrg := "test-provider-org"
	ipRoles := []string{authz.ProviderAdminRole}

	ipu := testVPCBuildUser(t, dbSession, uuid.NewString(), ipOrg, ipRoles)
	ip := testVPCSiteBuildInfrastructureProvider(t, dbSession, "test-provider", ipOrg, ipu)

	tnOrg := "test-tenant-org"
	tnRoles := []string{authz.TenantAdminRole}

	tnu := testVPCBuildUser(t, dbSession, uuid.NewString(), tnOrg, tnRoles)

	tn := testVPCBuildTenant(t, dbSession, "test-tenant", tnOrg, tnu)
	assert.NotNil(t, tn)

	st := testVPCBuildSite(t, dbSession, ip, "test-site", ipu)
	assert.NotNil(t, st)

	al := testVPCSiteBuildAllocation(t, dbSession, st, tn, "test-allocation", ipu)
	assert.NotNil(t, al)

	vpc1 := testVPCBuildVPC(t, dbSession, "test-vpc-1", ip, tn, st, cdb.GetStrPtr(cdbm.VpcEthernetVirtualizer), nil, nil, tnu, cdbm.VpcStatusPending)
	assert.NotNil(t, vpc1)

	vpc2 := testVPCBuildVPC(t, dbSession, "test-vpc-2", ip, tn, st, cdb.GetStrPtr(cdbm.VpcEthernetVirtualizer), nil, nil, tnu, cdbm.VpcStatusPending)
	assert.NotNil(t, vpc2)

	vpc3 := testVPCBuildVPC(t, dbSession, "test-vpc-3", ip, tn, st, cdb.GetStrPtr(cdbm.VpcEthernetVirtualizer), nil, nil, tnu, cdbm.VpcStatusDeleting)
	assert.NotNil(t, vpc3)

	vpc4 := testVPCBuildVPC(t, dbSession, "test-vpc-4", ip, tn, st, cdb.GetStrPtr(cdbm.VpcEthernetVirtualizer), nil, nil, tnu, cdbm.VpcStatusDeleting)
	assert.NotNil(t, vpc4)

	tSiteClientPool := testTemporalSiteClientPool(t)
	assert.NotNil(t, tSiteClientPool)

	temporalsuit := testsuite.WorkflowTestSuite{}
	env := temporalsuit.NewTestWorkflowEnvironment()

	type fields struct {
		dbSession      *cdb.Session
		siteClientPool *sc.ClientPool
		env            *testsuite.TestWorkflowEnvironment
	}

	type args struct {
		ctx               context.Context
		vpc               *cdbm.Vpc
		transactionID     *cwssaws.TransactionID
		vpcInfo           *cwssaws.VPCInfo
		expectVpcDeletion bool
	}

	tests := []struct {
		name   string
		fields fields
		args   args
	}{
		{
			name: "test VPC update on creation",
			fields: fields{
				dbSession:      dbSession,
				siteClientPool: tSiteClientPool,
				env:            env,
			},
			args: args{
				ctx: context.Background(),
				vpc: vpc1,
				transactionID: &cwssaws.TransactionID{
					ResourceId: vpc1.ID.String(),
					Timestamp:  timestamppb.Now(),
				},
				vpcInfo: &cwssaws.VPCInfo{
					Status:    cwssaws.WorkflowStatus_WORKFLOW_STATUS_SUCCESS,
					StatusMsg: "VPC was successfully created",
					Vpc: &cwssaws.Vpc{
						Id:                   &cwssaws.VpcId{Value: uuid.New().String()},
						Name:                 vpc1.ID.String(),
						TenantOrganizationId: vpc1.Org,
					},
					ObjectStatus: cwssaws.ObjectStatus_OBJECT_STATUS_CREATED,
				},
			},
		},
		{
			name: "test VPC update on deletion",
			fields: fields{
				dbSession:      dbSession,
				siteClientPool: tSiteClientPool,
				env:            env,
			},
			args: args{
				ctx: context.Background(),
				vpc: vpc1,
				transactionID: &cwssaws.TransactionID{
					ResourceId: vpc1.ID.String(),
					Timestamp:  timestamppb.Now(),
				},
				vpcInfo: &cwssaws.VPCInfo{
					Status:       cwssaws.WorkflowStatus_WORKFLOW_STATUS_SUCCESS,
					StatusMsg:    "VPC was successfully deleted",
					ObjectStatus: cwssaws.ObjectStatus_OBJECT_STATUS_DELETED,
				},
			},
		},
		{
			name: "test VPC update on error",
			fields: fields{
				dbSession:      dbSession,
				siteClientPool: tSiteClientPool,
				env:            env,
			},
			args: args{
				ctx: context.Background(),
				vpc: vpc2,
				transactionID: &cwssaws.TransactionID{
					ResourceId: vpc2.ID.String(),
					Timestamp:  timestamppb.Now(),
				},
				vpcInfo: &cwssaws.VPCInfo{
					Status:    cwssaws.WorkflowStatus_WORKFLOW_STATUS_FAILURE,
					StatusMsg: "error creating VPC",
				},
			},
		},
		{
			name: "test VPC update on error while deleting, arbitrary error on Site",
			fields: fields{
				dbSession:      dbSession,
				siteClientPool: tSiteClientPool,
				env:            env,
			},
			args: args{
				ctx: context.Background(),
				vpc: vpc4,
				transactionID: &cwssaws.TransactionID{
					ResourceId: vpc4.ID.String(),
					Timestamp:  timestamppb.Now(),
				},
				vpcInfo: &cwssaws.VPCInfo{
					Status:    cwssaws.WorkflowStatus_WORKFLOW_STATUS_FAILURE,
					StatusMsg: "arbitrary error",
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mv := ManageVpc{
				dbSession:      tt.fields.dbSession,
				siteClientPool: tSiteClientPool,
			}

			// Mock the "CreateVPC" workflow
			mtc := &tmocks.Client{}
			mv.siteClientPool.IDClientMap[st.ID.String()] = mtc

			err := mv.UpdateVpcInDB(tt.args.ctx, tt.args.transactionID, tt.args.vpcInfo)
			assert.NoError(t, err)

			vpcDAO := cdbm.NewVpcDAO(dbSession)
			uvpc, err := vpcDAO.GetByID(context.Background(), nil, tt.args.vpc.ID, nil)

			// Verify statuses
			if tt.args.vpcInfo.ObjectStatus == cwssaws.ObjectStatus_OBJECT_STATUS_CREATED {
				assert.Nil(t, err)
				assert.Equal(t, cdbm.VpcStatusReady, uvpc.Status)
			} else if tt.args.vpcInfo.ObjectStatus == cwssaws.ObjectStatus_OBJECT_STATUS_DELETED {
				assert.Nil(t, err)
				assert.Equal(t, cdbm.VpcStatusDeleting, uvpc.Status)
			} else if tt.args.vpcInfo.Status == cwssaws.WorkflowStatus_WORKFLOW_STATUS_FAILURE {
				if tt.args.expectVpcDeletion {
					assert.Error(t, cdb.ErrDoesNotExist)
					assert.Nil(t, uvpc)
				} else if tt.args.vpc.Status == cdbm.VpcStatusDeleting {
					assert.Equal(t, cdbm.VpcStatusDeleting, uvpc.Status)
					assert.Nil(t, err)
				} else {
					assert.Equal(t, cdbm.VpcStatusError, uvpc.Status)
					assert.Nil(t, err)
				}
			}
		})
	}
}

func TestManageVpc_UpdateVpcsInDB(t *testing.T) {
	ctx := context.Background()

	dbSession := testVPCInitDB(t)
	defer dbSession.Close()

	testVPCSetupSchema(t, dbSession)

	ipOrg := "test-provider-org"
	ipRoles := []string{authz.ProviderAdminRole}

	ipu := testVPCBuildUser(t, dbSession, uuid.NewString(), ipOrg, ipRoles)
	ip := testVPCSiteBuildInfrastructureProvider(t, dbSession, "test-provider", ipOrg, ipu)

	tnOrg := "test-tenant-org"
	tnRoles := []string{authz.TenantAdminRole}

	tnu := testVPCBuildUser(t, dbSession, uuid.NewString(), tnOrg, tnRoles)
	tn := testVPCBuildTenant(t, dbSession, "test-tenant", tnOrg, tnu)

	st := testVPCBuildSite(t, dbSession, ip, "test-site", ipu)
	st2 := testVPCBuildSite(t, dbSession, ip, "test-site-2", ipu)
	st3 := testVPCBuildSite(t, dbSession, ip, "test-site-3", ipu)

	vpc1 := testVPCBuildVPC(t, dbSession, "test-vpc-1", ip, tn, st, cdb.GetStrPtr(""), cdb.GetUUIDPtr(uuid.New()), nil, tnu, cdbm.VpcStatusProvisioning)

	vpc2 := testVPCBuildVPC(t, dbSession, "test-vpc-2", ip, tn, st, cdb.GetStrPtr(cdbm.VpcEthernetVirtualizer), cdb.GetUUIDPtr(uuid.New()), nil, tnu, cdbm.VpcStatusProvisioning)

	vpc3 := testVPCBuildVPC(t, dbSession, "test-vpc-3", ip, tn, st, cdb.GetStrPtr(cdbm.VpcEthernetVirtualizer), cdb.GetUUIDPtr(uuid.New()), nil, tnu, cdbm.VpcStatusDeleting)

	vpc4 := testVPCBuildVPC(t, dbSession, "test-vpc-4", ip, tn, st, cdb.GetStrPtr(cdbm.VpcEthernetVirtualizer), cdb.GetUUIDPtr(uuid.New()), nil, tnu, cdbm.VpcStatusDeleting)

	vpc5 := testVPCBuildVPC(t, dbSession, "test-vpc-5", ip, tn, st, cdb.GetStrPtr(cdbm.VpcEthernetVirtualizer), cdb.GetUUIDPtr(uuid.New()), nil, tnu, cdbm.VpcStatusDeleting)

	vpc6 := testVPCBuildVPC(t, dbSession, "test-vpc-6", ip, tn, st, cdb.GetStrPtr(cdbm.VpcEthernetVirtualizer), nil, nil, tnu, cdbm.VpcStatusDeleting)

	vpc7 := testVPCBuildVPC(t, dbSession, "test-vpc-7", ip, tn, st, cdb.GetStrPtr(cdbm.VpcEthernetVirtualizer), cdb.GetUUIDPtr(uuid.New()), nil, tnu, cdbm.VpcStatusReady)
	// Set created earlier than the inventory receipt interval
	_, err := dbSession.DB.Exec("UPDATE vpc SET created = ? WHERE id = ?", time.Now().Add(-time.Duration(cwutil.InventoryReceiptInterval)), vpc7.ID.String())
	assert.NoError(t, err)

	vpc8 := testVPCBuildVPC(t, dbSession, "test-vpc-8", ip, tn, st, cdb.GetStrPtr(cdbm.VpcEthernetVirtualizer), cdb.GetUUIDPtr(uuid.New()), nil, tnu, cdbm.VpcStatusReady)

	vpc9 := testVPCBuildVPC(t, dbSession, "test-vpc-9", ip, tn, st, cdb.GetStrPtr(cdbm.VpcEthernetVirtualizer), nil, nil, tnu, cdbm.VpcStatusProvisioning)

	vpc10 := testVPCBuildVPC(t, dbSession, "test-vpc-10", ip, tn, st, cdb.GetStrPtr(cdbm.VpcEthernetVirtualizer), nil, nil, tnu, cdbm.VpcStatusDeleting)

	vpc11 := testVPCBuildVPC(t, dbSession, "test-vpc-11", ip, tn, st, cdb.GetStrPtr(cdbm.VpcEthernetVirtualizer), cdb.GetUUIDPtr(uuid.New()), nil, tnu, cdbm.VpcStatusReady)
	// Set created earlier than the inventory receipt interval
	_, err = dbSession.DB.Exec("UPDATE vpc SET created = ? WHERE id = ?", time.Now().Add(-time.Duration(cwutil.InventoryReceiptInterval)), vpc11.ID.String())
	assert.NoError(t, err)

	vpcDAO := cdbm.NewVpcDAO(dbSession)
	vpc8, err = vpcDAO.Update(ctx, nil, cdbm.VpcUpdateInput{VpcID: vpc8.ID, Status: cdb.GetStrPtr(cdbm.VpcStatusError), IsMissingOnSite: cdb.GetBoolPtr(true)})
	assert.NoError(t, err)

	vpc12 := testVPCBuildVPC(t, dbSession, "test-vpc-12", ip, tn, st, nil, cdb.GetUUIDPtr(uuid.New()), nil, tnu, cdbm.VpcStatusReady)
	// Set propagation details for VPC21.
	// We'll expect these to be cleared later.
	vpc12.NetworkSecurityGroupPropagationDetails = &cdbm.NetworkSecurityGroupPropagationDetails{
		NetworkSecurityGroupPropagationObjectStatus: &cwssaws.NetworkSecurityGroupPropagationObjectStatus{},
	}
	cwu.TestUpdateVPC(t, dbSession, vpc12)

	vpc13 := testVPCBuildVPC(t, dbSession, "test-vpc-13", ip, tn, st, nil, cdb.GetUUIDPtr(uuid.New()), nil, tnu, cdbm.VpcStatusReady)

	// Build VPC inventory that is paginated
	// Generate data for 34 VPCs reported from Site Agent while Cloud has 38 VPCs
	pagedVpcs := []*cdbm.Vpc{}
	pagedInvIds := []string{}
	labels := map[string]string{}
	for i := 0; i < 38; i++ {

		// Making labels mismatch
		if i == 1 {
			labels = map[string]string{
				"west1": "gpu",
			}
		}

		vpc := testVPCBuildVPC(t, dbSession, fmt.Sprintf("test-vpc-paged-%d", i), ip, tn, st3, cdb.GetStrPtr(cdbm.VpcEthernetVirtualizer), cdb.GetUUIDPtr(uuid.New()), labels, tnu, cdbm.VpcStatusReady)
		// Update creation timestamp to be earlier than inventory processing interval
		_, err = dbSession.DB.Exec("UPDATE vpc SET created = ? WHERE id = ?", time.Now().Add(-time.Duration(cwutil.InventoryReceiptInterval*2)), vpc.ID.String())
		assert.NoError(t, err)
		pagedVpcs = append(pagedVpcs, vpc)
		pagedInvIds = append(pagedInvIds, vpc.ControllerVpcID.String())
	}

	pagedCtrlVpcs := []*cwssaws.Vpc{}
	for i := 0; i < 34; i++ {
		ctrlVpc := &cwssaws.Vpc{
			Id:   &cwssaws.VpcId{Value: pagedVpcs[i].ControllerVpcID.String()},
			Name: pagedVpcs[i].Name,
			Vni:  util.GetUint32Ptr(uint32(i)),
			Status: &cwssaws.VpcStatus{
				Vni: util.GetUint32Ptr(uint32(i)),
			},
		}

		if i == 1 {
			ctrlVpc.Metadata = &cwssaws.Metadata{
				Name:        "Test VPC",
				Description: "Test description",
				Labels: []*cwssaws.Label{
					{
						Key:   "west1",
						Value: db.GetStrPtr("gpu1"),
					},
				},
			}
		}
		pagedCtrlVpcs = append(pagedCtrlVpcs, ctrlVpc)
	}

	tSiteClientPool := testTemporalSiteClientPool(t)
	assert.NotNil(t, tSiteClientPool)

	temporalsuit := testsuite.WorkflowTestSuite{}
	env := temporalsuit.NewTestWorkflowEnvironment()

	// Mock UpdateVpc workflow from site-agent
	wrun := &tmocks.WorkflowRun{}
	wid := "test-workflow-id"
	wrun.On("GetID").Return(wid)

	workflowOptions1 := client.StartWorkflowOptions{
		ID:        "site-vpc-update-metadata-" + pagedVpcs[1].ID.String(),
		TaskQueue: queue.SiteTaskQueue,
	}

	mtc1 := &tmocks.Client{}
	mtc1.Mock.On("ExecuteWorkflow", context.Background(), workflowOptions1, "UpdateVPC", mock.Anything).Return(wrun, nil)

	nwvt := cwssaws.VpcVirtualizationType_FNN
	evt := cwssaws.VpcVirtualizationType_ETHERNET_VIRTUALIZER

	type fields struct {
		dbSession        *cdb.Session
		siteClientPool   *sc.ClientPool
		clientPoolClient *tmocks.Client
		env              *testsuite.TestWorkflowEnvironment
	}

	type args struct {
		ctx          context.Context
		siteID       uuid.UUID
		vpcInventory *cwssaws.VPCInventory
	}

	tests := []struct {
		name                              string
		fields                            fields
		args                              args
		updatedVpc                        *cdbm.Vpc
		readyVpcs                         []*cdbm.Vpc
		deletedVpcs                       []*cdbm.Vpc
		missingVpcs                       []*cdbm.Vpc
		restoredVpcs                      []*cdbm.Vpc
		unpairedVpcs                      []*cdbm.Vpc
		nsgPropagationDetailsClearedVpcs  []*cdbm.Vpc
		networkVirtualizationUpdatedVpcs  []*cdbm.Vpc
		ethernetVirtualizationUpdatedVpcs []*cdbm.Vpc
		routingProfileUpdatedVpcs         []*cdbm.Vpc
		routingProfileClearedVpcs         []*cdbm.Vpc
		readyStatusDetailVpcs             []*cdbm.Vpc
		requiredMetadataUpdate            bool
		metadataVpcUpdate                 *cdbm.Vpc
		wantErr                           bool
	}{
		{
			name: "test VPC inventory processing error, non-existent Site",
			fields: fields{
				dbSession:        dbSession,
				siteClientPool:   tSiteClientPool,
				clientPoolClient: mtc1,
				env:              env,
			},
			args: args{
				ctx:    ctx,
				siteID: uuid.New(),
				vpcInventory: &cwssaws.VPCInventory{
					Vpcs: []*cwssaws.Vpc{},
				},
			},
			wantErr: true,
		},
		{
			name: "test VPC inventory processing success",
			fields: fields{
				dbSession:        dbSession,
				siteClientPool:   tSiteClientPool,
				clientPoolClient: mtc1,
				env:              env,
			},
			args: args{
				ctx:    ctx,
				siteID: st.ID,
				vpcInventory: &cwssaws.VPCInventory{
					NetworkSecurityGroupPropagations: []*cwssaws.NetworkSecurityGroupPropagationObjectStatus{
						&cwssaws.NetworkSecurityGroupPropagationObjectStatus{
							Id:      vpc1.ID.String(),
							Status:  cwssaws.NetworkSecurityGroupPropagationStatus_NSG_PROP_STATUS_FULL,
							Details: db.GetStrPtr("nothing to see here"),
						},
					},
					Vpcs: []*cwssaws.Vpc{
						{
							Id:                        &cwssaws.VpcId{Value: vpc1.ID.String()},
							Name:                      vpc1.ID.String(),
							NetworkVirtualizationType: &nwvt,
						},
						{
							Id:   &cwssaws.VpcId{Value: vpc2.ControllerVpcID.String()},
							Name: vpc2.ID.String(),
						},
						{
							Id:   &cwssaws.VpcId{Value: vpc3.ControllerVpcID.String()},
							Name: vpc3.ID.String(),
						},
						{
							Id:   &cwssaws.VpcId{Value: vpc4.ControllerVpcID.String()},
							Name: vpc4.ID.String(),
						},
						{
							Id:   &cwssaws.VpcId{Value: vpc8.ControllerVpcID.String()},
							Name: vpc8.ID.String(),
						},
						{
							Id:   &cwssaws.VpcId{Value: uuid.NewString()},
							Name: vpc9.ID.String(),
						},
						{
							Id:   &cwssaws.VpcId{Value: uuid.NewString()},
							Name: vpc10.ID.String(),
						},
						{
							Id:                        &cwssaws.VpcId{Value: vpc12.ControllerVpcID.String()},
							Name:                      vpc12.ID.String(),
							NetworkVirtualizationType: &evt,
						},
						{
							Id:                        &cwssaws.VpcId{Value: vpc13.ControllerVpcID.String()},
							Name:                      vpc13.ID.String(),
							NetworkVirtualizationType: &evt,
						},
					},
				},
			},
			updatedVpc:                        vpc1,
			nsgPropagationDetailsClearedVpcs:  []*cdbm.Vpc{vpc12},
			networkVirtualizationUpdatedVpcs:  []*cdbm.Vpc{vpc1},
			ethernetVirtualizationUpdatedVpcs: []*cdbm.Vpc{vpc12, vpc13},
			deletedVpcs:                       []*cdbm.Vpc{vpc5, vpc6},
			missingVpcs:                       []*cdbm.Vpc{vpc7, vpc11},
			restoredVpcs:                      []*cdbm.Vpc{vpc8},
			unpairedVpcs:                      []*cdbm.Vpc{vpc9, vpc10},
			readyStatusDetailVpcs:             []*cdbm.Vpc{vpc1},
			wantErr:                           false,
		},
		{
			name: "test paged VPC inventory processing, empty inventory",
			fields: fields{
				dbSession:        dbSession,
				siteClientPool:   tSiteClientPool,
				clientPoolClient: mtc1,
				env:              env,
			},
			args: args{
				ctx:    ctx,
				siteID: st2.ID,
				vpcInventory: &cwssaws.VPCInventory{
					Vpcs:            []*cwssaws.Vpc{},
					Timestamp:       timestamppb.Now(),
					InventoryStatus: cwssaws.InventoryStatus_INVENTORY_STATUS_SUCCESS,
					InventoryPage: &cwssaws.InventoryPage{
						CurrentPage: 1,
						TotalPages:  0,
						PageSize:    25,
						TotalItems:  0,
						ItemIds:     []string{},
					},
				},
			},
		},
		{
			name: "test paged Instance inventory processing, first page",
			fields: fields{
				dbSession:        dbSession,
				siteClientPool:   tSiteClientPool,
				clientPoolClient: mtc1,
				env:              env,
			},
			args: args{
				ctx:    ctx,
				siteID: st3.ID,
				vpcInventory: &cwssaws.VPCInventory{
					Vpcs:      pagedCtrlVpcs[0:10],
					Timestamp: timestamppb.Now(),
					InventoryPage: &cwssaws.InventoryPage{
						CurrentPage: 1,
						TotalPages:  4,
						PageSize:    10,
						TotalItems:  34,
						ItemIds:     pagedInvIds[0:34],
					},
				},
			},
			readyVpcs: pagedVpcs[0:34],
		},
		{
			name: "test paged Instance inventory processing, last page",
			fields: fields{
				dbSession:        dbSession,
				siteClientPool:   tSiteClientPool,
				clientPoolClient: mtc1,
				env:              env,
			},
			args: args{
				ctx:    ctx,
				siteID: st3.ID,
				vpcInventory: &cwssaws.VPCInventory{
					Vpcs:      pagedCtrlVpcs[30:34],
					Timestamp: timestamppb.Now(),
					InventoryPage: &cwssaws.InventoryPage{
						CurrentPage: 4,
						TotalPages:  4,
						PageSize:    10,
						TotalItems:  34,
						ItemIds:     pagedInvIds[0:34],
					},
				},
			},
			readyVpcs:   pagedVpcs[0:34],
			missingVpcs: pagedVpcs[34:38],
		},
		{
			name: "test paged Instance inventory processing, initiate update VPC metadata workflow",
			fields: fields{
				dbSession:        dbSession,
				siteClientPool:   tSiteClientPool,
				clientPoolClient: mtc1,
				env:              env,
			},
			args: args{
				ctx:    ctx,
				siteID: st3.ID,
				vpcInventory: &cwssaws.VPCInventory{
					Vpcs:      pagedCtrlVpcs[0:10],
					Timestamp: timestamppb.Now(),
					InventoryPage: &cwssaws.InventoryPage{
						CurrentPage: 1,
						TotalPages:  4,
						PageSize:    10,
						TotalItems:  34,
						ItemIds:     pagedInvIds[0:34],
					},
				},
			},
			readyVpcs:              pagedVpcs[0:34],
			requiredMetadataUpdate: true,
			metadataVpcUpdate:      pagedVpcs[1],
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mv := ManageVpc{
				dbSession:      tt.fields.dbSession,
				siteClientPool: tt.fields.siteClientPool,
			}

			mv.siteClientPool.IDClientMap[tt.args.siteID.String()] = tt.fields.clientPoolClient

			_, err := mv.UpdateVpcsInDB(tt.args.ctx, tt.args.siteID, tt.args.vpcInventory)
			assert.Equal(t, tt.wantErr, err != nil)

			if tt.wantErr {
				return
			}

			vpcDAO := cdbm.NewVpcDAO(dbSession)

			for _, vpcPropStatus := range tt.args.vpcInventory.NetworkSecurityGroupPropagations {
				updatedVPC, _ := vpcDAO.GetByID(ctx, nil, uuid.MustParse(vpcPropStatus.Id), nil)

				// Prop details should not be nil
				assert.NotNil(t, updatedVPC.NetworkSecurityGroupPropagationDetails)

				// The details should match
				assert.Equal(
					t,
					updatedVPC.NetworkSecurityGroupPropagationDetails.NetworkSecurityGroupPropagationObjectStatus,
					vpcPropStatus,
					"\n%+v \n != \n %+v\n",
					updatedVPC.NetworkSecurityGroupPropagationDetails.NetworkSecurityGroupPropagationObjectStatus,
					vpcPropStatus,
				)
			}

			for _, vpc := range tt.nsgPropagationDetailsClearedVpcs {
				// If the VPC should not have propagation details according to the site
				// make sure the DB agrees.
				updatedVPC, _ := vpcDAO.GetByID(ctx, nil, vpc.ID, nil)
				assert.Nil(t, updatedVPC.NetworkSecurityGroupPropagationDetails)
			}

			// Check that VPC status was updated in DB for VPC1
			if tt.updatedVpc != nil {
				updatedVPC, _ := vpcDAO.GetByID(ctx, nil, tt.updatedVpc.ID, nil)
				assert.Equal(t, cdbm.VpcStatusReady, updatedVPC.Status)
			}

			for _, vpc := range tt.networkVirtualizationUpdatedVpcs {
				updatedNetworkVirtVPC, _ := vpcDAO.GetByID(ctx, nil, vpc.ID, nil)
				assert.Equal(t, nwvt.String(), *updatedNetworkVirtVPC.NetworkVirtualizationType)
			}

			for _, vpc := range tt.ethernetVirtualizationUpdatedVpcs {
				updatedEthernetVirtVPC, _ := vpcDAO.GetByID(ctx, nil, vpc.ID, nil)
				assert.Equal(t, evt.String(), *updatedEthernetVirtVPC.NetworkVirtualizationType)
			}

			for _, vpc := range tt.readyVpcs {
				rv, _ := vpcDAO.GetByID(ctx, nil, vpc.ID, nil)
				assert.False(t, rv.IsMissingOnSite)
				assert.Equal(t, cdbm.VpcStatusReady, rv.Status)
			}

			for _, vpc := range tt.deletedVpcs {
				_, err = vpcDAO.GetByID(ctx, nil, vpc.ID, nil)
				require.Equal(t, cdb.ErrDoesNotExist, err, fmt.Sprintf("VPC %s should have been deleted", vpc.Name))
			}

			for _, vpc := range tt.missingVpcs {
				uv, _ := vpcDAO.GetByID(ctx, nil, vpc.ID, nil)

				if uv.ControllerVpcID != nil {
					assert.True(t, uv.IsMissingOnSite)
					assert.Equal(t, cdbm.VpcStatusError, uv.Status)
				} else {
					assert.False(t, uv.IsMissingOnSite)
				}
			}

			for _, vpc := range tt.unpairedVpcs {
				uv, _ := vpcDAO.GetByID(ctx, nil, vpc.ID, nil)
				assert.NotNil(t, uv.ControllerVpcID)
				if vpc.Status != cdbm.VpcStatusDeleting {
					assert.Equal(t, cdbm.VpcStatusReady, uv.Status)
				}
			}

			for _, vpc := range tt.restoredVpcs {
				rv, _ := vpcDAO.GetByID(ctx, nil, vpc.ID, nil)
				assert.False(t, rv.IsMissingOnSite)
				assert.Equal(t, cdbm.VpcStatusReady, rv.Status)
			}

			if tt.requiredMetadataUpdate {
				assert.True(t, len(tt.fields.clientPoolClient.Calls) > 0)
				assert.Equal(t, len(tt.fields.clientPoolClient.Calls[0].Arguments), 4)

				scReq := tt.fields.clientPoolClient.Calls[0].Arguments[3].(*cwssaws.VpcUpdateRequest)
				assert.Equal(t, tt.metadataVpcUpdate.ID.String(), scReq.Id.Value)
			}

			statusDetailDAO := cdbm.NewStatusDetailDAO(dbSession)
			for _, vpc := range tt.readyStatusDetailVpcs {
				statusDetails, _, err := statusDetailDAO.GetAllByEntityID(ctx, nil, vpc.ID.String(), nil, nil, nil)
				require.NoError(t, err)
				require.Len(t, statusDetails, 1)
				assert.Equal(t, cdbm.VpcStatusReady, statusDetails[0].Status)
				require.NotNil(t, statusDetails[0].Message)
				assert.Equal(t, "VPC is ready for use", *statusDetails[0].Message)
			}
		})
	}
}

func TestNewManageVpc(t *testing.T) {
	type args struct {
		dbSession      *cdb.Session
		siteClientPool *sc.ClientPool
		tc             client.Client
	}

	dbSession := &cdb.Session{}
	keyPath, certPath := config.SetupTestCerts(t)
	defer os.Remove(keyPath)
	defer os.Remove(certPath)

	cfg := config.NewConfig()
	cfg.SetTemporalCertPath(certPath)
	cfg.SetTemporalKeyPath(keyPath)
	cfg.SetTemporalCaPath(certPath)
	tcfg, err := cfg.GetTemporalConfig()
	assert.NoError(t, err)
	scp := sc.NewClientPool(tcfg)

	wtc := &tmocks.Client{}

	tests := []struct {
		name string
		args args
		want ManageVpc
	}{
		{
			name: "test new ManageVpc instantiation",
			args: args{
				dbSession:      dbSession,
				siteClientPool: scp,
				tc:             wtc,
			},
			want: ManageVpc{
				dbSession:      dbSession,
				siteClientPool: scp,
				tc:             wtc,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := NewManageVpc(tt.args.dbSession, tt.args.siteClientPool, tt.args.tc); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("NewManageVpc() = %v, want %v", got, tt.want)
			}
		})
	}
}

// Test VPC Metrics - DELETE operations
func Test_VpcMetrics_Delete_DeletingOnly(t *testing.T) {
	// Case 1: deleting (should emit metric with duration now-t1)
	dbSession := util.TestInitDB(t)
	defer dbSession.Close()
	util.TestSetupSchema(t, dbSession)

	site := util.TestSetupSite(t, dbSession)
	reg := prometheus.NewRegistry()
	lifecycleMetrics := NewManageVpcLifecycleMetrics(reg, dbSession)
	testVpcID := uuid.New()

	// Set precise timestamps
	baseTime := time.Now().Add(-1 * time.Hour)
	t1 := baseTime                                     // deleting started
	deleteTime := baseTime.Add(200 * time.Millisecond) // delete happened 200ms later
	expectedDuration := deleteTime.Sub(t1)

	// t1: deleting
	util.TestBuildStatusDetailWithTime(t, dbSession, testVpcID.String(), cdbm.VpcStatusDeleting, nil, t1)

	// Process delete event
	ctx := context.Background()
	err := lifecycleMetrics.RecordVpcStatusTransitionMetrics(ctx, site.ID, []cwm.InventoryObjectLifecycleEvent{
		{ObjectID: testVpcID, Deleted: &deleteTime},
	})
	assert.NoError(t, err)

	// Verify metric was emitted with correct duration (200ms)
	util.TestAssertMetricExistsTimes(t, reg, "cloud_workflow_vpc_operation_latency_seconds", 1, map[string]string{
		"operation_type": "delete",
		"from_status":    cdbm.VpcStatusDeleting,
		"to_status":      "Deleted",
	}, expectedDuration)
}

func Test_VpcMetrics_Delete_MultipleDeleting(t *testing.T) {
	// Case 2: deleting -> deleting -> deleting (should emit metric with duration now-t1)
	dbSession := util.TestInitDB(t)
	defer dbSession.Close()
	util.TestSetupSchema(t, dbSession)

	site := util.TestSetupSite(t, dbSession)
	reg := prometheus.NewRegistry()
	lifecycleMetrics := NewManageVpcLifecycleMetrics(reg, dbSession)
	testVpcID := uuid.New()

	// Set precise timestamps
	baseTime := time.Now().Add(-1 * time.Hour)
	t1 := baseTime                                     // first deleting
	t2 := baseTime.Add(50 * time.Millisecond)          // second deleting
	t3 := baseTime.Add(100 * time.Millisecond)         // third deleting
	deleteTime := baseTime.Add(300 * time.Millisecond) // delete happened
	expectedDuration := deleteTime.Sub(t1)             // should use first deleting timestamp

	// t1: deleting
	util.TestBuildStatusDetailWithTime(t, dbSession, testVpcID.String(), cdbm.VpcStatusDeleting, nil, t1)

	// t2: deleting
	util.TestBuildStatusDetailWithTime(t, dbSession, testVpcID.String(), cdbm.VpcStatusDeleting, nil, t2)

	// t3: deleting
	util.TestBuildStatusDetailWithTime(t, dbSession, testVpcID.String(), cdbm.VpcStatusDeleting, nil, t3)

	// Process delete event
	ctx := context.Background()
	err := lifecycleMetrics.RecordVpcStatusTransitionMetrics(ctx, site.ID, []cwm.InventoryObjectLifecycleEvent{
		{ObjectID: testVpcID, Deleted: &deleteTime},
	})
	assert.NoError(t, err)

	// Verify metric was emitted (should use first deleting timestamp, duration 300ms)
	util.TestAssertMetricExistsTimes(t, reg, "cloud_workflow_vpc_operation_latency_seconds", 1, map[string]string{
		"operation_type": "delete",
		"from_status":    cdbm.VpcStatusDeleting,
		"to_status":      "Deleted",
	}, expectedDuration)
}

func Test_VpcMetrics_Delete_NoDeleting(t *testing.T) {
	// Case 3: ready (no deleting, should NOT emit metric)
	dbSession := util.TestInitDB(t)
	defer dbSession.Close()
	util.TestSetupSchema(t, dbSession)

	site := util.TestSetupSite(t, dbSession)
	reg := prometheus.NewRegistry()
	lifecycleMetrics := NewManageVpcLifecycleMetrics(reg, dbSession)
	testVpcID := uuid.New()

	// Set precise timestamps
	baseTime := time.Now().Add(-1 * time.Hour)
	t1 := baseTime
	deleteTime := baseTime.Add(100 * time.Millisecond)

	// t1: ready (no deleting status)
	util.TestBuildStatusDetailWithTime(t, dbSession, testVpcID.String(), cdbm.VpcStatusReady, nil, t1)

	// Process delete event
	ctx := context.Background()
	err := lifecycleMetrics.RecordVpcStatusTransitionMetrics(ctx, site.ID, []cwm.InventoryObjectLifecycleEvent{
		{ObjectID: testVpcID, Deleted: &deleteTime},
	})
	assert.NoError(t, err)

	// Verify NO metric was emitted (no deleting status found)
	util.TestAssertMetricExistsTimes(t, reg, "cloud_workflow_vpc_operation_latency_seconds", 0, nil, 0)
}
