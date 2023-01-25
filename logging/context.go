package logging

import (
	"context"
)

type loggerKeyT string

const loggerKey loggerKeyT = "logger-key"

func RegisterLogger(ctx context.Context, logger TfLogger) context.Context {
	return context.WithValue(ctx, loggerKey, logger)
}

func RetrieveLogger(ctx context.Context) TfLogger {
	return ctx.Value(loggerKey).(TfLogger)
}
