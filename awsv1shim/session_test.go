package awsv1shim

import (
	"fmt"
	"io/ioutil"
	"os"
	"reflect"
	"runtime"
	"testing"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/client"
	"github.com/aws/aws-sdk-go/aws/client/metadata"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/request"
	awsbase "github.com/hashicorp/aws-sdk-go-base"
	"github.com/hashicorp/aws-sdk-go-base/awsmocks"
	"github.com/hashicorp/aws-sdk-go-base/tfawserr"
)

func TestGetSessionOptions(t *testing.T) {
	oldEnv := awsmocks.InitSessionTestEnv()
	defer awsmocks.PopEnv(oldEnv)

	testCases := []struct {
		desc        string
		config      *awsbase.Config
		expectError bool
	}{
		{"BlankConfig",
			&awsbase.Config{},
			true,
		},
		{"ConfigWithCredentials",
			&awsbase.Config{AccessKey: "MockAccessKey", SecretKey: "MockSecretKey"},
			false,
		},
		{"ConfigWithCredsAndOptions",
			&awsbase.Config{AccessKey: "MockAccessKey", SecretKey: "MockSecretKey", Insecure: true, DebugLogging: true},
			false,
		},
	}

	for _, testCase := range testCases {
		tc := testCase

		t.Run(tc.desc, func(t *testing.T) {
			opts, err := getSessionOptions(tc.config)
			if err != nil && tc.expectError == false {
				t.Fatalf("GetSessionOptions(c) resulted in an error %s", err)
			}

			if opts == nil && tc.expectError == false {
				t.Error("GetSessionOptions(...) resulted in a nil set of options")
			}

			if err == nil && tc.expectError == true {
				t.Fatal("Expected error not found")
			}
		})

	}
}

// End-to-end testing for GetSession
func TestGetSession(t *testing.T) {
	testCases := []struct {
		Config                     *awsbase.Config
		Description                string
		EnableEc2MetadataServer    bool
		EnableEcsCredentialsServer bool
		EnableWebIdentityToken     bool
		EnvironmentVariables       map[string]string
		ExpectedCredentialsValue   credentials.Value
		ExpectedRegion             string
		ExpectedUserAgent          string
		ExpectedError              func(err error) bool
		MockStsEndpoints           []*awsmocks.MockEndpoint
		SharedConfigurationFile    string
		SharedCredentialsFile      string
	}{
		{
			Config:      &awsbase.Config{},
			Description: "no configuration or credentials",
			ExpectedError: func(err error) bool {
				return awsbase.IsNoValidCredentialSourcesError(err)
			},
		},
		{
			Config: &awsbase.Config{
				AccessKey: awsmocks.MockStaticAccessKey,
				Region:    "us-east-1",
				SecretKey: awsmocks.MockStaticSecretKey,
			},
			Description:              "config AccessKey",
			ExpectedCredentialsValue: awsmocks.MockStaticCredentialsV1,
			ExpectedRegion:           "us-east-1",
			MockStsEndpoints: []*awsmocks.MockEndpoint{
				awsmocks.MockStsGetCallerIdentityValidEndpoint,
			},
		},
		{
			Config: &awsbase.Config{
				AccessKey:             awsmocks.MockStaticAccessKey,
				AssumeRoleARN:         awsmocks.MockStsAssumeRoleArn,
				AssumeRoleSessionName: awsmocks.MockStsAssumeRoleSessionName,
				Region:                "us-east-1",
				SecretKey:             awsmocks.MockStaticSecretKey,
			},
			Description:              "config AccessKey config AssumeRoleARN access key",
			ExpectedCredentialsValue: awsmocks.MockStsAssumeRoleCredentialsV1,
			ExpectedRegion:           "us-east-1",
			MockStsEndpoints: []*awsmocks.MockEndpoint{
				awsmocks.MockStsAssumeRoleValidEndpoint,
				awsmocks.MockStsGetCallerIdentityValidEndpoint,
			},
		},
		{
			Config: &awsbase.Config{
				AccessKey:                 awsmocks.MockStaticAccessKey,
				AssumeRoleARN:             awsmocks.MockStsAssumeRoleArn,
				AssumeRoleDurationSeconds: 3600,
				AssumeRoleSessionName:     awsmocks.MockStsAssumeRoleSessionName,
				Region:                    "us-east-1",
				SecretKey:                 awsmocks.MockStaticSecretKey,
			},
			Description:              "config AssumeRoleDurationSeconds",
			ExpectedCredentialsValue: awsmocks.MockStsAssumeRoleCredentialsV1,
			ExpectedRegion:           "us-east-1",
			MockStsEndpoints: []*awsmocks.MockEndpoint{
				awsmocks.MockStsAssumeRoleValidEndpointWithOptions(map[string]string{"DurationSeconds": "3600"}),
				awsmocks.MockStsGetCallerIdentityValidEndpoint,
			},
		},
		{
			Config: &awsbase.Config{
				AccessKey:             awsmocks.MockStaticAccessKey,
				AssumeRoleARN:         awsmocks.MockStsAssumeRoleArn,
				AssumeRoleExternalID:  awsmocks.MockStsAssumeRoleExternalId,
				AssumeRoleSessionName: awsmocks.MockStsAssumeRoleSessionName,
				Region:                "us-east-1",
				SecretKey:             awsmocks.MockStaticSecretKey,
			},
			Description:              "config AssumeRoleExternalID",
			ExpectedCredentialsValue: awsmocks.MockStsAssumeRoleCredentialsV1,
			ExpectedRegion:           "us-east-1",
			MockStsEndpoints: []*awsmocks.MockEndpoint{
				awsmocks.MockStsAssumeRoleValidEndpointWithOptions(map[string]string{"ExternalId": awsmocks.MockStsAssumeRoleExternalId}),
				awsmocks.MockStsGetCallerIdentityValidEndpoint,
			},
		},
		{
			Config: &awsbase.Config{
				AccessKey:             awsmocks.MockStaticAccessKey,
				AssumeRoleARN:         awsmocks.MockStsAssumeRoleArn,
				AssumeRolePolicy:      awsmocks.MockStsAssumeRolePolicy,
				AssumeRoleSessionName: awsmocks.MockStsAssumeRoleSessionName,
				Region:                "us-east-1",
				SecretKey:             awsmocks.MockStaticSecretKey,
			},
			Description:              "config AssumeRolePolicy",
			ExpectedCredentialsValue: awsmocks.MockStsAssumeRoleCredentialsV1,
			ExpectedRegion:           "us-east-1",
			MockStsEndpoints: []*awsmocks.MockEndpoint{
				awsmocks.MockStsAssumeRoleValidEndpointWithOptions(map[string]string{"Policy": awsmocks.MockStsAssumeRolePolicy}),
				awsmocks.MockStsGetCallerIdentityValidEndpoint,
			},
		},
		{
			Config: &awsbase.Config{
				AccessKey:             awsmocks.MockStaticAccessKey,
				AssumeRoleARN:         awsmocks.MockStsAssumeRoleArn,
				AssumeRolePolicyARNs:  []string{awsmocks.MockStsAssumeRolePolicyArn},
				AssumeRoleSessionName: awsmocks.MockStsAssumeRoleSessionName,
				Region:                "us-east-1",
				SecretKey:             awsmocks.MockStaticSecretKey,
			},
			Description:              "config AssumeRolePolicyARNs",
			ExpectedCredentialsValue: awsmocks.MockStsAssumeRoleCredentialsV1,
			ExpectedRegion:           "us-east-1",
			MockStsEndpoints: []*awsmocks.MockEndpoint{
				awsmocks.MockStsAssumeRoleValidEndpointWithOptions(map[string]string{"PolicyArns.member.1.arn": awsmocks.MockStsAssumeRolePolicyArn}),
				awsmocks.MockStsGetCallerIdentityValidEndpoint,
			},
		},
		{
			Config: &awsbase.Config{
				AccessKey:             awsmocks.MockStaticAccessKey,
				AssumeRoleARN:         awsmocks.MockStsAssumeRoleArn,
				AssumeRoleSessionName: awsmocks.MockStsAssumeRoleSessionName,
				AssumeRoleTags: map[string]string{
					awsmocks.MockStsAssumeRoleTagKey: awsmocks.MockStsAssumeRoleTagValue,
				},
				Region:    "us-east-1",
				SecretKey: awsmocks.MockStaticSecretKey,
			},
			Description:              "config AssumeRoleTags",
			ExpectedCredentialsValue: awsmocks.MockStsAssumeRoleCredentialsV1,
			ExpectedRegion:           "us-east-1",
			MockStsEndpoints: []*awsmocks.MockEndpoint{
				awsmocks.MockStsAssumeRoleValidEndpointWithOptions(map[string]string{"Tags.member.1.Key": awsmocks.MockStsAssumeRoleTagKey, "Tags.member.1.Value": awsmocks.MockStsAssumeRoleTagValue}),
				awsmocks.MockStsGetCallerIdentityValidEndpoint,
			},
		},
		{
			Config: &awsbase.Config{
				AccessKey:             awsmocks.MockStaticAccessKey,
				AssumeRoleARN:         awsmocks.MockStsAssumeRoleArn,
				AssumeRoleSessionName: awsmocks.MockStsAssumeRoleSessionName,
				AssumeRoleTags: map[string]string{
					awsmocks.MockStsAssumeRoleTagKey: awsmocks.MockStsAssumeRoleTagValue,
				},
				AssumeRoleTransitiveTagKeys: []string{awsmocks.MockStsAssumeRoleTagKey},
				Region:                      "us-east-1",
				SecretKey:                   awsmocks.MockStaticSecretKey,
			},
			Description:              "config AssumeRoleTransitiveTagKeys",
			ExpectedCredentialsValue: awsmocks.MockStsAssumeRoleCredentialsV1,
			ExpectedRegion:           "us-east-1",
			MockStsEndpoints: []*awsmocks.MockEndpoint{
				awsmocks.MockStsAssumeRoleValidEndpointWithOptions(map[string]string{"Tags.member.1.Key": awsmocks.MockStsAssumeRoleTagKey, "Tags.member.1.Value": awsmocks.MockStsAssumeRoleTagValue, "TransitiveTagKeys.member.1": awsmocks.MockStsAssumeRoleTagKey}),
				awsmocks.MockStsGetCallerIdentityValidEndpoint,
			},
		},
		{
			Config: &awsbase.Config{
				Profile: "SharedCredentialsProfile",
				Region:  "us-east-1",
			},
			Description: "config Profile shared credentials profile aws_access_key_id",
			ExpectedCredentialsValue: credentials.Value{
				AccessKeyID:     "ProfileSharedCredentialsAccessKey",
				ProviderName:    credentials.SharedCredsProviderName,
				SecretAccessKey: "ProfileSharedCredentialsSecretKey",
			},
			ExpectedRegion: "us-east-1",
			MockStsEndpoints: []*awsmocks.MockEndpoint{
				awsmocks.MockStsGetCallerIdentityValidEndpoint,
			},
			SharedCredentialsFile: `
[default]
aws_access_key_id = DefaultSharedCredentialsAccessKey
aws_secret_access_key = DefaultSharedCredentialsSecretKey

[SharedCredentialsProfile]
aws_access_key_id = ProfileSharedCredentialsAccessKey
aws_secret_access_key = ProfileSharedCredentialsSecretKey
`,
		},
		{
			Config: &awsbase.Config{
				Profile: "SharedConfigurationProfile",
				Region:  "us-east-1",
			},
			Description:              "config Profile shared configuration credential_source Ec2InstanceMetadata",
			EnableEc2MetadataServer:  true,
			ExpectedCredentialsValue: awsmocks.MockStsAssumeRoleCredentialsV1,
			ExpectedRegion:           "us-east-1",
			MockStsEndpoints: []*awsmocks.MockEndpoint{
				awsmocks.MockStsAssumeRoleValidEndpoint,
				awsmocks.MockStsGetCallerIdentityValidEndpoint,
			},
			SharedConfigurationFile: fmt.Sprintf(`
[profile SharedConfigurationProfile]
credential_source = Ec2InstanceMetadata
role_arn = %[1]s
role_session_name = %[2]s
`, awsmocks.MockStsAssumeRoleArn, awsmocks.MockStsAssumeRoleSessionName),
		},
		{
			Config: &awsbase.Config{
				Profile: "SharedConfigurationProfile",
				Region:  "us-east-1",
			},
			Description: "config Profile shared configuration credential_source EcsContainer",
			EnvironmentVariables: map[string]string{
				"AWS_CONTAINER_CREDENTIALS_RELATIVE_URI": "/creds",
			},
			EnableEc2MetadataServer:    true,
			EnableEcsCredentialsServer: true,
			ExpectedCredentialsValue:   awsmocks.MockStsAssumeRoleCredentialsV1,
			ExpectedRegion:             "us-east-1",
			MockStsEndpoints: []*awsmocks.MockEndpoint{
				awsmocks.MockStsAssumeRoleValidEndpoint,
				awsmocks.MockStsGetCallerIdentityValidEndpoint,
			},
			SharedConfigurationFile: fmt.Sprintf(`
[profile SharedConfigurationProfile]
credential_source = EcsContainer
role_arn = %[1]s
role_session_name = %[2]s
`, awsmocks.MockStsAssumeRoleArn, awsmocks.MockStsAssumeRoleSessionName),
		},
		{
			Config: &awsbase.Config{
				Profile: "SharedConfigurationProfile",
				Region:  "us-east-1",
			},
			Description:              "config Profile shared configuration source_profile",
			ExpectedCredentialsValue: awsmocks.MockStsAssumeRoleCredentialsV1,
			ExpectedRegion:           "us-east-1",
			MockStsEndpoints: []*awsmocks.MockEndpoint{
				awsmocks.MockStsAssumeRoleValidEndpoint,
				awsmocks.MockStsGetCallerIdentityValidEndpoint,
			},
			SharedConfigurationFile: fmt.Sprintf(`
[profile SharedConfigurationProfile]
role_arn = %[1]s
role_session_name = %[2]s
source_profile = SharedConfigurationSourceProfile

[profile SharedConfigurationSourceProfile]
aws_access_key_id = SharedConfigurationSourceAccessKey
aws_secret_access_key = SharedConfigurationSourceSecretKey
`, awsmocks.MockStsAssumeRoleArn, awsmocks.MockStsAssumeRoleSessionName),
		},
		{
			Config: &awsbase.Config{
				Region: "us-east-1",
			},
			Description: "environment AWS_ACCESS_KEY_ID",
			EnvironmentVariables: map[string]string{
				"AWS_ACCESS_KEY_ID":     awsmocks.MockEnvAccessKey,
				"AWS_SECRET_ACCESS_KEY": awsmocks.MockEnvSecretKey,
			},
			ExpectedCredentialsValue: awsmocks.MockEnvCredentialsV1,
			ExpectedRegion:           "us-east-1",
			MockStsEndpoints: []*awsmocks.MockEndpoint{
				awsmocks.MockStsGetCallerIdentityValidEndpoint,
			},
		},
		{
			Config: &awsbase.Config{
				AssumeRoleARN:         awsmocks.MockStsAssumeRoleArn,
				AssumeRoleSessionName: awsmocks.MockStsAssumeRoleSessionName,
				Region:                "us-east-1",
			},
			Description: "environment AWS_ACCESS_KEY_ID config AssumeRoleARN access key",
			EnvironmentVariables: map[string]string{
				"AWS_ACCESS_KEY_ID":     awsmocks.MockEnvAccessKey,
				"AWS_SECRET_ACCESS_KEY": awsmocks.MockEnvSecretKey,
			},
			ExpectedCredentialsValue: awsmocks.MockStsAssumeRoleCredentialsV1,
			ExpectedRegion:           "us-east-1",
			MockStsEndpoints: []*awsmocks.MockEndpoint{
				awsmocks.MockStsAssumeRoleValidEndpoint,
				awsmocks.MockStsGetCallerIdentityValidEndpoint,
			},
		},
		{
			Config: &awsbase.Config{
				Region: "us-east-1",
			},
			Description: "environment AWS_PROFILE shared credentials profile aws_access_key_id",
			EnvironmentVariables: map[string]string{
				"AWS_PROFILE": "SharedCredentialsProfile",
			},
			ExpectedCredentialsValue: credentials.Value{
				AccessKeyID:     "ProfileSharedCredentialsAccessKey",
				ProviderName:    credentials.SharedCredsProviderName,
				SecretAccessKey: "ProfileSharedCredentialsSecretKey",
			},
			ExpectedRegion: "us-east-1",
			MockStsEndpoints: []*awsmocks.MockEndpoint{
				awsmocks.MockStsGetCallerIdentityValidEndpoint,
			},
			SharedCredentialsFile: `
[default]
aws_access_key_id = DefaultSharedCredentialsAccessKey
aws_secret_access_key = DefaultSharedCredentialsSecretKey

[SharedCredentialsProfile]
aws_access_key_id = ProfileSharedCredentialsAccessKey
aws_secret_access_key = ProfileSharedCredentialsSecretKey
`,
		},
		{
			Config: &awsbase.Config{
				Region: "us-east-1",
			},
			Description:             "environment AWS_PROFILE shared configuration credential_source Ec2InstanceMetadata",
			EnableEc2MetadataServer: true,
			EnvironmentVariables: map[string]string{
				"AWS_PROFILE": "SharedConfigurationProfile",
			},
			ExpectedCredentialsValue: awsmocks.MockStsAssumeRoleCredentialsV1,
			ExpectedRegion:           "us-east-1",
			MockStsEndpoints: []*awsmocks.MockEndpoint{
				awsmocks.MockStsAssumeRoleValidEndpoint,
				awsmocks.MockStsGetCallerIdentityValidEndpoint,
			},
			SharedConfigurationFile: fmt.Sprintf(`
[profile SharedConfigurationProfile]
credential_source = Ec2InstanceMetadata
role_arn = %[1]s
role_session_name = %[2]s
`, awsmocks.MockStsAssumeRoleArn, awsmocks.MockStsAssumeRoleSessionName),
		},
		{
			Config: &awsbase.Config{
				Region: "us-east-1",
			},
			Description:                "environment AWS_PROFILE shared configuration credential_source EcsContainer",
			EnableEc2MetadataServer:    true,
			EnableEcsCredentialsServer: true,
			EnvironmentVariables: map[string]string{
				"AWS_CONTAINER_CREDENTIALS_RELATIVE_URI": "/creds",
				"AWS_PROFILE":                            "SharedConfigurationProfile",
			},
			ExpectedCredentialsValue: awsmocks.MockStsAssumeRoleCredentialsV1,
			ExpectedRegion:           "us-east-1",
			MockStsEndpoints: []*awsmocks.MockEndpoint{
				awsmocks.MockStsAssumeRoleValidEndpoint,
				awsmocks.MockStsGetCallerIdentityValidEndpoint,
			},
			SharedConfigurationFile: fmt.Sprintf(`
[profile SharedConfigurationProfile]
credential_source = EcsContainer
role_arn = %[1]s
role_session_name = %[2]s
`, awsmocks.MockStsAssumeRoleArn, awsmocks.MockStsAssumeRoleSessionName),
		},
		{
			Config: &awsbase.Config{
				Region: "us-east-1",
			},
			Description: "environment AWS_PROFILE shared configuration source_profile",
			EnvironmentVariables: map[string]string{
				"AWS_PROFILE": "SharedConfigurationProfile",
			},
			ExpectedCredentialsValue: awsmocks.MockStsAssumeRoleCredentialsV1,
			ExpectedRegion:           "us-east-1",
			MockStsEndpoints: []*awsmocks.MockEndpoint{
				awsmocks.MockStsAssumeRoleValidEndpoint,
				awsmocks.MockStsGetCallerIdentityValidEndpoint,
			},
			SharedConfigurationFile: fmt.Sprintf(`
[profile SharedConfigurationProfile]
role_arn = %[1]s
role_session_name = %[2]s
source_profile = SharedConfigurationSourceProfile

[profile SharedConfigurationSourceProfile]
aws_access_key_id = SharedConfigurationSourceAccessKey
aws_secret_access_key = SharedConfigurationSourceSecretKey
`, awsmocks.MockStsAssumeRoleArn, awsmocks.MockStsAssumeRoleSessionName),
		},
		{
			Config: &awsbase.Config{
				Region: "us-east-1",
			},
			Description: "environment AWS_SESSION_TOKEN",
			EnvironmentVariables: map[string]string{
				"AWS_ACCESS_KEY_ID":     awsmocks.MockEnvAccessKey,
				"AWS_SECRET_ACCESS_KEY": awsmocks.MockEnvSecretKey,
				"AWS_SESSION_TOKEN":     awsmocks.MockEnvSessionToken,
			},
			ExpectedCredentialsValue: awsmocks.MockEnvCredentialsWithSessionToken,
			ExpectedRegion:           "us-east-1",
			MockStsEndpoints: []*awsmocks.MockEndpoint{
				awsmocks.MockStsGetCallerIdentityValidEndpoint,
			},
		},
		{
			Config: &awsbase.Config{
				Region: "us-east-1",
			},
			Description: "shared credentials default aws_access_key_id",
			ExpectedCredentialsValue: credentials.Value{
				AccessKeyID:     "DefaultSharedCredentialsAccessKey",
				ProviderName:    credentials.SharedCredsProviderName,
				SecretAccessKey: "DefaultSharedCredentialsSecretKey",
			},
			ExpectedRegion: "us-east-1",
			MockStsEndpoints: []*awsmocks.MockEndpoint{
				awsmocks.MockStsGetCallerIdentityValidEndpoint,
			},
			SharedCredentialsFile: `
[default]
aws_access_key_id = DefaultSharedCredentialsAccessKey
aws_secret_access_key = DefaultSharedCredentialsSecretKey
`,
		},
		{
			Config: &awsbase.Config{
				AssumeRoleARN:         awsmocks.MockStsAssumeRoleArn,
				AssumeRoleSessionName: awsmocks.MockStsAssumeRoleSessionName,
				Region:                "us-east-1",
			},
			Description:              "shared credentials default aws_access_key_id config AssumeRoleARN access key",
			ExpectedCredentialsValue: awsmocks.MockStsAssumeRoleCredentialsV1,
			ExpectedRegion:           "us-east-1",
			MockStsEndpoints: []*awsmocks.MockEndpoint{
				awsmocks.MockStsAssumeRoleValidEndpoint,
				awsmocks.MockStsGetCallerIdentityValidEndpoint,
			},
			SharedCredentialsFile: `
[default]
aws_access_key_id = DefaultSharedCredentialsAccessKey
aws_secret_access_key = DefaultSharedCredentialsSecretKey
`,
		},
		{
			Config: &awsbase.Config{
				Region: "us-east-1",
			},
			Description:              "web identity token access key",
			EnableEc2MetadataServer:  true,
			EnableWebIdentityToken:   true,
			ExpectedCredentialsValue: awsmocks.MockStsAssumeRoleWithWebIdentityCredentials,
			ExpectedRegion:           "us-east-1",
			MockStsEndpoints: []*awsmocks.MockEndpoint{
				awsmocks.MockStsAssumeRoleWithWebIdentityValidEndpoint,
				awsmocks.MockStsGetCallerIdentityValidEndpoint,
			},
		},
		{
			Config: &awsbase.Config{
				Region: "us-east-1",
			},
			Description:              "EC2 metadata access key",
			EnableEc2MetadataServer:  true,
			ExpectedCredentialsValue: awsmocks.MockEc2MetadataCredentialsV1,
			ExpectedRegion:           "us-east-1",
			MockStsEndpoints: []*awsmocks.MockEndpoint{
				awsmocks.MockStsGetCallerIdentityValidEndpoint,
			},
		},
		{
			Config: &awsbase.Config{
				AssumeRoleARN:         awsmocks.MockStsAssumeRoleArn,
				AssumeRoleSessionName: awsmocks.MockStsAssumeRoleSessionName,
				Region:                "us-east-1",
			},
			Description:              "EC2 metadata access key config AssumeRoleARN access key",
			EnableEc2MetadataServer:  true,
			ExpectedCredentialsValue: awsmocks.MockStsAssumeRoleCredentialsV1,
			ExpectedRegion:           "us-east-1",
			MockStsEndpoints: []*awsmocks.MockEndpoint{
				awsmocks.MockStsAssumeRoleValidEndpoint,
				awsmocks.MockStsGetCallerIdentityValidEndpoint,
			},
		},
		{
			Config: &awsbase.Config{
				Region: "us-east-1",
			},
			Description:                "ECS credentials access key",
			EnableEc2MetadataServer:    true,
			EnableEcsCredentialsServer: true,
			ExpectedCredentialsValue:   awsmocks.MockEcsCredentialsCredentials,
			ExpectedRegion:             "us-east-1",
			MockStsEndpoints: []*awsmocks.MockEndpoint{
				awsmocks.MockStsGetCallerIdentityValidEndpoint,
			},
		},
		{
			Config: &awsbase.Config{
				AssumeRoleARN:         awsmocks.MockStsAssumeRoleArn,
				AssumeRoleSessionName: awsmocks.MockStsAssumeRoleSessionName,
				Region:                "us-east-1",
			},
			Description:                "ECS credentials access key config AssumeRoleARN access key",
			EnableEc2MetadataServer:    true,
			EnableEcsCredentialsServer: true,
			ExpectedCredentialsValue:   awsmocks.MockStsAssumeRoleCredentialsV1,
			ExpectedRegion:             "us-east-1",
			MockStsEndpoints: []*awsmocks.MockEndpoint{
				awsmocks.MockStsAssumeRoleValidEndpoint,
				awsmocks.MockStsGetCallerIdentityValidEndpoint,
			},
		},
		{
			Config: &awsbase.Config{
				AccessKey: awsmocks.MockStaticAccessKey,
				Region:    "us-east-1",
				SecretKey: awsmocks.MockStaticSecretKey,
			},
			Description: "config AccessKey over environment AWS_ACCESS_KEY_ID",
			EnvironmentVariables: map[string]string{
				"AWS_ACCESS_KEY_ID":     awsmocks.MockEnvAccessKey,
				"AWS_SECRET_ACCESS_KEY": awsmocks.MockEnvSecretKey,
			},
			ExpectedCredentialsValue: awsmocks.MockStaticCredentialsV1,
			ExpectedRegion:           "us-east-1",
			MockStsEndpoints: []*awsmocks.MockEndpoint{
				awsmocks.MockStsGetCallerIdentityValidEndpoint,
			},
		},
		{
			Config: &awsbase.Config{
				AccessKey: awsmocks.MockStaticAccessKey,
				Region:    "us-east-1",
				SecretKey: awsmocks.MockStaticSecretKey,
			},
			Description:              "config AccessKey over shared credentials default aws_access_key_id",
			ExpectedCredentialsValue: awsmocks.MockStaticCredentialsV1,
			ExpectedRegion:           "us-east-1",
			MockStsEndpoints: []*awsmocks.MockEndpoint{
				awsmocks.MockStsGetCallerIdentityValidEndpoint,
			},
			SharedCredentialsFile: `
[default]
aws_access_key_id = DefaultSharedCredentialsAccessKey
aws_secret_access_key = DefaultSharedCredentialsSecretKey
`,
		},
		{
			Config: &awsbase.Config{
				AccessKey: awsmocks.MockStaticAccessKey,
				Region:    "us-east-1",
				SecretKey: awsmocks.MockStaticSecretKey,
			},
			Description:              "config AccessKey over EC2 metadata access key",
			EnableEc2MetadataServer:  true,
			ExpectedCredentialsValue: awsmocks.MockStaticCredentialsV1,
			ExpectedRegion:           "us-east-1",
			MockStsEndpoints: []*awsmocks.MockEndpoint{
				awsmocks.MockStsGetCallerIdentityValidEndpoint,
			},
		},
		{
			Config: &awsbase.Config{
				AccessKey: awsmocks.MockStaticAccessKey,
				Region:    "us-east-1",
				SecretKey: awsmocks.MockStaticSecretKey,
			},
			Description:                "config AccessKey over ECS credentials access key",
			EnableEc2MetadataServer:    true,
			EnableEcsCredentialsServer: true,
			ExpectedCredentialsValue:   awsmocks.MockStaticCredentialsV1,
			ExpectedRegion:             "us-east-1",
			MockStsEndpoints: []*awsmocks.MockEndpoint{
				awsmocks.MockStsGetCallerIdentityValidEndpoint,
			},
		},
		{
			Config: &awsbase.Config{
				Region: "us-east-1",
			},
			Description: "environment AWS_ACCESS_KEY_ID over shared credentials default aws_access_key_id",
			EnvironmentVariables: map[string]string{
				"AWS_ACCESS_KEY_ID":     awsmocks.MockEnvAccessKey,
				"AWS_SECRET_ACCESS_KEY": awsmocks.MockEnvSecretKey,
			},
			ExpectedCredentialsValue: awsmocks.MockEnvCredentialsV1,
			ExpectedRegion:           "us-east-1",
			MockStsEndpoints: []*awsmocks.MockEndpoint{
				awsmocks.MockStsGetCallerIdentityValidEndpoint,
			},
			SharedCredentialsFile: `
[default]
aws_access_key_id = DefaultSharedCredentialsAccessKey
aws_secret_access_key = DefaultSharedCredentialsSecretKey
`,
		},
		{
			Config: &awsbase.Config{
				Region: "us-east-1",
			},
			Description: "environment AWS_ACCESS_KEY_ID over EC2 metadata access key",
			EnvironmentVariables: map[string]string{
				"AWS_ACCESS_KEY_ID":     awsmocks.MockEnvAccessKey,
				"AWS_SECRET_ACCESS_KEY": awsmocks.MockEnvSecretKey,
			},
			EnableEc2MetadataServer:  true,
			ExpectedCredentialsValue: awsmocks.MockEnvCredentialsV1,
			ExpectedRegion:           "us-east-1",
			MockStsEndpoints: []*awsmocks.MockEndpoint{
				awsmocks.MockStsGetCallerIdentityValidEndpoint,
			},
		},
		{
			Config: &awsbase.Config{
				Region: "us-east-1",
			},
			Description: "environment AWS_ACCESS_KEY_ID over ECS credentials access key",
			EnvironmentVariables: map[string]string{
				"AWS_ACCESS_KEY_ID":     awsmocks.MockEnvAccessKey,
				"AWS_SECRET_ACCESS_KEY": awsmocks.MockEnvSecretKey,
			},
			EnableEc2MetadataServer:    true,
			EnableEcsCredentialsServer: true,
			ExpectedCredentialsValue:   awsmocks.MockEnvCredentialsV1,
			ExpectedRegion:             "us-east-1",
			MockStsEndpoints: []*awsmocks.MockEndpoint{
				awsmocks.MockStsGetCallerIdentityValidEndpoint,
			},
		},
		{
			Config: &awsbase.Config{
				Region: "us-east-1",
			},
			Description:             "shared credentials default aws_access_key_id over EC2 metadata access key",
			EnableEc2MetadataServer: true,
			ExpectedCredentialsValue: credentials.Value{
				AccessKeyID:     "DefaultSharedCredentialsAccessKey",
				ProviderName:    credentials.SharedCredsProviderName,
				SecretAccessKey: "DefaultSharedCredentialsSecretKey",
			},
			ExpectedRegion: "us-east-1",
			MockStsEndpoints: []*awsmocks.MockEndpoint{
				awsmocks.MockStsGetCallerIdentityValidEndpoint,
			},
			SharedCredentialsFile: `
[default]
aws_access_key_id = DefaultSharedCredentialsAccessKey
aws_secret_access_key = DefaultSharedCredentialsSecretKey
`,
		},
		{
			Config: &awsbase.Config{
				Region: "us-east-1",
			},
			Description:                "shared credentials default aws_access_key_id over ECS credentials access key",
			EnableEc2MetadataServer:    true,
			EnableEcsCredentialsServer: true,
			ExpectedCredentialsValue: credentials.Value{
				AccessKeyID:     "DefaultSharedCredentialsAccessKey",
				ProviderName:    credentials.SharedCredsProviderName,
				SecretAccessKey: "DefaultSharedCredentialsSecretKey",
			},
			ExpectedRegion: "us-east-1",
			MockStsEndpoints: []*awsmocks.MockEndpoint{
				awsmocks.MockStsGetCallerIdentityValidEndpoint,
			},
			SharedCredentialsFile: `
[default]
aws_access_key_id = DefaultSharedCredentialsAccessKey
aws_secret_access_key = DefaultSharedCredentialsSecretKey
`,
		},
		{
			Config: &awsbase.Config{
				Region: "us-east-1",
			},
			Description:                "ECS credentials access key over EC2 metadata access key",
			EnableEc2MetadataServer:    true,
			EnableEcsCredentialsServer: true,
			ExpectedCredentialsValue:   awsmocks.MockEcsCredentialsCredentials,
			ExpectedRegion:             "us-east-1",
			MockStsEndpoints: []*awsmocks.MockEndpoint{
				awsmocks.MockStsGetCallerIdentityValidEndpoint,
			},
		},
		{
			Config: &awsbase.Config{
				AccessKey:             awsmocks.MockStaticAccessKey,
				AssumeRoleARN:         awsmocks.MockStsAssumeRoleArn,
				AssumeRoleSessionName: awsmocks.MockStsAssumeRoleSessionName,
				DebugLogging:          true,
				Region:                "us-east-1",
				SecretKey:             awsmocks.MockStaticSecretKey,
			},
			Description: "assume role error",
			ExpectedError: func(err error) bool {
				return awsbase.IsCannotAssumeRoleError(err)
			},
			ExpectedRegion: "us-east-1",
			MockStsEndpoints: []*awsmocks.MockEndpoint{
				awsmocks.MockStsAssumeRoleInvalidEndpointInvalidClientTokenId,
				awsmocks.MockStsGetCallerIdentityValidEndpoint,
			},
		},
		{
			Config: &awsbase.Config{
				AccessKey: awsmocks.MockStaticAccessKey,
				Region:    "us-east-1",
				SecretKey: awsmocks.MockStaticSecretKey,
			},
			Description: "credential validation error",
			ExpectedError: func(err error) bool {
				return tfawserr.ErrCodeEquals(err, "AccessDenied")
			},
			MockStsEndpoints: []*awsmocks.MockEndpoint{
				awsmocks.MockStsGetCallerIdentityInvalidEndpointAccessDenied,
			},
		},
		{
			Config: &awsbase.Config{
				Profile: "SharedConfigurationProfile",
				Region:  "us-east-1",
			},
			Description: "session creation error",
			ExpectedError: func(err error) bool {
				return tfawserr.ErrCodeEquals(err, "CredentialRequiresARNError")
			},
			SharedConfigurationFile: `
[profile SharedConfigurationProfile]
source_profile = SourceSharedCredentials
`,
		},
		{
			Config: &awsbase.Config{
				AccessKey:           awsmocks.MockStaticAccessKey,
				Region:              "us-east-1",
				SecretKey:           awsmocks.MockStaticSecretKey,
				SkipCredsValidation: true,
			},
			Description:              "skip credentials validation",
			ExpectedCredentialsValue: awsmocks.MockStaticCredentialsV1,
			ExpectedRegion:           "us-east-1",
		},
		{
			Config: &awsbase.Config{
				Region:               "us-east-1",
				SkipMetadataApiCheck: true,
			},
			Description:             "skip EC2 metadata API check",
			EnableEc2MetadataServer: true,
			ExpectedError: func(err error) bool {
				return awsbase.IsNoValidCredentialSourcesError(err)
			},
			ExpectedRegion: "us-east-1",
		},
		{
			Config: &awsbase.Config{
				AccessKey: awsmocks.MockStaticAccessKey,
				Region:    "us-east-1",
				SecretKey: awsmocks.MockStaticSecretKey,
			},
			Description:              "standard User-Agent",
			ExpectedCredentialsValue: awsmocks.MockStaticCredentialsV1,
			ExpectedRegion:           "us-east-1",
			ExpectedUserAgent:        awsSdkGoUserAgent(),
			MockStsEndpoints: []*awsmocks.MockEndpoint{
				awsmocks.MockStsGetCallerIdentityValidEndpoint,
			},
		},
		{
			Config: &awsbase.Config{
				AccessKey: awsmocks.MockStaticAccessKey,
				Region:    "us-east-1",
				SecretKey: awsmocks.MockStaticSecretKey,
				UserAgentProducts: []*awsbase.UserAgentProduct{
					{
						Name:    "first",
						Version: "1.0",
					},
					{
						Name:    "second",
						Version: "1.2.3",
						Extra:   []string{"+https://www.example.com/"},
					},
				},
			},
			Description:              "customized User-Agent",
			ExpectedCredentialsValue: awsmocks.MockStaticCredentialsV1,
			ExpectedRegion:           "us-east-1",
			ExpectedUserAgent:        "first/1.0 second/1.2.3 (+https://www.example.com/) " + awsSdkGoUserAgent(),
			MockStsEndpoints: []*awsmocks.MockEndpoint{
				awsmocks.MockStsGetCallerIdentityValidEndpoint,
			},
		},
	}

	for _, testCase := range testCases {
		testCase := testCase

		t.Run(testCase.Description, func(t *testing.T) {
			oldEnv := awsmocks.InitSessionTestEnv()
			defer awsmocks.PopEnv(oldEnv)

			if testCase.EnableEc2MetadataServer {
				closeEc2Metadata := awsmocks.AwsMetadataApiMock(append(awsmocks.Ec2metadata_securityCredentialsEndpoints, awsmocks.Ec2metadata_instanceIdEndpoint, awsmocks.Ec2metadata_iamInfoEndpoint))
				defer closeEc2Metadata()
			}

			if testCase.EnableEcsCredentialsServer {
				closeEcsCredentials := awsmocks.EcsCredentialsApiMock()
				defer closeEcsCredentials()
			}

			if testCase.EnableWebIdentityToken {
				file, err := ioutil.TempFile("", "aws-sdk-go-base-web-identity-token-file")

				if err != nil {
					t.Fatalf("unexpected error creating temporary shared configuration file: %s", err)
				}

				defer os.Remove(file.Name())

				err = ioutil.WriteFile(file.Name(), []byte(awsmocks.MockWebIdentityToken), 0600)

				if err != nil {
					t.Fatalf("unexpected error writing shared configuration file: %s", err)
				}

				os.Setenv("AWS_ROLE_ARN", awsmocks.MockStsAssumeRoleWithWebIdentityArn)
				os.Setenv("AWS_ROLE_SESSION_NAME", awsmocks.MockStsAssumeRoleWithWebIdentitySessionName)
				os.Setenv("AWS_WEB_IDENTITY_TOKEN_FILE", file.Name())
			}

			closeSts, mockStsSession, err := awsmocks.GetMockedAwsApiSession("STS", testCase.MockStsEndpoints)
			defer closeSts()

			if err != nil {
				t.Fatalf("unexpected error creating mock STS server: %s", err)
			}

			if mockStsSession != nil && mockStsSession.Config != nil {
				testCase.Config.StsEndpoint = aws.StringValue(mockStsSession.Config.Endpoint)
			}

			if testCase.SharedConfigurationFile != "" {
				file, err := ioutil.TempFile("", "aws-sdk-go-base-shared-configuration-file")

				if err != nil {
					t.Fatalf("unexpected error creating temporary shared configuration file: %s", err)
				}

				defer os.Remove(file.Name())

				err = ioutil.WriteFile(file.Name(), []byte(testCase.SharedConfigurationFile), 0600)

				if err != nil {
					t.Fatalf("unexpected error writing shared configuration file: %s", err)
				}

				// Config does not provide a passthrough for session.Options.SharedConfigFiles
				os.Setenv("AWS_CONFIG_FILE", file.Name())
			}

			if testCase.SharedCredentialsFile != "" {
				file, err := ioutil.TempFile("", "aws-sdk-go-base-shared-credentials-file")

				if err != nil {
					t.Fatalf("unexpected error creating temporary shared credentials file: %s", err)
				}

				defer os.Remove(file.Name())

				err = ioutil.WriteFile(file.Name(), []byte(testCase.SharedCredentialsFile), 0600)

				if err != nil {
					t.Fatalf("unexpected error writing shared credentials file: %s", err)
				}

				// Config does not provide a passthrough for session.Options.SharedConfigFiles
				testCase.Config.CredsFilename = file.Name()
			}

			for k, v := range testCase.EnvironmentVariables {
				os.Setenv(k, v)
			}

			actualSession, err := GetSession(testCase.Config)

			if err != nil {
				if testCase.ExpectedError == nil {
					t.Fatalf("expected no error, got error: %s", err)
				}

				if !testCase.ExpectedError(err) {
					t.Fatalf("unexpected GetSession() error: %s", err)
				}

				t.Logf("received expected error: %s", err)
				return
			}

			if err == nil && testCase.ExpectedError != nil {
				t.Fatalf("expected error, got no error")
			}

			credentialsValue, err := actualSession.Config.Credentials.Get()

			if err != nil {
				t.Fatalf("unexpected credentials Get() error: %s", err)
			}

			if !reflect.DeepEqual(credentialsValue, testCase.ExpectedCredentialsValue) {
				t.Fatalf("unexpected credentials: %#v", credentialsValue)
			}

			if expected, actual := testCase.ExpectedRegion, aws.StringValue(actualSession.Config.Region); expected != actual {
				t.Fatalf("expected region (%s), got: %s", expected, actual)
			}

			if testCase.ExpectedUserAgent != "" {
				clientInfo := metadata.ClientInfo{
					Endpoint:    "http://endpoint",
					SigningName: "",
				}
				conn := client.New(*actualSession.Config, clientInfo, actualSession.Handlers)

				req := conn.NewRequest(&request.Operation{Name: "Operation"}, nil, nil)

				if err := req.Build(); err != nil {
					t.Fatalf("expect no Request.Build() error, got %s", err)
				}

				if e, a := testCase.ExpectedUserAgent, req.HTTPRequest.Header.Get("User-Agent"); e != a {
					t.Errorf("expected User-Agent (%s), got: %s", e, a)
				}
			}
		})
	}
}

func TestGetSessionWithAccountIDAndPartition(t *testing.T) {
	oldEnv := awsmocks.InitSessionTestEnv()
	defer awsmocks.PopEnv(oldEnv)

	ts := awsmocks.MockAwsApiServer("STS", []*awsmocks.MockEndpoint{
		awsmocks.MockStsGetCallerIdentityValidEndpoint,
	})
	defer ts.Close()

	testCases := []struct {
		desc              string
		config            *awsbase.Config
		expectedAcctID    string
		expectedPartition string
		expectedError     bool
	}{
		{"StandardProvider_Config", &awsbase.Config{
			AccessKey:         "MockAccessKey",
			SecretKey:         "MockSecretKey",
			Region:            "us-west-2",
			UserAgentProducts: []*awsbase.UserAgentProduct{{}},
			StsEndpoint:       ts.URL},
			"222222222222", "aws", false},
		{"SkipCredsValidation_Config", &awsbase.Config{
			AccessKey:           "MockAccessKey",
			SecretKey:           "MockSecretKey",
			Region:              "us-west-2",
			SkipCredsValidation: true,
			UserAgentProducts:   []*awsbase.UserAgentProduct{{}},
			StsEndpoint:         ts.URL},
			"222222222222", "aws", false},
		{"SkipRequestingAccountId_Config", &awsbase.Config{
			AccessKey:               "MockAccessKey",
			SecretKey:               "MockSecretKey",
			Region:                  "us-west-2",
			SkipCredsValidation:     true,
			SkipRequestingAccountId: true,
			UserAgentProducts:       []*awsbase.UserAgentProduct{{}},
			StsEndpoint:             ts.URL},
			"", "aws", false},
		// {"WithAssumeRole", &awsbase.Config{
		// 		AccessKey: "MockAccessKey",
		// 		SecretKey: "MockSecretKey",
		// 		Region: "us-west-2",
		// 		UserAgentProducts: []*awsbase.UserAgentProduct{{}},
		// 		AssumeRoleARN: "arn:aws:iam::222222222222:user/Alice"},
		// 	"222222222222", "aws"},
		{"NoCredentialProviders_Config", &awsbase.Config{
			AccessKey:         "",
			SecretKey:         "",
			Region:            "us-west-2",
			UserAgentProducts: []*awsbase.UserAgentProduct{{}},
			StsEndpoint:       ts.URL},
			"", "", true},
	}

	for _, testCase := range testCases {
		tc := testCase

		t.Run(tc.desc, func(t *testing.T) {
			sess, acctID, part, err := GetSessionWithAccountIDAndPartition(tc.config)
			if err != nil {
				if !tc.expectedError {
					t.Fatalf("expected no error, got: %s", err)
				}

				if !awsbase.IsNoValidCredentialSourcesError(err) {
					t.Fatalf("expected no valid credential sources error, got: %s", err)
				}

				t.Logf("received expected error: %s", err)
				return
			}

			if sess == nil {
				t.Error("unexpected empty session")
			}

			if acctID != tc.expectedAcctID {
				t.Errorf("expected account ID (%s), got: %s", tc.expectedAcctID, acctID)
			}

			if part != tc.expectedPartition {
				t.Errorf("expected partition (%s), got: %s", tc.expectedPartition, part)
			}
		})
	}
}

func awsSdkGoUserAgent() string {
	return fmt.Sprintf("%s/%s (%s; %s; %s)", aws.SDKName, aws.SDKVersion, runtime.Version(), runtime.GOOS, runtime.GOARCH)
}
