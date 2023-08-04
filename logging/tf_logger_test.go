// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package logging

import (
	"bytes"
	"context"
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-log/tflogtest"
)

func TestTfLoggerWarn(t *testing.T) {
	var buf bytes.Buffer
	ctx := tflogtest.RootLogger(context.Background(), &buf)

	loggerName := "test"
	expectedModule := fmt.Sprintf("provider.%s", loggerName)
	ctx, logger := NewTfLogger(ctx, loggerName)

	logger.Warn(ctx, "message", map[string]any{
		"one": int(1),
		"two": "two",
	})

	lines, err := tflogtest.MultilineJSONDecode(&buf)
	if err != nil {
		t.Fatalf("decoding log lines: %s", err)
	}

	if l := len(lines); l != 1 {
		t.Fatalf("expected 1 log entry, got %d\n%v", l, lines)
	}

	line := lines[0]
	if a, e := line["@level"], "warn"; a != e {
		t.Errorf("expected module %q, got %q", e, a)
	}
	if a, e := line["@module"], expectedModule; a != e {
		t.Errorf("expected module %q, got %q", e, a)
	}
	if a, e := line["@message"], "message"; a != e {
		t.Errorf("expected message %q, got %q", e, a)
	}
	if a, e := line["one"], float64(1); a != e {
		t.Errorf("expected field \"one\" %v, got %v", e, a)
	}
	if a, e := line["two"], "two"; a != e {
		t.Errorf("expected field \"two\" %q, got %q", e, a)
	}
}

func TestTfLoggerSetField(t *testing.T) {
	var buf bytes.Buffer
	ctx := tflogtest.RootLogger(context.Background(), &buf)

	loggerName := "test"
	ctx, logger := NewTfLogger(ctx, loggerName)

	ctxWithField := logger.SetField(ctx, "key", "value")

	logger.Warn(ctxWithField, "first")
	logger.Warn(ctxWithField, "second", map[string]any{
		"key": "other value",
	})

	lines, err := tflogtest.MultilineJSONDecode(&buf)
	if err != nil {
		t.Fatalf("ctxWithField: decoding log lines: %s", err)
	}

	line := lines[0]
	if a, e := line["key"], "value"; a != e {
		t.Errorf("expected field \"key\" %q, got %q", e, a)
	}

	line = lines[1]
	if a, e := line["key"], "other value"; a != e {
		t.Errorf("expected field \"key\" %q, got %q", e, a)
	}

	// logger.SetField does not affect the original context
	logger.Warn(ctx, "no fields")

	lines, err = tflogtest.MultilineJSONDecode(&buf)
	if err != nil {
		t.Fatalf("ctx: decoding log lines: %s", err)
	}

	line = lines[0]
	if val, ok := line["key"]; ok {
		t.Errorf("expected field \"key\" to  not be set, got %q", val)
	}
}
