// Copyright IBM Corp. 2015, 2025
// SPDX-License-Identifier: MPL-2.0

package logging

import (
	"context"
	"io"
	"testing"

	"github.com/hashicorp/go-hclog"
	"github.com/hashicorp/terraform-plugin-log/tflogtest"
	"github.com/hashicorp/terraform-plugin-log/tfsdklog"
)

const tflogRootName = "provider"

func TestTfLoggerWarn(t *testing.T) {
	testLoggerWarn(t, tflogRootName, tfLoggerFactory)
}

func TestTfLoggerSetField(t *testing.T) {
	testLoggerSetField(t, tflogRootName, tfLoggerFactory)
}

func TestTfLoggerIsDebug(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		level    hclog.Level
		expected bool
	}{
		"trace": {level: hclog.Trace, expected: true},
		"debug": {level: hclog.Debug, expected: true},
		"info":  {level: hclog.Info, expected: false},
		"warn":  {level: hclog.Warn, expected: false},
		"error": {level: hclog.Error, expected: false},
		"off":   {level: hclog.Off, expected: false},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			ctx := tfsdklog.NewRootProviderLogger(
				t.Context(),
				tfsdklog.WithLevel(tc.level),
				tfsdklog.WithoutLocation(),
			)
			_, logger := NewTfLogger(ctx)

			got := logger.IsDebug(ctx)

			if got != tc.expected {
				t.Errorf("IsDebug() = %v, want %v (level=%s)", got, tc.expected, tc.level)
			}
		})
	}
}

func tfLoggerFactory(ctx context.Context, name string, output io.Writer) (context.Context, Logger) {
	ctx = tflogtest.RootLogger(ctx, output)

	ctx, rootLogger := NewTfLogger(ctx)
	ctx, logger := rootLogger.SubLogger(ctx, name)

	return ctx, logger
}
