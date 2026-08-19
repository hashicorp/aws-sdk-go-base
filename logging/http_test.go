// Copyright IBM Corp. 2015, 2025
// SPDX-License-Identifier: MPL-2.0

package logging

import (
	"context"
	"io"
	"math"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"go.opentelemetry.io/otel/attribute"
)

func TestDefaultRequestBodyLogger(t *testing.T) {
	t.Parallel()

	// Build a large body well over maxRequestBodyLen (1024 bytes).
	// bufio.Scanner has a default buffer size of 4,096 bytes.
	// Each line is "abcdefgh\n" (9 bytes). 12,000 lines = 108,000 bytes.
	var largeBodyBuilder strings.Builder
	for range 12_000 {
		largeBodyBuilder.WriteString("abcdefgh\n")
	}
	largeBody := largeBodyBuilder.String()

	// ReadTruncatedBody writes lines until builder.Len() >= 1024, then appends
	// [truncated...].  9-byte lines: 114 lines = 1026 bytes ≥ 1024.
	var largeTruncatedBuilder strings.Builder
	for range 114 {
		largeTruncatedBuilder.WriteString("abcdefgh\n")
	}
	largeTruncatedBuilder.WriteString("[truncated...]")
	largeTruncated := largeTruncatedBuilder.String()

	tests := map[string]struct {
		requestBody     string
		expectedLogBody string
	}{
		"nil_body": {
			requestBody:     "",
			expectedLogBody: "",
		},
		"small_body": {
			requestBody:     "Action=ListUsers&Version=2010-05-08\n",
			expectedLogBody: "Action=ListUsers&Version=2010-05-08\n",
		},
		"large_body": {
			requestBody:     largeBody,
			expectedLogBody: largeTruncated,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			expectedFullBody := tc.requestBody

			req := &http.Request{
				Method: http.MethodPost,
				URL:    &url.URL{Scheme: "https", Host: "example.com", Path: "/"},
				Header: make(http.Header),
			}
			if tc.requestBody == "" {
				req.Body = http.NoBody
			} else {
				req.Body = io.NopCloser(strings.NewReader(tc.requestBody))
			}

			logger := &defaultRequestBodyLogger{}
			var attrs []attribute.KeyValue

			if err := logger.Log(t.Context(), req, &attrs); err != nil {
				t.Fatalf("Log() error = %v", err)
			}

			// Check the logged body attribute.
			var gotBody string
			for _, kv := range attrs {
				if string(kv.Key) == "http.request.body" {
					gotBody = kv.Value.AsString()
					break
				}
			}
			if gotBody != tc.expectedLogBody {
				t.Errorf("http.request.body:\n got: %q\nwant: %q", gotBody, tc.expectedLogBody)
			}

			// Check that req.Body is still fully readable (body restored).
			remaining, err := io.ReadAll(req.Body)
			if err != nil {
				t.Fatalf("reading req.Body after Log: %v", err)
			}
			if string(remaining) != expectedFullBody {
				t.Errorf("req.Body after Log:\n got: %q\nwant: %q", string(remaining), expectedFullBody)
			}
		})
	}
}

func BenchmarkRequestBodyLogger(b *testing.B) {
	// Use a body slightly larger than maxRequestBodyLen so the truncation path runs.
	var bodyBuilder strings.Builder
	for range 200 {
		bodyBuilder.WriteString("abcdefgh\n")
	}
	bodyStr := bodyBuilder.String()

	logger := &defaultRequestBodyLogger{}

	b.ReportAllocs()
	for b.Loop() {
		b.StopTimer()
		req := &http.Request{
			Method: http.MethodPost,
			URL:    &url.URL{Scheme: "https", Host: "example.com", Path: "/"},
			Header: make(http.Header),
			Body:   io.NopCloser(strings.NewReader(bodyStr)),
		}
		var attrs []attribute.KeyValue
		b.StartTimer()

		if err := logger.Log(b.Context(), req, &attrs); err != nil {
			b.Fatal(err)
		}
		req.Body.Close()
	}
}

func TestDefaultRequestBodyLoggerPooledBufferNotOverwritten(t *testing.T) {
	t.Parallel()

	const (
		firstBody  = "Action=GetPolicy&Version=2010-05-08\n"
		secondBody = "Action=GetCallerIdentity&Version=2011-06-15&Tag=platform-team\n"
	)

	newRequest := func(body string) *http.Request {
		return &http.Request{
			Method: http.MethodPost,
			URL:    &url.URL{Scheme: "https", Host: "iam.amazonaws.com", Path: "/"},
			Header: http.Header{"Content-Type": []string{"application/x-www-form-urlencoded"}},
			Body:   io.NopCloser(strings.NewReader(body)),
		}
	}

	logger := &defaultRequestBodyLogger{}

	// The sequence below is sequential so that it will always trigger an error if the buffer returned to
	// BufferPool by the first Log() is overwritten by the second Log() before the first request's body is consumed.

	// Log the first request.
	first := newRequest(firstBody)
	var firstAttrs []attribute.KeyValue
	if err := logger.Log(t.Context(), first, &firstAttrs); err != nil {
		t.Fatalf("Log(first) error = %v", err)
	}

	// Log a second request before the first one's body has been consumed.
	second := newRequest(secondBody)
	var secondAttrs []attribute.KeyValue
	if err := logger.Log(t.Context(), second, &secondAttrs); err != nil {
		t.Fatalf("Log(second) error = %v", err)
	}

	gotFirst, err := io.ReadAll(first.Body)
	if err != nil {
		t.Fatalf("ReadAll(first.Body) error = %v", err)
	}
	if string(gotFirst) != firstBody {
		t.Errorf("first request body corrupted after a second Log() call\n got: %q\nwant: %q", gotFirst, firstBody)
	}

	gotSecond, err := io.ReadAll(second.Body)
	if err != nil {
		t.Fatalf("ReadAll(second.Body) error = %v", err)
	}
	if string(gotSecond) != secondBody {
		t.Errorf("second request body corrupted\n got: %q\nwant: %q", gotSecond, secondBody)
	}
}

func TestFormatByteSize(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		size     int64
		expected string
	}{
		"unknown length":         {size: -1, expected: "unknown size"},
		"empty":                  {size: 0, expected: "0 bytes"},
		"bytes":                  {size: 512, expected: "512 bytes"},
		"largest in bytes":       {size: 1536, expected: "1,536 bytes"},
		"smallest in kilobytes":  {size: 1537, expected: "1.5 KB (1,537 bytes)"},
		"kilobytes":              {size: 2 * 1024, expected: "2.0 KB (2,048 bytes)"},
		"megabytes":              {size: 1024 * 1024, expected: "1.0 MB (1,048,576 bytes)"},
		"gigabytes":              {size: 1024 * 1024 * 1024, expected: "1.0 GB (1,073,741,824 bytes)"},
		"terabytes":              {size: 1024 * 1024 * 1024 * 1024, expected: "1.0 TB (1,099,511,627,776 bytes)"},
		"maximum S3 object size": {size: 5 * 1024 * 1024 * 1024 * 1024, expected: "5.0 TB (5,497,558,138,880 bytes)"},
		"petabytes":              {size: 1024 * 1024 * 1024 * 1024 * 1024, expected: "1.0 PB (1,125,899,906,842,624 bytes)"},
		"exabytes":               {size: 1024 * 1024 * 1024 * 1024 * 1024 * 1024, expected: "1.0 EB (1,152,921,504,606,846,976 bytes)"},
		"maximum int64":          {size: math.MaxInt64, expected: "8.0 EB (9,223,372,036,854,775,807 bytes)"},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			if a, e := formatByteSize(test.size), test.expected; a != e {
				t.Errorf("expected %q, got %q", e, a)
			}
		})
	}
}

func TestS3BodyRedacted(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		size        int64
		contentType string
		expected    string
	}{
		"with content type": {
			size:        1024 * 1024,
			contentType: "application/octet-stream",
			expected:    "[Redacted: 1.0 MB (1,048,576 bytes), Type: application/octet-stream]",
		},
		"without content type": {
			size:     1024 * 1024,
			expected: "[Redacted: 1.0 MB (1,048,576 bytes)]",
		},
		"unknown length": {
			size:        -1,
			contentType: "application/octet-stream",
			expected:    "[Redacted: unknown size, Type: application/octet-stream]",
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			if a, e := s3BodyRedacted(test.size, test.contentType), test.expected; a != e {
				t.Errorf("expected %q, got %q", e, a)
			}
		})
	}
}

func TestOutgoingLength(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		request  *http.Request
		expected int64
	}{
		"no body": {
			request:  &http.Request{},
			expected: 0,
		},
		"http.NoBody": {
			request:  &http.Request{Body: http.NoBody},
			expected: 0,
		},
		"known length": {
			request:  &http.Request{Body: io.NopCloser(strings.NewReader("body")), ContentLength: 4},
			expected: 4,
		},
		"unknown length": {
			request:  &http.Request{Body: io.NopCloser(strings.NewReader("body"))},
			expected: -1,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			if a, e := outgoingLength(test.request), test.expected; a != e {
				t.Errorf("expected %d, got %d", e, a)
			}
		})
	}
}

func TestS3ObjectBodyLoggers(t *testing.T) {
	t.Parallel()

	t.Run("request", func(t *testing.T) {
		t.Parallel()

		req := &http.Request{
			Body:          io.NopCloser(strings.NewReader("object contents")),
			ContentLength: 15,
			Header:        http.Header{"Content-Type": []string{"application/octet-stream"}},
		}

		var attrs []attribute.KeyValue
		logger := &s3ObjectRequestBodyLogger{}
		if err := logger.Log(context.Background(), req, &attrs); err != nil {
			t.Fatalf("unexpected error: %s", err)
		}

		expected := attribute.String("http.request.body", "[Redacted: 15 bytes, Type: application/octet-stream]")
		if len(attrs) != 1 || attrs[0] != expected {
			t.Errorf("expected %v, got %v", []attribute.KeyValue{expected}, attrs)
		}
	})

	t.Run("response", func(t *testing.T) {
		t.Parallel()

		resp := &http.Response{
			Body:          io.NopCloser(strings.NewReader("object contents")),
			ContentLength: 15,
			Header:        http.Header{"Content-Type": []string{"application/octet-stream"}},
		}

		var attrs []attribute.KeyValue
		logger := &S3ObjectResponseBodyLogger{}
		if err := logger.Log(context.Background(), resp, &attrs); err != nil {
			t.Fatalf("unexpected error: %s", err)
		}

		expected := attribute.String("http.response.body", "[Redacted: 15 bytes, Type: application/octet-stream]")
		if len(attrs) != 1 || attrs[0] != expected {
			t.Errorf("expected %v, got %v", []attribute.KeyValue{expected}, attrs)
		}
	})
}
