package mocks

import (
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/credentials/ec2rolecreds"
	"github.com/aws/aws-sdk-go/aws/credentials/endpointcreds"
	"github.com/aws/aws-sdk-go/aws/credentials/stscreds"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/hashicorp/aws-sdk-go-base/awsmocks"
)

// GetMockedAwsApiSession establishes an AWS session to a simulated AWS API server for a given service and route endpoints.
func GetMockedAwsApiSession(svcName string, endpoints []*awsmocks.MockEndpoint) (func(), *session.Session, error) {
	ts := awsmocks.MockAwsApiServer(svcName, endpoints)

	sc := credentials.NewStaticCredentials("accessKey", "secretKey", "")

	sess, err := session.NewSession(&aws.Config{
		Credentials:                   sc,
		Region:                        aws.String("us-east-1"),
		Endpoint:                      aws.String(ts.URL),
		CredentialsChainVerboseErrors: aws.Bool(true),
	})

	return ts.Close, sess, err
}

var (
	MockEc2MetadataCredentials = credentials.Value{
		AccessKeyID:     awsmocks.MockEc2MetadataAccessKey,
		ProviderName:    ec2rolecreds.ProviderName,
		SecretAccessKey: awsmocks.MockEc2MetadataSecretKey,
		SessionToken:    awsmocks.MockEc2MetadataSessionToken,
	}

	MockEcsCredentialsCredentials = credentials.Value{
		AccessKeyID:     awsmocks.MockEcsCredentialsAccessKey,
		ProviderName:    endpointcreds.ProviderName,
		SecretAccessKey: awsmocks.MockEcsCredentialsSecretKey,
		SessionToken:    awsmocks.MockEcsCredentialsSessionToken,
	}

	MockEnvCredentials = credentials.Value{
		AccessKeyID:     awsmocks.MockEnvAccessKey,
		ProviderName:    credentials.EnvProviderName,
		SecretAccessKey: awsmocks.MockEnvSecretKey,
	}

	MockEnvCredentialsWithSessionToken = credentials.Value{
		AccessKeyID:     awsmocks.MockEnvAccessKey,
		ProviderName:    credentials.EnvProviderName,
		SecretAccessKey: awsmocks.MockEnvSecretKey,
		SessionToken:    awsmocks.MockEnvSessionToken,
	}

	MockStaticCredentials = credentials.Value{
		AccessKeyID:     awsmocks.MockStaticAccessKey,
		ProviderName:    credentials.StaticProviderName,
		SecretAccessKey: awsmocks.MockStaticSecretKey,
	}

	MockStsAssumeRoleCredentials = credentials.Value{
		AccessKeyID:     awsmocks.MockStsAssumeRoleAccessKey,
		ProviderName:    stscreds.ProviderName,
		SecretAccessKey: awsmocks.MockStsAssumeRoleSecretKey,
		SessionToken:    awsmocks.MockStsAssumeRoleSessionToken,
	}

	MockStsAssumeRoleWithWebIdentityCredentials = credentials.Value{
		AccessKeyID:     awsmocks.MockStsAssumeRoleWithWebIdentityAccessKey,
		ProviderName:    stscreds.WebIdentityProviderName,
		SecretAccessKey: awsmocks.MockStsAssumeRoleWithWebIdentitySecretKey,
		SessionToken:    awsmocks.MockStsAssumeRoleWithWebIdentitySessionToken,
	}
)
