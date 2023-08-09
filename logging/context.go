// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package logging

import (
	"context"
)

type loggerKeyT string

const loggerKey loggerKeyT = "logger-key"

func RegisterLogger(ctx context.Context, logger Logger) context.Context {
	return context.WithValue(ctx, loggerKey, logger)
}

func RetrieveLogger(ctx context.Context) Logger {
	return ctx.Value(loggerKey).(Logger)
}
