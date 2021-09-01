package awsv1shim

import ( // nosemgrep: no-sdkv2-imports-in-awsv1shim
	"context"
	"fmt"
	"log"
	"os"

	awsv2 "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/endpoints"
	"github.com/aws/aws-sdk-go/aws/request"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/iam"
	"github.com/aws/aws-sdk-go/service/sts"
	awsbase "github.com/hashicorp/aws-sdk-go-base"
	"github.com/hashicorp/aws-sdk-go-base/tfawserr"
	"github.com/hashicorp/go-cleanhttp"
)

const (
	// appendUserAgentEnvVar is a conventionally used environment variable
	// containing additional HTTP User-Agent information.
	// If present and its value is non-empty, it is directly appended to the
	// User-Agent header for HTTP requests.
	appendUserAgentEnvVar = "TF_APPEND_USER_AGENT"

	// Maximum network retries.
	// We depend on the AWS Go SDK DefaultRetryer exponential backoff.
	// Ensure that if the AWS Config MaxRetries is set high (which it is by
	// default), that we only retry for a few seconds with typically
	// unrecoverable network errors, such as DNS lookup failures.
	maxNetworkRetryCount = 9
)

// getSessionOptions attempts to return valid AWS Go SDK session authentication
// options based on pre-existing credential provider, configured profile, or
// fallback to automatically a determined session via the AWS Go SDK.
func getSessionOptions(awsC *awsv2.Config, c *awsbase.Config) (*session.Options, error) {
	creds, err := awsC.Credentials.Retrieve(context.Background())
	if err != nil {
		return nil, fmt.Errorf("error accessing credentials: %w", err)
	}

	options := &session.Options{
		Config: aws.Config{
			Credentials: credentials.NewStaticCredentials(
				creds.AccessKeyID,
				creds.SecretAccessKey,
				creds.SessionToken,
			),
			EndpointResolver: endpointResolver(c),
			HTTPClient:       cleanhttp.DefaultClient(),
			MaxRetries:       aws.Int(0),
			Region:           aws.String(awsC.Region),
		},
		Profile:           c.Profile,                  // ¿Is this needed?
		SharedConfigState: session.SharedConfigEnable, // ¿Is this needed?
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
			return nil, c.NewNoValidCredentialSourcesError()
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
	// of PushBack. To properly keep the order given by the configuration, we
	// must reverse iterate through the products so the last item is PushFront
	// first through the first item being PushFront last.
	for i := len(c.UserAgentProducts) - 1; i >= 0; i-- {
		product := c.UserAgentProducts[i]
		sess.Handlers.Build.PushFront(request.MakeAddToUserAgentHandler(product.Name, product.Version, product.Extra...))
	}

	// Add custom input from ENV to the User-Agent request header
	// Reference: https://github.com/terraform-providers/terraform-provider-aws/issues/9149
	if v := os.Getenv(appendUserAgentEnvVar); v != "" {
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
		if r.RetryCount < maxNetworkRetryCount {
			return
		}
		// RequestError: send request failed
		// caused by: Post https://FQDN/: dial tcp: lookup FQDN: no such host
		if tfawserr.ErrMessageAndOrigErrContain(r.Error, "RequestError", "send request failed", "no such host") {
			log.Printf("[WARN] Disabling retries after next request due to networking issue")
			r.Retryable = aws.Bool(false)
		}
		// RequestError: send request failed
		// caused by: Post https://FQDN/: dial tcp IPADDRESS:443: connect: connection refused
		if tfawserr.ErrMessageAndOrigErrContain(r.Error, "RequestError", "send request failed", "connection refused") {
			log.Printf("[WARN] Disabling retries after next request due to networking issue")
			r.Retryable = aws.Bool(false)
		}
	})

	if !c.SkipCredsValidation {
		if _, _, err := getAccountIDAndPartitionFromSTSGetCallerIdentity(sts.New(sess)); err != nil {
			return nil, fmt.Errorf("error validating provider credentials: %w", err)
		}
	}

	return sess, nil
}

// GetSessionWithAccountIDAndPartition attempts to return valid AWS Go SDK session
// along with account ID and partition information if available
func GetSessionWithAccountIDAndPartition(awsC *awsv2.Config, c *awsbase.Config) (*session.Session, string, string, error) {
	sess, err := GetSession(awsC, c)

	if err != nil {
		return nil, "", "", err
	}

	if c.AssumeRoleARN != "" {
		accountID, partition, _ := parseAccountIDAndPartitionFromARN(c.AssumeRoleARN)
		return sess, accountID, partition, nil
	}

	iamClient := iam.New(sess)
	stsClient := sts.New(sess)

	if !c.SkipCredsValidation {
		accountID, partition, err := getAccountIDAndPartitionFromSTSGetCallerIdentity(stsClient)

		if err != nil {
			return nil, "", "", fmt.Errorf("error validating provider credentials: %w", err)
		}

		return sess, accountID, partition, nil
	}

	if !c.SkipRequestingAccountId {
		credentialsProviderName := ""

		if credentialsValue, err := awsC.Credentials.Retrieve(context.Background()); err == nil {
			credentialsProviderName = credentialsValue.Source
		}

		accountID, partition, err := getAccountIDAndPartition(iamClient, stsClient, credentialsProviderName)

		if err == nil {
			return sess, accountID, partition, nil
		}

		return nil, "", "", fmt.Errorf(
			"AWS account ID not previously found and failed retrieving via all available methods. "+
				"See https://www.terraform.io/docs/providers/aws/index.html#skip_requesting_account_id for workaround and implications. "+
				"Errors: %w", err)
	}

	var partition string
	if p, ok := endpoints.PartitionForRegion(endpoints.DefaultPartitions(), c.Region); ok {
		partition = p.ID()
	}

	return sess, "", partition, nil
}
