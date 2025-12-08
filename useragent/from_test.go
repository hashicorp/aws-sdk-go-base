// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package useragent

import (
	"reflect"
	"testing"

	"github.com/hashicorp/aws-sdk-go-base/v2/internal/config"
)

func TestFromSlice(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		s    []any
		want config.UserAgentProducts
	}{
		{
			"nil",
			nil,
			config.UserAgentProducts{},
		},
		{
			"non-string element",
			[]any{false},
			config.UserAgentProducts{config.UserAgentProduct{}},
		},
		{
			"valid string",
			[]any{"my-product/v1.2.3"},
			config.UserAgentProducts{
				config.UserAgentProduct{
					Name:    "my-product",
					Version: "v1.2.3",
				},
			},
		},
		{
			"valid and invalid string",
			[]any{"my-product/v1.2.3", "foo/bar/baz/qux"},
			config.UserAgentProducts{
				config.UserAgentProduct{
					Name:    "my-product",
					Version: "v1.2.3",
				},
				config.UserAgentProduct{},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := FromSlice(tt.s)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("FromSlice() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func Test_fromAny(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		v    any
		want config.UserAgentProduct
	}{
		{
			"nil",
			nil,
			config.UserAgentProduct{},
		},
		{
			"non-string",
			1,
			config.UserAgentProduct{},
		},
		{
			"empty",
			"",
			config.UserAgentProduct{},
		},
		{
			"name only",
			"my-product",
			config.UserAgentProduct{
				Name: "my-product",
			},
		},
		{
			"name and version",
			"my-product/v1.2.3",
			config.UserAgentProduct{
				Name:    "my-product",
				Version: "v1.2.3",
			},
		},
		{
			"name, version, and comment",
			"my-product/v1.2.3 (a comment)",
			config.UserAgentProduct{
				Name:    "my-product",
				Version: "v1.2.3",
				Comment: "a comment",
			},
		},
		{
			"comment malformed closing",
			"my-product/v1.2.3 (a comment",
			config.UserAgentProduct{
				Name:    "my-product",
				Version: "v1.2.3",
				Comment: "a comment",
			},
		},
		{
			"comment missing parenthesis",
			"my-product/v1.2.3 a comment",
			// This is a known edge case, but the processed output will render identical
			// to the original input despite the version and comment merging.
			config.UserAgentProduct{
				Name:    "my-product",
				Version: "v1.2.3 a comment",
			},
		},
		{
			"all the slash",
			"foo/bar/baz/qux",
			config.UserAgentProduct{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := fromAny(tt.v)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("fromAny() = %+v, want %+v", got, tt.want)
			}
		})
	}
}
