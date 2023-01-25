package awsbase

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httputil"
	"net/textproto"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsmiddleware "github.com/aws/aws-sdk-go-v2/aws/middleware"
	"github.com/aws/smithy-go/middleware"
	smithyhttp "github.com/aws/smithy-go/transport/http"
	"github.com/hashicorp/aws-sdk-go-base/v2/logging"
	"go.opentelemetry.io/otel/attribute"
	semconv "go.opentelemetry.io/otel/semconv/v1.12.0"
)

// type debugLogger struct{}

// func (l debugLogger) Logf(classification logging.Classification, format string, v ...interface{}) {
// 	s := fmt.Sprintf(format, v...)
// 	s = strings.ReplaceAll(s, "\r", "") // Works around https://github.com/jen20/teamcity-go-test/pull/2
// 	log.Printf("[%s] [aws-sdk-go-v2] %s", classification, s)
// }

// IAM Unique ID prefixes from
// https://docs.aws.amazon.com/IAM/latest/UserGuide/reference_identifiers.html#identifiers-unique-ids
var uniqueIDRegex = regexp.MustCompile(`(A3T[A-Z0-9]` +
	`|ABIA` + // STS service bearer token
	`|ACCA` + // Context-specific credential
	`|AGPA` + // User group
	`|AIDA` + // IAM user
	`|AIPA` + // EC2 instance profile
	`|AKIA` + // Access key
	`|ANPA` + // Managed policy
	`|ANVA` + // Version in a managed policy
	`|APKA` + // Public key
	`|AROA` + // Role
	`|ASCA` + // Certificate
	`|ASIA` + // STS temporary access key
	`)[A-Z0-9]{16,}`)

// Replaces the built-in logging middleware from https://github.com/aws/smithy-go/blob/main/transport/http/middleware_http_logging.go
// We want access to the request and response structs, and cannot get it from the built-in.
// The typical route of adding logging to the http.RoundTripper doesn't work for the AWS SDK for Go v2 without forcing us to manually implement
// configuration that the SDK handles for us.
type requestResponseLogger struct {
}

// ID is the middleware identifier.
func (r *requestResponseLogger) ID() string {
	return "TF_AWS_RequestResponseLogger"
}

func (r *requestResponseLogger) HandleDeserialize(ctx context.Context, in middleware.DeserializeInput, next middleware.DeserializeHandler,
) (
	out middleware.DeserializeOutput, metadata middleware.Metadata, err error,
) {
	logger := logging.RetrieveLogger(ctx)

	ctx = logger.SetField(ctx, "aws.sdk", "aws-sdk-go-v2")
	ctx = logger.SetField(ctx, "aws.service", awsmiddleware.GetServiceID(ctx))
	ctx = logger.SetField(ctx, "aws.operation", awsmiddleware.GetOperationName(ctx))

	region := awsmiddleware.GetRegion(ctx)
	ctx = logger.SetField(ctx, "aws.region", region)

	if signingRegion := awsmiddleware.GetSigningRegion(ctx); signingRegion != region {
		ctx = logger.SetField(ctx, "aws.signing_region", signingRegion)
	}

	if awsmiddleware.GetEndpointSource(ctx) == aws.EndpointSourceCustom {
		ctx = logger.SetField(ctx, "aws.custom_endpoint_source", true)
	}

	smithyRequest, ok := in.Request.(*smithyhttp.Request)
	if !ok {
		return out, metadata, fmt.Errorf("unknown request type %T", in.Request)
	}

	rc := smithyRequest.Build(ctx)

	requestFields, err := decomposeHTTPRequest(rc)
	if err != nil {
		return out, metadata, fmt.Errorf("decomposing request: %w", err)
	}
	logger.Debug(ctx, "HTTP Request Sent", requestFields)

	smithyRequest, err = smithyRequest.SetStream(rc.Body)
	if err != nil {
		return out, metadata, err
	}
	in.Request = smithyRequest

	start := time.Now()

	out, metadata, err = next.HandleDeserialize(ctx, in)

	elapsed := time.Since(start)

	if err == nil {
		smithyResponse, ok := out.RawResponse.(*smithyhttp.Response)
		if !ok {
			return out, metadata, fmt.Errorf("unknown response type: %T", out.RawResponse)
		}

		responseFields, err := decomposeHTTPResponse(smithyResponse.Response, elapsed)
		if err != nil {
			return out, metadata, fmt.Errorf("decomposing response: %w", err)
		}
		logger.Debug(ctx, "HTTP Response Received", responseFields)
	}

	return out, metadata, err
}

func decomposeHTTPRequest(req *http.Request) (map[string]any, error) {
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
	body = maskAWSAccessKey(body)

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
			if name != "SignedHeaders=" {
				// "Signature" or an unknown field
				value = "*****"
			}
			builder.WriteString(name)
			builder.WriteString(value)
			if i < len(components)-1 {
				builder.WriteString(", ")
			}
		}
		return key.String(fmt.Sprintf("%s %s", scheme, maskAWSAccessKey(builder.String()))), true
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

func decomposeHTTPResponse(resp *http.Response, elapsed time.Duration) (map[string]any, error) {
	var attributes []attribute.KeyValue

	attributes = append(attributes, attribute.Int64("http.duration", elapsed.Milliseconds()))

	attributes = append(attributes, semconv.HTTPAttributesFromHTTPStatusCode(resp.StatusCode)...)

	attributes = append(attributes, semconv.HTTPResponseContentLengthKey.Int64(resp.ContentLength))

	headerAttributes := decomposeResponseHeaders(resp)
	attributes = append(attributes, headerAttributes...)

	bodyAttribute, err := decomposeResponseBody(resp)
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

func decomposeResponseHeaders(resp *http.Response) []attribute.KeyValue {
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

func decomposeResponseBody(resp *http.Response) (attribute.KeyValue, error) {
	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return attribute.KeyValue{}, err
	}

	body := maskAWSAccessKey(string(respBytes))

	// Restore the body reader
	resp.Body = io.NopCloser(bytes.NewBuffer(respBytes))

	return attribute.String("http.response.body", body), nil
}

func maskAWSAccessKey(field string) string {
	field = uniqueIDRegex.ReplaceAllStringFunc(field, func(s string) string {
		return partialMaskString(s, 4, 4) //nolint:gomnd
	})
	return field
}

func partialMaskString(s string, first, last int) string {
	l := len(s)
	result := s[0:first]
	result += strings.Repeat("*", l-first-last)
	result += s[l-last:]
	return result
}
