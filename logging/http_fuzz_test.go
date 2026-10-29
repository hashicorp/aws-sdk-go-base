// Copyright IBM Corp. 2015, 2025
// SPDX-License-Identifier: MPL-2.0

package logging

import (
	"bufio"
	"bytes"
	"encoding/base64"
	"net/http"
	"reflect"
	"strings"
	"testing"
)

const (
	fuzzAccessKey = "AKIA5PX2H2S3LHEXAMPLE"
	fuzzSecretKey = "MfP3tIG15gibzIx7CSbhSNkgD5sSV4k2tWXgN8U8"
)

func FuzzAuthorizationHeaderAttribute(f *testing.F) {
	f.Add("AWS4-HMAC-SHA256 Credential=AKIA5PX2H2S3LHEXAMPLE, SignedHeaders=host, Signature=signature")
	f.Add("AWS4-HMAC-SHA256 Credential=AKIA5PX2H2S3LHEXAMPLE,")
	f.Add("Bearer secret-token")

	f.Fuzz(func(t *testing.T, header string) {
		// Arbitrary headers must never panic.
		_, _ = authorizationHeaderAttribute(header)

		const credential = "AKIA5PX2H2S3LHEXAMPLE"
		// Encoding keeps the fuzzed data in one syntactically valid parameter,
		// so every generated case exercises the sanitizer rather than rejection.
		parameter := base64.RawURLEncoding.EncodeToString([]byte(header))
		signature := "signature:" + parameter
		awsHeader := "AWS4-HMAC-SHA256 Credential=" + credential + ", SignedHeaders=host, Signature=" + signature
		got, ok := authorizationHeaderAttribute(awsHeader)
		if !ok {
			t.Fatal("authorizationHeaderAttribute() rejected a valid AWS4 header")
		}
		value := got.Value.AsString()
		if strings.Contains(value, signature) || strings.Contains(value, credential) {
			t.Fatalf("authorizationHeaderAttribute() leaked AWS4 data: %q", value)
		}

		nonAWSParameter := "parameter:" + parameter
		nonAWSHeader := "Bearer " + nonAWSParameter
		got, ok = authorizationHeaderAttribute(nonAWSHeader)
		if !ok {
			t.Fatal("authorizationHeaderAttribute() rejected a valid Bearer header")
		}
		if strings.Contains(got.Value.AsString(), nonAWSParameter) {
			t.Fatalf("authorizationHeaderAttribute() leaked non-AWS parameters: %q", got.Value.AsString())
		}
	})
}

func FuzzDecomposeRequestHeaders(f *testing.F) {
	f.Add("AWS4-HMAC-SHA256 Credential=AKIA5PX2H2S3LHEXAMPLE, SignedHeaders=host, Signature=signature", "attempt=2; max=3", "X-Example", "value", "token")
	f.Add("Bearer secret-token", "attempt=1;", "X-Example", "value", "")
	f.Add("Basic dXNlcjpwYXNz", "attempt=not-a-number;", "X-Example", "value", "token")
	f.Add("AWS4-HMAC-SHA256 Credential=AKIA5PX2H2S3LHEXAMPLE, Signature", "attempt=3;", "X-Example", "value", "")
	f.Add("AWS4-HMAC-SHA256 Credential=AKIA5PX2H2S3LHEXAMPLE,", "", "X-Example", "value", "")

	f.Fuzz(func(t *testing.T, authorization, retry, headerName, headerValue, token string) {
		// Raw authorization input exercises arbitrary parser input without
		// assuming every malformed value has a sanitized representation.
		_, _ = authorizationHeaderAttribute(authorization)
		authorization = "Bearer authorization:" + base64.RawURLEncoding.EncodeToString([]byte(authorization))
		header := make(http.Header)
		header.Set("Authorization", authorization)
		if retry != "" {
			header.Set("Amz-Sdk-Request", retry)
		}
		if headerName != "" {
			header[headerName] = []string{headerValue}
		}
		securityToken := "token:" + base64.RawURLEncoding.EncodeToString([]byte(token))
		if token != "" {
			header.Set("X-Amz-Security-Token", securityToken)
		}
		header.Set("Content-Length", "123")
		header.Set("User-Agent", "agent")

		before := header.Clone()
		first := attributeValues(decomposeRequestHeaders(&http.Request{Header: header}))
		second := attributeValues(decomposeRequestHeaders(&http.Request{Header: header}))
		if !reflect.DeepEqual(header, before) {
			t.Fatal("request header was modified")
		}
		if !reflect.DeepEqual(first, second) {
			t.Fatalf("decomposeRequestHeaders() is non-deterministic: %#v, %#v", first, second)
		}
		for _, key := range []string{"http.request.header.content_length", "http.request.header.user_agent"} {
			if _, ok := first[key]; ok {
				t.Fatalf("excluded header %q was emitted", key)
			}
		}
		if value, ok := first["http.request.header.authorization"]; ok && value == authorization {
			t.Fatalf("authorization leaked: %q", authorization)
		}
		if token != "" {
			if value, ok := first["http.request.header.x_amz_security_token"]; !ok || value != "*****" {
				t.Fatalf("security token attribute = %#v, want *****", value)
			}
		}
		for key := range first {
			if key == "" {
				t.Fatal("empty attribute key")
			}
		}
	})
}

func FuzzDecomposeResponseHeaders(f *testing.F) {
	f.Add("Content-Type", "application/json", "charset=utf-8", "123")
	f.Add("mIxEd-Case-Header", "value", "", "")
	f.Add("X-With-Hyphen", "first", "second", "")
	f.Add("X_With_Underscore", "value", "", "")
	f.Add("", "", "", "0")

	f.Fuzz(func(t *testing.T, headerName, firstValue, secondValue, contentLength string) {
		header := make(http.Header)
		if headerName != "" {
			header[headerName] = []string{firstValue}
			if secondValue != "" {
				header[headerName] = append(header[headerName], secondValue)
			}
		}
		if contentLength != "" {
			header.Set("Content-Length", contentLength)
		}

		before := header.Clone()
		first := attributeValues(DecomposeResponseHeaders(&http.Response{Header: header}))
		second := attributeValues(DecomposeResponseHeaders(&http.Response{Header: header}))
		if !reflect.DeepEqual(header, before) {
			t.Fatal("response header was modified")
		}
		if !reflect.DeepEqual(first, second) {
			t.Fatalf("DecomposeResponseHeaders() is non-deterministic: %#v, %#v", first, second)
		}
		if _, ok := first["http.response.header.content_length"]; ok {
			t.Fatal("content length was emitted")
		}
		for key := range first {
			if key == "" {
				t.Fatal("empty attribute key")
			}
		}
		if headerName != "" && !strings.EqualFold(headerName, "Content-Length") {
			key := responseHeaderAttributeName(headerName)
			got, ok := first[key]
			if !ok {
				t.Fatalf("header attribute %q was omitted", key)
			}
			if secondValue == "" {
				if _, ok := got.(string); !ok {
					t.Fatalf("single header type = %T, want string", got)
				}
			} else if _, ok := got.([]string); !ok {
				t.Fatalf("multiple header type = %T, want []string", got)
			}
		}
	})
}

func FuzzReadTruncatedBody(f *testing.F) {
	seed := func(body []byte, readerBufferSize int) {
		f.Add(body, readerBufferSize)
	}

	seed(nil, 1)
	seed([]byte("without newline"), 2)
	seed([]byte("with newline\n"), 3)
	seed([]byte("first\nsecond\n"), 4)
	seed([]byte("first\r\nsecond\r\n"), 5)
	seed([]byte("final carriage return\r"), 6)
	seed([]byte(strings.Repeat("a", MaxResponseBodyLen*2)), 7)
	seed([]byte(strings.Repeat("a", maxRequestBodyLen-1)), 8)
	seed([]byte(strings.Repeat("a", maxRequestBodyLen)), 9)
	seed([]byte(strings.Repeat("a", maxRequestBodyLen+1)), 10)
	seed([]byte(strings.Repeat("a", MaxResponseBodyLen-1)), 11)
	seed([]byte(strings.Repeat("a", MaxResponseBodyLen)), 12)
	seed([]byte(strings.Repeat("a", MaxResponseBodyLen+1)), 13)
	seed([]byte(strings.Repeat("short\n", 200)), 14)
	seed([]byte(strings.Repeat("a", 8)+"\n"+strings.Repeat("b", maxRequestBodyLen*2)), 15)
	seed([]byte(strings.Repeat("a", 64)+"\n"+strings.Repeat("b", MaxResponseBodyLen*2)), 16)
	seed([]byte(strings.Repeat("a", 128)+"\n"), 1)
	seed([]byte("before="+fuzzAccessKey+";"), 17)
	seed([]byte("before!"+fuzzSecretKey+";"), 18)
	seed([]byte(strings.Repeat(".", maxRequestBodyLen)+fuzzAccessKey+";"), 19)
	seed([]byte(strings.Repeat(".", maxRequestBodyLen)+fuzzSecretKey+";"), 20)
	seed([]byte(strings.Repeat(".", maxRequestBodyLen-len(truncationMarker)-8)+fuzzAccessKey+";"), 21)
	seed([]byte(strings.Repeat(".", maxRequestBodyLen-len(truncationMarker)-8)+fuzzSecretKey+";"), 22)
	seed([]byte(strings.Repeat(".", bodyLogBufferSize(maxRequestBodyLen)-len(fuzzAccessKey))+fuzzAccessKey+";"), bodyLogBufferSize(maxRequestBodyLen))
	seed([]byte(strings.Repeat(".", bodyLogBufferSize(maxRequestBodyLen)-len(fuzzSecretKey))+fuzzSecretKey+";"), bodyLogBufferSize(maxRequestBodyLen))

	f.Fuzz(func(t *testing.T, body []byte, readerBufferSize int) {
		maxReaderBufferSize := bodyLogBufferSize(MaxResponseBodyLen) * 2
		// Exercise fragmented reads without allowing fuzzed sizes to allocate
		// impractically large reader buffers.
		readerBufferSize = int(uint(readerBufferSize)%uint(maxReaderBufferSize)) + 1
		normalized := fuzzNormalizeBody(body)
		expectedMasked := MaskAWSSensitiveValues(normalized)

		for _, maxLength := range []int{maxRequestBodyLen, MaxResponseBodyLen} {
			reader := bufio.NewReaderSize(bytes.NewReader(body), readerBufferSize)
			got, err := ReadTruncatedBody(reader, maxLength)
			if err != nil {
				t.Fatalf("ReadTruncatedBody() error = %v", err)
			}

			// The output must never be longer than the limit.
			if len(got) > maxLength {
				t.Fatalf("ReadTruncatedBody() length = %d, limit = %d", len(got), maxLength)
			}

			// Secret values must never appear unmasked.
			for _, credential := range []string{fuzzAccessKey, fuzzSecretKey} {
				if !strings.Contains(expectedMasked, credential) && strings.Contains(got, credential) {
					t.Fatalf("ReadTruncatedBody() contains an unmasked credential: %q", got)
				}
			}

			if len(normalized) > maxLength {
				want := expectedMasked[:maxLength-len(truncationMarker)] + truncationMarker
				if got != want {
					t.Fatalf("ReadTruncatedBody() = %q, want truncated body %q", got, want)
				}
			} else {
				// When not truncated, the entire body must be returned with proper normalization and masking.
				if got != expectedMasked {
					t.Fatalf("ReadTruncatedBody() = %q, want complete body %q", got, expectedMasked)
				}
			}
		}
	})
}

// fuzzNormalizeBody mirrors the complete-body path: CRLF becomes LF, existing
// LF is retained, and a non-empty final line without LF receives one.
func fuzzNormalizeBody(body []byte) string {
	if len(body) == 0 {
		return ""
	}

	var normalized bytes.Buffer
	for len(body) > 0 {
		line, rest, found := bytes.Cut(body, []byte{'\n'})
		if !found {
			normalized.Write(line)
			normalized.WriteByte('\n')
			break
		}
		if len(line) > 0 && line[len(line)-1] == '\r' {
			line = line[:len(line)-1]
		}
		normalized.Write(line)
		normalized.WriteByte('\n')
		body = rest
	}

	return normalized.String()
}
