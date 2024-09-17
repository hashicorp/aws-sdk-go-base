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
