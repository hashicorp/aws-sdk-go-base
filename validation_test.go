package awsbase

import (
	"testing"
)

func TestValidateAccountID(t *testing.T) {
	var testCases = []struct {
		Description         string
		AccountID           string
		AllowedAccountIDs   []string
		ForbiddenAccountIDs []string
		ExpectError         bool
	}{
		{
			Description:         "Allowed if no allowed or forbidden account IDs",
			AccountID:           "123456789012",
			AllowedAccountIDs:   []string{},
			ForbiddenAccountIDs: []string{},
			ExpectError:         false,
		},
		{
			Description:         "Allowed if matches an allowed account ID",
			AccountID:           "123456789012",
			AllowedAccountIDs:   []string{"123456789012"},
			ForbiddenAccountIDs: []string{},
			ExpectError:         false,
		},
		{
			Description:         "Allowed if does not match a forbidden account ID",
			AccountID:           "123456789012",
			AllowedAccountIDs:   []string{},
			ForbiddenAccountIDs: []string{"111111111111"},
			ExpectError:         false,
		},
		{
			Description:         "Denied if matches a forbidden account ID",
			AccountID:           "123456789012",
			AllowedAccountIDs:   []string{},
			ForbiddenAccountIDs: []string{"123456789012"},
			ExpectError:         true,
		},
		{
			Description:         "Denied if does not match an allowed account ID",
			AccountID:           "123456789012",
			AllowedAccountIDs:   []string{"111111111111"},
			ForbiddenAccountIDs: []string{},
			ExpectError:         true,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.Description, func(t *testing.T) {
			err := ValidateAccountID(testCase.AccountID, testCase.AllowedAccountIDs, testCase.ForbiddenAccountIDs)
			if err != nil && !testCase.ExpectError {
				t.Fatalf("Expected no error, received error: %s", err)
			}
			if err == nil && testCase.ExpectError {
				t.Fatal("Expected error, received none")
			}
		})
	}
}

func TestValidateRegion(t *testing.T) {
	var testCases = []struct {
		Region      string
		ExpectError bool
	}{
		{
			Region:      "us-east-1",
			ExpectError: false,
		},
		{
			Region:      "us-gov-west-1",
			ExpectError: false,
		},
		{
			Region:      "cn-northwest-1",
			ExpectError: false,
		},
		{
			Region:      "invalid",
			ExpectError: true,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.Region, func(t *testing.T) {
			err := ValidateRegion(testCase.Region)
			if err != nil && !testCase.ExpectError {
				t.Fatalf("Expected no error, received error: %s", err)
			}
			if err == nil && testCase.ExpectError {
				t.Fatal("Expected error, received none")
			}
		})
	}
}
