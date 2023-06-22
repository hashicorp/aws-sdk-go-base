// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tfawserr

import (
	smithy "github.com/aws/smithy-go"
	"github.com/hashicorp/aws-sdk-go-base/v2/internal/errs"
)

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
