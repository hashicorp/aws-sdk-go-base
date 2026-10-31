// Copyright IBM Corp. 2015, 2025
// SPDX-License-Identifier: MPL-2.0

package logging

import (
	"bufio"
	"bytes"
	"encoding/base64"
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
