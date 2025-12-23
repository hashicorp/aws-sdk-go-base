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
	return slices.ApplyToAll(sl, func(v T) config.UserAgentProduct { return from(v) })
}

func from[T any](v T) config.UserAgentProduct {
	if s, ok := any(v).(string); ok {
		idx := strings.LastIndex(s, "/")
		if idx != -1 {
			name := s[:idx]
			version := s[idx+1:]

			parts := strings.Split(version, "(")
			if len(parts) == 2 { //nolint: mnd
				version = strings.TrimSpace(parts[0])
				comment := strings.TrimSuffix(parts[1], ")")
				return config.UserAgentProduct{Name: name, Version: version, Comment: comment}
			}
			return config.UserAgentProduct{Name: name, Version: version}
		}

		return config.UserAgentProduct{Name: s}
	}

	return config.UserAgentProduct{}
}
