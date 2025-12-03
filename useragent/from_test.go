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

func Test_fromString(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		s    string
		want config.UserAgentProduct
	}{
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
			"all the slash",
			"foo/bar/baz/qux",
			config.UserAgentProduct{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := fromString(tt.s)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("fromString() = %+v, want %+v", got, tt.want)
			}
		})
	}
}
