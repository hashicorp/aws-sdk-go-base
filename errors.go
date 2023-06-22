// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package awsbase

import (
	smithy "github.com/aws/smithy-go"
	"github.com/hashicorp/aws-sdk-go-base/v2/internal/config"
	"github.com/hashicorp/aws-sdk-go-base/v2/internal/errs"
)

// CannotAssumeRoleError occurs when AssumeRole cannot complete.
type CannotAssumeRoleError = config.CannotAssumeRoleError

// IsCannotAssumeRoleError returns true if the error contains the CannotAssumeRoleError type.
func IsCannotAssumeRoleError(err error) bool {
	return errs.IsA[CannotAssumeRoleError](err)
}

// NoValidCredentialSourcesError occurs when all credential lookup methods have been exhausted without results.
type NoValidCredentialSourcesError = config.NoValidCredentialSourcesError

// IsNoValidCredentialSourcesError returns true if the error contains the NoValidCredentialSourcesError type.
func IsNoValidCredentialSourcesError(err error) bool {
	return errs.IsA[NoValidCredentialSourcesError](err)
}

// AWS SDK for Go v2 variants of helpers in v2/awsv1shim/tfawserr.

// ErrCodeEquals returns true if the error matches all these conditions:
//   - err is of type smithy.APIError
//   - Error.Code() equals one of the passed codes
func ErrCodeEquals(err error, codes ...string) bool {
	if apiErr, ok := errs.As[smithy.APIError](err); ok {
		for _, code := range codes {
			if apiErr.ErrorCode() == code {
				return true
			}
		}
	}
	return false
}
