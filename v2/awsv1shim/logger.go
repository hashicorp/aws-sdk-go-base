package awsv1shim

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httputil"
	"net/textproto"
	"regexp"
	"strconv"
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/request"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"go.opentelemetry.io/otel/attribute"
	semconv "go.opentelemetry.io/otel/semconv/v1.12.0"
)

type debugLogger struct{}

func (l debugLogger) Log(args ...interface{}) {
	tokens := make([]string, 0, len(args))
	for _, arg := range args {
		if token, ok := arg.(string); ok {
			tokens = append(tokens, token)
		}
	}
	s := strings.Join(tokens, " ")
	s = strings.ReplaceAll(s, "\r", "") // Works around https://github.com/jen20/teamcity-go-test/pull/2
	log.Printf("[DEBUG] [aws-sdk-go] %s", s)
}

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

// Replaces the built-in logging middleware from https://github.com/aws/aws-sdk-go/blob/main/aws/client/logger.go
// We want access to the request struct, and cannot get it from the built-in.
// The typical route of adding logging to the http.RoundTripper doesn't work for the AWS SDK for Go v1 without forcing us to manually implement
// configuration that the SDK handles for us.
var requestLogger = request.NamedHandler{
	Name: "TF_AWS_RequestLogger",
	Fn:   logRequest,
}

func logRequest(r *request.Request) {
	ctx := r.Context()

	ctx = tflog.SetField(ctx, "aws.sdk", "aws-sdk-go")
	ctx = tflog.SetField(ctx, "aws.service", r.ClientInfo.ServiceID)
	ctx = tflog.SetField(ctx, "aws.operation", r.Operation.Name)

	region := aws.StringValue(r.Config.Region)
	ctx = tflog.SetField(ctx, "aws.region", region)

	if signingRegion := r.ClientInfo.SigningRegion; signingRegion != region {
		ctx = tflog.SetField(ctx, "aws.signing_region", signingRegion)
	}

	bodySeekable := aws.IsReaderSeekable(r.Body)

	requestFields, err := decomposeHTTPRequest(r.HTTPRequest)
	if err != nil {
		r.Config.Logger.Log(fmt.Sprintf(logReqErrMsg,
			r.ClientInfo.ServiceName, r.Operation.Name, err))
		return
	}

	if !bodySeekable {
		r.SetReaderBody(aws.ReadSeekCloser(r.HTTPRequest.Body))
	}
	// Reset the request body because dumpRequest will re-wrap the
	// r.HTTPRequest's Body as a NoOpCloser and will not be reset after
	// read by the HTTP client reader.
	if err := r.Error; err != nil {
		r.Config.Logger.Log(fmt.Sprintf(logReqErrMsg,
			r.ClientInfo.ServiceName, r.Operation.Name, err))
		return
	}

	tflog.Debug(ctx, "HTTP Request Sent", requestFields)
}

// TODO: Get rid of this
const logReqErrMsg = `DEBUG ERROR: Request %s/%s:
---[ REQUEST DUMP ERROR ]-----------------------------
%s
------------------------------------------------------`

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
