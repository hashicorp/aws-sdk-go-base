// Copyright IBM Corp. 2015, 2026
// SPDX-License-Identifier: MPL-2.0

package logging

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"go.opentelemetry.io/otel/attribute"
)

type readerWithError struct {
	data []byte
	err  error
}

func (r *readerWithError) Read(p []byte) (int, error) {
	if len(r.data) == 0 {
		return 0, r.err
	}
	n := copy(p, r.data)
	r.data = r.data[n:]
	return n, r.err
}

func TestReadTruncatedBodySmallLimit(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		body   string
		length int
		want   string
	}{
		"zero": {
			body:   "body",
			length: 0,
			want:   "",
		},
		"less_than_marker": {
			body:   strings.Repeat("a", len(truncationMarker)),
			length: len(truncationMarker) - 1,
			want:   truncationMarker[:len(truncationMarker)-1],
		},
		"marker_length": {
			body:   strings.Repeat("a", len(truncationMarker)+1),
			length: len(truncationMarker),
			want:   truncationMarker,
		},
		"full_body_fits": {
			body:   "body",
			length: len(truncationMarker) - 1,
			want:   "body\n",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			got, err := ReadTruncatedBody(bufio.NewReader(strings.NewReader(tc.body)), tc.length)
			if err != nil {
				t.Fatalf("ReadTruncatedBody() error = %v", err)
			}
			if got != tc.want {
				t.Errorf("ReadTruncatedBody() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestReadTruncatedBodyCumulativeLimit(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		body   string
		length int
	}{
		"request_mixed_lines": {
			body:   strings.Repeat("a", 20) + "\n" + strings.Repeat("b", 2*maxRequestBodyLen),
			length: maxRequestBodyLen,
		},
		"response_mixed_lines": {
			body:   strings.Repeat("a", 20) + "\n" + strings.Repeat("b", 2*MaxResponseBodyLen),
			length: MaxResponseBodyLen,
		},
		"request_unterminated_final_line": {
			body:   strings.Repeat("a", maxRequestBodyLen-10) + "\n" + "short-final-line",
			length: maxRequestBodyLen,
		},
		"response_unterminated_final_line": {
			body:   strings.Repeat("a", MaxResponseBodyLen-10) + "\n" + "short-final-line",
			length: MaxResponseBodyLen,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			got, err := ReadTruncatedBody(bufio.NewReader(strings.NewReader(tc.body)), tc.length)
			if err != nil {
				t.Fatalf("ReadTruncatedBody() error = %v", err)
			}

			want := tc.body[:tc.length-len(truncationMarker)] + truncationMarker
			if got != want {
				t.Errorf("ReadTruncatedBody() = %q, want %q", got, want)
			}
			if len(got) != tc.length {
				t.Errorf("ReadTruncatedBody() length = %d, want %d", len(got), tc.length)
			}
		})
	}
}

func TestReadTruncatedBodyReturnsReaderErrorAfterTruncation(t *testing.T) {
	t.Parallel()

	want := errors.New("read body")
	tests := map[string][]byte{
		"unterminated": []byte(strings.Repeat("a", bodyLogBufferSize(maxRequestBodyLen))),
		"terminated": append(
			[]byte(strings.Repeat("a", bodyLogBufferSize(maxRequestBodyLen)-1)),
			'\n',
		),
	}

	for name, data := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			reader := bufio.NewReaderSize(&readerWithError{
				data: data,
				err:  want,
			}, bodyLogBufferSize(maxRequestBodyLen))

			_, err := ReadTruncatedBody(reader, maxRequestBodyLen)
			if !errors.Is(err, want) {
				t.Fatalf("ReadTruncatedBody() error = %v, want %v", err, want)
			}
		})
	}
}

func TestAuthorizationHeaderAttribute(t *testing.T) {
	t.Parallel()

	const (
		credential = "AKIA5PX2H2S3LHEXAMPLE"
		signature  = "0123456789abcdef"
	)
	tests := map[string]struct {
		header string
		want   string
		ok     bool
	}{
		"valid_sigv4": {
			header: "AWS4-HMAC-SHA256 Credential=" + credential + ", SignedHeaders=host;x-amz-date, Signature=" + signature,
			want:   "AWS4-HMAC-SHA256 Credential=AKIA*************MPLE, SignedHeaders=host;x-amz-date, Signature=*****",
			ok:     true,
		},
		"non_aws_scheme": {
			header: "Bearer secret-token",
			want:   "Bearer ************",
			ok:     true,
		},
		"missing_equals": {
			header: "AWS4-HMAC-SHA256 Credential=" + credential + ", Signature",
		},
		"trailing_comma": {
			header: "AWS4-HMAC-SHA256 Credential=" + credential + ",",
		},
		"empty_component_name": {
			header: "AWS4-HMAC-SHA256 =value",
		},
		"empty_values": {
			header: "AWS4-HMAC-SHA256 Credential=, SignedHeaders=, Signature=",
			want:   "AWS4-HMAC-SHA256 Credential=, SignedHeaders=, Signature=*****",
			ok:     true,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			got, ok := authorizationHeaderAttribute(tc.header)
			if ok != tc.ok {
				t.Fatalf("authorizationHeaderAttribute() ok = %t, want %t", ok, tc.ok)
			}
			if ok && got.Value.AsString() != tc.want {
				t.Errorf("authorizationHeaderAttribute() = %q, want %q", got.Value.AsString(), tc.want)
			}
		})
	}
}

func TestDecomposeRequestHeadersSensitiveValues(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		authorization string
		wantAuth      string
	}{
		"valid": {
			authorization: "AWS4-HMAC-SHA256 Credential=AKIA5PX2H2S3LHEXAMPLE, SignedHeaders=host, Signature=signature",
			wantAuth:      "AWS4-HMAC-SHA256 Credential=AKIA*************MPLE, SignedHeaders=host, Signature=*****",
		},
		"missing_equals": {
			authorization: "AWS4-HMAC-SHA256 Credential=AKIA5PX2H2S3LHEXAMPLE, Signature",
		},
		"trailing_comma": {
			authorization: "AWS4-HMAC-SHA256 Credential=AKIA5PX2H2S3LHEXAMPLE,",
		},
		"empty_component_name": {
			authorization: "AWS4-HMAC-SHA256 =sensitive-value",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			req := &http.Request{Header: http.Header{
				"Authorization":        {tc.authorization},
				"X-Amz-Security-Token": {"first-token", "second-token"},
				"Amz-Sdk-Request":      {"attempt=3; max=5", "attempt=9; max=10"},
			}}

			attrs := decomposeRequestHeaders(req)
			values := make(map[string]any, len(attrs))
			for _, attr := range attrs {
				values[string(attr.Key)] = attr.Value.AsInterface()
			}

			gotAuth, hasAuth := values["http.request.header.authorization"]
			if tc.wantAuth == "" {
				if hasAuth {
					t.Errorf("authorization = %q, want absent", gotAuth)
				}
			} else if gotAuth != tc.wantAuth {
				t.Errorf("authorization = %q, want %q", gotAuth, tc.wantAuth)
			}
			if got, want := values["http.request.header.x_amz_security_token"], "*****"; got != want {
				t.Errorf("security token = %q, want %q", got, want)
			}
			if got, want := values["http.resend_count"], int64(3); got != want {
				t.Errorf("resend count = %v, want %v", got, want)
			}
		})
	}
}

func TestDefaultRequestBodyLogger(t *testing.T) {
	t.Parallel()

	// Build a large body well over maxRequestBodyLen (1024 bytes).
	// Each line is "abcdefgh\n" (9 bytes). 12,000 lines = 108,000 bytes.
	var largeBodyBuilder strings.Builder
	for range 12_000 {
		largeBodyBuilder.WriteString("abcdefgh\n")
	}
	largeBody := largeBodyBuilder.String()

	largeTruncated := largeBody[:maxRequestBodyLen-len(truncationMarker)] + truncationMarker
	longLineBody := strings.Repeat("a", 128*1024)
	nearLimitBody := strings.Repeat("a", maxRequestBodyLen-3) + "\n"
	exactLimitTerminatedBody := strings.Repeat("a", maxRequestBodyLen-1) + "\n"
	exactLimitUnterminatedBody := strings.Repeat("a", maxRequestBodyLen-1)
	oneByteOverTerminatedBody := strings.Repeat("a", maxRequestBodyLen) + "\n"
	oneByteOverUnterminatedBody := strings.Repeat("a", maxRequestBodyLen)
	truncatedLimitBody := strings.Repeat("a", maxRequestBodyLen-len(truncationMarker)) + truncationMarker
	const (
		secretKey = "MfP3tIG15gibzIx7CSbhSNkgD5sSV4k2tWXgN8U8"
		accessKey = "AKIA5PX2H2S3LHEXAMPLE"
	)
	secretAtBoundary := strings.Repeat(".", maxRequestBodyLen-len(truncationMarker)-8) + secretKey + ";"
	accessKeyAtBoundary := strings.Repeat(".", maxRequestBodyLen-len(truncationMarker)-8) + accessKey + ";"

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
		"near_limit_body": {
			requestBody:     nearLimitBody,
			expectedLogBody: nearLimitBody,
		},
		"exact_limit_terminated_body": {
			requestBody:     exactLimitTerminatedBody,
			expectedLogBody: exactLimitTerminatedBody,
		},
		"exact_limit_unterminated_body": {
			requestBody:     exactLimitUnterminatedBody,
			expectedLogBody: exactLimitTerminatedBody,
		},
		"one_byte_over_terminated_body": {
			requestBody:     oneByteOverTerminatedBody,
			expectedLogBody: truncatedLimitBody,
		},
		"one_byte_over_unterminated_body": {
			requestBody:     oneByteOverUnterminatedBody,
			expectedLogBody: truncatedLimitBody,
		},
		"large_body": {
			requestBody:     largeBody,
			expectedLogBody: largeTruncated,
		},
		"long_line": {
			requestBody:     longLineBody,
			expectedLogBody: strings.Repeat("a", maxRequestBodyLen-len(truncationMarker)) + truncationMarker,
		},
		"secret_key_crosses_truncation_boundary": {
			requestBody:     secretAtBoundary,
			expectedLogBody: strings.Repeat(".", maxRequestBodyLen-len(truncationMarker)-8) + "MfP3****" + truncationMarker,
		},
		"access_key_crosses_truncation_boundary": {
			requestBody:     accessKeyAtBoundary,
			expectedLogBody: strings.Repeat(".", maxRequestBodyLen-len(truncationMarker)-8) + "AKIA****" + truncationMarker,
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

// Sinks hold benchmark results in package scope so the compiler cannot treat
// the benchmarked calls as dead code and eliminate them.
var (
	// headerAttributesSink receives the []attribute.KeyValue result of the
	// header-decomposition benchmarks.
	headerAttributesSink []attribute.KeyValue
	// attributeKeyValueSink receives the attribute returned by the helper benchmarks.
	attributeKeyValueSink attribute.KeyValue
	// boolSink receives the ok return value from the helper benchmarks.
	boolSink bool
)

// benchSignedHeader is a realistic SigV4-signed request: Authorization with an
// AWS4 scheme, a security token, and an Amz-Sdk-Request retry attempt.
var benchSignedHeader = http.Header{
	"Authorization":        {"AWS4-HMAC-SHA256 Credential=AKIA5PX2H2S3LHEXAMPLE/20231030/us-east-1/s3/aws4_request, SignedHeaders=host;x-amz-content-sha256;x-amz-date, Signature=aabbccddeeff00112233445566778899aabbccddeeff00112233445566778899"},
	"X-Amz-Security-Token": {"FwoGZXIvYXdzEXAMPLEsecuritytokenvalue"},
	"Amz-Sdk-Request":      {"attempt=2; max=3"},
	"X-Amz-Date":           {"20231030T120000Z"},
	"X-Amz-Content-Sha256": {"e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"},
	"Content-Type":         {"application/x-amz-json-1.0"},
	"User-Agent":           {"aws-sdk-go-v2/1.0"},
	"Content-Length":       {"42"},
}

// manyHeaders returns a header map with n distinct single-valued headers.
func manyHeaders(n int) http.Header {
	h := make(http.Header, n)
	for i := range n {
		h.Set(fmt.Sprintf("X-Custom-Header-%d", i), fmt.Sprintf("value-%d", i))
	}
	return h
}

// largeAuthHeader returns an AWS4 Authorization header whose SignedHeaders list
// is padded so the whole header value is at least targetBytes long. This
// stresses the bytes.Buffer / MaskAWSSensitiveValuesBytes path in
// authorizationHeaderAttribute, the worst case for memory in this function.
func largeAuthHeader(targetBytes int) http.Header {
	var signed strings.Builder
	signed.WriteString("host")
	for i := 0; signed.Len() < targetBytes; i++ {
		fmt.Fprintf(&signed, ";x-amz-meta-header-%d", i)
	}
	v := "AWS4-HMAC-SHA256 Credential=AKIA5PX2H2S3LHEXAMPLE/20231030/us-east-1/s3/aws4_request, SignedHeaders=" +
		signed.String() + ", Signature=aabbccddeeff00112233445566778899aabbccddeeff00112233445566778899"
	return http.Header{"Authorization": {v}}
}

// manyComponentAuthHeader returns an AWS4 Authorization header with n
// comma-separated components. authorizationHeaderAttribute splits the
// parameter string on commas and allocates per component, so this is the worst
// case for allocation count (as opposed to largeAuthHeader, which stresses
// total byte size via a single long field).
func manyComponentAuthHeader(n int) http.Header {
	components := make([]string, 0, n+2)
	components = append(components, "Credential=AKIA5PX2H2S3LHEXAMPLE/20231030/us-east-1/s3/aws4_request")
	for i := range n {
		components = append(components, fmt.Sprintf("Signature%d=aabbccddeeff00112233445566778899", i))
	}
	components = append(components, "SignedHeaders=host")
	v := "AWS4-HMAC-SHA256 " + strings.Join(components, ", ")
	return http.Header{"Authorization": {v}}
}

func BenchmarkDecomposeRequestHeaders(b *testing.B) {
	benchmarks := map[string]http.Header{
		"empty": {},
		"minimal": {
			"X-Custom-Header": {"value"},
		},
		"realistic_signed": benchSignedHeader,
		"security_token_only": {
			"X-Amz-Security-Token": {"FwoGZXIvYXdzEXAMPLEsecuritytokenvalue"},
		},
		"resend_only": {
			"Amz-Sdk-Request": {"attempt=2; max=3"},
		},
		// Header-count sweep (#4): shows how allocations scale with the number
		// of distinct headers.
		"many_headers_1":   manyHeaders(1),
		"many_headers_25":  manyHeaders(25),
		"many_headers_100": manyHeaders(100),
		// Authorization byte-size sweep (#3): shows how memory scales with the
		// length of a single long AWS4 field.
		"large_auth_256B": largeAuthHeader(256),
		"large_auth_1KiB": largeAuthHeader(1024),
		"large_auth_4KiB": largeAuthHeader(4 * 1024),
		// AWS4 component-count worst case (#5): many comma-separated components.
		"many_component_auth_50":  manyComponentAuthHeader(50),
		"many_component_auth_200": manyComponentAuthHeader(200),
		"multi_valued": {
			"X-Multiple":  {"first", "second", "third", "fourth"},
			"X-Multiple2": {"a", "b", "c", "d", "e", "f"},
		},
	}

	for name, header := range benchmarks {
		b.Run(name, func(b *testing.B) {
			// decomposeRequestHeaders clones req.Header internally and never
			// mutates the original, so a single request can be reused across
			// iterations without the input drifting between runs.
			req := &http.Request{Header: header}
			b.ReportAllocs()
			for b.Loop() {
				headerAttributesSink = decomposeRequestHeaders(req)
			}
		})
	}
}

// benchResponseHeader is a realistic AWS API response header set.
var benchResponseHeader = http.Header{
	"Content-Type":     {"application/x-amz-json-1.0"},
	"Content-Length":   {"1234"},
	"X-Amzn-Requestid": {"a1b2c3d4-5678-90ab-cdef-EXAMPLE11111"},
	"X-Amz-Id-2":       {"EXAMPLEidentifier2valuethatislongerthanmost=="},
	"Date":             {"Thu, 30 Oct 2026 12:00:00 GMT"},
	"Server":           {"AmazonS3"},
	"X-Amz-Request-Id": {"EXAMPLEREQUESTID1"},
	"Vary":             {"Accept-Encoding"},
}

func BenchmarkDecomposeResponseHeaders(b *testing.B) {
	benchmarks := map[string]http.Header{
		"empty": {},
		"minimal": {
			"Content-Type": {"application/json"},
		},
		"realistic": benchResponseHeader,
		// content_length_only exercises the header.Del("Content-Length") branch
		// unique to DecomposeResponseHeaders; it should produce near-empty output.
		"content_length_only": {
			"Content-Length": {"1234"},
		},
		// large_value_4KiB shows how memory scales with the length of a single
		// header value, independent of header count.
		"large_value_4KiB": {
			"X-Amz-Meta-Data": {strings.Repeat("a", 4*1024)},
		},
		// Header-count sweep: shows how allocations scale with header count.
		"many_headers_1":   manyHeaders(1),
		"many_headers_25":  manyHeaders(25),
		"many_headers_100": manyHeaders(100),
		// Multi-valued headers (e.g. several Set-Cookie values) are preserved as
		// string slices rather than collapsed by cleanUpHeaderAttributes.
		"multi_valued": {
			"Set-Cookie": {"a=1", "b=2", "c=3", "d=4"},
			"Vary":       {"Accept-Encoding", "Origin"},
		},
	}

	for name, header := range benchmarks {
		b.Run(name, func(b *testing.B) {
			// DecomposeResponseHeaders clones resp.Header internally and never
			// mutates the original, so a single response can be reused across
			// iterations without the input drifting between runs.
			resp := &http.Response{Header: header}
			b.ReportAllocs()
			for b.Loop() {
				headerAttributesSink = DecomposeResponseHeaders(resp)
			}
		})
	}
}

func TestResendCountAttribute(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		input      string
		expectedKV attribute.KeyValue
		expectedOK bool
	}{
		"typical retry": {
			input:      "attempt=3; max=5",
			expectedKV: attribute.Int("http.resend_count", 3),
			expectedOK: true,
		},
		"first attempt not logged": {
			input:      "attempt=1; max=5",
			expectedOK: false,
		},
		"large count": {
			input:      "attempt=100; max=200",
			expectedKV: attribute.Int("http.resend_count", 100),
			expectedOK: true,
		},
		"leading marker with prefix": {
			input:      "prefix attempt=2; suffix",
			expectedKV: attribute.Int("http.resend_count", 2),
			expectedOK: true,
		},
		"empty": {
			input:      "",
			expectedOK: false,
		},
		"no marker": {
			input:      "max=5",
			expectedOK: false,
		},
		"no digits": {
			input:      "attempt=not-a-number;",
			expectedOK: false,
		},
		"missing semicolon": {
			input:      "attempt=5",
			expectedOK: false,
		},
		"empty digits": {
			input:      "attempt=;",
			expectedOK: false,
		},
		"space before semicolon": {
			input:      "attempt=5 ;",
			expectedOK: false,
		},
		"case sensitive": {
			input:      "ATTEMPT=2;",
			expectedOK: false,
		},
		"overflow is ignored": {
			input:      "attempt=99999999999999999999999999;",
			expectedOK: false,
		},
		// The parser inspects only the first "attempt=" occurrence, so a
		// malformed first marker is not rescued by a later well-formed one.
		"only first marker inspected": {
			input:      "attempt=x; attempt=2;",
			expectedOK: false,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			gotKV, gotOK := resendCountAttribute(tc.input)
			if gotOK != tc.expectedOK {
				t.Fatalf("resendCountAttribute(%q) ok = %v, want %v", tc.input, gotOK, tc.expectedOK)
			}
			if tc.expectedOK && gotKV != tc.expectedKV {
				t.Errorf("resendCountAttribute(%q) = %v, want %v", tc.input, gotKV, tc.expectedKV)
			}
		})
	}
}

func BenchmarkResendCountAttribute(b *testing.B) {
	benchmarks := map[string]string{
		"match":     "attempt=3; max=5",
		"no_resend": "attempt=1; max=5",
		"malformed": "attempt=not-a-number;",
	}

	for name, v := range benchmarks {
		b.Run(name, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				attributeKeyValueSink, boolSink = resendCountAttribute(v)
			}
		})
	}
}

func BenchmarkAuthorizationHeaderAttribute(b *testing.B) {
	benchmarks := map[string]string{
		"aws4":                    benchSignedHeader.Get("Authorization"),
		"non_aws_bearer":          "Bearer bearer-token",
		"large_aws4_256B":         largeAuthHeader(256).Get("Authorization"),
		"large_aws4_1KiB":         largeAuthHeader(1024).Get("Authorization"),
		"large_aws4_4KiB":         largeAuthHeader(4 * 1024).Get("Authorization"),
		"many_component_aws4_50":  manyComponentAuthHeader(50).Get("Authorization"),
		"many_component_aws4_200": manyComponentAuthHeader(200).Get("Authorization"),
	}

	for name, v := range benchmarks {
		b.Run(name, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				attributeKeyValueSink, boolSink = authorizationHeaderAttribute(v)
			}
		})
	}
}

func BenchmarkCleanUpHeaderAttributes(b *testing.B) {
	// Build a mix of single-valued (collapsed) and multi-valued (preserved)
	// string-slice attributes, matching what httpconv.RequestHeader produces.
	attrs := make([]attribute.KeyValue, 0, 25)
	for i := range 25 {
		key := RequestHeaderAttributeKey(fmt.Sprintf("X-Custom-Header-%d", i))
		if i%5 == 0 {
			attrs = append(attrs, key.StringSlice([]string{"first", "second"}))
		} else {
			attrs = append(attrs, key.StringSlice([]string{fmt.Sprintf("value-%d", i)}))
		}
	}

	b.ReportAllocs()
	for b.Loop() {
		headerAttributesSink = cleanUpHeaderAttributes(attrs)
	}
}
