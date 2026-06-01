// SPDX-FileCopyrightText: Copyright (c) 2026 NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package model

import (
	validation "github.com/go-ozzo/ozzo-validation/v4"
	validationis "github.com/go-ozzo/ozzo-validation/v4/is"
)

const (
	// CapabilityNameTargetedInstanceCreation is the capability name for targeted instance creation.
	// It is the first (and currently only) scoped tenant capability.
	CapabilityNameTargetedInstanceCreation = "TargetedInstanceCreation"

	validationErrorUnknownCapabilityName = "unknown capability name"
	validationErrorEnabledRequired       = "enabled must be specified"
)

// SupportedCapabilityNames is the set of capability names the capabilities endpoint accepts.
var SupportedCapabilityNames = []interface{}{
	CapabilityNameTargetedInstanceCreation,
}

// APITenantCapabilityUpdateRequest is the data structure to capture a tenant admin's request
// to enable or disable a scoped capability for a resolved set of sites.
//
// An empty Sites array means "all eligible sites" (optionally restricted by
// InfrastructureProviderID); a non-empty array scopes the change to the listed sites.
type APITenantCapabilityUpdateRequest struct {
	// CapabilityName is the capability being toggled, e.g. "TargetedInstanceCreation".
	CapabilityName string `json:"capabilityName"`
	// Sites is the list of Site IDs to scope the change to. Empty ⇒ all eligible sites.
	Sites []string `json:"sites"`
	// InfrastructureProviderID optionally restricts the resolved site set to sites owned
	// by that provider. When Sites is non-empty, it is used for validation only.
	InfrastructureProviderID *string `json:"infrastructureProviderId"`
	// Enabled selects the operation: true opts in, false opts out, for the resolved sites.
	Enabled *bool `json:"enabled"`
}

// Validate ensures the values passed in request are acceptable
func (tcur APITenantCapabilityUpdateRequest) Validate() error {
	return validation.ValidateStruct(&tcur,
		validation.Field(&tcur.CapabilityName,
			validation.Required.Error(validationErrorValueRequired),
			validation.In(SupportedCapabilityNames...).Error(validationErrorUnknownCapabilityName)),
		validation.Field(&tcur.Sites,
			validation.Each(validationis.UUID.Error(validationErrorInvalidUUID))),
		validation.Field(&tcur.InfrastructureProviderID,
			validationis.UUID.Error(validationErrorInvalidUUID)),
		validation.Field(&tcur.Enabled,
			validation.NotNil.Error(validationErrorEnabledRequired)),
	)
}

// APITenantCapability is the API representation of the result of a capabilities update:
// the capability, whether it was enabled or disabled, and the resolved sites it was applied to.
type APITenantCapability struct {
	// CapabilityName is the capability that was toggled.
	CapabilityName string `json:"capabilityName"`
	// Enabled reflects the operation applied to the resolved sites.
	Enabled bool `json:"enabled"`
	// SiteIDs are the Site IDs the change was applied to.
	SiteIDs []string `json:"siteIds"`
}
