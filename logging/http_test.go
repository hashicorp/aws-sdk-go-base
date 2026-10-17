// Copyright IBM Corp. 2015, 2025
// SPDX-License-Identifier: MPL-2.0

package logging

import (
	"bufio"
	"bytes"
	"context"
	"errors"
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

type countingBufferPool struct {
	buffer *bytes.Buffer
	gets   int
	puts   int
}

func (p *countingBufferPool) Get() *bytes.Buffer {
	p.gets++
	return p.buffer
}

func (p *countingBufferPool) Put(buffer *bytes.Buffer) {
	p.puts++
	buffer.Reset()
}

func (r *readerWithError) Read(p []byte) (int, error) {
	if len(r.data) == 0 {
		return 0, r.err
	}
	n := copy(p, r.data)
	r.data = r.data[n:]
	return n, r.err
}

func (r *readerWithError) Close() error {
	return nil
}

type stagedReader struct {
	stages [][]byte
	errs   []error
	reads  int
}

func (r *stagedReader) Read(p []byte) (int, error) {
	r.reads++
	if len(r.stages) == 0 {
		return 0, errors.New("unexpected read from paused stream")
	}

	n := copy(p, r.stages[0])
	err := r.errs[0]
	r.stages = r.stages[1:]
	r.errs = r.errs[1:]
	return n, err
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

func TestReadTruncatedBodyCRLF(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		body       string
		length     int
		readerSize int
		want       string
	}{
		"normal": {
			body:   "first\r\nsecond\r\n",
			length: maxRequestBodyLen,
			want:   "first\nsecond\n",
		},
		"unterminated_trailing_carriage_return": {
			body:   "first\r",
			length: maxRequestBodyLen,
			want:   "first\r\n",
		},
		"request_boundary": {
			body:       strings.Repeat("a", bodyLogBufferSize(maxRequestBodyLen)-1) + "\r\n",
			length:     MaxResponseBodyLen,
			readerSize: bodyLogBufferSize(maxRequestBodyLen),
			want:       strings.Repeat("a", bodyLogBufferSize(maxRequestBodyLen)-1) + "\n",
		},
		"response_boundary": {
			body:       strings.Repeat("a", bodyLogBufferSize(MaxResponseBodyLen)-1) + "\r\n",
			length:     bodyLogBufferSize(MaxResponseBodyLen),
			readerSize: bodyLogBufferSize(MaxResponseBodyLen),
			want:       strings.Repeat("a", bodyLogBufferSize(MaxResponseBodyLen)-1) + "\n",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			readerSize := tc.readerSize
			if readerSize == 0 {
				readerSize = bodyLogBufferSize(tc.length)
			}
			reader := bufio.NewReaderSize(strings.NewReader(tc.body), readerSize)
			got, err := ReadTruncatedBody(reader, tc.length)
			if err != nil {
				t.Fatalf("ReadTruncatedBody() error = %v", err)
			}
			if got != tc.want {
				t.Errorf("ReadTruncatedBody() = %q, want %q", got, tc.want)
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
		"terminated_with_trailing_data": append(
			[]byte(strings.Repeat("a", bodyLogBufferSize(maxRequestBodyLen)-2)),
			'\n', 'x',
		),
	}

	for name, data := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			reader, observedErr := newTrackedBodyReader(&readerWithError{
				data: data,
				err:  want,
			}, maxRequestBodyLen)

			_, err := readTruncatedBody(reader, maxRequestBodyLen, observedErr)
			if !errors.Is(err, want) {
				t.Fatalf("ReadTruncatedBody() error = %v, want %v", err, want)
			}
		})
	}
}

func TestReadTruncatedBodyReturnsWithoutReadingNextStage(t *testing.T) {
	t.Parallel()

	// The first stage fills the default buffer, contains all required lookahead,
	// and leaves a buffered tail. A second read represents a paused request body.
	firstStage := strings.Repeat("a", 1049) + "\n" + strings.Repeat("b", 15)
	staged := &stagedReader{stages: [][]byte{[]byte(firstStage)}, errs: []error{nil}}

	reader, observedErr := newTrackedBodyReader(staged, maxRequestBodyLen)
	got, err := readTruncatedBody(reader, maxRequestBodyLen, observedErr)
	if err != nil {
		t.Fatalf("ReadTruncatedBody() error = %v", err)
	}

	want := strings.Repeat("a", maxRequestBodyLen-len(truncationMarker)) + truncationMarker
	if got != want {
		t.Errorf("ReadTruncatedBody() = %q, want %q", got, want)
	}
	if staged.reads != 1 {
		t.Errorf("Read() calls = %d, want 1", staged.reads)
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
		"components_without_whitespace": {
			header: "AWS4-HMAC-SHA256 Credential=" + credential + ",SignedHeaders=host,Signature=" + signature,
			want:   "AWS4-HMAC-SHA256 Credential=AKIA*************MPLE, SignedHeaders=host, Signature=*****",
			ok:     true,
		},
		"component_whitespace": {
			header: "AWS4-HMAC-SHA256   Credential=" + credential + " ,  SignedHeaders=host  , Signature=" + signature + "  ",
			want:   "AWS4-HMAC-SHA256 Credential=AKIA*************MPLE, SignedHeaders=host, Signature=*****",
			ok:     true,
		},
		"unknown_component": {
			header: "AWS4-HMAC-SHA256 Credential=" + credential + ", Extra=sensitive-value, SignedHeaders=host",
			want:   "AWS4-HMAC-SHA256 Credential=AKIA*************MPLE, Extra=*****, SignedHeaders=host",
			ok:     true,
		},
		"value_contains_equals": {
			header: "AWS4-HMAC-SHA256 Credential=" + credential + "=suffix, SignedHeaders=host=extra, Signature=" + signature + "=more",
			want:   "AWS4-HMAC-SHA256 Credential=AKIA*************MPLE=suffix, SignedHeaders=host=extra, Signature=*****",
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

func TestDefaultRequestBodyLoggerReturnsBufferOnReadError(t *testing.T) {
	t.Parallel()

	pool := &countingBufferPool{buffer: &bytes.Buffer{}}

	want := errors.New("read body")
	req := &http.Request{Body: &readerWithError{
		data: []byte("body"),
		err:  want,
	}}

	err := (&defaultRequestBodyLogger{pool: pool}).Log(t.Context(), req, nil)
	if !errors.Is(err, want) {
		t.Fatalf("Log() error = %v, want %v", err, want)
	}

	if pool.gets != 1 || pool.puts != 1 {
		t.Errorf("pool calls = Get:%d Put:%d, want Get:1 Put:1", pool.gets, pool.puts)
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
			pool := &countingBufferPool{buffer: &bytes.Buffer{}}

			req := &http.Request{
				Method: http.MethodPost,
				URL:    &url.URL{Scheme: "https", Host: "example.com", Path: "/"},
				Header: make(http.Header),
			}
			var original *testReadCloser
			if tc.requestBody == "" {
				req.Body = http.NoBody
			} else {
				original = &testReadCloser{Reader: strings.NewReader(tc.requestBody)}
				req.Body = original
			}

			logger := &defaultRequestBodyLogger{pool: pool}
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

			if original == nil {
				return
			}

			if err := req.Body.Close(); err != nil {
				t.Fatalf("first req.Body.Close() error = %v", err)
			}
			if pool.puts != 1 {
				t.Errorf("pool Put() calls after first Close() = %d, want 1", pool.puts)
			}
			if original.closeCalls != 1 {
				t.Errorf("original Close() calls after first Close() = %d, want 1", original.closeCalls)
			}

			if err := req.Body.Close(); err != nil {
				t.Fatalf("second req.Body.Close() error = %v", err)
			}
			if pool.puts != 1 {
				t.Errorf("pool Put() calls after second Close() = %d, want 1", pool.puts)
			}
			if original.closeCalls != 1 {
				t.Errorf("original Close() calls after second Close() = %d, want 1", original.closeCalls)
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
