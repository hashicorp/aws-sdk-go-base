// Copyright IBM Corp. 2015, 2026
// SPDX-License-Identifier: MPL-2.0

package awsbase

import (
	"io"
	"net/http"
	"strings"
	"testing"

	"go.opentelemetry.io/otel/attribute"
)

func TestDefaultResponseBodyLogger(t *testing.T) {
	t.Parallel()

	// Build a large body well over MaxResponseBodyLen (4096 bytes).
	// Each line is "abcdefgh\n" (9 bytes). 12,000 lines = 108,000 bytes.
	var largeBodyBuilder strings.Builder
	for range 12_000 {
		largeBodyBuilder.WriteString("abcdefgh\n")
	}
	largeBody := largeBodyBuilder.String()

	// ReadTruncatedBody writes lines until builder.Len() >= 4096, then appends
	// [truncated...].  9-byte lines: 456 lines = 4104 bytes ≥ 4096.
	var largeTruncatedBuilder strings.Builder
	for range 456 {
		largeTruncatedBuilder.WriteString("abcdefgh\n")
	}
	largeTruncatedBuilder.WriteString("[truncated...]")
	largeTruncated := largeTruncatedBuilder.String()

	tests := map[string]struct {
		responseBody    string
		expectedLogBody string
	}{
		"small_body": {
			responseBody:    "Action=ListUsers&Version=2010-05-08\n",
			expectedLogBody: "Action=ListUsers&Version=2010-05-08\n",
		},
		"large_body": {
			responseBody:    largeBody,
			expectedLogBody: largeTruncated,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			resp := &http.Response{
				Body: io.NopCloser(strings.NewReader(tc.responseBody)),
			}

			logger := &defaultResponseBodyLogger{}
			var attrs []attribute.KeyValue

			if err := logger.Log(t.Context(), resp, &attrs); err != nil {
				t.Fatalf("Log() error = %v", err)
			}

			// Check the logged body attribute.
			var gotBody string
			for _, kv := range attrs {
				if string(kv.Key) == "http.response.body" {
					gotBody = kv.Value.AsString()
					break
				}
			}
			if gotBody != tc.expectedLogBody {
				t.Errorf("http.response.body:\n got: %q\nwant: %q", gotBody, tc.expectedLogBody)
			}

			// Check that resp.Body is still fully readable (body restored).
			remaining, err := io.ReadAll(resp.Body)
			if err != nil {
				t.Fatalf("reading resp.Body after Log: %v", err)
			}
			if string(remaining) != tc.responseBody {
				t.Errorf("resp.Body after Log:\n got: %q\nwant: %q", string(remaining), tc.responseBody)
			}
		})
	}
}

func BenchmarkResponseBodyLogger(b *testing.B) {
	// Use a large body (50,000 lines = ~450 KB) to show the benefit of early
	// truncation: the scanner stops after ~456 lines but the old io.ReadAll
	// had to buffer the entire body.
	var bodyBuilder strings.Builder
	for range 50_000 {
		bodyBuilder.WriteString("abcdefgh\n")
	}
	bodyStr := bodyBuilder.String()

	logger := &defaultResponseBodyLogger{}

	b.ReportAllocs()
	for b.Loop() {
		b.StopTimer()
		resp := &http.Response{
			Body: io.NopCloser(strings.NewReader(bodyStr)),
		}
		b.StartTimer()

		var attrs []attribute.KeyValue
		if err := logger.Log(b.Context(), resp, &attrs); err != nil {
			b.Fatal(err)
		}
		resp.Body.Close()
	}
}

// TestDefaultResponseBodyLoggerPooledBufferAliasing mirrors
// TestDefaultRequestBodyLoggerPooledBufferAliasing in logging/http_test.go for
// the response side: Log() must not restore resp.Body from a buffer it has
// already handed back to BufferPool, or a later Get() corrupts a response that
// has not been deserialised yet.
func TestDefaultResponseBodyLoggerPooledBufferAliasing(t *testing.T) {
	t.Parallel()

	const (
		firstBody  = `{"GetPolicyResult":{"Policy":{"PolicyName":"first"}}}`
		secondBody = `{"GetCallerIdentityResult":{"Account":"280735953869","Arn":"arn:aws:sts::280735953869:assumed-role/second"}}`
	)

	newResponse := func(body string) *http.Response {
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(body)),
		}
	}

	logger := &defaultResponseBodyLogger{}

	first := newResponse(firstBody)
	var firstAttrs []attribute.KeyValue
	if err := logger.Log(t.Context(), first, &firstAttrs); err != nil {
		t.Fatalf("Log(first) error = %v", err)
	}

	second := newResponse(secondBody)
	var secondAttrs []attribute.KeyValue
	if err := logger.Log(t.Context(), second, &secondAttrs); err != nil {
		t.Fatalf("Log(second) error = %v", err)
	}

	gotFirst, err := io.ReadAll(first.Body)
	if err != nil {
		t.Fatalf("ReadAll(first.Body) error = %v", err)
	}
	if string(gotFirst) != firstBody {
		t.Errorf("first response body corrupted after a second Log() call\n got: %q\nwant: %q", gotFirst, firstBody)
	}

	gotSecond, err := io.ReadAll(second.Body)
	if err != nil {
		t.Fatalf("ReadAll(second.Body) error = %v", err)
	}
	if string(gotSecond) != secondBody {
		t.Errorf("second response body corrupted\n got: %q\nwant: %q", gotSecond, secondBody)
	}
}
