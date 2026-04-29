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

package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// SchemeGroupVersion for nico types
var SchemeGroupVersion = schema.GroupVersion{
	Group:   "nico.nvidia.io",
	Version: "v1",
}

// ForgeLegacySchemeGroupVersion is the legacy CRD group used by pre-NICo site agents.
// The cloud REST side must support both groups during the transition period so that
// existing sites can still connect until their site agent is upgraded.
// TODO: remove ForgeLegacySchemeGroupVersion once all site agents have migrated to
// the nico.nvidia.io CRD group and forge.nvidia.io Sites no longer exist.
var ForgeLegacySchemeGroupVersion = schema.GroupVersion{
	Group:   "forge.nvidia.io",
	Version: "v1",
}

// other standard variables
var (
	SchemeBuilder      runtime.SchemeBuilder
	localSchemeBuilder = &SchemeBuilder
	AddToScheme        = localSchemeBuilder.AddToScheme

	// TODO: remove ForgeSchemeBuilder / ForgeAddToScheme once forge legacy support is dropped.
	ForgeSchemeBuilder runtime.SchemeBuilder
	forgeSchemeBuilder = &ForgeSchemeBuilder
	ForgeAddToScheme   = forgeSchemeBuilder.AddToScheme
)

func init() {
	// We only register manually written functions here. The registration of the
	// generated functions takes place in the generated files. The separation
	// makes the code compile even when the generated files are missing.
	localSchemeBuilder.Register(addKnownTypes)
	// TODO: remove forge registration once all site agents migrated to nico.nvidia.io.
	forgeSchemeBuilder.Register(addForgeLegacyKnownTypes)
}

// Resource takes an unqualified resource and returns a Group qualified GroupResource
func Resource(resource string) schema.GroupResource {
	return SchemeGroupVersion.WithResource(resource).GroupResource()
}

// addForgeLegacyKnownTypes registers the same Site types under the legacy forge.nvidia.io group
// so the REST client can deserialize responses from pre-NICo site agents.
// TODO: remove once all site agents are migrated to nico.nvidia.io.
func addForgeLegacyKnownTypes(scheme *runtime.Scheme) error {
	scheme.AddKnownTypes(
		ForgeLegacySchemeGroupVersion,
		&Site{},
		&SiteList{},
	)
	scheme.AddKnownTypes(
		ForgeLegacySchemeGroupVersion,
		&metav1.Status{},
	)
	metav1.AddToGroupVersion(scheme, ForgeLegacySchemeGroupVersion)
	return nil
}

// Adds the list of known types to the given scheme.
func addKnownTypes(scheme *runtime.Scheme) error {
	scheme.AddKnownTypes(
		SchemeGroupVersion,
		&Site{},
		&SiteList{},
	)

	scheme.AddKnownTypes(
		SchemeGroupVersion,
		&metav1.Status{},
	)

	metav1.AddToGroupVersion(
		scheme,
		SchemeGroupVersion,
	)

	return nil
}
