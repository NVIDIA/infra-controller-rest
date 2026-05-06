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

package model

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/NVIDIA/infra-controller-rest/api/pkg/api/model/util"
	cdb "github.com/NVIDIA/infra-controller-rest/db/pkg/db"
	cdbm "github.com/NVIDIA/infra-controller-rest/db/pkg/db/model"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
)

func TestAPIVpcCreateRequest_Validate(t *testing.T) {
	type fields struct {
		Name                      string
		Description               *string
		SiteID                    string
		NetworkVirtualizationType *string
		Labels                    map[string]string
		Vni                       *int
		RoutingProfile            *string
	}
	tests := []struct {
		name    string
		fields  fields
		wantErr bool
	}{
		{
			name: "test valid VPC create request",
			fields: fields{
				Name:        "test-name",
				Description: cdb.GetStrPtr("Test description"),
				SiteID:      uuid.NewString(),
			},
			wantErr: false,
		},
		{
			name: "test valid VPC create request - invalid names are specified names exceeded 256 char",
			fields: fields{
				Name:        "apvhhigcgctlgiwtbrgldkegmnwuqcibutndlholygxvhzrpinziepszvpmopvzkybykrwgvzojtssorabkrnawgjzeuuerphsnecipubeuzrpewkfuvwoeybagaxpvjvzvbzqznyfmcpbxrhbdkhewiepykfjeejeqatswgrlhqkgnvwqmatejufnsjgelcugcoccybywdrnlyvsegsegorygwdvurgktpuzyrsoutspsnyzynliaxwseazqmimp",
				Description: cdb.GetStrPtr("Test description"),
				SiteID:      uuid.NewString(),
			},
			wantErr: true,
		},
		{
			name: "test invalid VPC create request - invalid Site ID",
			fields: fields{
				Name:   "test-name",
				SiteID: "invalid-uuid",
			},
			wantErr: true,
		},
		{
			name: "test invalid VPC create request - invalid Network Virtualization Type",
			fields: fields{
				Name:                      "test-name",
				Description:               cdb.GetStrPtr("Test description"),
				SiteID:                    uuid.NewString(),
				NetworkVirtualizationType: cdb.GetStrPtr("VPC"),
			},
			wantErr: true,
		},
		{
			name: "test valid VPC create request - valid labels are specified",
			fields: fields{
				Name:   "test-name",
				SiteID: uuid.NewString(),
				Labels: map[string]string{
					"name":        "a-nv100-VPC",
					"description": "",
				},
			},
			wantErr: false,
		},
		{
			name: "test valid VPC create request - routing profile for FNN",
			fields: fields{
				Name:                      "test-name",
				SiteID:                    uuid.NewString(),
				NetworkVirtualizationType: cdb.GetStrPtr(cdbm.VpcFNN),
				RoutingProfile:            cdb.GetStrPtr(APIVpcRoutingProfileInternal),
			},
			wantErr: false,
		},
		{
			name: "test invalid VPC create request - routing profile on non-FNN VPC",
			fields: fields{
				Name:                      "test-name",
				SiteID:                    uuid.NewString(),
				NetworkVirtualizationType: cdb.GetStrPtr(cdbm.VpcEthernetVirtualizer),
				RoutingProfile:            cdb.GetStrPtr(APIVpcRoutingProfileInternal),
			},
			wantErr: true,
		},
		{
			name: "test valid VPC create request - routing profile when network virtualization type is omitted",
			fields: fields{
				Name:           "test-name",
				SiteID:         uuid.NewString(),
				RoutingProfile: cdb.GetStrPtr(APIVpcRoutingProfileExternal),
			},
			wantErr: false,
		},
		{
			name: "test invalid VPC create request - routing profile too short",
			fields: fields{
				Name:                      "test-name",
				SiteID:                    uuid.NewString(),
				NetworkVirtualizationType: cdb.GetStrPtr(cdbm.VpcFNN),
				RoutingProfile:            cdb.GetStrPtr("ab"),
			},
			wantErr: true,
		},
		{
			name: "test invalid VPC create request - routing profile starts with non-letter",
			fields: fields{
				Name:                      "test-name",
				SiteID:                    uuid.NewString(),
				NetworkVirtualizationType: cdb.GetStrPtr(cdbm.VpcFNN),
				RoutingProfile:            cdb.GetStrPtr("1internal"),
			},
			wantErr: true,
		},
		{
			name: "test invalid VPC create request - routing profile contains underscore",
			fields: fields{
				Name:                      "test-name",
				SiteID:                    uuid.NewString(),
				NetworkVirtualizationType: cdb.GetStrPtr(cdbm.VpcFNN),
				RoutingProfile:            cdb.GetStrPtr("privileged_internal"),
			},
			wantErr: true,
		},
		{
			name: "test invalid VPC create request - routing profile is unsupported",
			fields: fields{
				Name:                      "test-name",
				SiteID:                    uuid.NewString(),
				NetworkVirtualizationType: cdb.GetStrPtr(cdbm.VpcFNN),
				RoutingProfile:            cdb.GetStrPtr("tenant-edge"),
			},
			wantErr: true,
		},
		{
			name: "test invalid VPC create request - invalid VNI",
			fields: fields{
				Name:   "test-name",
				SiteID: uuid.NewString(),
				Vni:    cdb.GetIntPtr(70000),
			},
			wantErr: true,
		},
		{
			name: "test valid VPC create request - invalid labels are specified key is empty",
			fields: fields{
				Name:   "test-name",
				SiteID: uuid.NewString(),
				Labels: map[string]string{
					"name": "a-nv200=VPC",
					"":     "test",
				},
			},
			wantErr: true,
		},
		{
			name: "test valid VPC create request - invalid labels are specified both key and value are empty",
			fields: fields{
				Name:   "test-name",
				SiteID: uuid.NewString(),
				Labels: map[string]string{
					"name": "a-nv300=VPC",
					"":     "",
				},
			},
			wantErr: true,
		},
		{
			name: "test valid VPC create request - invalid labels are specified key has char more than 256",
			fields: fields{
				Name:   "test-name",
				SiteID: uuid.NewString(),
				Labels: map[string]string{
					"ygsV9MoUjep1rCwbQskkF9wfMolE3oDTCcxuYSJCx9TLKepCIku9pnHfIkxCxHkb7ucbsBL4hyLqQaHoEqpTBmfoX4Un7sGvQdHGZ7nb68JJEJ3ocFAtyCMCBt66z3ldnTqp8SXXOIhNsOh35MLYQjI8557Pu6o91TsEBqyTz0yz68HHmfNgJoreHpXfeujq4cpElUXXbQ3xfFICkNyghXgFZ0MLs2o0u1Nd29aB113X5g3FKJBCskW6eBULNmeFFG61DMM37q": "a-nv300=VPC",
				},
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			vcr := APIVpcCreateRequest{
				Name:                      tt.fields.Name,
				Description:               tt.fields.Description,
				SiteID:                    tt.fields.SiteID,
				NetworkVirtualizationType: tt.fields.NetworkVirtualizationType,
				Labels:                    tt.fields.Labels,
				Vni:                       tt.fields.Vni,
				RoutingProfile:            tt.fields.RoutingProfile,
			}

			if err := vcr.Validate(); (err != nil) != tt.wantErr {
				marshalledErr, _ := json.Marshal(err)
				t.Errorf("APIVpcCreateRequest.Validate() error = %v, wantErr %v", string(marshalledErr), tt.wantErr)
			}
		})
	}
}

func TestAPIVpcUpdateRequest_Validate(t *testing.T) {
	type fields struct {
		Name        string
		Description *string
		Labels      map[string]string
	}
	tests := []struct {
		name    string
		fields  fields
		wantErr bool
	}{
		{
			name: "test valid VPC update request",
			fields: fields{
				Name:        "test-name",
				Description: cdb.GetStrPtr("Test description"),
			},
			wantErr: false,
		},
		{
			name: "test valid VPC update request - invalid names are specified names exceeded 256 char",
			fields: fields{
				Name:        "apvhhigcgctlgiwtbrgldkegmnwuqcibutndlholygxvhzrpinziepszvpmopvzkybykrwgvzojtssorabkrnawgjzeuuerphsnecipubeuzrpewkfuvwoeybagaxpvjvzvbzqznyfmcpbxrhbdkhewiepykfjeejeqatswgrlhqkgnvwqmatejufnsjgelcugcoccybywdrnlyvsegsegorygwdvurgktpuzyrsoutspsnyzynliaxwseazqmimp",
				Description: cdb.GetStrPtr("Test description"),
			},
			wantErr: true,
		},
		{
			name: "test valid VPC update request - valid labels are specified",
			fields: fields{
				Name: "test-name",
				Labels: map[string]string{
					"name":        "a-nv100-VPC",
					"description": "",
				},
			},
			wantErr: false,
		},
		{
			name: "test valid VPC update request - invalid labels are specified key is empty",
			fields: fields{
				Name: "test-name",
				Labels: map[string]string{
					"name": "a-nv200=VPC",
					"":     "test",
				},
			},
			wantErr: true,
		},
		{
			name: "test valid VPC update request - invalid labels are specified both key and value are empty",
			fields: fields{
				Name: "test-name",
				Labels: map[string]string{
					"name": "a-nv300=VPC",
					"":     "",
				},
			},
			wantErr: true,
		},
		{
			name: "test valid VPC update request - invalid labels are specified key has char more than 128",
			fields: fields{
				Name: "test-name",
				Labels: map[string]string{
					"morethan128charmorethan128charmorethan128charmorethan128charmorethan128charmorethan128charmorethan128charmorethan128charmorethan128charmorethan128charmorethan128charmorethan128char": "a-nv300=VPC",
					"": "",
				},
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			vur := APIVpcUpdateRequest{
				Name:        &tt.fields.Name,
				Description: tt.fields.Description,
				Labels:      tt.fields.Labels,
			}

			if err := vur.Validate(); (err != nil) != tt.wantErr {
				marshalledErr, _ := json.Marshal(err)
				t.Errorf("APIVpcUpdateRequest.Validate() error = %v, wantErr %v", string(marshalledErr), tt.wantErr)
			}
		})
	}
}

func TestAPIVpcVirtualizationUpdateRequest_Validate(t *testing.T) {
	vpcObj1 := &cdbm.Vpc{
		ID:                        uuid.New(),
		Name:                      "test",
		Org:                       "test",
		SiteID:                    uuid.New(),
		TenantID:                  uuid.New(),
		InfrastructureProviderID:  uuid.New(),
		NetworkVirtualizationType: cdb.GetStrPtr("ETHERNET_VIRTUALIZER"),
		Created:                   cdb.GetCurTime(),
		Updated:                   cdb.GetCurTime(),
	}

	vpcObj2 := &cdbm.Vpc{
		ID:                        uuid.New(),
		Name:                      "test1",
		Org:                       "test1",
		SiteID:                    uuid.New(),
		TenantID:                  uuid.New(),
		InfrastructureProviderID:  uuid.New(),
		NetworkVirtualizationType: cdb.GetStrPtr("FNN"),
		Created:                   cdb.GetCurTime(),
		Updated:                   cdb.GetCurTime(),
	}

	type fields struct {
		NetworkVirtualizationType string
		inputVpc                  *cdbm.Vpc
	}
	tests := []struct {
		name    string
		fields  fields
		wantErr bool
	}{
		{
			name: "test valid VPC virtualization update request",
			fields: fields{
				NetworkVirtualizationType: "FNN",
				inputVpc:                  vpcObj1,
			},
			wantErr: false,
		},
		{
			name: "test invalid VPC virtualization update request - support only FNN",
			fields: fields{
				NetworkVirtualizationType: "ETHERNET_VIRTUALIZER",
				inputVpc:                  vpcObj1,
			},
			wantErr: true,
		},
		{
			name: "test invalid VPC virtualization update request - existing vpc already FNN",
			fields: fields{
				NetworkVirtualizationType: "FNN",
				inputVpc:                  vpcObj2,
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			vvur := APIVpcVirtualizationUpdateRequest{
				NetworkVirtualizationType: tt.fields.NetworkVirtualizationType,
			}

			if err := vvur.Validate(tt.fields.inputVpc); (err != nil) != tt.wantErr {
				marshalledErr, _ := json.Marshal(err)
				t.Errorf("APIVpcVirtualizationUpdateRequest.Validate() error = %v, wantErr %v", string(marshalledErr), tt.wantErr)
			}
		})
	}
}

func TestNewAPIVpc(t *testing.T) {
	type args struct {
		dbVpc cdbm.Vpc
		dbsds []cdbm.StatusDetail
	}

	dbVpc := cdbm.Vpc{
		ID:                        uuid.New(),
		Name:                      "test-vpc",
		Description:               cdb.GetStrPtr("Test VPC Description"),
		Org:                       "test-org",
		TenantID:                  uuid.New(),
		SiteID:                    uuid.New(),
		NetworkVirtualizationType: cdb.GetStrPtr(cdbm.VpcEthernetVirtualizer),
		RoutingProfile:            cdb.GetStrPtr(apiVpcRoutingProfileSiteInternal),
		ControllerVpcID:           cdb.GetUUIDPtr(uuid.New()),
		// The normal expectation is that Vni and ActiveVni match or
		// that Vni is simply null, but we want to test for correctness
		// in the conversion from the record in the DB and the API struct.
		Vni:       cdb.GetIntPtr(555),
		ActiveVni: cdb.GetIntPtr(777),
		Labels: map[string]string{
			"zone": "1",
			"west": "2",
		},
		Status:  cdbm.SiteStatusPending,
		Created: time.Now(),
		Updated: time.Now(),
	}

	dbsds := []cdbm.StatusDetail{
		{
			ID:      uuid.New(),
			Status:  cdbm.SiteStatusPending,
			Created: time.Now(),
			Updated: time.Now(),
		},
	}

	apidbsh := []APIStatusDetail{}
	for _, dbsd := range dbsds {
		apidbsh = append(apidbsh, NewAPIStatusDetail(dbsd))
	}

	tests := []struct {
		name string
		args args
		want APIVpc
	}{
		{
			name: "get new APIVpc returns stored routing profile",
			args: args{
				dbVpc: dbVpc,
				dbsds: dbsds,
			},
			want: APIVpc{
				ID:                        dbVpc.ID.String(),
				Name:                      dbVpc.Name,
				Description:               dbVpc.Description,
				Org:                       dbVpc.Org,
				InfrastructureProviderID:  util.GetUUIDPtrToStrPtr(&dbVpc.InfrastructureProviderID),
				TenantID:                  util.GetUUIDPtrToStrPtr(&dbVpc.TenantID),
				SiteID:                    util.GetUUIDPtrToStrPtr(&dbVpc.SiteID),
				NetworkVirtualizationType: dbVpc.NetworkVirtualizationType,
				RoutingProfile:            cdb.GetStrPtr(APIVpcRoutingProfileInternal),
				ControllerVpcID:           util.GetUUIDPtrToStrPtr(dbVpc.ControllerVpcID),
				RequestedVni:              dbVpc.Vni,
				Vni:                       dbVpc.ActiveVni,
				Status:                    dbVpc.Status,
				Labels: map[string]string{
					"zone": "1",
					"west": "2",
				},
				StatusHistory: apidbsh,
				Created:       dbVpc.Created,
				Updated:       dbVpc.Updated,
			},
		},
		{
			name: "get new APIVpc includes routing profile for FNN VPC",
			args: args{
				dbVpc: func() cdbm.Vpc {
					fnnVpc := dbVpc
					fnnVpc.NetworkVirtualizationType = cdb.GetStrPtr(cdbm.VpcFNN)
					return fnnVpc
				}(),
				dbsds: dbsds,
			},
			want: APIVpc{
				ID:                        dbVpc.ID.String(),
				Name:                      dbVpc.Name,
				Description:               dbVpc.Description,
				Org:                       dbVpc.Org,
				InfrastructureProviderID:  util.GetUUIDPtrToStrPtr(&dbVpc.InfrastructureProviderID),
				TenantID:                  util.GetUUIDPtrToStrPtr(&dbVpc.TenantID),
				SiteID:                    util.GetUUIDPtrToStrPtr(&dbVpc.SiteID),
				NetworkVirtualizationType: cdb.GetStrPtr(cdbm.VpcFNN),
				RoutingProfile:            cdb.GetStrPtr(APIVpcRoutingProfileInternal),
				ControllerVpcID:           util.GetUUIDPtrToStrPtr(dbVpc.ControllerVpcID),
				RequestedVni:              dbVpc.Vni,
				Vni:                       dbVpc.ActiveVni,
				Status:                    dbVpc.Status,
				Labels: map[string]string{
					"zone": "1",
					"west": "2",
				},
				StatusHistory: apidbsh,
				Created:       dbVpc.Created,
				Updated:       dbVpc.Updated,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NewAPIVpc(tt.args.dbVpc, tt.args.dbsds)

			assert.Equal(t, tt.want.ID, got.ID)
			assert.Equal(t, tt.want.Name, got.Name)
			assert.Equal(t, tt.want.Description, got.Description)
			assert.Equal(t, tt.want.Org, got.Org)
			assert.Equal(t, *tt.want.InfrastructureProviderID, *got.InfrastructureProviderID)
			assert.Equal(t, *tt.want.TenantID, *got.TenantID)
			assert.Equal(t, *tt.want.SiteID, *got.SiteID)
			assert.Equal(t, tt.want.NetworkVirtualizationType, got.NetworkVirtualizationType)
			assert.Equal(t, tt.want.RoutingProfile, got.RoutingProfile)
			assert.Equal(t, *tt.want.ControllerVpcID, *got.ControllerVpcID)
			if tt.want.Vni != nil {
				assert.NotNil(t, got.Vni)
				assert.Equal(t, *tt.want.Vni, *got.Vni)
			}
			if tt.want.RequestedVni != nil {
				assert.NotNil(t, got.RequestedVni)
				assert.Equal(t, *tt.want.RequestedVni, *got.RequestedVni)
			}
			assert.Equal(t, len(tt.want.Labels), len(got.Labels))
			assert.Equal(t, tt.want.Status, got.Status)
			assert.Equal(t, tt.want.StatusHistory, got.StatusHistory)
			assert.Equal(t, tt.want.Created, got.Created)
			assert.Equal(t, tt.want.Updated, got.Updated)
		})
	}
}
