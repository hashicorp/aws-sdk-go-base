package logging

import (
	"context"

	"github.com/hashicorp/terraform-plugin-log/tflog"
)

type TfLogger string

func (l TfLogger) Warn(ctx context.Context, msg string, fields ...map[string]any) {
	if l == "" {
		tflog.Warn(ctx, msg, fields...)
	} else {
		tflog.SubsystemWarn(ctx, string(l), msg, fields...)
	}
}

func (l TfLogger) Info(ctx context.Context, msg string, fields ...map[string]any) {
	if l == "" {
		tflog.Info(ctx, msg, fields...)
	} else {
		tflog.SubsystemInfo(ctx, string(l), msg, fields...)
	}
}

// func (l tfLogger) Infof(ctx context.Context, format string, v ...any) {
// 	msg := fmt.Sprintf(format, v...)
// 	if l == "" {
// 		tflog.Info(ctx, msg)
// 	} else {
// 		tflog.SubsystemInfo(ctx, string(l), msg)
// 	}
// }

func (l TfLogger) Debug(ctx context.Context, msg string, fields ...map[string]any) {
	if l == "" {
		tflog.Debug(ctx, msg, fields...)
	} else {
		tflog.SubsystemDebug(ctx, string(l), msg, fields...)
	}
}

func (l TfLogger) SetField(ctx context.Context, key string, value any) context.Context {
	if l == "" {
		return tflog.SetField(ctx, key, value)
	} else {
		return tflog.SubsystemSetField(ctx, string(l), key, value)
	}
}
