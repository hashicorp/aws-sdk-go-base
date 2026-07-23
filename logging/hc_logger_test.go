// Copyright IBM Corp. 2015, 2025
// SPDX-License-Identifier: MPL-2.0

package logging

import (
	"context"
	"io"
	"testing"

	"github.com/hashicorp/go-hclog"
)

const hclogRootName = "hc-log-test"

func TestHcLoggerWarn(t *testing.T) {
	testLoggerWarn(t, hclogRootName, hcLoggerFactory)
}

func TestHcLoggerSetField(t *testing.T) {
	testLoggerSetField(t, hclogRootName, hcLoggerFactory)
}

func TestHcLoggerIsDebug(t *testing.T) {
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

			hclogger := hclog.New(&hclog.LoggerOptions{
				Name:  hclogRootName,
				Level: tc.level,
			})
			ctx, logger := NewHcLogger(t.Context(), hclogger)

			got := logger.IsDebug(ctx)

			if got != tc.expected {
				t.Errorf("IsDebug() = %v, want %v (level=%s)", got, tc.expected, tc.level)
			}
		})
	}
}

func hcLoggerFactory(ctx context.Context, name string, output io.Writer) (context.Context, Logger) {
	hclogger := configureHcLogger(output)

	ctx, rootLogger := NewHcLogger(ctx, hclogger)
	ctx, logger := rootLogger.SubLogger(ctx, name)

	return ctx, logger
}

// configureHcLogger configures the default logger with settings suitable for testing:
//
//   - Log level set to TRACE
//   - Written to the io.Writer passed in, such as a bytes.Buffer
//   - Log entries are in JSON format, and can be decoded using multilineJSONDecode
//   - Caller information is not included
//   - Timestamp is not included
func configureHcLogger(output io.Writer) hclog.Logger {
	logger := hclog.NewInterceptLogger(&hclog.LoggerOptions{
		Name:              hclogRootName,
		Level:             hclog.Trace,
		Output:            output,
		IndependentLevels: true,
		JSONFormat:        true,
		IncludeLocation:   false,
		DisableTime:       true,
	})

	return logger
}
