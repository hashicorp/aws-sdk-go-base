package endpoints_test

import (
	"testing"

	"github.com/hashicorp/aws-sdk-go-base/v2/internal/endpoints"
)

func TestPartitionForRegion(t *testing.T) {
	testcases := map[string]struct {
		expected string
	}{
		"us-east-1": {
			expected: "aws",
		},
		"me-central-1": {
			expected: "aws",
		},
		"cn-north-1": {
			expected: "aws-cn",
		},
		"us-gov-west-1": {
			expected: "aws-us-gov",
		},
	}

	for region, testcase := range testcases {
		got := endpoints.PartitionForRegion(region)

		if got != testcase.expected {
			t.Errorf("expected Partition %q for Region %q, got %q", testcase.expected, region, got)
		}
	}
}
