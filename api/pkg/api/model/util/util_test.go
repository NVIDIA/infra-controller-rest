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

package util

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIPv4UsableHostAddrsFromCidr(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name       string
		cidr       string
		want       uint64
		wantErrMsg string
	}{
		{"/24", "192.168.0.0/24", 254, ""},
		{"/16", "10.0.0.0/16", 65534, ""},
		{"/32", "10.1.2.3/32", 1, ""},
		{"/31", "10.1.2.0/31", 2, ""},
		{"IPv6", "2001:db8::/64", 0, "usage stats support IPv4 only"},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := IPv4UsableHostAddrsFromCidr(tc.cidr)
			if tc.wantErrMsg != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.wantErrMsg)
				assert.Equal(t, uint64(0), got)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestSubnetUsageFromInterfaceCounts(t *testing.T) {
	t.Parallel()
	u, err := subnetUsageFromInterfaceCounts(254, 0, "192.168.0.0/24")
	require.NoError(t, err)
	assert.Equal(t, uint64(252), u.AvailableIPs)
	assert.Equal(t, uint64(2), u.AcquiredIPs)
	assert.Equal(t, uint64(0), u.AvailableSmallestPrefixes)
	assert.Equal(t, uint64(0), u.AcquiredPrefixes)
	require.Len(t, u.AvailablePrefixes, 10)
	assert.Equal(t, "192.168.0.3/32", u.AvailablePrefixes[0])

	u3, err := subnetUsageFromInterfaceCounts(254, 3, "192.168.0.0/24")
	require.NoError(t, err)
	assert.Equal(t, uint64(249), u3.AvailableIPs)
	assert.Equal(t, uint64(5), u3.AcquiredIPs)
	assert.Equal(t, uint64(3), u3.AcquiredPrefixes)
}

func TestVpcPrefixUsageFromInterfaceCounts(t *testing.T) {
	t.Parallel()
	u, err := vpcPrefixUsageFromInterfaceCounts(254, 1, "192.168.0.0/24")
	require.NoError(t, err)
	assert.Equal(t, uint64(252), u.AvailableIPs)
	assert.Equal(t, uint64(2), u.AcquiredIPs)
	assert.Equal(t, uint64(126), u.AvailableSmallestPrefixes)
	assert.Equal(t, uint64(1), u.AcquiredPrefixes)
	require.NotEmpty(t, u.AvailablePrefixes)
	assert.Equal(t, "192.168.0.2/31", u.AvailablePrefixes[0])

	u0, err := vpcPrefixUsageFromInterfaceCounts(254, 0, "192.168.0.0/24")
	require.NoError(t, err)
	assert.Equal(t, uint64(254), u0.AvailableIPs)
	assert.Equal(t, uint64(0), u0.AcquiredIPs)
	assert.Equal(t, uint64(127), u0.AvailableSmallestPrefixes)
	assert.Equal(t, "192.168.0.0/31", u0.AvailablePrefixes[0])
}
