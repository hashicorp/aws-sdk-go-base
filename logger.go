// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package awsbase

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/textproto"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsmiddleware "github.com/aws/aws-sdk-go-v2/aws/middleware"
	smithylogging "github.com/aws/smithy-go/logging"
	"github.com/aws/smithy-go/middleware"
	smithyhttp "github.com/aws/smithy-go/transport/http"
	"github.com/hashicorp/aws-sdk-go-base/v2/logging"
	"go.opentelemetry.io/contrib/instrumentation/github.com/aws/aws-sdk-go-v2/otelaws"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/semconv/v1.17.0/httpconv"
)

type debugLogger struct {
	ctx context.Context
}

func (l debugLogger) Logf(classification smithylogging.Classification, format string, v ...interface{}) {
	s := fmt.Sprintf(format, v...)
	if l.ctx != nil {
		logger := logging.RetrieveLogger(l.ctx)
		switch classification {
		case smithylogging.Debug:
			logger.Debug(l.ctx, s)
		case smithylogging.Warn:
			logger.Warn(l.ctx, s)
		}
	} else {
		s = strings.ReplaceAll(s, "\r", "") // Works around https://github.com/jen20/teamcity-go-test/pull/2
		log.Printf("[%s] missing_context: %s "+string(logging.AwsSdkKey)+"="+awsSdkGoV2Val, classification, s)
	}
}

func (l debugLogger) WithContext(ctx context.Context) smithylogging.Logger {
	return &debugLogger{
		ctx: ctx,
	}
}

const awsSdkGoV2Val = "aws-sdk-go-v2"

func awsSDKv2Attr() attribute.KeyValue {
	return logging.AwsSdkKey.String(awsSdkGoV2Val)
}

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

	region := awsmiddleware.GetRegion(ctx)

	attributes := []attribute.KeyValue{
		otelaws.SystemAttr(),
		otelaws.ServiceAttr(awsmiddleware.GetServiceID(ctx)),
		otelaws.RegionAttr(region),
		otelaws.OperationAttr(awsmiddleware.GetOperationName(ctx)),
		awsSDKv2Attr(),
	}
	if signingRegion := awsmiddleware.GetSigningRegion(ctx); signingRegion != region {
		attributes = append(attributes, logging.SigningRegion(signingRegion))
	}
	if awsmiddleware.GetEndpointSource(ctx) == aws.EndpointSourceCustom {
		attributes = append(attributes, logging.CustomEndpoint(true))
	}

	for _, attribute := range attributes {
		ctx = logger.SetField(ctx, string(attribute.Key), attribute.Value.AsInterface())
	}

	smithyRequest, ok := in.Request.(*smithyhttp.Request)
	if !ok {
		return out, metadata, fmt.Errorf("unknown request type %T", in.Request)
	}

	rc := smithyRequest.Build(ctx)

	requestFields, err := logging.DecomposeHTTPRequest(ctx, rc)
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

func decomposeHTTPResponse(resp *http.Response, elapsed time.Duration) (map[string]any, error) {
	var attributes []attribute.KeyValue

	attributes = append(attributes, attribute.Int64("http.duration", elapsed.Milliseconds()))

	attributes = append(attributes, httpconv.ClientResponse(resp)...)

	attributes = append(attributes, logging.DecomposeResponseHeaders(resp)...)

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

func decomposeResponseBody(resp *http.Response) (kv attribute.KeyValue, err error) {
	content, err := io.ReadAll(resp.Body)
	if err != nil {
		return kv, err
	}

	// Restore the body reader
	resp.Body = io.NopCloser(bytes.NewBuffer(content))

	reader := textproto.NewReader(bufio.NewReader(bytes.NewReader(content)))

	body, err := logging.ReadTruncatedBody(reader, logging.MaxResponseBodyLen)
	if err != nil {
		return kv, err
	}

	return attribute.String("http.response.body", body), nil
}
