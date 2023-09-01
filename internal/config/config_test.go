// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package config

import (
	"testing"
)

func TestConfig_VerifyAccountIDAllowed(t *testing.T) {
	tests := []struct {
		name      string
		config    Config
		accountID string
		wantErr   bool
	}{
		{
			"empty",
			Config{},
			"1234",
			false,
		},
		{
			"allowed",
			Config{
				AllowedAccountIds: []string{"1234"},
			},
			"1234",
			false,
		},
		{
			"not allowed",
			Config{
				AllowedAccountIds: []string{"5678"},
			},
			"1234",
			true,
		},
		{
			"forbidden",
			Config{
				ForbiddenAccountIds: []string{"1234"},
			},
			"1234",
			true,
		},
		{
			"not forbidden",
			Config{
				ForbiddenAccountIds: []string{"5678"},
			},
			"1234",
			false,
		},
		{
			// In practice the upstream interfaces (AWS Provider, S3 Backend, etc.) should make
			// these conflict, but documenting the behavior for completeness.
			"allowed and forbidden",
			Config{
				AllowedAccountIds:   []string{"1234"},
				ForbiddenAccountIds: []string{"1234"},
			},
			"1234",
			true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.config.VerifyAccountIDAllowed(tt.accountID); (err != nil) != tt.wantErr {
				t.Errorf("Config.VerifyAccountIDAllowed() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
