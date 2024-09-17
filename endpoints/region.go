// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package endpoints

type Region struct {
	id          string
	description string
	partitionID string
}

func (r *Region) ID() string {
	return r.id
}

func (r *Region) Description() string {
	return r.description
}

func (r *Region) PartitionID() string {
	return r.partitionID
}
