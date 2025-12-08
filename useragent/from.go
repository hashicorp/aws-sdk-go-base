// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package useragent

import (
	"strings"

	"github.com/hashicorp/aws-sdk-go-base/v2/internal/config"
	"github.com/hashicorp/aws-sdk-go-base/v2/internal/slices"
)

// FromSlice applies the conversion defined in [fromAny] to all elements
// of a slice
//
// Slices of types which cannot assert to a string, empty string values, and string
// values which do not match the expected `{product}/{version} ({comment})`
// pattern (where version and comment are optional) return a zero value struct.
func FromSlice[T any](sl []T) config.UserAgentProducts {
	return slices.ApplyToAll(sl, func(v T) config.UserAgentProduct { return fromAny(v) })
}

func fromAny(v any) config.UserAgentProduct {
	if s, ok := v.(string); ok {
		parts := strings.Split(s, "/")
		switch len(parts) {
		case 1:
			return config.UserAgentProduct{Name: parts[0]}
		case 2: //nolint: mnd
			subparts := strings.Split(parts[1], "(")
			if len(subparts) == 2 { //nolint: mnd
				version := strings.TrimSpace(subparts[0])
				comment := strings.TrimSuffix(subparts[1], ")")
				return config.UserAgentProduct{Name: parts[0], Version: version, Comment: comment}
			}
			return config.UserAgentProduct{Name: parts[0], Version: parts[1]}
		}
	}

	return config.UserAgentProduct{}
}
