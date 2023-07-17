// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package awsbase

import (
	"github.com/hashicorp/aws-sdk-go-base/v2/diag"
	"github.com/hashicorp/aws-sdk-go-base/v2/internal/config"
)

// CannotAssumeRoleError occurs when AssumeRole cannot complete.
type CannotAssumeRoleError = config.CannotAssumeRoleError

// IsCannotAssumeRoleError returns true if the error contains the CannotAssumeRoleError type.
func IsCannotAssumeRoleError(diag diag.Diagnostic) bool {
	_, ok := diag.(CannotAssumeRoleError)
	return ok
}

// NoValidCredentialSourcesError occurs when all credential lookup methods have been exhausted without results.
type NoValidCredentialSourcesError = config.NoValidCredentialSourcesError

// IsNoValidCredentialSourcesError returns true if the diagnostic is a NoValidCredentialSourcesError.
func IsNoValidCredentialSourcesError(diag diag.Diagnostic) bool {
	_, ok := diag.(NoValidCredentialSourcesError)
	return ok
}

// ContainsNoValidCredentialSourcesError returns true if the diagnostics contains a NoValidCredentialSourcesError type.
func ContainsNoValidCredentialSourcesError(diags diag.Diagnostics) bool {
	for _, diag := range diags {
		if IsNoValidCredentialSourcesError(diag) {
			return true
		}
	}
	return false
}
