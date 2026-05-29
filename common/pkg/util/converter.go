// SPDX-FileCopyrightText: Copyright (c) 2026 NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package util

// IntPtrToUint32Ptr converts a `*int` to a `*uint32`. nil in, nil out.
// Callers must ensure the int sits in `[0, MaxUint32]`; the cast
// otherwise silently wraps. Under the proto-conversion convention,
// that bound is the responsibility of the request-side `Validate`
// (which rejects negatives and overflow with a 400) on the
// API-inbound path, or guaranteed by construction on the proto-inbound
// path (where values originate from a proto `uint32` field).
func IntPtrToUint32Ptr(i *int) *uint32 {
	if i == nil {
		return nil
	}
	u := uint32(*i) //nolint:gosec // bounded upstream by Validate / proto-source.
	return &u
}
