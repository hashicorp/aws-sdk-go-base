// Copyright IBM Corp. 2015, 2026
// SPDX-License-Identifier: MPL-2.0

package awsbase

import (
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/hashicorp/aws-sdk-go-base/v2/logging"
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

	const truncationMarker = "[truncated...]"
	largeTruncated := largeBody[:logging.MaxResponseBodyLen-len(truncationMarker)] + truncationMarker
	longLineBody := strings.Repeat("a", 128*1024)
	const (
		secretKey = "MfP3tIG15gibzIx7CSbhSNkgD5sSV4k2tWXgN8U8"
		accessKey = "AKIA5PX2H2S3LHEXAMPLE"
	)
	secretAtBoundary := strings.Repeat(".", logging.MaxResponseBodyLen-len(truncationMarker)-8) + secretKey + ";"
	accessKeyAtBoundary := strings.Repeat(".", logging.MaxResponseBodyLen-len(truncationMarker)-8) + accessKey + ";"

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
		"long_line": {
			responseBody:    longLineBody,
			expectedLogBody: strings.Repeat("a", logging.MaxResponseBodyLen-len(truncationMarker)) + truncationMarker,
		},
		"secret_key_crosses_truncation_boundary": {
			responseBody:    secretAtBoundary,
			expectedLogBody: strings.Repeat(".", logging.MaxResponseBodyLen-len(truncationMarker)-8) + "MfP3****" + truncationMarker,
		},
		"access_key_crosses_truncation_boundary": {
			responseBody:    accessKeyAtBoundary,
			expectedLogBody: strings.Repeat(".", logging.MaxResponseBodyLen-len(truncationMarker)-8) + "AKIA****" + truncationMarker,
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

func TestDefaultResponseBodyLoggerPooledBufferNotOverwritten(t *testing.T) {
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

	// The sequence below is sequential so that it will always trigger an error if the buffer returned to
	// BufferPool by the first Log() is overwritten by the second Log() before the first request's body is consumed.

	// Log the first response.
	first := newResponse(firstBody)
	var firstAttrs []attribute.KeyValue
	if err := logger.Log(t.Context(), first, &firstAttrs); err != nil {
		t.Fatalf("Log(first) error = %v", err)
	}

	// Log a second response before the first one's body has been consumed.
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
