// Copyright IBM Corp. 2015, 2025
// SPDX-License-Identifier: MPL-2.0

package test

import (
	"context"
	"testing"

	"github.com/hashicorp/aws-sdk-go-base/v2/logging"
)

func Context(t *testing.T) context.Context {
	return logging.RegisterLogger(t.Context(), logging.TfLogger(t.Name()))
}
