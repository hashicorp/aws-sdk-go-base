// Copyright IBM Corp. 2015, 2025
// SPDX-License-Identifier: MPL-2.0

package logging

import (
	"io"
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
