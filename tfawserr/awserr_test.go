// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tfawserr

import (
	"fmt"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/sts/types"
	"github.com/aws/aws-sdk-go/aws"
	smithy "github.com/aws/smithy-go"
	awsbase "github.com/hashicorp/aws-sdk-go-base/v2"
)

func TestErrCodeEquals(t *testing.T) {
	testCases := []struct {
		Name     string
		Err      error
		Codes    []string
		Expected bool
	}{
		{
			Name: "nil error",
		},
		{
			Name: "Top-level CannotAssumeRoleError",
			Err:  awsbase.CannotAssumeRoleError{},
		},
		{
			Name:     "Top-level smithy.GenericAPIError matching first code",
			Err:      &smithy.GenericAPIError{Code: "TestCode", Message: "TestMessage"},
			Codes:    []string{"TestCode"},
			Expected: true,
		},
		{
			Name:     "Top-level smithy.GenericAPIError matching last code",
			Err:      &smithy.GenericAPIError{Code: "TestCode", Message: "TestMessage"},
			Codes:    []string{"NotMatching", "TestCode"},
			Expected: true,
		},
		{
			Name: "Top-level smithy.GenericAPIError no code",
			Err:  &smithy.GenericAPIError{Code: "TestCode", Message: "TestMessage"},
		},
		{
			Name:  "Top-level smithy.GenericAPIError non-matching codes",
			Err:   &smithy.GenericAPIError{Code: "TestCode", Message: "TestMessage"},
			Codes: []string{"NotMatching", "AlsoNotMatching"},
		},
		{
			Name:     "Wrapped smithy.GenericAPIError matching first code",
			Err:      fmt.Errorf("test: %w", &smithy.GenericAPIError{Code: "TestCode", Message: "TestMessage"}),
			Codes:    []string{"TestCode"},
			Expected: true,
		},
		{
			Name:     "Wrapped smithy.GenericAPIError matching last code",
			Err:      fmt.Errorf("test: %w", &smithy.GenericAPIError{Code: "TestCode", Message: "TestMessage"}),
			Codes:    []string{"NotMatching", "TestCode"},
			Expected: true,
		},
		{
			Name:  "Wrapped smithy.GenericAPIError non-matching codes",
			Err:   fmt.Errorf("test: %w", &smithy.GenericAPIError{Code: "TestCode", Message: "TestMessage"}),
			Codes: []string{"NotMatching", "AlsoNotMatching"},
		},
		{
			Name:     "Top-level sts ExpiredTokenException matching first code",
			Err:      &types.ExpiredTokenException{ErrorCodeOverride: aws.String("TestCode"), Message: aws.String("TestMessage")},
			Codes:    []string{"TestCode"},
			Expected: true,
		},
		{
			Name:     "Top-level sts ExpiredTokenException matching last code",
			Err:      &types.ExpiredTokenException{ErrorCodeOverride: aws.String("TestCode"), Message: aws.String("TestMessage")},
			Codes:    []string{"NotMatching", "TestCode"},
			Expected: true,
		},
		{
			Name:     "Wrapped sts ExpiredTokenException matching first code",
			Err:      fmt.Errorf("test: %w", &types.ExpiredTokenException{ErrorCodeOverride: aws.String("TestCode"), Message: aws.String("TestMessage")}),
			Codes:    []string{"TestCode"},
			Expected: true,
		},
		{
			Name:     "Wrapped sts ExpiredTokenException matching last code",
			Err:      fmt.Errorf("test: %w", &types.ExpiredTokenException{ErrorCodeOverride: aws.String("TestCode"), Message: aws.String("TestMessage")}),
			Codes:    []string{"NotMatching", "TestCode"},
			Expected: true,
		},
	}

	for _, testCase := range testCases {
		testCase := testCase

		t.Run(testCase.Name, func(t *testing.T) {
			got := ErrCodeEquals(testCase.Err, testCase.Codes...)

			if got != testCase.Expected {
				t.Errorf("got %t, expected %t", got, testCase.Expected)
			}
		})
	}
}
