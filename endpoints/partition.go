// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package endpoints

import (
	"regexp"
)

type Partition struct {
	id          string
	name        string
	dnsSuffix   string
	regionRegex *regexp.Regexp
}

func (p *Partition) ID() string {
	return p.id
}

func (p *Partition) Name() string {
	return p.name
}

func (p *Partition) DNSSuffix() string {
	return p.dnsSuffix
}

func (p *Partition) RegionRegex() *regexp.Regexp {
	return p.regionRegex
}
