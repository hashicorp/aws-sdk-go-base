// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package awsbase

import (
	"fmt"
	"testing"
)

func TestIsCannotAssumeRoleError(t *testing.T) {
	testCases := []struct {
		Name     string
		Err      error
		Expected bool
	}{
		{
			Name: "nil error",
		},
		{
			Name: "Top-level NoValidCredentialSourcesError",
			Err:  NoValidCredentialSourcesError{},
		},
		{
			Name:     "Top-level CannotAssumeRoleError",
			Err:      CannotAssumeRoleError{},
			Expected: true,
		},
		{
			Name:     "Nested CannotAssumeRoleError",
			Err:      fmt.Errorf("test: %w", CannotAssumeRoleError{}),
			Expected: true,
		},
	}

	for _, testCase := range testCases {
		testCase := testCase

		t.Run(testCase.Name, func(t *testing.T) {
			got := IsCannotAssumeRoleError(testCase.Err)

			if got != testCase.Expected {
				t.Errorf("got %t, expected %t", got, testCase.Expected)
			}
		})
	}
}

func TestIsNoValidCredentialSourcesError(t *testing.T) {
	testCases := []struct {
		Name     string
		Err      error
		Expected bool
	}{
		{
			Name: "nil error",
		},
		{
			Name: "Top-level CannotAssumeRoleError",
			Err:  CannotAssumeRoleError{},
		},
		{
			Name:     "Top-level NoValidCredentialSourcesError",
			Err:      NoValidCredentialSourcesError{},
			Expected: true,
		},
		{
			Name:     "Nested NoValidCredentialSourcesError",
			Err:      fmt.Errorf("test: %w", NoValidCredentialSourcesError{}),
			Expected: true,
		},
	}

	for _, testCase := range testCases {
		testCase := testCase

		t.Run(testCase.Name, func(t *testing.T) {
			got := IsNoValidCredentialSourcesError(testCase.Err)

			if got != testCase.Expected {
				t.Errorf("got %t, expected %t", got, testCase.Expected)
			}
		})
	}
}
