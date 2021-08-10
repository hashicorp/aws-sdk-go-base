package awsv1shim

import (
	"errors"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/arn"
	"github.com/aws/aws-sdk-go/aws/awserr"
	awsCredentials "github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/credentials/ec2rolecreds"
	"github.com/aws/aws-sdk-go/aws/credentials/stscreds"
	"github.com/aws/aws-sdk-go/aws/ec2metadata"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/iam"
	"github.com/aws/aws-sdk-go/service/sts"
	awsbase "github.com/hashicorp/aws-sdk-go-base"
	"github.com/hashicorp/aws-sdk-go-base/tfawserr"
	"github.com/hashicorp/go-cleanhttp"
	"github.com/hashicorp/go-multierror"
	homedir "github.com/mitchellh/go-homedir"
)

// getAccountIDAndPartition gets the account ID and associated partition.
func getAccountIDAndPartition(iamconn *iam.IAM, stsconn *sts.STS, authProviderName string) (string, string, error) {
	var accountID, partition string
	var err, errors error

	if authProviderName == ec2rolecreds.ProviderName {
		accountID, partition, err = getAccountIDAndPartitionFromEC2Metadata()
	} else {
		accountID, partition, err = getAccountIDAndPartitionFromIAMGetUser(iamconn)
	}
	if accountID != "" {
		return accountID, partition, nil
	}
	errors = multierror.Append(errors, err)

	accountID, partition, err = getAccountIDAndPartitionFromSTSGetCallerIdentity(stsconn)
	if accountID != "" {
		return accountID, partition, nil
	}
	errors = multierror.Append(errors, err)

	accountID, partition, err = getAccountIDAndPartitionFromIAMListRoles(iamconn)
	if accountID != "" {
		return accountID, partition, nil
	}
	errors = multierror.Append(errors, err)

	return accountID, partition, errors
}

// getAccountIDAndPartitionFromEC2Metadata gets the account ID and associated
// partition from EC2 metadata.
func getAccountIDAndPartitionFromEC2Metadata() (string, string, error) {
	log.Println("[DEBUG] Trying to get account information via EC2 Metadata")

	cfg := &aws.Config{}
	setOptionalEndpoint(cfg)
	sess, err := session.NewSession(cfg)
	if err != nil {
		return "", "", fmt.Errorf("error creating EC2 Metadata session: %w", err)
	}

	metadataClient := ec2metadata.New(sess)
	info, err := metadataClient.IAMInfo()
	if err != nil {
		// We can end up here if there's an issue with the instance metadata service
		// or if we're getting credentials from AdRoll's Hologram (in which case IAMInfo will
		// error out).
		err = fmt.Errorf("failed getting account information via EC2 Metadata IAM information: %w", err)
		log.Printf("[DEBUG] %s", err)
		return "", "", err
	}

	return parseAccountIDAndPartitionFromARN(info.InstanceProfileArn)
}

// getAccountIDAndPartitionFromIAMGetUser gets the account ID and associated
// partition from IAM.
func getAccountIDAndPartitionFromIAMGetUser(iamconn *iam.IAM) (string, string, error) {
	log.Println("[DEBUG] Trying to get account information via iam:GetUser")

	output, err := iamconn.GetUser(&iam.GetUserInput{})
	if err != nil {
		// AccessDenied and ValidationError can be raised
		// if credentials belong to federated profile, so we ignore these
		if awsErr, ok := err.(awserr.Error); ok {
			switch awsErr.Code() {
			case "AccessDenied", "InvalidClientTokenId", "ValidationError":
				return "", "", nil
			}
		}
		err = fmt.Errorf("failed getting account information via iam:GetUser: %w", err)
		log.Printf("[DEBUG] %s", err)
		return "", "", err
	}

	if output == nil || output.User == nil {
		err = errors.New("empty iam:GetUser response")
		log.Printf("[DEBUG] %s", err)
		return "", "", err
	}

	return parseAccountIDAndPartitionFromARN(aws.StringValue(output.User.Arn))
}

// getAccountIDAndPartitionFromIAMListRoles gets the account ID and associated
// partition from listing IAM roles.
func getAccountIDAndPartitionFromIAMListRoles(iamconn *iam.IAM) (string, string, error) {
	log.Println("[DEBUG] Trying to get account information via iam:ListRoles")

	output, err := iamconn.ListRoles(&iam.ListRolesInput{
		MaxItems: aws.Int64(int64(1)),
	})
	if err != nil {
		err = fmt.Errorf("failed getting account information via iam:ListRoles: %w", err)
		log.Printf("[DEBUG] %s", err)
		return "", "", err
	}

	if output == nil || len(output.Roles) < 1 {
		err = fmt.Errorf("empty iam:ListRoles response")
		log.Printf("[DEBUG] %s", err)
		return "", "", err
	}

	return parseAccountIDAndPartitionFromARN(aws.StringValue(output.Roles[0].Arn))
}

// getAccountIDAndPartitionFromSTSGetCallerIdentity gets the account ID and associated
// partition from STS caller identity.
func getAccountIDAndPartitionFromSTSGetCallerIdentity(stsconn *sts.STS) (string, string, error) {
	log.Println("[DEBUG] Trying to get account information via sts:GetCallerIdentity")

	output, err := stsconn.GetCallerIdentity(&sts.GetCallerIdentityInput{})
	if err != nil {
		return "", "", fmt.Errorf("error calling sts:GetCallerIdentity: %w", err)
	}

	if output == nil || output.Arn == nil {
		err = errors.New("empty sts:GetCallerIdentity response")
		log.Printf("[DEBUG] %s", err)
		return "", "", err
	}

	return parseAccountIDAndPartitionFromARN(aws.StringValue(output.Arn))
}

func parseAccountIDAndPartitionFromARN(inputARN string) (string, string, error) {
	arn, err := arn.Parse(inputARN)
	if err != nil {
		return "", "", fmt.Errorf("error parsing ARN (%s): %s", inputARN, err)
	}
	return arn.AccountID, arn.Partition, nil
}

// getCredentialsFromSession returns credentials derived from a session. A
// session uses the AWS SDK Go chain of providers so may use a provider (e.g.,
// ProcessProvider) that is not part of the Terraform provider chain.
func getCredentialsFromSession(c *awsbase.Config) (*awsCredentials.Credentials, error) {
	log.Printf("[INFO] Attempting to use session-derived credentials")

	// Avoid setting HTTPClient here as it will prevent the ec2metadata
	// client from automatically lowering the timeout to 1 second.
	options := &session.Options{
		Config: aws.Config{
			CredentialsChainVerboseErrors: aws.Bool(true),
			EndpointResolver:              endpointResolver(c),
			MaxRetries:                    aws.Int(0),
			Region:                        aws.String(c.Region),
		},
		Profile:           c.Profile,
		SharedConfigState: session.SharedConfigEnable,
	}

	sess, err := session.NewSessionWithOptions(*options)
	if err != nil {
		if tfawserr.ErrCodeEquals(err, "NoCredentialProviders") {
			return nil, c.NewNoValidCredentialSourcesError(err)
		}
		return nil, fmt.Errorf("Error creating AWS session: %w", err)
	}

	creds := sess.Config.Credentials
	cp, err := sess.Config.Credentials.Get()
	if err != nil {
		return nil, c.NewNoValidCredentialSourcesError(err)
	}

	log.Printf("[INFO] Successfully derived credentials from session")
	log.Printf("[INFO] AWS Auth provider used: %q", cp.ProviderName)
	return creds, nil
}

// getCredentials gets credentials from environment, shared credentials file,
// environment AWS_SHARED_CREDENTIALS_FILE, the session (which may include a credential process),
// or ECS/EC2 metadata endpoints. getCredentials also validates the credentials
// and the ability to assume a role or will return an error if unsuccessful.
func getCredentials(c *awsbase.Config) (*awsCredentials.Credentials, error) {
	sharedCredentialsFilename, err := homedir.Expand(c.CredsFilename)

	if err != nil {
		return nil, fmt.Errorf("error expanding shared credentials filename: %w", err)
	}

	// build a chain provider, lazy-evaluated by aws-sdk
	providers := []awsCredentials.Provider{
		&awsCredentials.StaticProvider{Value: awsCredentials.Value{
			AccessKeyID:     c.AccessKey,
			SecretAccessKey: c.SecretKey,
			SessionToken:    c.Token,
		}},
		&awsCredentials.EnvProvider{},
		&awsCredentials.SharedCredentialsProvider{
			Filename: sharedCredentialsFilename,
			Profile:  c.Profile,
		},
	}

	// Validate the credentials before returning them
	creds := awsCredentials.NewChainCredentials(providers)
	cp, err := creds.Get()
	if err != nil {
		if tfawserr.ErrCodeEquals(err, "NoCredentialProviders") {
			creds, err = getCredentialsFromSession(c)
			if err != nil {
				return nil, err
			}
		} else {
			return nil, fmt.Errorf("Error loading credentials for AWS Provider: %w", err)
		}
	} else {
		log.Printf("[INFO] AWS Auth provider used: %q", cp.ProviderName)
	}

	// This is the "normal" flow (i.e. not assuming a role)
	if c.AssumeRoleARN == "" {
		return creds, nil
	}

	// Otherwise we need to construct an STS client with the main credentials, and verify
	// that we can assume the defined role.
	log.Printf("[INFO] Attempting to AssumeRole %s (SessionName: %q, ExternalId: %q)",
		c.AssumeRoleARN, c.AssumeRoleSessionName, c.AssumeRoleExternalID)

	awsConfig := &aws.Config{
		CredentialsChainVerboseErrors: aws.Bool(true),
		Credentials:                   creds,
		EndpointResolver:              endpointResolver(c),
		Region:                        aws.String(c.Region),
		MaxRetries:                    aws.Int(c.MaxRetries),
		HTTPClient:                    cleanhttp.DefaultClient(),
	}

	if c.DebugLogging {
		awsConfig.LogLevel = aws.LogLevel(aws.LogDebugWithHTTPBody | aws.LogDebugWithRequestRetries | aws.LogDebugWithRequestErrors)
		awsConfig.Logger = debugLogger{}
	}

	assumeRoleSession, err := session.NewSession(awsConfig)

	if err != nil {
		return nil, fmt.Errorf("error creating assume role session: %w", err)
	}

	stsclient := sts.New(assumeRoleSession)
	assumeRoleProvider := &stscreds.AssumeRoleProvider{
		Client:  stsclient,
		RoleARN: c.AssumeRoleARN,
	}

	if c.AssumeRoleDurationSeconds > 0 {
		assumeRoleProvider.Duration = time.Duration(c.AssumeRoleDurationSeconds) * time.Second
	}

	if c.AssumeRoleExternalID != "" {
		assumeRoleProvider.ExternalID = aws.String(c.AssumeRoleExternalID)
	}

	if c.AssumeRolePolicy != "" {
		assumeRoleProvider.Policy = aws.String(c.AssumeRolePolicy)
	}

	if len(c.AssumeRolePolicyARNs) > 0 {
		var policyDescriptorTypes []*sts.PolicyDescriptorType

		for _, policyARN := range c.AssumeRolePolicyARNs {
			policyDescriptorType := &sts.PolicyDescriptorType{
				Arn: aws.String(policyARN),
			}
			policyDescriptorTypes = append(policyDescriptorTypes, policyDescriptorType)
		}

		assumeRoleProvider.PolicyArns = policyDescriptorTypes
	}

	if c.AssumeRoleSessionName != "" {
		assumeRoleProvider.RoleSessionName = c.AssumeRoleSessionName
	}

	if len(c.AssumeRoleTags) > 0 {
		var tags []*sts.Tag

		for k, v := range c.AssumeRoleTags {
			tag := &sts.Tag{
				Key:   aws.String(k),
				Value: aws.String(v),
			}
			tags = append(tags, tag)
		}

		assumeRoleProvider.Tags = tags
	}

	if len(c.AssumeRoleTransitiveTagKeys) > 0 {
		assumeRoleProvider.TransitiveTagKeys = aws.StringSlice(c.AssumeRoleTransitiveTagKeys)
	}

	providers = []awsCredentials.Provider{assumeRoleProvider}

	assumeRoleCreds := awsCredentials.NewChainCredentials(providers)
	_, err = assumeRoleCreds.Get()
	if err != nil {
		return nil, c.NewCannotAssumeRoleError(err)
	}

	return assumeRoleCreds, nil
}

func setOptionalEndpoint(cfg *aws.Config) string {
	endpoint := os.Getenv("AWS_METADATA_URL")
	if endpoint != "" {
		log.Printf("[INFO] Setting custom metadata endpoint: %q", endpoint)
		cfg.Endpoint = aws.String(endpoint)
		return endpoint
	}
	return ""
}
