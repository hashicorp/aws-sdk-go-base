// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tfawserr

import (
	"fmt"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sts/types"
	smithy "github.com/aws/smithy-go"
)

func TestErrCodeEquals(t *testing.T) {
	testCases := map[string]struct {
		Err      error
		Codes    []string
		Expected bool
	}{
		"nil error": {
			Err:      nil,
			Expected: false,
		},
		"other error": {
			Err:      fmt.Errorf("other error"),
			Expected: false,
		},
		"Top-level smithy.GenericAPIError matching first code": {
			Err:      &smithy.GenericAPIError{Code: "TestCode", Message: "TestMessage"},
			Codes:    []string{"TestCode"},
			Expected: true,
		},
		"Top-level smithy.GenericAPIError matching last code": {
			Err:      &smithy.GenericAPIError{Code: "TestCode", Message: "TestMessage"},
			Codes:    []string{"NotMatching", "TestCode"},
			Expected: true,
		},
		"Top-level smithy.GenericAPIError no code": {
			Err: &smithy.GenericAPIError{Code: "TestCode", Message: "TestMessage"},
		},
		"Top-level smithy.GenericAPIError non-matching codes": {
			Err:   &smithy.GenericAPIError{Code: "TestCode", Message: "TestMessage"},
			Codes: []string{"NotMatching", "AlsoNotMatching"},
		},
		"Wrapped smithy.GenericAPIError matching first code": {
			Err:      fmt.Errorf("test: %w", &smithy.GenericAPIError{Code: "TestCode", Message: "TestMessage"}),
			Codes:    []string{"TestCode"},
			Expected: true,
		},
		"Wrapped smithy.GenericAPIError matching last code": {
			Err:      fmt.Errorf("test: %w", &smithy.GenericAPIError{Code: "TestCode", Message: "TestMessage"}),
			Codes:    []string{"NotMatching", "TestCode"},
			Expected: true,
		},
		"Wrapped smithy.GenericAPIError non-matching codes": {
			Err:   fmt.Errorf("test: %w", &smithy.GenericAPIError{Code: "TestCode", Message: "TestMessage"}),
			Codes: []string{"NotMatching", "AlsoNotMatching"},
		},
		"Top-level sts ExpiredTokenException matching first code": {
			Err:      &types.ExpiredTokenException{ErrorCodeOverride: aws.String("TestCode"), Message: aws.String("TestMessage")},
			Codes:    []string{"TestCode"},
			Expected: true,
		},
		"Top-level sts ExpiredTokenException matching last code": {
			Err:      &types.ExpiredTokenException{ErrorCodeOverride: aws.String("TestCode"), Message: aws.String("TestMessage")},
			Codes:    []string{"NotMatching", "TestCode"},
			Expected: true,
		},
		"Wrapped sts ExpiredTokenException matching first code": {
			Err:      fmt.Errorf("test: %w", &types.ExpiredTokenException{ErrorCodeOverride: aws.String("TestCode"), Message: aws.String("TestMessage")}),
			Codes:    []string{"TestCode"},
			Expected: true,
		},
		"Wrapped sts ExpiredTokenException matching last code": {
			Err:      fmt.Errorf("test: %w", &types.ExpiredTokenException{ErrorCodeOverride: aws.String("TestCode"), Message: aws.String("TestMessage")}),
			Codes:    []string{"NotMatching", "TestCode"},
			Expected: true,
		},
	}

	for name, testCase := range testCases {
		testCase := testCase

		t.Run(name, func(t *testing.T) {
			got := ErrCodeEquals(testCase.Err, testCase.Codes...)

			if got != testCase.Expected {
				t.Errorf("got %t, expected %t", got, testCase.Expected)
			}
		})
	}
}
