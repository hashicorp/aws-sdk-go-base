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
		req := &http.Request{
			Method: http.MethodPost,
			URL:    &url.URL{Scheme: "https", Host: "example.com", Path: "/"},
			Header: make(http.Header),
			Body:   io.NopCloser(strings.NewReader(bodyStr)),
		}
		var attrs []attribute.KeyValue
		if err := logger.Log(b.Context(), req, &attrs); err != nil {
			b.Fatal(err)
		}
	}
}
