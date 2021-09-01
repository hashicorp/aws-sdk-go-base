package awsbase

import (
	"context"
	"crypto/tls"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsmiddleware "github.com/aws/aws-sdk-go-v2/aws/middleware"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials/stscreds"
	"github.com/aws/aws-sdk-go-v2/feature/ec2/imds"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/aws/aws-sdk-go-v2/service/sts/types"
	"github.com/aws/smithy-go/logging"
	"github.com/aws/smithy-go/middleware"
	"github.com/hashicorp/go-cleanhttp"
)

const (
	// appendUserAgentEnvVar is a conventionally used environment variable
	// containing additional HTTP User-Agent information.
	// If present and its value is non-empty, it is directly appended to the
	// User-Agent header for HTTP requests.
	appendUserAgentEnvVar = "TF_APPEND_USER_AGENT"
)

func GetAwsConfig(ctx context.Context, c *Config) (aws.Config, error) {
	credentialsProvider := credentialsProvider(c)

	var logMode aws.ClientLogMode
	var logger logging.Logger
	if c.DebugLogging {
		logMode = aws.LogRequestWithBody | aws.LogResponseWithBody | aws.LogRetries
		logger = debugLogger{}
	}

	imdsEnableState := imds.ClientDefaultEnableState
	if c.SkipMetadataApiCheck {
		imdsEnableState = imds.ClientDisabled
		// This should not be needed, but https://github.com/aws/aws-sdk-go-v2/issues/1398
		os.Setenv("AWS_EC2_METADATA_DISABLED", "true")
	}

	httpClient := cleanhttp.DefaultClient()
	if c.Insecure {
		transport := httpClient.Transport.(*http.Transport)
		transport.TLSClientConfig = &tls.Config{
			InsecureSkipVerify: true,
		}
	}

	apiOptions := make([]func(*middleware.Stack) error, 0)
	if len(c.UserAgentProducts) > 0 {
		apiOptions = append(apiOptions, func(stack *middleware.Stack) error {
			// Because the default User-Agent middleware prepends itself to the contents of the User-Agent header,
			// we have to run after it and also prepend our custom User-Agent
			return stack.Build.Add(customUserAgentMiddleware(c), middleware.After)
		})
	}
	if v := os.Getenv(appendUserAgentEnvVar); v != "" {
		log.Printf("[DEBUG] Using additional User-Agent Info: %s", v)
		apiOptions = append(apiOptions, awsmiddleware.AddUserAgentKey(v))
	}

	cfg, err := config.LoadDefaultConfig(ctx,
		config.WithCredentialsProvider(credentialsProvider),
		config.WithRegion(c.Region),
		config.WithSharedCredentialsFiles([]string{c.CredsFilename}),
		config.WithSharedConfigProfile(c.Profile),
		config.WithEndpointResolver(endpointResolver(c)),
		config.WithClientLogMode(logMode),
		config.WithLogger(logger),
		config.WithEC2IMDSClientEnableState(imdsEnableState),
		config.WithHTTPClient(httpClient),
		config.WithAPIOptions(apiOptions),
		// FIXME: This should only be set for retrieving Creds
		config.WithRetryer(func() aws.Retryer {
			return aws.NopRetryer{}
		}),
	)
	if err != nil {
		return cfg, fmt.Errorf("loading configuration: %w", err)
	}

	_, err = cfg.Credentials.Retrieve(ctx)
	if err != nil {
		return cfg, c.NewNoValidCredentialSourcesError()
	}

	if c.AssumeRoleARN == "" {
		return cfg, err
	}

	// When assuming a role, we need to first authenticate the base credentials above, then assume the desired role
	log.Printf("[INFO] Attempting to AssumeRole %s (SessionName: %q, ExternalId: %q)",
		c.AssumeRoleARN, c.AssumeRoleSessionName, c.AssumeRoleExternalID)

	client := sts.NewFromConfig(cfg)

	appCreds := stscreds.NewAssumeRoleProvider(client, c.AssumeRoleARN, func(opts *stscreds.AssumeRoleOptions) {
		opts.RoleSessionName = c.AssumeRoleSessionName
		opts.Duration = time.Duration(c.AssumeRoleDurationSeconds) * time.Second

		if c.AssumeRoleExternalID != "" {
			opts.ExternalID = aws.String(c.AssumeRoleExternalID)
		}

		if c.AssumeRolePolicy != "" {
			opts.Policy = aws.String(c.AssumeRolePolicy)
		}

		if len(c.AssumeRolePolicyARNs) > 0 {
			var policyDescriptorTypes []types.PolicyDescriptorType

			for _, policyARN := range c.AssumeRolePolicyARNs {
				policyDescriptorType := types.PolicyDescriptorType{
					Arn: aws.String(policyARN),
				}
				policyDescriptorTypes = append(policyDescriptorTypes, policyDescriptorType)
			}

			opts.PolicyARNs = policyDescriptorTypes
		}

		if len(c.AssumeRoleTags) > 0 {
			var tags []types.Tag

			for k, v := range c.AssumeRoleTags {
				tag := types.Tag{
					Key:   aws.String(k),
					Value: aws.String(v),
				}
				tags = append(tags, tag)
			}

			opts.Tags = tags
		}

		if len(c.AssumeRoleTransitiveTagKeys) > 0 {
			opts.TransitiveTagKeys = c.AssumeRoleTransitiveTagKeys
		}
	})
	_, err = appCreds.Retrieve(ctx)
	if err != nil {
		return aws.Config{}, c.NewCannotAssumeRoleError(err)
	}

	cfg.Credentials = aws.NewCredentialsCache(appCreds)

	return cfg, err
}
