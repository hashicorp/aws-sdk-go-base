// Copyright IBM Corp. 2015, 2026
// SPDX-License-Identifier: MPL-2.0

package tfawserr

import (
	"slices"
	"strings"

	smithy "github.com/aws/smithy-go"
	smithyhttp "github.com/aws/smithy-go/transport/http"
	"github.com/hashicorp/aws-sdk-go-base/v2/internal/errs"
)

// ErrCodeEquals returns true if the error matches all these conditions:
//   - err is of type smithy.APIError
//   - APIError.ErrorCode() equals one of the passed codes
func ErrCodeEquals(err error, codes ...string) bool {
	if apiErr, ok := errs.As[smithy.APIError](err); ok {
		if slices.Contains(codes, apiErr.ErrorCode()) {
			return true
		}
	}
	return false
}

// ErrCodeContains returns true if the error matches all these conditions:
//   - err is of type smithy.APIError
//   - APIError.ErrorCode() contains code
func ErrCodeContains(err error, code string) bool {
	if apiErr, ok := errs.As[smithy.APIError](err); ok {
		return strings.Contains(apiErr.ErrorCode(), code)
	}
	return false
}

// ErrMessageContains returns true if the error matches all these conditions:
//   - err is of type smithy.APIError
//   - APIError.ErrorCode() equals code
//   - APIError.ErrorMessage() contains message
func ErrMessageContains(err error, code string, message string) bool {
	if apiErr, ok := errs.As[smithy.APIError](err); ok {
		return apiErr.ErrorCode() == code && strings.Contains(apiErr.ErrorMessage(), message)
	}
	return false
}

// ErrMessageContainsAny returns true if the error matches all these conditions:
//   - err is of type smithy.APIError
//   - APIError.ErrorCode() equals code
//   - APIError.ErrorMessage() contains one of the passed messages
func ErrMessageContainsAny(err error, code string, messages ...string) bool {
	for _, message := range messages {
		if ErrMessageContains(err, code, message) {
			return true
		}
	}

	return false
}

// ErrCodeEqualsNested returns true if any error in err's tree is of type
// smithy.APIError with an APIError.ErrorCode() equal to one of the passed codes.
//
// Some AWS APIs return a generic error code with a more specific one nested
// inside it. ErrCodeEquals cannot match the nested code, because errors.As
// stops at the outermost smithy.APIError in the tree and only that error's code
// is compared.
func ErrCodeEqualsNested(err error, codes ...string) bool {
	return anyAPIError(err, func(apiErr smithy.APIError) bool {
		return slices.Contains(codes, apiErr.ErrorCode())
	})
}

// anyAPIError reports whether any error in err's tree is a smithy.APIError for
// which f returns true. The tree is walked in the same order as errors.As,
// following both Unwrap() error and Unwrap() []error.
func anyAPIError(err error, f func(smithy.APIError) bool) bool {
	for err != nil {
		if apiErr, ok := err.(smithy.APIError); ok && f(apiErr) {
			return true
		}

		switch x := err.(type) {
		case interface{ Unwrap() error }:
			err = x.Unwrap()
		case interface{ Unwrap() []error }:
			for _, err := range x.Unwrap() {
				if anyAPIError(err, f) {
					return true
				}
			}
			return false
		default:
			return false
		}
	}

	return false
}

// ErrHTTPStatusCodeEquals returns true if the error matches all these conditions:
//   - err is of type smithyhttp.ResponseError
//   - ResponseError.HTTPStatusCode() equals one of the passed status codes
func ErrHTTPStatusCodeEquals(err error, statusCodes ...int) bool {
	if respErr, ok := errs.As[*smithyhttp.ResponseError](err); ok {
		if slices.Contains(statusCodes, respErr.HTTPStatusCode()) {
			return true
		}
	}
	return false
}
