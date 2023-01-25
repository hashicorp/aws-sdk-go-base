package logging

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httputil"
	"net/textproto"
	"regexp"
	"strconv"
	"strings"

	"go.opentelemetry.io/otel/attribute"
	semconv "go.opentelemetry.io/otel/semconv/v1.4.0"
)

func DecomposeHTTPRequest(req *http.Request) (map[string]any, error) {
	var attributes []attribute.KeyValue

	attributes = append(attributes, semconv.HTTPClientAttributesFromHTTPRequest(req)...)

	headerAttributes := decomposeRequestHeaders(req)
	attributes = append(attributes, headerAttributes...)

	bodyAttribute, err := decomposeRequestBody(req)
	if err != nil {
		return nil, err
	}
	attributes = append(attributes, bodyAttribute)

	result := make(map[string]any, len(attributes))
	for _, attribute := range attributes {
		result[string(attribute.Key)] = attribute.Value.AsInterface()
	}

	return result, nil
}

func decomposeRequestHeaders(req *http.Request) []attribute.KeyValue {
	header := req.Header.Clone()

	// Handled directly from the Request
	header.Del("Content-Length")
	header.Del("User-Agent")

	results := make([]attribute.KeyValue, 0, len(header)+1)

	attempt := header.Values("Amz-Sdk-Request")
	if len(attempt) > 0 {
		if resendAttribute, ok := resendCountAttribute(attempt[0]); ok {
			results = append(results, resendAttribute)
		}
	}

	auth := header.Values("Authorization")
	if len(auth) > 0 {
		if authHeader, ok := authorizationHeaderAttribute(auth[0]); ok {
			results = append(results, authHeader)
		}
	}
	header.Del("Authorization")

	securityToken := header.Values("X-Amz-Security-Token")
	if len(securityToken) > 0 {
		results = append(results, requestHeaderAttribute("X-Amz-Security-Token").String("*****"))
	}
	header.Del("X-Amz-Security-Token")

	for k := range header {
		results = append(results, newRequestHeaderAttribute(k, header.Values(k)))
	}

	return results
}

func decomposeRequestBody(req *http.Request) (attribute.KeyValue, error) {
	reqBytes, err := httputil.DumpRequestOut(req, true)
	if err != nil {
		return attribute.KeyValue{}, err
	}

	reader := textproto.NewReader(bufio.NewReader(bytes.NewReader(reqBytes)))

	if _, err = reader.ReadLine(); err != nil {
		return attribute.KeyValue{}, err
	}

	if _, err = reader.ReadMIMEHeader(); err != nil {
		return attribute.KeyValue{}, err
	}

	var builder strings.Builder
	for {
		line, err := reader.ReadContinuedLine()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return attribute.KeyValue{}, err
		}
		builder.WriteString(line)
	}

	body := builder.String()
	body = MaskAWSAccessKey(body)

	return attribute.String("http.request.body", body), nil
}

func newRequestHeaderAttribute(k string, v []string) attribute.KeyValue {
	key := requestHeaderAttribute(k)
	if len(v) == 1 {
		return key.String(v[0])
	} else {
		return key.StringSlice(v)
	}
}

func requestHeaderAttribute(k string) attribute.Key {
	return attribute.Key(requestHeaderAttributeName(k))
}

func requestHeaderAttributeName(k string) string {
	return fmt.Sprintf("http.request.header.%s", normalizeHeaderName(k))
}

func normalizeHeaderName(k string) string {
	canonical := http.CanonicalHeaderKey(k)
	lower := strings.ToLower(canonical)
	return strings.ReplaceAll(lower, "-", "_")
}

func authorizationHeaderAttribute(v string) (attribute.KeyValue, bool) {
	parts := regexp.MustCompile(`\s+`).Split(v, 2) //nolint:gomnd
	if len(parts) != 2 {                           //nolint:gomnd
		return attribute.KeyValue{}, false
	}
	scheme := parts[0]
	if scheme == "" {
		return attribute.KeyValue{}, false
	}
	params := parts[1]
	if params == "" {
		return attribute.KeyValue{}, false
	}

	key := requestHeaderAttribute("Authorization")
	if strings.HasPrefix(scheme, "AWS4-") {
		components := regexp.MustCompile(`,\s+`).Split(params, -1)
		var builder strings.Builder
		builder.Grow(len(params))
		for i, component := range components {
			parts := strings.SplitAfterN(component, "=", 2)
			name := parts[0]
			value := parts[1]
			if name != "SignedHeaders=" && name != "Credential=" {
				// "Signature" or an unknown field
				value = "*****"
			}
			builder.WriteString(name)
			builder.WriteString(value)
			if i < len(components)-1 {
				builder.WriteString(", ")
			}
		}
		return key.String(fmt.Sprintf("%s %s", scheme, MaskAWSAccessKey(builder.String()))), true
	} else {
		return key.String(fmt.Sprintf("%s %s", scheme, strings.Repeat("*", len(params)))), true
	}
}

func resendCountAttribute(v string) (kv attribute.KeyValue, ok bool) {
	re := regexp.MustCompile(`attempt=(\d+);`)
	match := re.FindStringSubmatch(v)
	if len(match) != 2 { //nolint:gomnd
		return
	}

	attempt, err := strconv.Atoi(match[1])
	if err != nil {
		return
	}

	if attempt > 1 {
		return attribute.Int("http.resend_count", attempt), true
	}

	return
}

func DecomposeResponseHeaders(resp *http.Response) []attribute.KeyValue {
	header := resp.Header.Clone()

	// Handled directly from the Response
	header.Del("Content-Length")

	results := make([]attribute.KeyValue, 0, len(header)+1)

	for k := range header {
		results = append(results, newResponseHeaderAttribute(k, header.Values(k)))
	}

	return results
}

func newResponseHeaderAttribute(k string, v []string) attribute.KeyValue {
	key := requestResponseAttribute(k)
	if len(v) == 1 {
		return key.String(v[0])
	} else {
		return key.StringSlice(v)
	}
}

func requestResponseAttribute(k string) attribute.Key {
	return attribute.Key(requestResponseAttributeName(k))
}

func requestResponseAttributeName(k string) string {
	return fmt.Sprintf("http.response.header.%s", normalizeHeaderName(k))
}
