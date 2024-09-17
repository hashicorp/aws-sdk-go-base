// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package endpoints_test

import (
	"testing"

	"github.com/hashicorp/aws-sdk-go-base/v2/endpoints"
)

func TestDefaultPartitions(t *testing.T) {
	t.Parallel()

	got := endpoints.DefaultPartitions()
	if len(got) == 0 {
		t.Fatalf("expected partitions, got none")
	}
}

func TestPartitionForRegion(t *testing.T) {
	t.Parallel()

	testcases := map[string]struct {
		expectedFound bool
		expectedID    string
	}{
		"us-east-1": {
			expectedFound: true,
			expectedID:    "aws",
		},
		"us-gov-west-1": {
			expectedFound: true,
			expectedID:    "aws-us-gov",
		},
		"not-found": {
			expectedFound: false,
		},
		"us-east-17": {
			expectedFound: true,
			expectedID:    "aws",
		},
	}

	ps := endpoints.DefaultPartitions()
	for region, testcase := range testcases {
		gotID, gotFound := endpoints.PartitionForRegion(ps, region)

		if gotFound != testcase.expectedFound {
			t.Errorf("expected PartitionFound %t for Region %q, got %t", testcase.expectedFound, region, gotFound)
		}
		if gotID.ID() != testcase.expectedID {
			t.Errorf("expected PartitionID %q for Region %q, got %q", testcase.expectedID, region, gotID.ID())
		}
	}

}
