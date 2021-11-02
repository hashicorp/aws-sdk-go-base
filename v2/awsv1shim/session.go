package awsv1shim

import ( // nosemgrep: no-sdkv2-imports-in-awsv1shim
	"context"
	"fmt"
	"log"
	"net/http"
	"os"

	awsv2 "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/request"
	"github.com/aws/aws-sdk-go/aws/session"
	awsbase "github.com/hashicorp/aws-sdk-go-base/v2"
	"github.com/hashicorp/aws-sdk-go-base/v2/awsv1shim/v2/tfawserr"
	"github.com/hashicorp/aws-sdk-go-base/v2/internal/constants"
	"github.com/hashicorp/aws-sdk-go-base/v2/internal/httpclient"
)

// getSessionOptions attempts to return valid AWS Go SDK session authentication
// options based on pre-existing credential provider, configured profile, or
// fallback to automatically a determined session via the AWS Go SDK.
func getSessionOptions(awsC *awsv2.Config, c *awsbase.Config) (*session.Options, error) {
	creds, err := awsC.Credentials.Retrieve(context.Background())
	if err != nil {
		return nil, fmt.Errorf("error accessing credentials: %w", err)
	}

	httpClient, ok := awsC.HTTPClient.(*http.Client)
	if !ok { // This is unlikely, but technically possible
		httpClient, err = httpclient.DefaultHttpClient(c)
		if err != nil {
			return nil, err
		}
	}
	options := &session.Options{
		Config: aws.Config{
			Credentials: credentials.NewStaticCredentials(
				creds.AccessKeyID,
				creds.SecretAccessKey,
				creds.SessionToken,
			),
			EndpointResolver: endpointResolver(c),
			HTTPClient:       httpClient,
			MaxRetries:       aws.Int(0),
			Region:           aws.String(awsC.Region),
		},
	}

	// This needs its own debugger. Don't reuse or wrap the AWS SDK for Go v2 logger, since it hardcodes the string "aws-sdk-go-v2"
	if c.DebugLogging {
		options.Config.LogLevel = aws.LogLevel(aws.LogDebugWithHTTPBody | aws.LogDebugWithRequestRetries | aws.LogDebugWithRequestErrors)
		options.Config.Logger = debugLogger{}
	}

	return options, nil
}

// GetSession attempts to return valid AWS Go SDK session.
func GetSession(awsC *awsv2.Config, c *awsbase.Config) (*session.Session, error) {
	options, err := getSessionOptions(awsC, c)
	if err != nil {
		return nil, err
	}

	sess, err := session.NewSessionWithOptions(*options)
	if err != nil {
		if tfawserr.ErrCodeEquals(err, "NoCredentialProviders") {
			return nil, c.NewNoValidCredentialSourcesError(err)
		}
		return nil, fmt.Errorf("Error creating AWS session: %w", err)
	}

	if c.MaxRetries > 0 {
		sess = sess.Copy(&aws.Config{MaxRetries: aws.Int(c.MaxRetries)})
	}

	// AWS SDK Go automatically adds a User-Agent product to HTTP requests,
	// which contains helpful information about the SDK version and runtime.
	// The configuration of additional User-Agent header products should take
	// precedence over that product. Since the AWS SDK Go request package
	// functions only append, we must PushFront on the build handlers instead
	// of PushBack.
	if c.APNInfo != nil {
		sess.Handlers.Build.PushFront(
			request.MakeAddToUserAgentFreeFormHandler(c.APNInfo.BuildUserAgentString()),
		)
	}

	if len(c.UserAgent) > 0 {
		sess.Handlers.Build.PushBack(request.MakeAddToUserAgentFreeFormHandler(c.UserAgent.BuildUserAgentString()))
	}

	// Add custom input from ENV to the User-Agent request header
	// Reference: https://github.com/terraform-providers/terraform-provider-aws/issues/9149
	if v := os.Getenv(constants.AppendUserAgentEnvVar); v != "" {
		log.Printf("[DEBUG] Using additional User-Agent Info: %s", v)
		sess.Handlers.Build.PushBack(request.MakeAddToUserAgentFreeFormHandler(v))
	}

	// Generally, we want to configure a lower retry theshold for networking issues
	// as the session retry threshold is very high by default and can mask permanent
	// networking failures, such as a non-existent service endpoint.
	// MaxRetries will override this logic if it has a lower retry threshold.
	// NOTE: This logic can be fooled by other request errors raising the retry count
	//       before any networking error occurs
	sess.Handlers.Retry.PushBack(func(r *request.Request) {
		if r.RetryCount < constants.MaxNetworkRetryCount {
			return
		}
		// RequestError: send request failed
		// caused by: Post https://FQDN/: dial tcp: lookup FQDN: no such host
		if tfawserr.ErrMessageAndOrigErrContain(r.Error, request.ErrCodeRequestError, "send request failed", "no such host") {
			log.Printf("[WARN] Disabling retries after next request due to networking issue")
			r.Retryable = aws.Bool(false)
		}
		// RequestError: send request failed
		// caused by: Post https://FQDN/: dial tcp IPADDRESS:443: connect: connection refused
		if tfawserr.ErrMessageAndOrigErrContain(r.Error, request.ErrCodeRequestError, "send request failed", "connection refused") {
			log.Printf("[WARN] Disabling retries after next request due to networking issue")
			r.Retryable = aws.Bool(false)
		}
	})

	return sess, nil
}
