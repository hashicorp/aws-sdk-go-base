// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package validation

import (
	"fmt"

	"github.com/hashicorp/aws-sdk-go-base/v2/internal/endpoints"
)

type InvalidRegionError struct {
	region string
}

func (e *InvalidRegionError) Error() string {
	return fmt.Sprintf("Invalid AWS Region: %s", e.region)
}

// SupportedRegion checks if the given region is a valid AWS region.
func SupportedRegion(region string) error {
	for _, partition := range endpoints.Partitions() {
		for _, partitionRegion := range partition.Regions() {
			if region == partitionRegion {
				return nil
			}
		}
	}

	return &InvalidRegionError{
		region: region,
	}
}
