package mocks

import (
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/credentials/ec2rolecreds"
	"github.com/aws/aws-sdk-go-v2/credentials/endpointcreds"
	"github.com/aws/aws-sdk-go-v2/credentials/stscreds"
	"github.com/hashicorp/aws-sdk-go-base/awsmocks"
)

// GetMockedAwsApiSessionV2 establishes an AWS session to a simulated AWS API server for a given service and route endpoints.
func GetMockedAwsApiSessionV2(svcName string, endpoints []*awsmocks.MockEndpoint) (func(), aws.Config, string) {
	ts := awsmocks.MockAwsApiServer(svcName, endpoints)

	sc := credentials.NewStaticCredentialsProvider("accessKey", "secretKey", "")

	awsConfig := aws.Config{
		Credentials: sc,
		Region:      "us-east-1",
		EndpointResolver: aws.EndpointResolverFunc(func(service, region string) (aws.Endpoint, error) {
			return aws.Endpoint{
				URL:    ts.URL,
				Source: aws.EndpointSourceCustom,
			}, nil
		}),
	}

	return ts.Close, awsConfig, ts.URL
}

var (
	MockEc2MetadataCredentials = aws.Credentials{
		AccessKeyID:     awsmocks.MockEc2MetadataAccessKey,
		Source:          ec2rolecreds.ProviderName,
		SecretAccessKey: awsmocks.MockEc2MetadataSecretKey,
		SessionToken:    awsmocks.MockEc2MetadataSessionToken,
		CanExpire:       true,
	}

	MockEcsCredentialsCredentials = aws.Credentials{
		AccessKeyID:     awsmocks.MockEcsCredentialsAccessKey,
		SecretAccessKey: awsmocks.MockEcsCredentialsSecretKey,
		SessionToken:    awsmocks.MockEcsCredentialsSessionToken,
		CanExpire:       true,
		Source:          endpointcreds.ProviderName,
	}

	MockEnvCredentials = aws.Credentials{
		AccessKeyID:     awsmocks.MockEnvAccessKey,
		SecretAccessKey: awsmocks.MockEnvSecretKey,
		Source:          config.CredentialsSourceName,
	}

	MockEnvCredentialsWithSessionToken = aws.Credentials{
		AccessKeyID:     awsmocks.MockEnvAccessKey,
		SecretAccessKey: awsmocks.MockEnvSecretKey,
		SessionToken:    awsmocks.MockEnvSessionToken,
		Source:          config.CredentialsSourceName,
	}

	MockStaticCredentials = aws.Credentials{
		AccessKeyID:     awsmocks.MockStaticAccessKey,
		SecretAccessKey: awsmocks.MockStaticSecretKey,
		Source:          credentials.StaticCredentialsName,
	}

	MockStsAssumeRoleCredentials = aws.Credentials{
		AccessKeyID:     awsmocks.MockStsAssumeRoleAccessKey,
		SecretAccessKey: awsmocks.MockStsAssumeRoleSecretKey,
		SessionToken:    awsmocks.MockStsAssumeRoleSessionToken,
		Source:          stscreds.ProviderName,
		CanExpire:       true,
	}

	MockStsAssumeRoleWithWebIdentityCredentials = aws.Credentials{
		AccessKeyID:     awsmocks.MockStsAssumeRoleWithWebIdentityAccessKey,
		SecretAccessKey: awsmocks.MockStsAssumeRoleWithWebIdentitySecretKey,
		SessionToken:    awsmocks.MockStsAssumeRoleWithWebIdentitySessionToken,
		Source:          stscreds.WebIdentityProviderName,
		CanExpire:       true,
	}
)
