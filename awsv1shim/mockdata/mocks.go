package mockdata

import (
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/credentials/ec2rolecreds"
	"github.com/aws/aws-sdk-go/aws/credentials/endpointcreds"
	"github.com/aws/aws-sdk-go/aws/credentials/stscreds"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/hashicorp/aws-sdk-go-base/servicemocks"
)

// GetMockedAwsApiSession establishes an AWS session to a simulated AWS API server for a given service and route endpoints.
func GetMockedAwsApiSession(svcName string, endpoints []*servicemocks.MockEndpoint) (func(), *session.Session, error) {
	ts := servicemocks.MockAwsApiServer(svcName, endpoints)

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
		AccessKeyID:     servicemocks.MockEc2MetadataAccessKey,
		ProviderName:    ec2rolecreds.ProviderName,
		SecretAccessKey: servicemocks.MockEc2MetadataSecretKey,
		SessionToken:    servicemocks.MockEc2MetadataSessionToken,
	}

	MockEcsCredentialsCredentials = credentials.Value{
		AccessKeyID:     servicemocks.MockEcsCredentialsAccessKey,
		ProviderName:    endpointcreds.ProviderName,
		SecretAccessKey: servicemocks.MockEcsCredentialsSecretKey,
		SessionToken:    servicemocks.MockEcsCredentialsSessionToken,
	}

	MockEnvCredentials = credentials.Value{
		AccessKeyID:     servicemocks.MockEnvAccessKey,
		ProviderName:    credentials.EnvProviderName,
		SecretAccessKey: servicemocks.MockEnvSecretKey,
	}

	MockEnvCredentialsWithSessionToken = credentials.Value{
		AccessKeyID:     servicemocks.MockEnvAccessKey,
		ProviderName:    credentials.EnvProviderName,
		SecretAccessKey: servicemocks.MockEnvSecretKey,
		SessionToken:    servicemocks.MockEnvSessionToken,
	}

	MockStaticCredentials = credentials.Value{
		AccessKeyID:     servicemocks.MockStaticAccessKey,
		ProviderName:    credentials.StaticProviderName,
		SecretAccessKey: servicemocks.MockStaticSecretKey,
	}

	MockStsAssumeRoleCredentials = credentials.Value{
		AccessKeyID:     servicemocks.MockStsAssumeRoleAccessKey,
		ProviderName:    stscreds.ProviderName,
		SecretAccessKey: servicemocks.MockStsAssumeRoleSecretKey,
		SessionToken:    servicemocks.MockStsAssumeRoleSessionToken,
	}

	MockStsAssumeRoleWithWebIdentityCredentials = credentials.Value{
		AccessKeyID:     servicemocks.MockStsAssumeRoleWithWebIdentityAccessKey,
		ProviderName:    stscreds.WebIdentityProviderName,
		SecretAccessKey: servicemocks.MockStsAssumeRoleWithWebIdentitySecretKey,
		SessionToken:    servicemocks.MockStsAssumeRoleWithWebIdentitySessionToken,
	}
)
