package awsv1shim

import (
	"fmt"

	"github.com/aws/aws-sdk-go/aws/endpoints"
)

// ValidateRegion checks if the given region is a valid AWS region.
func ValidateRegion(region string) error {
	for _, partition := range endpoints.DefaultPartitions() {
		for _, partitionRegion := range partition.Regions() {
			if region == partitionRegion.ID() {
				return nil
			}
		}
	}

	return fmt.Errorf("Invalid AWS Region: %s", region)
}
