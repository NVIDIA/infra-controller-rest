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
	"context"
	"errors"
	"fmt"
	"net/netip"
	"strings"

	cipam "github.com/NVIDIA/infra-controller-rest/ipam"
	"github.com/google/uuid"
	"github.com/uptrace/bun"
	"go4.org/netipx"

	cdbm "github.com/NVIDIA/infra-controller-rest/db/pkg/db/model"
	cwssaws "github.com/NVIDIA/infra-controller-rest/workflow-schema/schema/site-agent/workflows/v1"
	"gopkg.in/yaml.v3"
)

const (
	// configuration for phone home
	SitePhoneHomeName    = "phone_home"
	SitePhoneHomePost    = "POST"
	SitePhoneHomePostAll = "all"
	SitePhoneHomeUrl     = "url"
	SiteCloudConfig      = "#cloud-config"

	// maxInterfaceUsagePreviewPrefixes caps AvailablePrefixes samples for interface-derived Subnet/VPC Prefix usage.
	maxInterfaceUsagePreviewPrefixes = 10
)

// Walks through the yaml nodes looking for a cloud-init phone-home block
// If `url` is nil, then any phone-home block found will be removed.
// If `url` is non-nil, then the phone-home block will only be removed if
// if the URL matches the value of `url`
func RemovePhoneHomeFromUserData(documentRoot *yaml.Node, url *string) error {

	if documentRoot == nil || documentRoot.Kind != yaml.MappingNode {
		return fmt.Errorf("node must be non-nil MappingNode for user-data removal")
	}

	contentLen := len(documentRoot.Content)

	// If phone-home is being disabled, then delete
	// any phone-home data that might exist.
	// Go through the YAML nodes and look for our target.
	// We've previously determined that documentRoot is a
	// valid MappingNode, so the contents wil be pairs of nodes
	// representing key/value pairs of the map.
	//
	// Note there are no breaks or early returns because a user
	// could have submitted valid but nonsensical YAML with
	// multiple phone-home blocks.
	for i := 0; i < contentLen; i += 2 {
		mapKeyNode := documentRoot.Content[i]
		mapValueNode := documentRoot.Content[i+1]

		// No breaks or early-returns here because the user could have submitted
		// valid but nonsensical YAML that includes a phone-home block multiple times.
		if mapKeyNode.Kind == yaml.ScalarNode && mapKeyNode.Value == SitePhoneHomeName {
			// Check if the next node is a map, which will be the phone_home map itself.
			if mapValueNode.Kind == yaml.MappingNode {

				if url == nil {
					// Snip out the target while preserving the order of the nodes.
					// We have to snip out the key (phone_home) and the value
					// (the actual map node), so +2
					// We're working with pairs here, so the second slice-expression
					// won't violate bounds.
					documentRoot.Content = append(documentRoot.Content[:i], documentRoot.Content[i+2:]...)

					// Shift the "pointer" backwards since we
					// just modified documentRoot.Content "in-place"
					i -= 2

					// Reduce the loop limit since the
					// list being worked on is shorter now.
					contentLen = len(documentRoot.Content)
					continue
				}

				// Get the nodes in the map.
				phoneHomeMapSubNodes := mapValueNode.Content

				// Go through the map nodes and look for the URL key.
				// Again, MappingNode, so we can expect k/v node pairs.
				for j := 0; j < len(phoneHomeMapSubNodes); j += 2 {

					phoneHomeMapKeyNode := phoneHomeMapSubNodes[j]
					phoneHomeMapValueNode := phoneHomeMapSubNodes[j+1]
					if phoneHomeMapKeyNode.Kind == yaml.ScalarNode && phoneHomeMapKeyNode.Value == SitePhoneHomeUrl {
						if phoneHomeMapValueNode.Value == *url {
							documentRoot.Content = append(documentRoot.Content[:i], documentRoot.Content[i+2:]...)
							i -= 2
							contentLen = len(documentRoot.Content)
						}
					}
				}

			}
		}
	}

	return nil
}

func InsertPhoneHomeIntoUserData(documentRoot *yaml.Node, url string) error {
	if documentRoot == nil || documentRoot.Kind != yaml.MappingNode {
		return fmt.Errorf("node must be non-nil MappingNode for user-data insertion")
	}

	if documentRoot.Content == nil {
		documentRoot.Content = []*yaml.Node{}
	}

	// Remove any existing phone-home block found before we insert a new one.
	if err := RemovePhoneHomeFromUserData(documentRoot, nil); err != nil {
		return err
	}

	// Build the PhoneHome user-data section.
	phoneHomeConfigMap := map[string]string{}
	phoneHomeConfigMap[SitePhoneHomeUrl] = url
	phoneHomeConfigMap[SitePhoneHomePost] = SitePhoneHomePostAll

	// Encode it into a new YAML node so we can
	// add it to the root content later.
	phoneHomeValueNode := &yaml.Node{}
	if err := phoneHomeValueNode.Encode(phoneHomeConfigMap); err != nil {
		return errors.New("failed to insert phone-home into userData")
	}
	phoneHomeKeyNode := &yaml.Node{}
	phoneHomeKeyNode.SetString(SitePhoneHomeName)

	// Append the node that we can marshal it back out later.
	documentRoot.Content = append(documentRoot.Content, phoneHomeKeyNode, phoneHomeValueNode)

	// Ensure #cloud-config is present as a head comment
	foundCloudConfig := false
	for _, node := range documentRoot.Content {
		if node.HeadComment == SiteCloudConfig {
			foundCloudConfig = true
			break
		}
	}

	if !foundCloudConfig {
		if documentRoot.Kind == yaml.MappingNode {
			if documentRoot.HeadComment == "" {
				documentRoot.HeadComment = SiteCloudConfig
			}
		}
	}

	return nil
}

// ProtobufLabelsFromAPILabels converts API labels (map[string]string) to protobuf labels ([]*cwssaws.Label)
func ProtobufLabelsFromAPILabels(labels map[string]string) []*cwssaws.Label {
	if labels == nil {
		return nil
	}
	protoLabels := []*cwssaws.Label{}
	for k, v := range labels {
		protoLabels = append(protoLabels, &cwssaws.Label{
			Key:   k,
			Value: &v,
		})
	}
	return protoLabels
}

// ipv4UsableHostAddressesForPrefixBits returns assignable IPv4 host count for a prefix length (/32 and /31 special-cased).
func ipv4UsableHostAddressesForPrefixBits(prefixBits int) uint64 {
	if prefixBits < 0 || prefixBits > 32 {
		return 0
	}
	hostBits := 32 - prefixBits
	switch {
	case hostBits == 0:
		return 1
	case prefixBits == 31:
		return 2
	default:
		total := uint64(1) << uint(hostBits)
		if total >= 2 {
			return total - 2
		}
		return total
	}
}

// IPv4UsableHostAddrsFromCidr returns usable IPv4 host addresses for the given CIDR for interface/instance usage stats.
func IPv4UsableHostAddrsFromCidr(cidr string) (uint64, error) {
	p, err := netip.ParsePrefix(cidr)
	if err != nil {
		return 0, err
	}
	if !p.Addr().Is4() {
		return 0, fmt.Errorf("usage stats support IPv4 only, got %s", cidr)
	}
	return ipv4UsableHostAddressesForPrefixBits(int(p.Bits())), nil
}

func vpcPrefixIPv4Cidr(vp *cdbm.VpcPrefix) string {
	if vp == nil {
		return ""
	}
	if strings.Contains(vp.Prefix, "/") {
		return vp.Prefix
	}
	return fmt.Sprintf("%s/%d", vp.Prefix, vp.PrefixLength)
}

func subnetIPv4Cidr(sn *cdbm.Subnet) (string, error) {
	if sn == nil {
		return "", fmt.Errorf("subnet is nil")
	}
	if sn.IPv4Prefix == nil || *sn.IPv4Prefix == "" {
		return "", fmt.Errorf("subnet has no IPv4 prefix")
	}
	p := *sn.IPv4Prefix
	if strings.Contains(p, "/") {
		return p, nil
	}
	return fmt.Sprintf("%s/%d", p, sn.PrefixLength), nil
}

func subSatUint64(a, b uint64) uint64 {
	if a > b {
		return a - b
	}
	return 0
}

// countInterfacesForSubnetOrVpc counts Instance Ethernet interface rows (logical `interface` table; InfiniBand/NVLink use other tables).
func countInterfacesForSubnetOrVpc(ctx context.Context, db bun.IDB, subnetID *uuid.UUID, vpcPrefixID *uuid.UUID) (uint64, error) {
	var row struct {
		Iface int64 `bun:"iface_count"`
	}
	switch {
	case subnetID != nil:
		err := db.NewRaw(
			`SELECT count(*) AS iface_count FROM "interface" AS ifc WHERE ifc.subnet_id = ? AND ifc.deleted IS NULL`,
			*subnetID,
		).Scan(ctx, &row)
		if err != nil {
			return 0, err
		}
	case vpcPrefixID != nil:
		err := db.NewRaw(
			`SELECT count(*) AS iface_count FROM "interface" AS ifc WHERE ifc.vpc_prefix_id = ? AND ifc.deleted IS NULL`,
			*vpcPrefixID,
		).Scan(ctx, &row)
		if err != nil {
			return 0, err
		}
	default:
		return 0, fmt.Errorf("usage stats: need subnet or vpc prefix id")
	}
	return uint64(row.Iface), nil
}

func forEachIPv4UsableHost(p netip.Prefix, fn func(addr netip.Addr) bool) {
	ipp := p.Masked()
	bits := int(ipp.Bits())
	if bits < 0 || bits > 32 || !ipp.Addr().Is4() {
		return
	}
	hostBits := 32 - bits
	switch {
	case hostBits == 0:
		fn(ipp.Addr())
	case bits == 31:
		r := netipx.RangeOfPrefix(ipp)
		if !fn(r.From()) {
			return
		}
		fn(r.To())
	default:
		r := netipx.RangeOfPrefix(ipp)
		first := r.From().Next()
		last := r.To().Prev()
		for a := first; a.Compare(last) <= 0; a = a.Next() {
			if !fn(a) {
				return
			}
		}
	}
}

// firstIPv432HostPreview returns up to limit IPv4 /32 strings from usable hosts, skipping the first skipHosts usable addresses in enumeration order.
func firstIPv432HostPreview(cidr string, skipHosts uint64, limit int) ([]string, error) {
	if limit <= 0 {
		return nil, nil
	}
	p, err := netip.ParsePrefix(cidr)
	if err != nil {
		return nil, err
	}
	if !p.Addr().Is4() {
		return nil, nil
	}
	var out []string
	var idx uint64
	forEachIPv4UsableHost(p, func(addr netip.Addr) bool {
		if idx >= skipHosts {
			px, err := addr.Prefix(32)
			if err == nil {
				out = append(out, px.String())
			}
			if len(out) >= limit {
				return false
			}
		}
		idx++
		return true
	})
	return out, nil
}

// firstIPv431PrefixPreview returns up to limit IPv4 /31 prefixes inside parentCidr, skipping the first skipBlocks aligned /31 blocks.
func firstIPv431PrefixPreview(parentCidr string, skipBlocks uint64, limit int) ([]string, error) {
	if limit <= 0 {
		return nil, nil
	}
	p, err := netip.ParsePrefix(parentCidr)
	if err != nil {
		return nil, err
	}
	if !p.Addr().Is4() {
		return nil, nil
	}
	var out []string
	var blockIdx uint64
	first := netipx.RangeOfPrefix(p).From()
	for p.Contains(first) {
		second := first.Next()
		if !p.Contains(second) {
			break
		}
		if blockIdx >= skipBlocks {
			p31, err := first.Prefix(31)
			if err == nil {
				out = append(out, p31.String())
			}
			if len(out) >= limit {
				break
			}
		}
		blockIdx++
		first = second.Next()
	}
	return out, nil
}

func subnetUsageFromInterfaceCounts(totalUsableIPv4Hosts, ifaceCount uint64, cidr string) (*cipam.Usage, error) {
	acquiredIPs := ifaceCount + 2
	availableIPs := subSatUint64(totalUsableIPv4Hosts, ifaceCount+2)
	var prefixes []string
	if availableIPs > 0 {
		var err error
		prefixes, err = firstIPv432HostPreview(cidr, ifaceCount+2, maxInterfaceUsagePreviewPrefixes)
		if err != nil {
			return nil, err
		}
	}
	return &cipam.Usage{
		AvailableIPs:              availableIPs,
		AcquiredIPs:               acquiredIPs,
		AvailableSmallestPrefixes: 0,
		AvailablePrefixes:         prefixes,
		AcquiredPrefixes:          ifaceCount,
	}, nil
}

func vpcPrefixUsageFromInterfaceCounts(totalUsableIPv4Hosts, ifaceCount uint64, cidr string) (*cipam.Usage, error) {
	acquiredIPs := ifaceCount * 2
	availableIPs := subSatUint64(totalUsableIPv4Hosts, ifaceCount*2)
	availableSmallest := availableIPs / 2
	var prefixes []string
	if availableIPs > 0 {
		var err error
		prefixes, err = firstIPv431PrefixPreview(cidr, ifaceCount, maxInterfaceUsagePreviewPrefixes)
		if err != nil {
			return nil, err
		}
	}
	return &cipam.Usage{
		AvailableIPs:              availableIPs,
		AcquiredIPs:               acquiredIPs,
		AvailableSmallestPrefixes: availableSmallest,
		AvailablePrefixes:         prefixes,
		AcquiredPrefixes:          ifaceCount,
	}, nil
}

// GetInterfaceBasedUsageForVpcPrefix returns ipam.Usage-shaped stats from VPC prefix IPv4 size and
// Ethernet interface rows for this vpc_prefix_id (Core allocates instance IPs; not the IPAM subtree).
func GetInterfaceBasedUsageForVpcPrefix(ctx context.Context, db bun.IDB, vp *cdbm.VpcPrefix) (*cipam.Usage, error) {
	if vp == nil {
		return nil, fmt.Errorf("vpc prefix is nil")
	}
	cidr := vpcPrefixIPv4Cidr(vp)
	if cidr == "" {
		return nil, fmt.Errorf("vpc prefix has no IPv4 CIDR")
	}
	usable, err := IPv4UsableHostAddrsFromCidr(cidr)
	if err != nil {
		return nil, err
	}
	ifaceCount, err := countInterfacesForSubnetOrVpc(ctx, db, nil, &vp.ID)
	if err != nil {
		return nil, err
	}
	return vpcPrefixUsageFromInterfaceCounts(usable, ifaceCount, cidr)
}

// GetInterfaceBasedUsageForSubnet returns ipam.Usage-shaped stats from subnet IPv4 CIDR and Ethernet `interface` rows for subnet_id.
func GetInterfaceBasedUsageForSubnet(ctx context.Context, db bun.IDB, sn *cdbm.Subnet) (*cipam.Usage, error) {
	if sn == nil {
		return nil, fmt.Errorf("subnet is nil")
	}
	cidr, err := subnetIPv4Cidr(sn)
	if err != nil {
		return nil, err
	}
	usable, err := IPv4UsableHostAddrsFromCidr(cidr)
	if err != nil {
		return nil, err
	}
	ifaceCount, err := countInterfacesForSubnetOrVpc(ctx, db, &sn.ID, nil)
	if err != nil {
		return nil, err
	}
	return subnetUsageFromInterfaceCounts(usable, ifaceCount, cidr)
}
