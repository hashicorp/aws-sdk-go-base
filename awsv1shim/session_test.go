package awsv1shim

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"runtime"
	"testing"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/client"
	"github.com/aws/aws-sdk-go/aws/client/metadata"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/request"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	awsbase "github.com/hashicorp/aws-sdk-go-base"
	"github.com/hashicorp/aws-sdk-go-base/awsmocks"
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
			awsConfig, err := awsbase.GetAwsConfig(context.Background(), tc.config)
			if err != nil && tc.expectError == false {
				t.Fatalf("GetAwsConfig() resulted in an error %s", err)
			}
			if err == nil && tc.expectError == true {
				t.Fatal("Expected error not returned by GetAwsConfig()")
			}

			opts, err := getSessionOptions(&awsConfig, tc.config)
			if err != nil && tc.expectError == false {
				t.Fatalf("getSessionOptions() resulted in an error %s", err)
			}

			if opts == nil && tc.expectError == false {
				t.Error("getSessionOptions() resulted in a nil set of options")
			}

			if err == nil && tc.expectError == true {
				t.Fatal("Expected error not returned by getSessionOptions()")
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
		// 		{
		// 			Config: &awsbase.Config{
		// 				Profile: "SharedConfigurationProfile",
		// 				Region:  "us-east-1",
		// 			},
		// 			Description: "config Profile shared configuration credential_source EcsContainer",
		// 			EnvironmentVariables: map[string]string{
		// 				"AWS_CONTAINER_CREDENTIALS_RELATIVE_URI": "/creds",
		// 			},
		// 			EnableEc2MetadataServer:    true,
		// 			EnableEcsCredentialsServer: true,
		// 			ExpectedCredentialsValue:   awsmocks.MockStsAssumeRoleCredentialsV1,
		// 			ExpectedRegion:             "us-east-1",
		// 			MockStsEndpoints: []*awsmocks.MockEndpoint{
		// 				awsmocks.MockStsAssumeRoleValidEndpoint,
		// 				awsmocks.MockStsGetCallerIdentityValidEndpoint,
		// 			},
		// 			SharedConfigurationFile: fmt.Sprintf(`
		// [profile SharedConfigurationProfile]
		// credential_source = EcsContainer
		// role_arn = %[1]s
		// role_session_name = %[2]s
		// `, awsmocks.MockStsAssumeRoleArn, awsmocks.MockStsAssumeRoleSessionName),
		// 		},
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
		// 		{
		// 			Config: &awsbase.Config{
		// 				Region: "us-east-1",
		// 			},
		// 			Description:                "environment AWS_PROFILE shared configuration credential_source EcsContainer",
		// 			EnableEc2MetadataServer:    true,
		// 			EnableEcsCredentialsServer: true,
		// 			EnvironmentVariables: map[string]string{
		// 				"AWS_CONTAINER_CREDENTIALS_RELATIVE_URI": "/creds",
		// 				"AWS_PROFILE":                            "SharedConfigurationProfile",
		// 			},
		// 			ExpectedCredentialsValue: awsmocks.MockStsAssumeRoleCredentialsV1,
		// 			ExpectedRegion:           "us-east-1",
		// 			MockStsEndpoints: []*awsmocks.MockEndpoint{
		// 				awsmocks.MockStsAssumeRoleValidEndpoint,
		// 				awsmocks.MockStsGetCallerIdentityValidEndpoint,
		// 			},
		// 			SharedConfigurationFile: fmt.Sprintf(`
		// [profile SharedConfigurationProfile]
		// credential_source = EcsContainer
		// role_arn = %[1]s
		// role_session_name = %[2]s
		// `, awsmocks.MockStsAssumeRoleArn, awsmocks.MockStsAssumeRoleSessionName),
		// 		},
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
			ExpectedCredentialsValue: awsmocks.MockEnvCredentialsWithSessionTokenV1,
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
			ExpectedCredentialsValue: awsmocks.MockStsAssumeRoleWithWebIdentityCredentialsV1,
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
			ExpectedCredentialsValue:   awsmocks.MockEcsCredentialsCredentialsV1,
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
			ExpectedCredentialsValue:   awsmocks.MockEcsCredentialsCredentialsV1,
			ExpectedRegion:             "us-east-1",
			MockStsEndpoints: []*awsmocks.MockEndpoint{
				awsmocks.MockStsGetCallerIdentityValidEndpoint,
			},
		},
		{
			Config: &awsbase.Config{
				AccessKey: awsmocks.MockStaticAccessKey,
				SecretKey: awsmocks.MockStaticSecretKey,
			},
			Description:              "retrieve region from shared configuration file",
			ExpectedCredentialsValue: awsmocks.MockStaticCredentialsV1,
			ExpectedRegion:           "us-east-1",
			MockStsEndpoints: []*awsmocks.MockEndpoint{
				awsmocks.MockStsGetCallerIdentityValidEndpoint,
			},
			SharedConfigurationFile: `
[default]
region = us-east-1
`,
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
		// 		{
		// 			Config: &awsbase.Config{
		// 				AccessKey: awsmocks.MockStaticAccessKey,
		// 				Region:    "us-east-1",
		// 				SecretKey: awsmocks.MockStaticSecretKey,
		// 			},
		// 			Description: "credential validation error",
		// 			ExpectedError: func(err error) bool {
		// 				return tfawserr.ErrCodeEquals(err, "AccessDenied")
		// 			},
		// 			MockStsEndpoints: []*awsmocks.MockEndpoint{
		// 				awsmocks.MockStsGetCallerIdentityInvalidEndpointAccessDenied,
		// 			},
		// 		},

		// TODO: handle both GetAwsConfig() and GetSession() errors
		// 		{
		// 			Config: &awsbase.Config{
		// 				Profile: "SharedConfigurationProfile",
		// 				Region:  "us-east-1",
		// 			},
		// 			Description: "session creation error",
		// 			ExpectedError: func(err error) bool {
		// 				return tfawserr.ErrCodeEquals(err, "CredentialRequiresARNError")
		// 			},
		// 			SharedConfigurationFile: `
		// [profile SharedConfigurationProfile]
		// source_profile = SourceSharedCredentials
		// `,
		// 		},
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

				testCase.Config.SharedConfigFilename = file.Name()
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

				testCase.Config.SharedCredentialsFilename = file.Name()
			}

			for k, v := range testCase.EnvironmentVariables {
				os.Setenv(k, v)
			}

			awsConfig, err := awsbase.GetAwsConfig(context.Background(), testCase.Config)
			if err != nil {
				if testCase.ExpectedError == nil {
					t.Fatalf("expected no error from GetAwsConfig(), got '%[1]T' error: %[1]s", err)
				}

				if !testCase.ExpectedError(err) {
					t.Fatalf("unexpected GetAwsConfig() '%[1]T' error: %[1]s", err)
				}

				t.Logf("received expected error: %s", err)
				return
			}
			actualSession, err := GetSession(&awsConfig, testCase.Config)
			if err != nil {
				if testCase.ExpectedError == nil {
					t.Fatalf("expected no error from GetSession(), got '%[1]T' error: %[1]s", err)
				}

				if !testCase.ExpectedError(err) {
					t.Fatalf("unexpected GetSession() '%[1]T' error: %[1]s", err)
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

			if diff := cmp.Diff(credentialsValue, testCase.ExpectedCredentialsValue, cmpopts.IgnoreFields(credentials.Value{}, "ProviderName")); diff != "" {
				t.Fatalf("unexpected credentials: (- got, + expected)\n%s", diff)
			}
			// TODO: return correct credentials.ProviderName
			// TODO: test credentials.ExpiresAt()

			if expected, actual := testCase.ExpectedRegion, aws.StringValue(actualSession.Config.Region); expected != actual {
				t.Fatalf("expected region (%s), got: %s", expected, actual)
			}
		})
	}
}

func TestUserAgentProducts(t *testing.T) {
	testCases := []struct {
		Config               *awsbase.Config
		Description          string
		EnvironmentVariables map[string]string
		ExpectedUserAgent    string
		MockStsEndpoints     []*awsmocks.MockEndpoint
	}{
		{
			Config: &awsbase.Config{
				AccessKey: awsmocks.MockStaticAccessKey,
				Region:    "us-east-1",
				SecretKey: awsmocks.MockStaticSecretKey,
			},
			Description:       "standard User-Agent",
			ExpectedUserAgent: awsSdkGoUserAgent(),
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
			Description: "customized User-Agent TF_APPEND_USER_AGENT",
			EnvironmentVariables: map[string]string{
				appendUserAgentEnvVar: "Last",
			},
			ExpectedUserAgent: awsSdkGoUserAgent() + " Last",
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
			Description:       "customized User-Agent",
			ExpectedUserAgent: "first/1.0 second/1.2.3 (+https://www.example.com/) " + awsSdkGoUserAgent(),
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
			Description: "customized User-Agent Products and TF_APPEND_USER_AGENT",
			EnvironmentVariables: map[string]string{
				appendUserAgentEnvVar: "Last",
			},
			ExpectedUserAgent: "first/1.0 second/1.2.3 (+https://www.example.com/) " + awsSdkGoUserAgent() + " Last",
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

			for k, v := range testCase.EnvironmentVariables {
				os.Setenv(k, v)
			}

			closeSts, mockStsSession, err := awsmocks.GetMockedAwsApiSession("STS", testCase.MockStsEndpoints)
			defer closeSts()

			if err != nil {
				t.Fatalf("unexpected error creating mock STS server: %s", err)
			}

			if mockStsSession != nil && mockStsSession.Config != nil {
				testCase.Config.StsEndpoint = aws.StringValue(mockStsSession.Config.Endpoint)
			}

			awsConfig, err := awsbase.GetAwsConfig(context.Background(), testCase.Config)
			if err != nil {
				t.Fatalf("GetAwsConfig() returned error: %s", err)
			}
			actualSession, err := GetSession(&awsConfig, testCase.Config)
			if err != nil {
				t.Fatalf("error in GetSession() '%[1]T': %[1]s", err)
			}

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
				t.Errorf("expected User-Agent %q, got: %q", e, a)
			}
		})
	}
}

func awsSdkGoUserAgent() string {
	return fmt.Sprintf("%s/%s (%s; %s; %s)", aws.SDKName, aws.SDKVersion, runtime.Version(), runtime.GOOS, runtime.GOARCH)
}

func TestGetSessionWithAccountIDAndPartition(t *testing.T) {
	oldEnv := awsmocks.InitSessionTestEnv()
	defer awsmocks.PopEnv(oldEnv)

	testCases := []struct {
		desc              string
		config            *awsbase.Config
		expectedAcctID    string
		expectedPartition string
		expectError       bool
		mockStsEndpoints  []*awsmocks.MockEndpoint
	}{
		{
			"StandardProvider_Config",
			&awsbase.Config{
				AccessKey: "MockAccessKey",
				SecretKey: "MockSecretKey",
				Region:    "us-west-2"},
			"222222222222", "aws", false,
			[]*awsmocks.MockEndpoint{
				awsmocks.MockStsGetCallerIdentityValidEndpoint,
			},
		},
		{
			"SkipCredsValidation_Config",
			&awsbase.Config{
				AccessKey:           "MockAccessKey",
				SecretKey:           "MockSecretKey",
				Region:              "us-west-2",
				SkipCredsValidation: true},
			"222222222222", "aws", false,
			[]*awsmocks.MockEndpoint{
				awsmocks.MockStsGetCallerIdentityValidEndpoint,
			},
		},
		{
			"SkipRequestingAccountId_Config",
			&awsbase.Config{
				AccessKey:               "MockAccessKey",
				SecretKey:               "MockSecretKey",
				Region:                  "us-west-2",
				SkipCredsValidation:     true,
				SkipRequestingAccountId: true},
			"", "aws", false, []*awsmocks.MockEndpoint{},
		},
		{
			"WithAssumeRole",
			&awsbase.Config{
				AccessKey:             "MockAccessKey",
				SecretKey:             "MockSecretKey",
				Region:                "us-west-2",
				AssumeRoleARN:         awsmocks.MockStsAssumeRoleArn,
				AssumeRoleSessionName: awsmocks.MockStsAssumeRoleSessionName},
			"555555555555", "aws", false, []*awsmocks.MockEndpoint{
				awsmocks.MockStsAssumeRoleValidEndpoint,
				awsmocks.MockStsGetCallerIdentityValidEndpoint,
			},
		},
		{
			"NoCredentialProviders_Config",
			&awsbase.Config{
				AccessKey: "",
				SecretKey: "",
				Region:    "us-west-2"},
			"", "", true, []*awsmocks.MockEndpoint{},
		},
	}

	for _, testCase := range testCases {
		tc := testCase

		t.Run(tc.desc, func(t *testing.T) {
			ts := awsmocks.MockAwsApiServer("STS", tc.mockStsEndpoints)
			defer ts.Close()
			tc.config.StsEndpoint = ts.URL

			awsConfig, err := awsbase.GetAwsConfig(context.Background(), tc.config)
			if err != nil {
				if !tc.expectError {
					t.Fatalf("expected no error from GetAwsConfig(), got: %s", err)
				}

				if !awsbase.IsNoValidCredentialSourcesError(err) {
					t.Fatalf("expected no valid credential sources error, got: %s", err)
				}

				t.Logf("received expected error: %s", err)
				return
			}
			sess, acctID, part, err := GetSessionWithAccountIDAndPartition(&awsConfig, tc.config)
			if err != nil {
				if !tc.expectError {
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
