// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package endpoints

import (
	"regexp"
)

// Partition represents an AWS partition.
// See https://docs.aws.amazon.com/whitepapers/latest/aws-fault-isolation-boundaries/partitions.html.
type Partition struct {
	id          string
	name        string
	dnsSuffix   string
	regionRegex *regexp.Regexp
}

// ID returns the identifier of the partition.
func (p Partition) ID() string {
	return p.id
}

// Name returns the name of the partition.
func (p Partition) Name() string {
	return p.name
}

// DNSSuffix returns the base domain name of the partition.
func (p Partition) DNSSuffix() string {
	return p.dnsSuffix
}

// RegionRegex return the regular expression that matches Region IDs for the partition.
func (p Partition) RegionRegex() *regexp.Regexp {
	return p.regionRegex
}

// DefaultPartitions returns a list of the partitions.
func DefaultPartitions() []Partition {
	partitions := make([]Partition, len(partitionsAndRegions))

	for _, v := range partitionsAndRegions {
		partitions = append(partitions, v.partition)
	}

	return partitions
}
