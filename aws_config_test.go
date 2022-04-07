package awsbase

import (
	"context"
	"errors"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/aws/retry"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/feature/ec2/imds"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/aws/smithy-go/middleware"
	smithyhttp "github.com/aws/smithy-go/transport/http"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/hashicorp/aws-sdk-go-base/v2/internal/awsconfig"
	"github.com/hashicorp/aws-sdk-go-base/v2/internal/constants"
	"github.com/hashicorp/aws-sdk-go-base/v2/mockdata"
	"github.com/hashicorp/aws-sdk-go-base/v2/servicemocks"
)

const (
	// Shockingly, this is not defined in the SDK
	sharedConfigCredentialsProvider = "SharedConfigCredentials"
)

func TestGetAwsConfig(t *testing.T) {
	testCases := []struct {
		Config                     *Config
		Description                string
		EnableEc2MetadataServer    bool
		EnableEcsCredentialsServer bool
		EnableWebIdentityToken     bool
		EnvironmentVariables       map[string]string
		ExpectedCredentialsValue   aws.Credentials
		ExpectedRegion             string
		ExpectedError              func(err error) bool
		MockStsEndpoints           []*servicemocks.MockEndpoint
		SharedConfigurationFile    string
		SharedCredentialsFile      string
	}{
		{
			Config:      &Config{},
			Description: "no configuration or credentials",
			ExpectedError: func(err error) bool {
				return IsNoValidCredentialSourcesError(err)
			},
		},
		{
			Config: &Config{
				AccessKey: servicemocks.MockStaticAccessKey,
				Region:    "us-east-1",
				SecretKey: servicemocks.MockStaticSecretKey,
			},
			Description:              "config AccessKey",
			ExpectedCredentialsValue: mockdata.MockStaticCredentials,
			ExpectedRegion:           "us-east-1",
			MockStsEndpoints: []*servicemocks.MockEndpoint{
				servicemocks.MockStsGetCallerIdentityValidEndpoint,
			},
		},
		{
			Config: &Config{
				AccessKey: servicemocks.MockStaticAccessKey,
				AssumeRole: &AssumeRole{
					RoleARN:     servicemocks.MockStsAssumeRoleArn,
					SessionName: servicemocks.MockStsAssumeRoleSessionName,
				},
				Region:    "us-east-1",
				SecretKey: servicemocks.MockStaticSecretKey,
			},
			Description:              "config AccessKey config AssumeRoleARN access key",
			ExpectedCredentialsValue: mockdata.MockStsAssumeRoleCredentials,
			ExpectedRegion:           "us-east-1",
			MockStsEndpoints: []*servicemocks.MockEndpoint{
				servicemocks.MockStsAssumeRoleValidEndpoint,
				servicemocks.MockStsGetCallerIdentityValidEndpoint,
			},
		},
		{
			Config: &Config{
				AccessKey: servicemocks.MockStaticAccessKey,
				AssumeRole: &AssumeRole{
					RoleARN:     servicemocks.MockStsAssumeRoleArn,
					Duration:    1 * time.Hour,
					SessionName: servicemocks.MockStsAssumeRoleSessionName,
				},
				Region:    "us-east-1",
				SecretKey: servicemocks.MockStaticSecretKey,
			},
			Description:              "config AssumeRoleDurationSeconds",
			ExpectedCredentialsValue: mockdata.MockStsAssumeRoleCredentials,
			ExpectedRegion:           "us-east-1",
			MockStsEndpoints: []*servicemocks.MockEndpoint{
				servicemocks.MockStsAssumeRoleValidEndpointWithOptions(map[string]string{"DurationSeconds": "3600"}),
				servicemocks.MockStsGetCallerIdentityValidEndpoint,
			},
		},
		{
			Config: &Config{
				AccessKey: servicemocks.MockStaticAccessKey,
				AssumeRole: &AssumeRole{
					RoleARN:     servicemocks.MockStsAssumeRoleArn,
					ExternalID:  servicemocks.MockStsAssumeRoleExternalId,
					SessionName: servicemocks.MockStsAssumeRoleSessionName,
				},
				Region:    "us-east-1",
				SecretKey: servicemocks.MockStaticSecretKey,
			},
			Description:              "config AssumeRoleExternalID",
			ExpectedCredentialsValue: mockdata.MockStsAssumeRoleCredentials,
			ExpectedRegion:           "us-east-1",
			MockStsEndpoints: []*servicemocks.MockEndpoint{
				servicemocks.MockStsAssumeRoleValidEndpointWithOptions(map[string]string{"ExternalId": servicemocks.MockStsAssumeRoleExternalId}),
				servicemocks.MockStsGetCallerIdentityValidEndpoint,
			},
		},
		{
			Config: &Config{
				AccessKey: servicemocks.MockStaticAccessKey,
				AssumeRole: &AssumeRole{
					RoleARN:     servicemocks.MockStsAssumeRoleArn,
					Policy:      servicemocks.MockStsAssumeRolePolicy,
					SessionName: servicemocks.MockStsAssumeRoleSessionName,
				},
				Region:    "us-east-1",
				SecretKey: servicemocks.MockStaticSecretKey,
			},
			Description:              "config AssumeRolePolicy",
			ExpectedCredentialsValue: mockdata.MockStsAssumeRoleCredentials,
			ExpectedRegion:           "us-east-1",
			MockStsEndpoints: []*servicemocks.MockEndpoint{
				servicemocks.MockStsAssumeRoleValidEndpointWithOptions(map[string]string{"Policy": servicemocks.MockStsAssumeRolePolicy}),
				servicemocks.MockStsGetCallerIdentityValidEndpoint,
			},
		},
		{
			Config: &Config{
				AccessKey: servicemocks.MockStaticAccessKey,
				AssumeRole: &AssumeRole{
					RoleARN:     servicemocks.MockStsAssumeRoleArn,
					PolicyARNs:  []string{servicemocks.MockStsAssumeRolePolicyArn},
					SessionName: servicemocks.MockStsAssumeRoleSessionName,
				},
				Region:    "us-east-1",
				SecretKey: servicemocks.MockStaticSecretKey,
			},
			Description:              "config AssumeRolePolicyARNs",
			ExpectedCredentialsValue: mockdata.MockStsAssumeRoleCredentials,
			ExpectedRegion:           "us-east-1",
			MockStsEndpoints: []*servicemocks.MockEndpoint{
				servicemocks.MockStsAssumeRoleValidEndpointWithOptions(map[string]string{"PolicyArns.member.1.arn": servicemocks.MockStsAssumeRolePolicyArn}),
				servicemocks.MockStsGetCallerIdentityValidEndpoint,
			},
		},
		{
			Config: &Config{
				AccessKey: servicemocks.MockStaticAccessKey,
				AssumeRole: &AssumeRole{
					RoleARN:     servicemocks.MockStsAssumeRoleArn,
					SessionName: servicemocks.MockStsAssumeRoleSessionName,
					Tags: map[string]string{
						servicemocks.MockStsAssumeRoleTagKey: servicemocks.MockStsAssumeRoleTagValue,
					},
				},
				Region:    "us-east-1",
				SecretKey: servicemocks.MockStaticSecretKey,
			},
			Description:              "config AssumeRoleTags",
			ExpectedCredentialsValue: mockdata.MockStsAssumeRoleCredentials,
			ExpectedRegion:           "us-east-1",
			MockStsEndpoints: []*servicemocks.MockEndpoint{
				servicemocks.MockStsAssumeRoleValidEndpointWithOptions(map[string]string{"Tags.member.1.Key": servicemocks.MockStsAssumeRoleTagKey, "Tags.member.1.Value": servicemocks.MockStsAssumeRoleTagValue}),
				servicemocks.MockStsGetCallerIdentityValidEndpoint,
			},
		},
		{
			Config: &Config{
				AccessKey: servicemocks.MockStaticAccessKey,
				AssumeRole: &AssumeRole{
					RoleARN:     servicemocks.MockStsAssumeRoleArn,
					SessionName: servicemocks.MockStsAssumeRoleSessionName,
					Tags: map[string]string{
						servicemocks.MockStsAssumeRoleTagKey: servicemocks.MockStsAssumeRoleTagValue,
					},
					TransitiveTagKeys: []string{servicemocks.MockStsAssumeRoleTagKey},
				},
				Region:    "us-east-1",
				SecretKey: servicemocks.MockStaticSecretKey,
			},
			Description:              "config AssumeRoleTransitiveTagKeys",
			ExpectedCredentialsValue: mockdata.MockStsAssumeRoleCredentials,
			ExpectedRegion:           "us-east-1",
			MockStsEndpoints: []*servicemocks.MockEndpoint{
				servicemocks.MockStsAssumeRoleValidEndpointWithOptions(map[string]string{"Tags.member.1.Key": servicemocks.MockStsAssumeRoleTagKey, "Tags.member.1.Value": servicemocks.MockStsAssumeRoleTagValue, "TransitiveTagKeys.member.1": servicemocks.MockStsAssumeRoleTagKey}),
				servicemocks.MockStsGetCallerIdentityValidEndpoint,
			},
		},
		{
			Config: &Config{
				Profile: "SharedCredentialsProfile",
				Region:  "us-east-1",
			},
			Description: "config Profile shared credentials profile aws_access_key_id",
			ExpectedCredentialsValue: aws.Credentials{
				AccessKeyID:     "ProfileSharedCredentialsAccessKey",
				SecretAccessKey: "ProfileSharedCredentialsSecretKey",
				Source:          sharedConfigCredentialsProvider,
			},
			ExpectedRegion: "us-east-1",
			MockStsEndpoints: []*servicemocks.MockEndpoint{
				servicemocks.MockStsGetCallerIdentityValidEndpoint,
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
			Config: &Config{
				Profile: "SharedConfigurationProfile",
				Region:  "us-east-1",
			},
			Description:              "config Profile shared configuration credential_source Ec2InstanceMetadata",
			EnableEc2MetadataServer:  true,
			ExpectedCredentialsValue: mockdata.MockStsAssumeRoleCredentials,
			ExpectedRegion:           "us-east-1",
			MockStsEndpoints: []*servicemocks.MockEndpoint{
				servicemocks.MockStsAssumeRoleValidEndpoint,
				servicemocks.MockStsGetCallerIdentityValidEndpoint,
			},
			SharedConfigurationFile: fmt.Sprintf(`
[profile SharedConfigurationProfile]
credential_source = Ec2InstanceMetadata
role_arn = %[1]s
role_session_name = %[2]s
`, servicemocks.MockStsAssumeRoleArn, servicemocks.MockStsAssumeRoleSessionName),
		},
		// 		{
		// 			Config: &Config{
		// 				Profile: "SharedConfigurationProfile",
		// 				Region:  "us-east-1",
		// 			},
		// 			Description: "config Profile shared configuration credential_source EcsContainer",
		// 			EnvironmentVariables: map[string]string{
		// 				"AWS_CONTAINER_CREDENTIALS_RELATIVE_URI": "/creds",
		// 			},
		// 			EnableEc2MetadataServer:    true,
		// 			EnableEcsCredentialsServer: true,
		// 			ExpectedCredentialsValue:   mockdata.MockStsAssumeRoleCredentialsV2,
		// 			ExpectedRegion:             "us-east-1",
		// 			MockStsEndpoints: []*servicemocks.MockEndpoint{
		// 				servicemocks.MockStsAssumeRoleValidEndpoint,
		// 				servicemocks.MockStsGetCallerIdentityValidEndpoint,
		// 			},
		// 			SharedConfigurationFile: fmt.Sprintf(`
		// [profile SharedConfigurationProfile]
		// credential_source = EcsContainer
		// role_arn = %[1]s
		// role_session_name = %[2]s
		// `, servicemocks.MockStsAssumeRoleArn, servicemocks.MockStsAssumeRoleSessionName),
		// 		},
		{
			Config: &Config{
				Profile: "SharedConfigurationProfile",
				Region:  "us-east-1",
			},
			Description:              "config Profile shared configuration source_profile",
			ExpectedCredentialsValue: mockdata.MockStsAssumeRoleCredentials,
			ExpectedRegion:           "us-east-1",
			MockStsEndpoints: []*servicemocks.MockEndpoint{
				servicemocks.MockStsAssumeRoleValidEndpoint,
				servicemocks.MockStsGetCallerIdentityValidEndpoint,
			},
			SharedConfigurationFile: fmt.Sprintf(`
[profile SharedConfigurationProfile]
role_arn = %[1]s
role_session_name = %[2]s
source_profile = SharedConfigurationSourceProfile

[profile SharedConfigurationSourceProfile]
aws_access_key_id = SharedConfigurationSourceAccessKey
aws_secret_access_key = SharedConfigurationSourceSecretKey
`, servicemocks.MockStsAssumeRoleArn, servicemocks.MockStsAssumeRoleSessionName),
		},
		{
			Config: &Config{
				Region: "us-east-1",
			},
			Description: "environment AWS_ACCESS_KEY_ID",
			EnvironmentVariables: map[string]string{
				"AWS_ACCESS_KEY_ID":     servicemocks.MockEnvAccessKey,
				"AWS_SECRET_ACCESS_KEY": servicemocks.MockEnvSecretKey,
			},
			ExpectedCredentialsValue: mockdata.MockEnvCredentials,
			ExpectedRegion:           "us-east-1",
			MockStsEndpoints: []*servicemocks.MockEndpoint{
				servicemocks.MockStsGetCallerIdentityValidEndpoint,
			},
		},
		{
			Config: &Config{
				AssumeRole: &AssumeRole{
					RoleARN:     servicemocks.MockStsAssumeRoleArn,
					SessionName: servicemocks.MockStsAssumeRoleSessionName,
				},
				Region: "us-east-1",
			},
			Description: "environment AWS_ACCESS_KEY_ID config AssumeRoleARN access key",
			EnvironmentVariables: map[string]string{
				"AWS_ACCESS_KEY_ID":     servicemocks.MockEnvAccessKey,
				"AWS_SECRET_ACCESS_KEY": servicemocks.MockEnvSecretKey,
			},
			ExpectedCredentialsValue: mockdata.MockStsAssumeRoleCredentials,
			ExpectedRegion:           "us-east-1",
			MockStsEndpoints: []*servicemocks.MockEndpoint{
				servicemocks.MockStsAssumeRoleValidEndpoint,
				servicemocks.MockStsGetCallerIdentityValidEndpoint,
			},
		},
		{
			Config: &Config{
				Region: "us-east-1",
			},
			Description: "environment AWS_PROFILE shared credentials profile aws_access_key_id",
			EnvironmentVariables: map[string]string{
				"AWS_PROFILE": "SharedCredentialsProfile",
			},
			ExpectedCredentialsValue: aws.Credentials{
				AccessKeyID:     "ProfileSharedCredentialsAccessKey",
				SecretAccessKey: "ProfileSharedCredentialsSecretKey",
				Source:          sharedConfigCredentialsProvider,
			},
			ExpectedRegion: "us-east-1",
			MockStsEndpoints: []*servicemocks.MockEndpoint{
				servicemocks.MockStsGetCallerIdentityValidEndpoint,
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
			Config: &Config{
				Region: "us-east-1",
			},
			Description:             "environment AWS_PROFILE shared configuration credential_source Ec2InstanceMetadata",
			EnableEc2MetadataServer: true,
			EnvironmentVariables: map[string]string{
				"AWS_PROFILE": "SharedConfigurationProfile",
			},
			ExpectedCredentialsValue: mockdata.MockStsAssumeRoleCredentials,
			ExpectedRegion:           "us-east-1",
			MockStsEndpoints: []*servicemocks.MockEndpoint{
				servicemocks.MockStsAssumeRoleValidEndpoint,
				servicemocks.MockStsGetCallerIdentityValidEndpoint,
			},
			SharedConfigurationFile: fmt.Sprintf(`
[profile SharedConfigurationProfile]
credential_source = Ec2InstanceMetadata
role_arn = %[1]s
role_session_name = %[2]s
`, servicemocks.MockStsAssumeRoleArn, servicemocks.MockStsAssumeRoleSessionName),
		},
		// 		{
		// 			Config: &Config{
		// 				Region: "us-east-1",
		// 			},
		// 			Description:                "environment AWS_PROFILE shared configuration credential_source EcsContainer",
		// 			EnableEc2MetadataServer:    true,
		// 			EnableEcsCredentialsServer: true,
		// 			EnvironmentVariables: map[string]string{
		// 				"AWS_CONTAINER_CREDENTIALS_RELATIVE_URI": "/creds",
		// 				"AWS_PROFILE":                            "SharedConfigurationProfile",
		// 			},
		// 			ExpectedCredentialsValue: mockdata.MockStsAssumeRoleCredentialsV2,
		// 			ExpectedRegion:           "us-east-1",
		// 			MockStsEndpoints: []*servicemocks.MockEndpoint{
		// 				servicemocks.MockStsAssumeRoleValidEndpoint,
		// 				servicemocks.MockStsGetCallerIdentityValidEndpoint,
		// 			},
		// 			SharedConfigurationFile: fmt.Sprintf(`
		// [profile SharedConfigurationProfile]
		// credential_source = EcsContainer
		// role_arn = %[1]s
		// role_session_name = %[2]s
		// `, servicemocks.MockStsAssumeRoleArn, servicemocks.MockStsAssumeRoleSessionName),
		// 		},
		{
			Config: &Config{
				Region: "us-east-1",
			},
			Description: "environment AWS_PROFILE shared configuration source_profile",
			EnvironmentVariables: map[string]string{
				"AWS_PROFILE": "SharedConfigurationProfile",
			},
			ExpectedCredentialsValue: mockdata.MockStsAssumeRoleCredentials,
			ExpectedRegion:           "us-east-1",
			MockStsEndpoints: []*servicemocks.MockEndpoint{
				servicemocks.MockStsAssumeRoleValidEndpoint,
				servicemocks.MockStsGetCallerIdentityValidEndpoint,
			},
			SharedConfigurationFile: fmt.Sprintf(`
[profile SharedConfigurationProfile]
role_arn = %[1]s
role_session_name = %[2]s
source_profile = SharedConfigurationSourceProfile

[profile SharedConfigurationSourceProfile]
aws_access_key_id = SharedConfigurationSourceAccessKey
aws_secret_access_key = SharedConfigurationSourceSecretKey
`, servicemocks.MockStsAssumeRoleArn, servicemocks.MockStsAssumeRoleSessionName),
		},
		{
			Config: &Config{
				Region: "us-east-1",
			},
			Description: "environment AWS_SESSION_TOKEN",
			EnvironmentVariables: map[string]string{
				"AWS_ACCESS_KEY_ID":     servicemocks.MockEnvAccessKey,
				"AWS_SECRET_ACCESS_KEY": servicemocks.MockEnvSecretKey,
				"AWS_SESSION_TOKEN":     servicemocks.MockEnvSessionToken,
			},
			ExpectedCredentialsValue: mockdata.MockEnvCredentialsWithSessionToken,
			ExpectedRegion:           "us-east-1",
			MockStsEndpoints: []*servicemocks.MockEndpoint{
				servicemocks.MockStsGetCallerIdentityValidEndpoint,
			},
		},
		{
			Config: &Config{
				Region: "us-east-1",
			},
			Description: "shared credentials default aws_access_key_id",
			ExpectedCredentialsValue: aws.Credentials{
				AccessKeyID:     "DefaultSharedCredentialsAccessKey",
				SecretAccessKey: "DefaultSharedCredentialsSecretKey",
				Source:          sharedConfigCredentialsProvider,
			},
			ExpectedRegion: "us-east-1",
			MockStsEndpoints: []*servicemocks.MockEndpoint{
				servicemocks.MockStsGetCallerIdentityValidEndpoint,
			},
			SharedCredentialsFile: `
[default]
aws_access_key_id = DefaultSharedCredentialsAccessKey
aws_secret_access_key = DefaultSharedCredentialsSecretKey
`,
		},
		{
			Config: &Config{
				AssumeRole: &AssumeRole{
					RoleARN:     servicemocks.MockStsAssumeRoleArn,
					SessionName: servicemocks.MockStsAssumeRoleSessionName,
				},
				Region: "us-east-1",
			},
			Description:              "shared credentials default aws_access_key_id config AssumeRoleARN access key",
			ExpectedCredentialsValue: mockdata.MockStsAssumeRoleCredentials,
			ExpectedRegion:           "us-east-1",
			MockStsEndpoints: []*servicemocks.MockEndpoint{
				servicemocks.MockStsAssumeRoleValidEndpoint,
				servicemocks.MockStsGetCallerIdentityValidEndpoint,
			},
			SharedCredentialsFile: `
[default]
aws_access_key_id = DefaultSharedCredentialsAccessKey
aws_secret_access_key = DefaultSharedCredentialsSecretKey
`,
		},
		{
			Config: &Config{
				Region: "us-east-1",
			},
			Description:              "web identity token access key",
			EnableWebIdentityToken:   true,
			ExpectedCredentialsValue: mockdata.MockStsAssumeRoleWithWebIdentityCredentials,
			ExpectedRegion:           "us-east-1",
			MockStsEndpoints: []*servicemocks.MockEndpoint{
				servicemocks.MockStsAssumeRoleWithWebIdentityValidEndpoint,
				servicemocks.MockStsGetCallerIdentityValidEndpoint,
			},
		},
		{
			Config: &Config{
				Region: "us-east-1",
			},
			Description:              "EC2 metadata access key",
			EnableEc2MetadataServer:  true,
			ExpectedCredentialsValue: mockdata.MockEc2MetadataCredentials,
			ExpectedRegion:           "us-east-1",
			MockStsEndpoints: []*servicemocks.MockEndpoint{
				servicemocks.MockStsGetCallerIdentityValidEndpoint,
			},
		},
		{
			Config: &Config{
				AssumeRole: &AssumeRole{
					RoleARN:     servicemocks.MockStsAssumeRoleArn,
					SessionName: servicemocks.MockStsAssumeRoleSessionName,
				},
				Region: "us-east-1",
			},
			Description:              "EC2 metadata access key config AssumeRoleARN access key",
			EnableEc2MetadataServer:  true,
			ExpectedCredentialsValue: mockdata.MockStsAssumeRoleCredentials,
			ExpectedRegion:           "us-east-1",
			MockStsEndpoints: []*servicemocks.MockEndpoint{
				servicemocks.MockStsAssumeRoleValidEndpoint,
				servicemocks.MockStsGetCallerIdentityValidEndpoint,
			},
		},
		{
			Config: &Config{
				Region: "us-east-1",
			},
			Description:                "ECS credentials access key",
			EnableEcsCredentialsServer: true,
			ExpectedCredentialsValue:   mockdata.MockEcsCredentialsCredentials,
			ExpectedRegion:             "us-east-1",
			MockStsEndpoints: []*servicemocks.MockEndpoint{
				servicemocks.MockStsGetCallerIdentityValidEndpoint,
			},
		},
		{
			Config: &Config{
				AssumeRole: &AssumeRole{
					RoleARN:     servicemocks.MockStsAssumeRoleArn,
					SessionName: servicemocks.MockStsAssumeRoleSessionName,
				},
				Region: "us-east-1",
			},
			Description:                "ECS credentials access key config AssumeRoleARN access key",
			EnableEcsCredentialsServer: true,
			ExpectedCredentialsValue:   mockdata.MockStsAssumeRoleCredentials,
			ExpectedRegion:             "us-east-1",
			MockStsEndpoints: []*servicemocks.MockEndpoint{
				servicemocks.MockStsAssumeRoleValidEndpoint,
				servicemocks.MockStsGetCallerIdentityValidEndpoint,
			},
		},
		{
			Config: &Config{
				AccessKey: servicemocks.MockStaticAccessKey,
				Region:    "us-east-1",
				SecretKey: servicemocks.MockStaticSecretKey,
			},
			Description: "config AccessKey over environment AWS_ACCESS_KEY_ID",
			EnvironmentVariables: map[string]string{
				"AWS_ACCESS_KEY_ID":     servicemocks.MockEnvAccessKey,
				"AWS_SECRET_ACCESS_KEY": servicemocks.MockEnvSecretKey,
			},
			ExpectedCredentialsValue: mockdata.MockStaticCredentials,
			ExpectedRegion:           "us-east-1",
			MockStsEndpoints: []*servicemocks.MockEndpoint{
				servicemocks.MockStsGetCallerIdentityValidEndpoint,
			},
		},
		{
			Config: &Config{
				AccessKey: servicemocks.MockStaticAccessKey,
				Region:    "us-east-1",
				SecretKey: servicemocks.MockStaticSecretKey,
			},
			Description:              "config AccessKey over shared credentials default aws_access_key_id",
			ExpectedCredentialsValue: mockdata.MockStaticCredentials,
			ExpectedRegion:           "us-east-1",
			MockStsEndpoints: []*servicemocks.MockEndpoint{
				servicemocks.MockStsGetCallerIdentityValidEndpoint,
			},
			SharedCredentialsFile: `
[default]
aws_access_key_id = DefaultSharedCredentialsAccessKey
aws_secret_access_key = DefaultSharedCredentialsSecretKey
`,
		},
		{
			Config: &Config{
				AccessKey: servicemocks.MockStaticAccessKey,
				Region:    "us-east-1",
				SecretKey: servicemocks.MockStaticSecretKey,
			},
			Description:              "config AccessKey over EC2 metadata access key",
			ExpectedCredentialsValue: mockdata.MockStaticCredentials,
			ExpectedRegion:           "us-east-1",
			MockStsEndpoints: []*servicemocks.MockEndpoint{
				servicemocks.MockStsGetCallerIdentityValidEndpoint,
			},
		},
		{
			Config: &Config{
				AccessKey: servicemocks.MockStaticAccessKey,
				Region:    "us-east-1",
				SecretKey: servicemocks.MockStaticSecretKey,
			},
			Description:                "config AccessKey over ECS credentials access key",
			EnableEcsCredentialsServer: true,
			ExpectedCredentialsValue:   mockdata.MockStaticCredentials,
			ExpectedRegion:             "us-east-1",
			MockStsEndpoints: []*servicemocks.MockEndpoint{
				servicemocks.MockStsGetCallerIdentityValidEndpoint,
			},
		},
		{
			Config: &Config{
				Region: "us-east-1",
			},
			Description: "environment AWS_ACCESS_KEY_ID over shared credentials default aws_access_key_id",
			EnvironmentVariables: map[string]string{
				"AWS_ACCESS_KEY_ID":     servicemocks.MockEnvAccessKey,
				"AWS_SECRET_ACCESS_KEY": servicemocks.MockEnvSecretKey,
			},
			ExpectedCredentialsValue: mockdata.MockEnvCredentials,
			ExpectedRegion:           "us-east-1",
			MockStsEndpoints: []*servicemocks.MockEndpoint{
				servicemocks.MockStsGetCallerIdentityValidEndpoint,
			},
			SharedCredentialsFile: `
[default]
aws_access_key_id = DefaultSharedCredentialsAccessKey
aws_secret_access_key = DefaultSharedCredentialsSecretKey
`,
		},
		{
			Config: &Config{
				Region: "us-east-1",
			},
			Description: "environment AWS_ACCESS_KEY_ID over EC2 metadata access key",
			EnvironmentVariables: map[string]string{
				"AWS_ACCESS_KEY_ID":     servicemocks.MockEnvAccessKey,
				"AWS_SECRET_ACCESS_KEY": servicemocks.MockEnvSecretKey,
			},
			ExpectedCredentialsValue: mockdata.MockEnvCredentials,
			ExpectedRegion:           "us-east-1",
			MockStsEndpoints: []*servicemocks.MockEndpoint{
				servicemocks.MockStsGetCallerIdentityValidEndpoint,
			},
		},
		{
			Config: &Config{
				Region: "us-east-1",
			},
			Description: "environment AWS_ACCESS_KEY_ID over ECS credentials access key",
			EnvironmentVariables: map[string]string{
				"AWS_ACCESS_KEY_ID":     servicemocks.MockEnvAccessKey,
				"AWS_SECRET_ACCESS_KEY": servicemocks.MockEnvSecretKey,
			},
			EnableEcsCredentialsServer: true,
			ExpectedCredentialsValue:   mockdata.MockEnvCredentials,
			ExpectedRegion:             "us-east-1",
			MockStsEndpoints: []*servicemocks.MockEndpoint{
				servicemocks.MockStsGetCallerIdentityValidEndpoint,
			},
		},
		{
			Config: &Config{
				Region: "us-east-1",
			},
			Description: "shared credentials default aws_access_key_id over EC2 metadata access key",
			ExpectedCredentialsValue: aws.Credentials{
				AccessKeyID:     "DefaultSharedCredentialsAccessKey",
				SecretAccessKey: "DefaultSharedCredentialsSecretKey",
				Source:          sharedConfigCredentialsProvider,
			},
			ExpectedRegion: "us-east-1",
			MockStsEndpoints: []*servicemocks.MockEndpoint{
				servicemocks.MockStsGetCallerIdentityValidEndpoint,
			},
			SharedCredentialsFile: `
[default]
aws_access_key_id = DefaultSharedCredentialsAccessKey
aws_secret_access_key = DefaultSharedCredentialsSecretKey
`,
		},
		{
			Config: &Config{
				Region: "us-east-1",
			},
			Description:                "shared credentials default aws_access_key_id over ECS credentials access key",
			EnableEcsCredentialsServer: true,
			ExpectedCredentialsValue: aws.Credentials{
				AccessKeyID:     "DefaultSharedCredentialsAccessKey",
				SecretAccessKey: "DefaultSharedCredentialsSecretKey",
				Source:          sharedConfigCredentialsProvider,
			},
			ExpectedRegion: "us-east-1",
			MockStsEndpoints: []*servicemocks.MockEndpoint{
				servicemocks.MockStsGetCallerIdentityValidEndpoint,
			},
			SharedCredentialsFile: `
[default]
aws_access_key_id = DefaultSharedCredentialsAccessKey
aws_secret_access_key = DefaultSharedCredentialsSecretKey
`,
		},
		{
			Config: &Config{
				Region: "us-east-1",
			},
			Description:                "ECS credentials access key over EC2 metadata access key",
			EnableEcsCredentialsServer: true,
			ExpectedCredentialsValue:   mockdata.MockEcsCredentialsCredentials,
			ExpectedRegion:             "us-east-1",
			MockStsEndpoints: []*servicemocks.MockEndpoint{
				servicemocks.MockStsGetCallerIdentityValidEndpoint,
			},
		},
		{
			Config: &Config{
				AccessKey: servicemocks.MockStaticAccessKey,
				SecretKey: servicemocks.MockStaticSecretKey,
			},
			Description:              "retrieve region from shared configuration file",
			ExpectedCredentialsValue: mockdata.MockStaticCredentials,
			ExpectedRegion:           "us-east-1",
			MockStsEndpoints: []*servicemocks.MockEndpoint{
				servicemocks.MockStsGetCallerIdentityValidEndpoint,
			},
			SharedConfigurationFile: `
[default]
region = us-east-1
`,
		},
		{
			Config: &Config{
				AccessKey: servicemocks.MockStaticAccessKey,
				AssumeRole: &AssumeRole{
					RoleARN:     servicemocks.MockStsAssumeRoleArn,
					SessionName: servicemocks.MockStsAssumeRoleSessionName,
				},
				Region:    "us-east-1",
				SecretKey: servicemocks.MockStaticSecretKey,
			},
			Description: "assume role error",
			ExpectedError: func(err error) bool {
				return IsCannotAssumeRoleError(err)
			},
			ExpectedRegion: "us-east-1",
			MockStsEndpoints: []*servicemocks.MockEndpoint{
				servicemocks.MockStsAssumeRoleInvalidEndpointInvalidClientTokenId,
				servicemocks.MockStsGetCallerIdentityValidEndpoint,
			},
		},
		// {
		// 	Config: &Config{
		// 		AccessKey: servicemocks.MockStaticAccessKey,
		// 		Region:    "us-east-1",
		// 		SecretKey: servicemocks.MockStaticSecretKey,
		// 	},
		// 	Description: "credential validation error",
		// 	ExpectedError: func(err error) bool {
		// 		return tfawserr.ErrCodeEquals(err, "AccessDenied")
		// 	},
		// 	MockStsEndpoints: []*servicemocks.MockEndpoint{
		// 		servicemocks.MockStsGetCallerIdentityInvalidEndpointAccessDenied,
		// 	},
		// },
		{
			Config: &Config{
				Profile: "SharedConfigurationProfile",
				Region:  "us-east-1",
			},
			Description: "session creation error",
			ExpectedError: func(err error) bool {
				var e config.CredentialRequiresARNError
				return errors.As(err, &e)
			},
			SharedConfigurationFile: `
[profile SharedConfigurationProfile]
source_profile = SourceSharedCredentials
`,
		},
		{
			Config: &Config{
				AccessKey:           servicemocks.MockStaticAccessKey,
				Region:              "us-east-1",
				SecretKey:           servicemocks.MockStaticSecretKey,
				SkipCredsValidation: true,
			},
			Description:              "skip credentials validation",
			ExpectedCredentialsValue: mockdata.MockStaticCredentials,
			ExpectedRegion:           "us-east-1",
		},
		{
			Config: &Config{
				Region:                  "us-east-1",
				SkipEC2MetadataApiCheck: true,
			},
			Description: "skip EC2 Metadata API check",
			ExpectedError: func(err error) bool {
				return IsNoValidCredentialSourcesError(err)
			},
			ExpectedRegion: "us-east-1",
		},
		{
			Config: &Config{
				Region: "us-east-1",
			},
			Description: "invalid profile name from envvar",
			EnvironmentVariables: map[string]string{
				"AWS_PROFILE": "no-such-profile",
			},
			ExpectedError: func(err error) bool {
				var e config.SharedConfigProfileNotExistError
				return errors.As(err, &e)
			},
			SharedCredentialsFile: `
[some-profile]
aws_access_key_id = DefaultSharedCredentialsAccessKey
aws_secret_access_key = DefaultSharedCredentialsSecretKey
`,
		},
		{
			Config: &Config{
				Profile: "no-such-profile",
				Region:  "us-east-1",
			},
			Description: "invalid profile name from config",
			ExpectedError: func(err error) bool {
				var e config.SharedConfigProfileNotExistError
				return errors.As(err, &e)
			},
			SharedCredentialsFile: `
[some-profile]
aws_access_key_id = DefaultSharedCredentialsAccessKey
aws_secret_access_key = DefaultSharedCredentialsSecretKey
`,
		},
		{
			Config:      &Config{},
			Description: "AWS_ACCESS_KEY_ID overrides AWS_PROFILE",
			EnvironmentVariables: map[string]string{
				"AWS_ACCESS_KEY_ID":     servicemocks.MockEnvAccessKey,
				"AWS_SECRET_ACCESS_KEY": servicemocks.MockEnvSecretKey,
				"AWS_PROFILE":           "SharedCredentialsProfile",
			},
			SharedCredentialsFile: `
[default]
aws_access_key_id = DefaultSharedCredentialsAccessKey
aws_secret_access_key = DefaultSharedCredentialsSecretKey

[SharedCredentialsProfile]
aws_access_key_id = ProfileSharedCredentialsAccessKey
aws_secret_access_key = ProfileSharedCredentialsSecretKey
`,
			MockStsEndpoints: []*servicemocks.MockEndpoint{
				servicemocks.MockStsGetCallerIdentityValidEndpoint,
			},
			ExpectedCredentialsValue: mockdata.MockEnvCredentials,
		},
		{
			Config: &Config{
				Region: "us-east-1",
			},
			Description: "AWS_ACCESS_KEY_ID does not override invalid profile name from envvar",
			EnvironmentVariables: map[string]string{
				"AWS_ACCESS_KEY_ID":     servicemocks.MockEnvAccessKey,
				"AWS_SECRET_ACCESS_KEY": servicemocks.MockEnvSecretKey,
				"AWS_PROFILE":           "no-such-profile",
			},
			ExpectedError: func(err error) bool {
				var e config.SharedConfigProfileNotExistError
				return errors.As(err, &e)
			},
			SharedCredentialsFile: `
[some-profile]
aws_access_key_id = DefaultSharedCredentialsAccessKey
aws_secret_access_key = DefaultSharedCredentialsSecretKey
`,
		},
	}

	for _, testCase := range testCases {
		testCase := testCase

		t.Run(testCase.Description, func(t *testing.T) {
			oldEnv := servicemocks.InitSessionTestEnv()
			defer servicemocks.PopEnv(oldEnv)

			if testCase.EnableEc2MetadataServer {
				closeEc2Metadata := servicemocks.AwsMetadataApiMock(append(
					servicemocks.Ec2metadata_securityCredentialsEndpoints,
					servicemocks.Ec2metadata_instanceIdEndpoint,
					servicemocks.Ec2metadata_iamInfoEndpoint,
				))
				defer closeEc2Metadata()
			}

			if testCase.EnableEcsCredentialsServer {
				closeEcsCredentials := servicemocks.EcsCredentialsApiMock()
				defer closeEcsCredentials()
			}

			if testCase.EnableWebIdentityToken {
				file, err := ioutil.TempFile("", "aws-sdk-go-base-web-identity-token-file")

				if err != nil {
					t.Fatalf("unexpected error creating temporary web identity token file: %s", err)
				}

				defer os.Remove(file.Name())

				err = ioutil.WriteFile(file.Name(), []byte(servicemocks.MockWebIdentityToken), 0600)

				if err != nil {
					t.Fatalf("unexpected error writing web identity token file: %s", err)
				}

				os.Setenv("AWS_ROLE_ARN", servicemocks.MockStsAssumeRoleWithWebIdentityArn)
				os.Setenv("AWS_ROLE_SESSION_NAME", servicemocks.MockStsAssumeRoleWithWebIdentitySessionName)
				os.Setenv("AWS_WEB_IDENTITY_TOKEN_FILE", file.Name())
			}

			closeSts, _, stsEndpoint := mockdata.GetMockedAwsApiSession("STS", testCase.MockStsEndpoints)
			defer closeSts()

			testCase.Config.StsEndpoint = stsEndpoint

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

				testCase.Config.SharedConfigFiles = []string{file.Name()}
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

				testCase.Config.SharedCredentialsFiles = []string{file.Name()}
				if testCase.ExpectedCredentialsValue.Source == sharedConfigCredentialsProvider {
					testCase.ExpectedCredentialsValue.Source = sharedConfigCredentialsSource(file.Name())
				}
			}

			for k, v := range testCase.EnvironmentVariables {
				os.Setenv(k, v)
			}

			awsConfig, err := GetAwsConfig(context.Background(), testCase.Config)

			if err != nil {
				if testCase.ExpectedError == nil {
					t.Fatalf("expected no error, got '%[1]T' error: %[1]s", err)
				}

				if !testCase.ExpectedError(err) {
					t.Fatalf("unexpected GetAwsConfig() '%[1]T' error: %[1]s", err)
				}

				t.Logf("received expected '%[1]T' error: %[1]s", err)
				return
			}

			if err == nil && testCase.ExpectedError != nil {
				t.Fatalf("expected error, got no error")
			}

			credentialsValue, err := awsConfig.Credentials.Retrieve(context.Background())

			if err != nil {
				t.Fatalf("unexpected credentials Retrieve() error: %s", err)
			}

			if diff := cmp.Diff(credentialsValue, testCase.ExpectedCredentialsValue, cmpopts.IgnoreFields(aws.Credentials{}, "Expires")); diff != "" {
				t.Fatalf("unexpected credentials: (- got, + expected)\n%s", diff)
			}
			// TODO: test credentials.Expires

			if expected, actual := testCase.ExpectedRegion, awsConfig.Region; expected != actual {
				t.Fatalf("expected region (%s), got: %s", expected, actual)
			}
		})
	}
}

func TestUserAgentProducts(t *testing.T) {
	testCases := []struct {
		Config               *Config
		Description          string
		EnvironmentVariables map[string]string
		ExpectedUserAgent    string
	}{
		{
			Config: &Config{
				AccessKey: servicemocks.MockStaticAccessKey,
				Region:    "us-east-1",
				SecretKey: servicemocks.MockStaticSecretKey,
			},
			Description:       "standard User-Agent",
			ExpectedUserAgent: awsSdkGoUserAgent(),
		},
		{
			Config: &Config{
				AccessKey: servicemocks.MockStaticAccessKey,
				Region:    "us-east-1",
				SecretKey: servicemocks.MockStaticSecretKey,
			},
			Description: "customized User-Agent TF_APPEND_USER_AGENT",
			EnvironmentVariables: map[string]string{
				constants.AppendUserAgentEnvVar: "Last",
			},
			ExpectedUserAgent: awsSdkGoUserAgent() + " Last",
		},
		{
			Config: &Config{
				AccessKey: servicemocks.MockStaticAccessKey,
				Region:    "us-east-1",
				SecretKey: servicemocks.MockStaticSecretKey,
				APNInfo: &APNInfo{
					PartnerName: "partner",
					Products: []UserAgentProduct{
						{
							Name:    "first",
							Version: "1.2.3",
						},
						{
							Name:    "second",
							Version: "1.0.2",
							Comment: "a comment",
						},
					},
				},
			},
			Description:       "APN User-Agent Products",
			ExpectedUserAgent: "APN/1.0 partner/1.0 first/1.2.3 second/1.0.2 (a comment) " + awsSdkGoUserAgent(),
		},
		{
			Config: &Config{
				AccessKey: servicemocks.MockStaticAccessKey,
				Region:    "us-east-1",
				SecretKey: servicemocks.MockStaticSecretKey,
				APNInfo: &APNInfo{
					PartnerName: "partner",
					Products: []UserAgentProduct{
						{
							Name:    "first",
							Version: "1.2.3",
						},
						{
							Name:    "second",
							Version: "1.0.2",
						},
					},
				},
			},
			Description: "APN User-Agent Products and TF_APPEND_USER_AGENT",
			EnvironmentVariables: map[string]string{
				constants.AppendUserAgentEnvVar: "Last",
			},
			ExpectedUserAgent: "APN/1.0 partner/1.0 first/1.2.3 second/1.0.2 " + awsSdkGoUserAgent() + " Last",
		},
		{
			Config: &Config{
				AccessKey: servicemocks.MockStaticAccessKey,
				Region:    "us-east-1",
				SecretKey: servicemocks.MockStaticSecretKey,
				UserAgent: []UserAgentProduct{
					{
						Name:    "first",
						Version: "1.2.3",
					},
					{
						Name:    "second",
						Version: "1.0.2",
						Comment: "a comment",
					},
				},
			},
			Description:       "User-Agent Products",
			ExpectedUserAgent: awsSdkGoUserAgent() + " first/1.2.3 second/1.0.2 (a comment)",
		},
		{
			Config: &Config{
				AccessKey: servicemocks.MockStaticAccessKey,
				Region:    "us-east-1",
				SecretKey: servicemocks.MockStaticSecretKey,
				APNInfo: &APNInfo{
					PartnerName: "partner",
					Products: []UserAgentProduct{
						{
							Name:    "first",
							Version: "1.2.3",
						},
						{
							Name:    "second",
							Version: "1.0.2",
							Comment: "a comment",
						},
					},
				},
				UserAgent: []UserAgentProduct{
					{
						Name:    "third",
						Version: "4.5.6",
					},
					{
						Name:    "fourth",
						Version: "2.1",
					},
				},
			},
			Description:       "APN and User-Agent Products",
			ExpectedUserAgent: "APN/1.0 partner/1.0 first/1.2.3 second/1.0.2 (a comment) " + awsSdkGoUserAgent() + " third/4.5.6 fourth/2.1",
		},
	}

	var (
		httpUserAgent string
		httpSdkAgent  string
	)

	errCancelOperation := fmt.Errorf("Cancelling request")

	readUserAgent := middleware.FinalizeMiddlewareFunc("ReadUserAgent", func(_ context.Context, in middleware.FinalizeInput, next middleware.FinalizeHandler) (middleware.FinalizeOutput, middleware.Metadata, error) {
		request, ok := in.Request.(*smithyhttp.Request)
		if !ok {
			t.Fatalf("Expected *github.com/aws/smithy-go/transport/http.Request, got %s", fullTypeName(in.Request))
		}
		httpUserAgent = request.UserAgent()
		httpSdkAgent = request.Header.Get("X-Amz-User-Agent")

		return middleware.FinalizeOutput{}, middleware.Metadata{}, errCancelOperation
	})

	for _, testCase := range testCases {
		testCase := testCase

		t.Run(testCase.Description, func(t *testing.T) {
			oldEnv := servicemocks.InitSessionTestEnv()
			defer servicemocks.PopEnv(oldEnv)

			for k, v := range testCase.EnvironmentVariables {
				os.Setenv(k, v)
			}

			testCase.Config.SkipCredsValidation = true

			awsConfig, err := GetAwsConfig(context.Background(), testCase.Config)
			if err != nil {
				t.Fatalf("error in GetAwsConfig() '%[1]T': %[1]s", err)
			}

			client := stsClient(awsConfig, testCase.Config)

			_, err = client.GetCallerIdentity(context.Background(), &sts.GetCallerIdentityInput{},
				func(opts *sts.Options) {
					opts.APIOptions = append(opts.APIOptions, func(stack *middleware.Stack) error {
						return stack.Finalize.Add(readUserAgent, middleware.Before)
					})
				},
			)
			if err == nil {
				t.Fatal("Expected an error, got none")
			} else if !errors.Is(err, errCancelOperation) {
				t.Fatalf("Unexpected error: %s", err)
			}

			var userAgentParts []string
			for _, v := range strings.Split(httpUserAgent, " ") {
				if !strings.HasPrefix(v, "api/") {
					userAgentParts = append(userAgentParts, v)
				}
			}
			cleanedUserAgent := strings.Join(userAgentParts, " ")

			if testCase.ExpectedUserAgent != cleanedUserAgent {
				t.Errorf("expected User-Agent %q, got %q", testCase.ExpectedUserAgent, cleanedUserAgent)
			}

			// The header X-Amz-User-Agent was disabled but not removed in v1.3.0 (2021-03-18)
			if httpSdkAgent != "" {
				t.Errorf("expected header X-Amz-User-Agent to not be set, got %q", httpSdkAgent)
			}
		})
	}
}

func awsSdkGoUserAgent() string {
	// See https://github.com/aws/aws-sdk-go-v2/blob/994cb2c7c1c822dc628949e7ae2941b9c856ccb3/aws/middleware/user_agent_test.go#L18
	return fmt.Sprintf("%s/%s os/%s lang/go/%s md/GOOS/%s md/GOARCH/%s", aws.SDKName, aws.SDKVersion, getNormalizedOSName(), strings.TrimPrefix(runtime.Version(), "go"), runtime.GOOS, runtime.GOARCH)
}

// Copied from https://github.com/aws/aws-sdk-go-v2/blob/main/aws/middleware/osname.go
func getNormalizedOSName() (os string) {
	switch runtime.GOOS {
	case "android":
		os = "android"
	case "linux":
		os = "linux"
	case "windows":
		os = "windows"
	case "darwin":
		os = "macos"
	case "ios":
		os = "ios"
	default:
		os = "other"
	}
	return os
}

func fullTypeName(i interface{}) string {
	return fullValueTypeName(reflect.ValueOf(i))
}

func fullValueTypeName(v reflect.Value) string {
	if v.Kind() == reflect.Ptr {
		return "*" + fullValueTypeName(reflect.Indirect(v))
	}

	requestType := v.Type()
	return fmt.Sprintf("%s.%s", requestType.PkgPath(), requestType.Name())
}

func TestRegion(t *testing.T) {
	testCases := map[string]struct {
		Config                  *Config
		EnvironmentVariables    map[string]string
		IMDSRegion              string
		SharedConfigurationFile string
		ExpectedRegion          string
	}{
		"no configuration": {
			Config: &Config{
				AccessKey: servicemocks.MockStaticAccessKey,
				SecretKey: servicemocks.MockStaticSecretKey,
			},
			ExpectedRegion: "",
		},

		"config": {
			Config: &Config{
				AccessKey: servicemocks.MockStaticAccessKey,
				Region:    "us-east-1",
				SecretKey: servicemocks.MockStaticSecretKey,
			},
			ExpectedRegion: "us-east-1",
		},

		"AWS_REGION": {
			Config: &Config{
				AccessKey: servicemocks.MockStaticAccessKey,
				SecretKey: servicemocks.MockStaticSecretKey,
			},
			EnvironmentVariables: map[string]string{
				"AWS_REGION": "us-east-1",
			},
			ExpectedRegion: "us-east-1",
		},
		"AWS_DEFAULT_REGION": {
			Config: &Config{
				AccessKey: servicemocks.MockStaticAccessKey,
				SecretKey: servicemocks.MockStaticSecretKey,
			},
			EnvironmentVariables: map[string]string{
				"AWS_DEFAULT_REGION": "us-east-1",
			},
			ExpectedRegion: "us-east-1",
		},
		"AWS_REGION overrides AWS_DEFAULT_REGION": {
			Config: &Config{
				AccessKey: servicemocks.MockStaticAccessKey,
				SecretKey: servicemocks.MockStaticSecretKey,
			},
			EnvironmentVariables: map[string]string{
				"AWS_REGION":         "us-east-1",
				"AWS_DEFAULT_REGION": "us-west-2",
			},
			ExpectedRegion: "us-east-1",
		},

		"shared configuration file": {
			Config: &Config{
				AccessKey: servicemocks.MockStaticAccessKey,
				SecretKey: servicemocks.MockStaticSecretKey,
			},
			SharedConfigurationFile: `
[default]
region = us-east-1
`,
			ExpectedRegion: "us-east-1",
		},

		"IMDS": {
			Config:         &Config{},
			IMDSRegion:     "us-east-1",
			ExpectedRegion: "us-east-1",
		},

		"config overrides AWS_REGION": {
			Config: &Config{
				AccessKey: servicemocks.MockStaticAccessKey,
				SecretKey: servicemocks.MockStaticSecretKey,
				Region:    "us-east-1",
			},
			EnvironmentVariables: map[string]string{
				"AWS_REGION": "us-west-2",
			},
			ExpectedRegion: "us-east-1",
		},
		"config overrides AWS_DEFAULT_REGION": {
			Config: &Config{
				AccessKey: servicemocks.MockStaticAccessKey,
				SecretKey: servicemocks.MockStaticSecretKey,
				Region:    "us-east-1",
			},
			EnvironmentVariables: map[string]string{
				"AWS_DEFAULT_REGION": "us-west-2",
			},
			ExpectedRegion: "us-east-1",
		},

		"config overrides IMDS": {
			Config: &Config{
				AccessKey: servicemocks.MockStaticAccessKey,
				SecretKey: servicemocks.MockStaticSecretKey,
				Region:    "us-west-2",
			},
			IMDSRegion:     "us-east-1",
			ExpectedRegion: "us-west-2",
		},

		"AWS_REGION overrides shared configuration": {
			Config: &Config{
				AccessKey: servicemocks.MockStaticAccessKey,
				SecretKey: servicemocks.MockStaticSecretKey,
			},
			EnvironmentVariables: map[string]string{
				"AWS_REGION": "us-east-1",
			},
			SharedConfigurationFile: `
[default]
region = us-west-2
`,
			ExpectedRegion: "us-east-1",
		},
		"AWS_DEFAULT_REGION overrides shared configuration": {
			Config: &Config{
				AccessKey: servicemocks.MockStaticAccessKey,
				SecretKey: servicemocks.MockStaticSecretKey,
			},
			EnvironmentVariables: map[string]string{
				"AWS_DEFAULT_REGION": "us-east-1",
			},
			SharedConfigurationFile: `
[default]
region = us-west-2
`,
			ExpectedRegion: "us-east-1",
		},

		"AWS_REGION overrides IMDS": {
			Config: &Config{
				AccessKey: servicemocks.MockStaticAccessKey,
				SecretKey: servicemocks.MockStaticSecretKey,
			},
			EnvironmentVariables: map[string]string{
				"AWS_REGION": "us-east-1",
			},
			IMDSRegion:     "us-west-2",
			ExpectedRegion: "us-east-1",
		},
	}

	for testName, testCase := range testCases {
		testCase := testCase

		t.Run(testName, func(t *testing.T) {
			oldEnv := servicemocks.InitSessionTestEnv()
			defer servicemocks.PopEnv(oldEnv)

			for k, v := range testCase.EnvironmentVariables {
				os.Setenv(k, v)
			}

			if testCase.IMDSRegion != "" {
				closeEc2Metadata := servicemocks.AwsMetadataApiMock(append(
					servicemocks.Ec2metadata_securityCredentialsEndpoints,
					servicemocks.Ec2metadata_instanceIdEndpoint,
					servicemocks.Ec2metadata_iamInfoEndpoint,
					servicemocks.Ec2metadata_instanceIdentityEndpoint(testCase.IMDSRegion),
				))
				defer closeEc2Metadata()
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

				testCase.Config.SharedConfigFiles = []string{file.Name()}
			}

			testCase.Config.SkipCredsValidation = true

			awsConfig, err := GetAwsConfig(context.Background(), testCase.Config)
			if err != nil {
				t.Fatalf("error in GetAwsConfig() '%[1]T': %[1]s", err)
			}

			if a, e := awsConfig.Region, testCase.ExpectedRegion; a != e {
				t.Errorf("expected Region %q, got: %q", e, a)
			}
		})
	}
}

func TestMaxAttempts(t *testing.T) {
	testCases := map[string]struct {
		Config                  *Config
		EnvironmentVariables    map[string]string
		SharedConfigurationFile string
		ExpectedMaxAttempts     int
	}{
		"no configuration": {
			Config: &Config{
				AccessKey: servicemocks.MockStaticAccessKey,
				SecretKey: servicemocks.MockStaticSecretKey,
			},
			ExpectedMaxAttempts: retry.DefaultMaxAttempts,
		},

		"config": {
			Config: &Config{
				AccessKey:  servicemocks.MockStaticAccessKey,
				SecretKey:  servicemocks.MockStaticSecretKey,
				MaxRetries: 5,
			},
			ExpectedMaxAttempts: 5,
		},

		"AWS_MAX_ATTEMPTS": {
			Config: &Config{
				AccessKey: servicemocks.MockStaticAccessKey,
				SecretKey: servicemocks.MockStaticSecretKey,
			},
			EnvironmentVariables: map[string]string{
				"AWS_MAX_ATTEMPTS": "5",
			},
			ExpectedMaxAttempts: 5,
		},

		// 		"shared configuration file": {
		// 			Config: &Config{
		// 				AccessKey: servicemocks.MockStaticAccessKey,
		// 				SecretKey: servicemocks.MockStaticSecretKey,
		// 			},
		// 			SharedConfigurationFile: `
		// [default]
		// max_attempts = 5
		// `,
		// 			ExpectedMaxAttempts: 5,
		// 		},

		"config overrides AWS_MAX_ATTEMPTS": {
			Config: &Config{
				AccessKey:  servicemocks.MockStaticAccessKey,
				SecretKey:  servicemocks.MockStaticSecretKey,
				MaxRetries: 10,
			},
			EnvironmentVariables: map[string]string{
				"AWS_MAX_ATTEMPTS": "5",
			},
			ExpectedMaxAttempts: 10,
		},

		// 		"AWS_MAX_ATTEMPTS overrides shared configuration": {
		// 			Config: &Config{
		// 				AccessKey: servicemocks.MockStaticAccessKey,
		// 				SecretKey: servicemocks.MockStaticSecretKey,
		// 			},
		// 			EnvironmentVariables: map[string]string{
		// 				"AWS_MAX_ATTEMPTS": "5",
		// 			},
		// 			SharedConfigurationFile: `
		// [default]
		// max_attempts = 10
		// `,
		// 			ExpectedMaxAttempts: 5,
		// 		},
	}

	for testName, testCase := range testCases {
		testCase := testCase

		t.Run(testName, func(t *testing.T) {
			oldEnv := servicemocks.InitSessionTestEnv()
			defer servicemocks.PopEnv(oldEnv)

			for k, v := range testCase.EnvironmentVariables {
				os.Setenv(k, v)
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

				testCase.Config.SharedConfigFiles = []string{file.Name()}
			}

			testCase.Config.SkipCredsValidation = true

			awsConfig, err := GetAwsConfig(context.Background(), testCase.Config)
			if err != nil {
				t.Fatalf("error in GetAwsConfig() '%[1]T': %[1]s", err)
			}

			retryer := awsConfig.Retryer()
			if retryer == nil {
				t.Fatal("no retryer set")
			}
			if a, e := retryer.MaxAttempts(), testCase.ExpectedMaxAttempts; a != e {
				t.Errorf(`expected MaxAttempts "%d", got: "%d"`, e, a)
			}
		})
	}
}

func TestServiceEndpointTypes(t *testing.T) {
	testCases := map[string]struct {
		Config                            *Config
		EnvironmentVariables              map[string]string
		SharedConfigurationFile           string
		ExpectedUseFIPSEndpointState      aws.FIPSEndpointState
		ExpectedUseDualStackEndpointState aws.DualStackEndpointState
	}{
		"normal endpoint": {
			Config: &Config{
				AccessKey: servicemocks.MockStaticAccessKey,
				SecretKey: servicemocks.MockStaticSecretKey,
			},
			ExpectedUseFIPSEndpointState:      aws.FIPSEndpointStateUnset,
			ExpectedUseDualStackEndpointState: aws.DualStackEndpointStateUnset,
		},

		// FIPS Endpoint
		"FIPS endpoint config": {
			Config: &Config{
				AccessKey:       servicemocks.MockStaticAccessKey,
				SecretKey:       servicemocks.MockStaticSecretKey,
				UseFIPSEndpoint: true,
			},
			ExpectedUseFIPSEndpointState: aws.FIPSEndpointStateEnabled,
		},
		"FIPS endpoint envvar": {
			Config: &Config{
				AccessKey: servicemocks.MockStaticAccessKey,
				SecretKey: servicemocks.MockStaticSecretKey,
			},
			EnvironmentVariables: map[string]string{
				"AWS_USE_FIPS_ENDPOINT": "true",
			},
			ExpectedUseFIPSEndpointState: aws.FIPSEndpointStateEnabled,
		},
		"FIPS endpoint shared configuration file": {
			Config: &Config{
				AccessKey: servicemocks.MockStaticAccessKey,
				SecretKey: servicemocks.MockStaticSecretKey,
			},
			SharedConfigurationFile: `
[default]
use_fips_endpoint = true
`,
			ExpectedUseFIPSEndpointState: aws.FIPSEndpointStateEnabled,
		},
		"FIPS endpoint config overrides env var": {
			Config: &Config{
				AccessKey:       servicemocks.MockStaticAccessKey,
				SecretKey:       servicemocks.MockStaticSecretKey,
				UseFIPSEndpoint: true,
			},
			EnvironmentVariables: map[string]string{
				"AWS_USE_FIPS_ENDPOINT": "true",
			},
			ExpectedUseFIPSEndpointState: aws.FIPSEndpointStateEnabled,
		},
		"FIPS endpoint env var overrides shared configuration file": {
			Config: &Config{
				AccessKey: servicemocks.MockStaticAccessKey,
				SecretKey: servicemocks.MockStaticSecretKey,
			},
			EnvironmentVariables: map[string]string{
				"AWS_USE_FIPS_ENDPOINT": "true",
			},
			SharedConfigurationFile: `
[default]
use_fips_endpoint = false
`,
			ExpectedUseFIPSEndpointState: aws.FIPSEndpointStateEnabled,
		},

		// DualStack Endpoint
		"DualStack endpoint config": {
			Config: &Config{
				AccessKey:            servicemocks.MockStaticAccessKey,
				SecretKey:            servicemocks.MockStaticSecretKey,
				UseDualStackEndpoint: true,
			},
			ExpectedUseDualStackEndpointState: aws.DualStackEndpointStateEnabled,
		},
		"DualStack endpoint envvar": {
			Config: &Config{
				AccessKey: servicemocks.MockStaticAccessKey,
				SecretKey: servicemocks.MockStaticSecretKey,
			},
			EnvironmentVariables: map[string]string{
				"AWS_USE_DUALSTACK_ENDPOINT": "true",
			},
			ExpectedUseDualStackEndpointState: aws.DualStackEndpointStateEnabled,
		},
		"DualStack endpoint shared configuration file": {
			Config: &Config{
				AccessKey: servicemocks.MockStaticAccessKey,
				SecretKey: servicemocks.MockStaticSecretKey,
			},
			SharedConfigurationFile: `
[default]
use_dualstack_endpoint = true
`,
			ExpectedUseDualStackEndpointState: aws.DualStackEndpointStateEnabled,
		},
		"DualStack endpoint config overrides env var": {
			Config: &Config{
				AccessKey:            servicemocks.MockStaticAccessKey,
				SecretKey:            servicemocks.MockStaticSecretKey,
				UseDualStackEndpoint: true,
			},
			EnvironmentVariables: map[string]string{
				"AWS_USE_DUALSTACK_ENDPOINT": "true",
			},
			ExpectedUseDualStackEndpointState: aws.DualStackEndpointStateEnabled,
		},
		"DualStack endpoint env var overrides shared configuration file": {
			Config: &Config{
				AccessKey: servicemocks.MockStaticAccessKey,
				SecretKey: servicemocks.MockStaticSecretKey,
			},
			EnvironmentVariables: map[string]string{
				"AWS_USE_DUALSTACK_ENDPOINT": "true",
			},
			SharedConfigurationFile: `
[default]
use_dualstack_endpoint = false
`,
			ExpectedUseDualStackEndpointState: aws.DualStackEndpointStateEnabled,
		},

		// FIPS and DualStack Endpoint
		"Both endpoints config": {
			Config: &Config{
				AccessKey:            servicemocks.MockStaticAccessKey,
				SecretKey:            servicemocks.MockStaticSecretKey,
				UseDualStackEndpoint: true,
				UseFIPSEndpoint:      true,
			},
			ExpectedUseFIPSEndpointState:      aws.FIPSEndpointStateEnabled,
			ExpectedUseDualStackEndpointState: aws.DualStackEndpointStateEnabled,
		},
		"Both endpoints FIPS config DualStack envvar": {
			Config: &Config{
				AccessKey:       servicemocks.MockStaticAccessKey,
				SecretKey:       servicemocks.MockStaticSecretKey,
				UseFIPSEndpoint: true,
			},
			EnvironmentVariables: map[string]string{
				"AWS_USE_DUALSTACK_ENDPOINT": "true",
			},
			ExpectedUseFIPSEndpointState:      aws.FIPSEndpointStateEnabled,
			ExpectedUseDualStackEndpointState: aws.DualStackEndpointStateEnabled,
		},
		"Both endpoints FIPS shared configuration file DualStack config": {
			Config: &Config{
				AccessKey:            servicemocks.MockStaticAccessKey,
				SecretKey:            servicemocks.MockStaticSecretKey,
				UseDualStackEndpoint: true,
			},
			SharedConfigurationFile: `
[default]
use_fips_endpoint = true
`,
			ExpectedUseFIPSEndpointState:      aws.FIPSEndpointStateEnabled,
			ExpectedUseDualStackEndpointState: aws.DualStackEndpointStateEnabled,
		},
	}

	for testName, testCase := range testCases {
		testCase := testCase

		t.Run(testName, func(t *testing.T) {
			oldEnv := servicemocks.InitSessionTestEnv()
			defer servicemocks.PopEnv(oldEnv)

			for k, v := range testCase.EnvironmentVariables {
				os.Setenv(k, v)
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

				testCase.Config.SharedConfigFiles = []string{file.Name()}
			}

			testCase.Config.SkipCredsValidation = true

			awsConfig, err := GetAwsConfig(context.Background(), testCase.Config)
			if err != nil {
				t.Fatalf("error in GetAwsConfig() '%[1]T': %[1]s", err)
			}

			useFIPSState, _, err := awsconfig.ResolveUseFIPSEndpoint(context.Background(), awsConfig.ConfigSources)
			if err != nil {
				t.Fatalf("error in ResolveUseFIPSEndpoint: %s", err)
			}
			if a, e := useFIPSState, testCase.ExpectedUseFIPSEndpointState; a != e {
				t.Errorf("expected UseFIPSEndpoint %q, got: %q", awsconfig.FIPSEndpointStateString(e), awsconfig.FIPSEndpointStateString(a))
			}

			useDualStackState, _, err := awsconfig.ResolveUseDualStackEndpoint(context.Background(), awsConfig.ConfigSources)
			if err != nil {
				t.Fatalf("error in ResolveUseDualStackEndpoint: %s", err)
			}
			if a, e := useDualStackState, testCase.ExpectedUseDualStackEndpointState; a != e {
				t.Errorf("expected UseDualStackEndpoint %q, got: %q", awsconfig.DualStackEndpointStateString(e), awsconfig.DualStackEndpointStateString(a))
			}
		})
	}
}

func TestEC2MetadataServiceEndpoint(t *testing.T) {
	testCases := map[string]struct {
		Config                             *Config
		EnvironmentVariables               map[string]string
		SharedConfigurationFile            string
		ExpectedEC2MetadataServiceEndpoint string
	}{
		"no configuration": {
			Config: &Config{
				AccessKey: servicemocks.MockStaticAccessKey,
				SecretKey: servicemocks.MockStaticSecretKey,
			},
			ExpectedEC2MetadataServiceEndpoint: "",
		},

		"config": {
			Config: &Config{
				AccessKey:                  servicemocks.MockStaticAccessKey,
				SecretKey:                  servicemocks.MockStaticSecretKey,
				EC2MetadataServiceEndpoint: "https://127.0.0.1:1234",
			},
			ExpectedEC2MetadataServiceEndpoint: "https://127.0.0.1:1234",
		},

		"envvar": {
			Config: &Config{
				AccessKey: servicemocks.MockStaticAccessKey,
				SecretKey: servicemocks.MockStaticSecretKey,
			},
			EnvironmentVariables: map[string]string{
				"AWS_EC2_METADATA_SERVICE_ENDPOINT": "https://127.0.0.1:1234",
			},
			ExpectedEC2MetadataServiceEndpoint: "https://127.0.0.1:1234",
		},
		"deprecated envvar": {
			Config: &Config{
				AccessKey: servicemocks.MockStaticAccessKey,
				SecretKey: servicemocks.MockStaticSecretKey,
			},
			EnvironmentVariables: map[string]string{
				"AWS_METADATA_URL": "https://127.0.0.1:1234",
			},
			ExpectedEC2MetadataServiceEndpoint: "https://127.0.0.1:1234",
		},
		"envvar overrides deprecated envvar": {
			Config: &Config{
				AccessKey: servicemocks.MockStaticAccessKey,
				SecretKey: servicemocks.MockStaticSecretKey,
			},
			EnvironmentVariables: map[string]string{
				"AWS_METADATA_URL":                  "https://127.1.1.1:1111",
				"AWS_EC2_METADATA_SERVICE_ENDPOINT": "https://127.0.0.1:1234",
			},
			ExpectedEC2MetadataServiceEndpoint: "https://127.0.0.1:1234",
		},

		"shared configuration file": {
			Config: &Config{
				AccessKey: servicemocks.MockStaticAccessKey,
				SecretKey: servicemocks.MockStaticSecretKey,
			},
			SharedConfigurationFile: `
[default]
ec2_metadata_service_endpoint = https://127.0.0.1:1234
`,
			ExpectedEC2MetadataServiceEndpoint: "https://127.0.0.1:1234",
		},

		"config overrides envvar": {
			Config: &Config{
				AccessKey:                  servicemocks.MockStaticAccessKey,
				SecretKey:                  servicemocks.MockStaticSecretKey,
				EC2MetadataServiceEndpoint: "https://127.0.0.1:1234",
			},
			EnvironmentVariables: map[string]string{
				"AWS_EC2_METADATA_SERVICE_ENDPOINT": "https://127.1.1.1:1111",
			},
			ExpectedEC2MetadataServiceEndpoint: "https://127.0.0.1:1234",
		},
		"config overrides deprecated envvar": {
			Config: &Config{
				AccessKey:                  servicemocks.MockStaticAccessKey,
				SecretKey:                  servicemocks.MockStaticSecretKey,
				EC2MetadataServiceEndpoint: "https://127.0.0.1:1234",
			},
			EnvironmentVariables: map[string]string{
				"AWS_METADATA_URL": "https://127.1.1.1:1111",
			},
			ExpectedEC2MetadataServiceEndpoint: "https://127.0.0.1:1234",
		},

		"envvar overrides shared configuration": {
			Config: &Config{
				AccessKey: servicemocks.MockStaticAccessKey,
				SecretKey: servicemocks.MockStaticSecretKey,
			},
			EnvironmentVariables: map[string]string{
				"AWS_EC2_METADATA_SERVICE_ENDPOINT": "https://127.0.0.1:1234",
			},
			SharedConfigurationFile: `
[default]
ec2_metadata_service_endpoint = https://127.1.1.1:1111
`,
			ExpectedEC2MetadataServiceEndpoint: "https://127.0.0.1:1234",
		},
		"deprecated envvar overrides shared configuration": {
			Config: &Config{
				AccessKey: servicemocks.MockStaticAccessKey,
				SecretKey: servicemocks.MockStaticSecretKey,
			},
			EnvironmentVariables: map[string]string{
				"AWS_METADATA_URL": "https://127.0.0.1:1234",
			},
			SharedConfigurationFile: `
[default]
ec2_metadata_service_endpoint = https://127.1.1.1:1111
`,
			ExpectedEC2MetadataServiceEndpoint: "https://127.0.0.1:1234",
		},
	}

	for testName, testCase := range testCases {
		testCase := testCase

		t.Run(testName, func(t *testing.T) {
			oldEnv := servicemocks.InitSessionTestEnv()
			defer servicemocks.PopEnv(oldEnv)

			for k, v := range testCase.EnvironmentVariables {
				os.Setenv(k, v)
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

				testCase.Config.SharedConfigFiles = []string{file.Name()}
			}

			testCase.Config.SkipCredsValidation = true

			awsConfig, err := GetAwsConfig(context.Background(), testCase.Config)
			if err != nil {
				t.Fatalf("error in GetAwsConfig() '%[1]T': %[1]s", err)
			}

			ec2MetadataServiceEndpoint, _, err := awsconfig.ResolveEC2IMDSEndpointConfig(awsConfig.ConfigSources)
			if err != nil {
				t.Fatalf("error in ResolveEC2IMDSEndpointConfig: %s", err)
			}
			if a, e := ec2MetadataServiceEndpoint, testCase.ExpectedEC2MetadataServiceEndpoint; a != e {
				t.Errorf("expected EC2MetadataServiceEndpoint %q, got: %q", e, a)
			}
		})
	}
}

func TestEC2MetadataServiceEndpointMode(t *testing.T) {
	testCases := map[string]struct {
		Config                                 *Config
		EnvironmentVariables                   map[string]string
		SharedConfigurationFile                string
		ExpectedEC2MetadataServiceEndpointMode imds.EndpointModeState
	}{
		"no configuration": {
			Config: &Config{
				AccessKey: servicemocks.MockStaticAccessKey,
				SecretKey: servicemocks.MockStaticSecretKey,
			},
			ExpectedEC2MetadataServiceEndpointMode: imds.EndpointModeStateUnset,
		},

		"config": {
			Config: &Config{
				AccessKey:                      servicemocks.MockStaticAccessKey,
				SecretKey:                      servicemocks.MockStaticSecretKey,
				EC2MetadataServiceEndpointMode: EC2MetadataEndpointModeIPv4,
			},
			ExpectedEC2MetadataServiceEndpointMode: imds.EndpointModeStateIPv4,
		},

		"envvar": {
			Config: &Config{
				AccessKey: servicemocks.MockStaticAccessKey,
				SecretKey: servicemocks.MockStaticSecretKey,
			},
			EnvironmentVariables: map[string]string{
				"AWS_EC2_METADATA_SERVICE_ENDPOINT_MODE": EC2MetadataEndpointModeIPv6,
			},
			ExpectedEC2MetadataServiceEndpointMode: imds.EndpointModeStateIPv6,
		},

		"shared configuration file": {
			Config: &Config{
				AccessKey: servicemocks.MockStaticAccessKey,
				SecretKey: servicemocks.MockStaticSecretKey,
			},
			SharedConfigurationFile: `
[default]
ec2_metadata_service_endpoint_mode = IPv6
`,
			ExpectedEC2MetadataServiceEndpointMode: imds.EndpointModeStateIPv6,
		},

		"config overrides envvar": {
			Config: &Config{
				AccessKey:                      servicemocks.MockStaticAccessKey,
				SecretKey:                      servicemocks.MockStaticSecretKey,
				EC2MetadataServiceEndpointMode: EC2MetadataEndpointModeIPv4,
			},
			EnvironmentVariables: map[string]string{
				"AWS_EC2_METADATA_SERVICE_ENDPOINT_MODE": EC2MetadataEndpointModeIPv6,
			},
			ExpectedEC2MetadataServiceEndpointMode: imds.EndpointModeStateIPv4,
		},

		"envvar overrides shared configuration": {
			Config: &Config{
				AccessKey: servicemocks.MockStaticAccessKey,
				SecretKey: servicemocks.MockStaticSecretKey,
			},
			EnvironmentVariables: map[string]string{
				"AWS_EC2_METADATA_SERVICE_ENDPOINT_MODE": EC2MetadataEndpointModeIPv6,
			},
			SharedConfigurationFile: `
[default]
ec2_metadata_service_endpoint_mode = IPv4
`,
			ExpectedEC2MetadataServiceEndpointMode: imds.EndpointModeStateIPv6,
		},
	}

	for testName, testCase := range testCases {
		testCase := testCase

		t.Run(testName, func(t *testing.T) {
			oldEnv := servicemocks.InitSessionTestEnv()
			defer servicemocks.PopEnv(oldEnv)

			for k, v := range testCase.EnvironmentVariables {
				os.Setenv(k, v)
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

				testCase.Config.SharedConfigFiles = []string{file.Name()}
			}

			testCase.Config.SkipCredsValidation = true

			awsConfig, err := GetAwsConfig(context.Background(), testCase.Config)
			if err != nil {
				t.Fatalf("error in GetAwsConfig() '%[1]T': %[1]s", err)
			}

			ec2MetadataServiceEndpointMode, _, err := awsconfig.ResolveEC2IMDSEndpointModeConfig(awsConfig.ConfigSources)
			if err != nil {
				t.Fatalf("error in ResolveEC2IMDSEndpointConfig: %s", err)
			}
			if a, e := ec2MetadataServiceEndpointMode, testCase.ExpectedEC2MetadataServiceEndpointMode; a != e {
				t.Errorf("expected EC2MetadataServiceEndpointMode %q, got: %q", awsconfig.EC2IMDSEndpointModeString(e), awsconfig.EC2IMDSEndpointModeString(a))
			}
		})
	}
}

func TestCustomCABundle(t *testing.T) {
	testCases := map[string]struct {
		Config                              *Config
		SetConfig                           bool
		SetEnvironmentVariable              bool
		SetSharedConfigurationFile          bool
		SetSharedConfigurationFileToInvalid bool
		ExpandEnvVars                       bool
		EnvironmentVariables                map[string]string
		ExpectTLSClientConfigRootCAsSet     bool
	}{
		"no configuration": {
			Config: &Config{
				AccessKey: servicemocks.MockStaticAccessKey,
				SecretKey: servicemocks.MockStaticSecretKey,
			},
			ExpectTLSClientConfigRootCAsSet: false,
		},

		"config": {
			Config: &Config{
				AccessKey: servicemocks.MockStaticAccessKey,
				SecretKey: servicemocks.MockStaticSecretKey,
			},
			SetConfig:                       true,
			ExpectTLSClientConfigRootCAsSet: true,
		},

		"expanded config": {
			Config: &Config{
				AccessKey: servicemocks.MockStaticAccessKey,
				SecretKey: servicemocks.MockStaticSecretKey,
			},
			SetConfig:                       true,
			ExpandEnvVars:                   true,
			ExpectTLSClientConfigRootCAsSet: true,
		},

		"envvar": {
			Config: &Config{
				AccessKey: servicemocks.MockStaticAccessKey,
				SecretKey: servicemocks.MockStaticSecretKey,
			},
			SetEnvironmentVariable:          true,
			ExpectTLSClientConfigRootCAsSet: true,
		},

		"shared configuration file": {
			Config: &Config{
				AccessKey: servicemocks.MockStaticAccessKey,
				SecretKey: servicemocks.MockStaticSecretKey,
			},
			SetSharedConfigurationFile:      true,
			ExpectTLSClientConfigRootCAsSet: true,
		},

		"config overrides envvar": {
			Config: &Config{
				AccessKey: servicemocks.MockStaticAccessKey,
				SecretKey: servicemocks.MockStaticSecretKey,
			},
			SetConfig: true,
			EnvironmentVariables: map[string]string{
				"AWS_CA_BUNDLE": "no-such-file",
			},
			ExpectTLSClientConfigRootCAsSet: true,
		},

		"envvar overrides shared configuration": {
			Config: &Config{
				AccessKey: servicemocks.MockStaticAccessKey,
				SecretKey: servicemocks.MockStaticSecretKey,
			},
			SetEnvironmentVariable:              true,
			SetSharedConfigurationFileToInvalid: true,
			ExpectTLSClientConfigRootCAsSet:     true,
		},
	}

	for testName, testCase := range testCases {
		testCase := testCase

		t.Run(testName, func(t *testing.T) {
			oldEnv := servicemocks.InitSessionTestEnv()
			defer servicemocks.PopEnv(oldEnv)

			for k, v := range testCase.EnvironmentVariables {
				os.Setenv(k, v)
			}

			tempdir, err := ioutil.TempDir("", "temp")
			if err != nil {
				t.Fatalf("error creating temp dir: %s", err)
			}
			defer os.Remove(tempdir)
			os.Setenv("TMPDIR", tempdir)

			pemFile, err := servicemocks.TempPEMFile()
			defer os.Remove(pemFile)
			if err != nil {
				t.Fatalf("error creating PEM file: %s", err)
			}

			if testCase.ExpandEnvVars {
				tmpdir := os.Getenv("TMPDIR")
				rel, err := filepath.Rel(tmpdir, pemFile)
				if err != nil {
					t.Fatalf("error making path relative: %s", err)
				}
				t.Logf("relative: %s", rel)
				pemFile = filepath.Join("$TMPDIR", rel)
				t.Logf("env tempfile: %s", pemFile)
			}

			if testCase.SetConfig {
				testCase.Config.CustomCABundle = pemFile
			}

			if testCase.SetEnvironmentVariable {
				os.Setenv("AWS_CA_BUNDLE", pemFile)
			}

			if testCase.SetSharedConfigurationFile {
				file, err := ioutil.TempFile("", "aws-sdk-go-base-shared-configuration-file")

				if err != nil {
					t.Fatalf("unexpected error creating temporary shared configuration file: %s", err)
				}

				defer os.Remove(file.Name())

				err = ioutil.WriteFile(
					file.Name(),
					[]byte(fmt.Sprintf(`
[default]
ca_bundle = %s
`, pemFile)),
					0600)

				if err != nil {
					t.Fatalf("unexpected error writing shared configuration file: %s", err)
				}

				testCase.Config.SharedConfigFiles = []string{file.Name()}
			}

			if testCase.SetSharedConfigurationFileToInvalid {
				file, err := ioutil.TempFile("", "aws-sdk-go-base-shared-configuration-file")

				if err != nil {
					t.Fatalf("unexpected error creating temporary shared configuration file: %s", err)
				}

				defer os.Remove(file.Name())

				err = ioutil.WriteFile(
					file.Name(),
					[]byte(`
[default]
ca_bundle = no-such-file
`),
					0600)

				if err != nil {
					t.Fatalf("unexpected error writing shared configuration file: %s", err)
				}

				testCase.Config.SharedConfigFiles = []string{file.Name()}
			}

			testCase.Config.SkipCredsValidation = true

			awsConfig, err := GetAwsConfig(context.Background(), testCase.Config)
			if err != nil {
				t.Fatalf("error in GetAwsConfig() '%[1]T': %[1]s", err)
			}

			type transportGetter interface {
				GetTransport() *http.Transport
			}

			trGetter := awsConfig.HTTPClient.(transportGetter)
			tr := trGetter.GetTransport()

			if a, e := tr.TLSClientConfig.RootCAs != nil, testCase.ExpectTLSClientConfigRootCAsSet; a != e {
				t.Errorf("expected(%t) CA Bundle, got: %t", e, a)
			}
		})
	}
}

func TestAssumeRoleWithWebIdentity(t *testing.T) {
	testCases := map[string]struct {
		Config                     *Config
		SetConfig                  bool
		EnvironmentVariables       map[string]string
		SetEnvironmentVariable     bool
		SharedConfigurationFile    string
		SetSharedConfigurationFile bool
		ExpectedCredentialsValue   aws.Credentials
		MockStsEndpoints           []*servicemocks.MockEndpoint
	}{
		// "config": {
		// 	Config:                   &Config{},
		// 	SetConfig:                true,
		// 	ExpectedCredentialsValue: mockdata.MockStsAssumeRoleWithWebIdentityCredentials,
		// 	MockStsEndpoints: []*servicemocks.MockEndpoint{
		// 		servicemocks.MockStsAssumeRoleWithWebIdentityValidEndpoint,
		// 	},
		// },

		"envvar": {
			Config: &Config{},
			EnvironmentVariables: map[string]string{
				"AWS_ROLE_ARN":          servicemocks.MockStsAssumeRoleWithWebIdentityArn,
				"AWS_ROLE_SESSION_NAME": servicemocks.MockStsAssumeRoleWithWebIdentitySessionName,
			},
			SetEnvironmentVariable:   true,
			ExpectedCredentialsValue: mockdata.MockStsAssumeRoleWithWebIdentityCredentials,
			MockStsEndpoints: []*servicemocks.MockEndpoint{
				servicemocks.MockStsAssumeRoleWithWebIdentityValidEndpoint,
			},
		},

		"shared configuration file": {
			Config: &Config{},
			SharedConfigurationFile: fmt.Sprintf(`
[default]
role_arn = %[1]s
role_session_name = %[2]s
`, servicemocks.MockStsAssumeRoleWithWebIdentityArn, servicemocks.MockStsAssumeRoleWithWebIdentitySessionName),
			SetSharedConfigurationFile: true,
			ExpectedCredentialsValue:   mockdata.MockStsAssumeRoleWithWebIdentityCredentials,
			MockStsEndpoints: []*servicemocks.MockEndpoint{
				servicemocks.MockStsAssumeRoleWithWebIdentityValidEndpoint,
			},
		},

		// "config overrides envvar": {
		// 	Config:    &Config{},
		// 	SetConfig: true,
		// 	EnvironmentVariables: map[string]string{
		// 		"AWS_ROLE_ARN":                servicemocks.MockStsAssumeRoleWithWebIdentityArn,
		// 		"AWS_ROLE_SESSION_NAME":       servicemocks.MockStsAssumeRoleWithWebIdentitySessionName,
		// 		"AWS_WEB_IDENTITY_TOKEN_FILE": "no-such-file",
		// 	},
		// 	ExpectedCredentialsValue: mockdata.MockStsAssumeRoleWithWebIdentityCredentials,
		// 	MockStsEndpoints: []*servicemocks.MockEndpoint{
		// 		servicemocks.MockStsAssumeRoleWithWebIdentityValidEndpoint,
		// 	},
		// },

		"envvar overrides shared configuration": {
			Config: &Config{},
			EnvironmentVariables: map[string]string{
				"AWS_ROLE_ARN":          servicemocks.MockStsAssumeRoleWithWebIdentityArn,
				"AWS_ROLE_SESSION_NAME": servicemocks.MockStsAssumeRoleWithWebIdentitySessionName,
			},
			SetEnvironmentVariable: true,
			SharedConfigurationFile: fmt.Sprintf(`
[default]
role_arn = %[1]s
role_session_name = %[2]s
web_identity_token_file = no-such-file
`, servicemocks.MockStsAssumeRoleWithWebIdentityArn, servicemocks.MockStsAssumeRoleWithWebIdentitySessionName),
			ExpectedCredentialsValue: mockdata.MockStsAssumeRoleWithWebIdentityCredentials,
			MockStsEndpoints: []*servicemocks.MockEndpoint{
				servicemocks.MockStsAssumeRoleWithWebIdentityValidEndpoint,
			},
		},
	}

	for testName, testCase := range testCases {
		testCase := testCase

		t.Run(testName, func(t *testing.T) {
			oldEnv := servicemocks.InitSessionTestEnv()
			defer servicemocks.PopEnv(oldEnv)

			for k, v := range testCase.EnvironmentVariables {
				os.Setenv(k, v)
			}

			closeSts, _, stsEndpoint := mockdata.GetMockedAwsApiSession("STS", testCase.MockStsEndpoints)
			defer closeSts()

			testCase.Config.StsEndpoint = stsEndpoint

			tokenFile, err := ioutil.TempFile("", "aws-sdk-go-base-web-identity-token-file")
			if err != nil {
				t.Fatalf("unexpected error creating temporary web identity token file: %s", err)
			}

			defer os.Remove(tokenFile.Name())

			err = ioutil.WriteFile(tokenFile.Name(), []byte(servicemocks.MockWebIdentityToken), 0600)

			if err != nil {
				t.Fatalf("unexpected error writing web identity token file: %s", err)
			}

			if testCase.SetEnvironmentVariable {
				os.Setenv("AWS_WEB_IDENTITY_TOKEN_FILE", tokenFile.Name())
			}

			if testCase.SharedConfigurationFile != "" {
				file, err := ioutil.TempFile("", "aws-sdk-go-base-shared-configuration-file")

				if err != nil {
					t.Fatalf("unexpected error creating temporary shared configuration file: %s", err)
				}

				defer os.Remove(file.Name())

				if testCase.SetSharedConfigurationFile {
					testCase.SharedConfigurationFile += fmt.Sprintf("web_identity_token_file = %s\n", tokenFile.Name())
				}

				err = ioutil.WriteFile(file.Name(), []byte(testCase.SharedConfigurationFile), 0600)

				if err != nil {
					t.Fatalf("unexpected error writing shared configuration file: %s", err)
				}

				testCase.Config.SharedConfigFiles = []string{file.Name()}
			}

			testCase.Config.SkipCredsValidation = true

			awsConfig, err := GetAwsConfig(context.Background(), testCase.Config)
			if err != nil {
				t.Fatalf("error in GetAwsConfig() '%[1]T': %[1]s", err)
			}

			credentialsValue, err := awsConfig.Credentials.Retrieve(context.Background())

			if err != nil {
				t.Fatalf("unexpected credentials Retrieve() error: %s", err)
			}

			if diff := cmp.Diff(credentialsValue, testCase.ExpectedCredentialsValue, cmpopts.IgnoreFields(aws.Credentials{}, "Expires")); diff != "" {
				t.Fatalf("unexpected credentials: (- got, + expected)\n%s", diff)
			}
		})
	}
}

func TestGetAwsConfigWithAccountIDAndPartition(t *testing.T) {
	oldEnv := servicemocks.InitSessionTestEnv()
	defer servicemocks.PopEnv(oldEnv)

	testCases := []struct {
		desc              string
		config            *Config
		expectedAcctID    string
		expectedPartition string
		expectError       bool
		mockStsEndpoints  []*servicemocks.MockEndpoint
	}{
		{
			desc: "StandardProvider_Config",
			config: &Config{
				AccessKey: "MockAccessKey",
				SecretKey: "MockSecretKey",
				Region:    "us-west-2"},
			expectedAcctID: "222222222222", expectedPartition: "aws",
			mockStsEndpoints: []*servicemocks.MockEndpoint{
				servicemocks.MockStsGetCallerIdentityValidEndpoint,
			},
		},
		{
			desc: "SkipCredsValidation_Config",
			config: &Config{
				AccessKey:           "MockAccessKey",
				SecretKey:           "MockSecretKey",
				Region:              "us-west-2",
				SkipCredsValidation: true},
			expectedAcctID: "222222222222", expectedPartition: "aws",
			mockStsEndpoints: []*servicemocks.MockEndpoint{
				servicemocks.MockStsGetCallerIdentityValidEndpoint,
			},
		},
		{
			desc: "SkipRequestingAccountId_Config",
			config: &Config{
				AccessKey:               "MockAccessKey",
				SecretKey:               "MockSecretKey",
				Region:                  "us-west-2",
				SkipCredsValidation:     true,
				SkipRequestingAccountId: true},
			expectedAcctID: "", expectedPartition: "aws",
			mockStsEndpoints: []*servicemocks.MockEndpoint{},
		},
		{
			desc: "WithAssumeRole",
			config: &Config{
				AccessKey: "MockAccessKey",
				SecretKey: "MockSecretKey",
				Region:    "us-west-2",
				AssumeRole: &AssumeRole{
					RoleARN:     servicemocks.MockStsAssumeRoleArn,
					SessionName: servicemocks.MockStsAssumeRoleSessionName,
				},
			},
			expectedAcctID: "555555555555", expectedPartition: "aws",
			mockStsEndpoints: []*servicemocks.MockEndpoint{
				servicemocks.MockStsAssumeRoleValidEndpoint,
				servicemocks.MockStsGetCallerIdentityValidAssumedRoleEndpoint,
			},
		},
	}

	for _, testCase := range testCases {
		tc := testCase

		t.Run(tc.desc, func(t *testing.T) {
			ts := servicemocks.MockAwsApiServer("STS", tc.mockStsEndpoints)
			defer ts.Close()
			tc.config.StsEndpoint = ts.URL

			awsConfig, err := GetAwsConfig(context.Background(), tc.config)
			if err != nil {
				t.Fatalf("expected no error from GetAwsConfig(), got: %s", err)
			}
			acctID, part, err := GetAwsAccountIDAndPartition(context.Background(), awsConfig, tc.config)
			if err != nil {
				if !tc.expectError {
					t.Fatalf("expected no error, got: %s", err)
				}

				if !IsNoValidCredentialSourcesError(err) {
					t.Fatalf("expected no valid credential sources error, got: %s", err)
				}

				t.Logf("received expected error: %s", err)
				return
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

type mockRetryableError struct{ b bool }

func (m mockRetryableError) RetryableError() bool { return m.b }
func (m mockRetryableError) Error() string {
	return fmt.Sprintf("mock retryable %t", m.b)
}

func TestRetryHandlers(t *testing.T) {
	const maxRetries = 10

	testcases := map[string]struct {
		NextHandler   func() middleware.FinalizeHandler
		ExpectResults retry.AttemptResults
		Err           error
	}{
		"stops at maxRetries for retryable errors": {
			NextHandler: func() middleware.FinalizeHandler {
				num := 0
				reqsErrs := make([]error, maxRetries)
				for i := 0; i < maxRetries; i++ {
					reqsErrs[i] = mockRetryableError{b: true}
				}
				return middleware.FinalizeHandlerFunc(func(ctx context.Context, in middleware.FinalizeInput) (out middleware.FinalizeOutput, metadata middleware.Metadata, err error) {
					if num >= len(reqsErrs) {
						err = fmt.Errorf("more requests than expected")
					} else {
						err = reqsErrs[num]
						num++
					}
					return out, metadata, err
				})
			},
			Err: fmt.Errorf("exceeded maximum number of attempts"),
			ExpectResults: func() retry.AttemptResults {
				results := retry.AttemptResults{
					Results: make([]retry.AttemptResult, maxRetries),
				}
				for i := 0; i < maxRetries-1; i++ {
					results.Results[i] = retry.AttemptResult{
						Err:       mockRetryableError{b: true},
						Retryable: true,
						Retried:   true,
					}
				}
				results.Results[maxRetries-1] = retry.AttemptResult{
					Err:       &retry.MaxAttemptsError{Attempt: maxRetries, Err: mockRetryableError{b: true}},
					Retryable: true,
				}
				return results
			}(),
		},
		"stops at MaxNetworkRetryCount for 'no such host' errors": {
			NextHandler: func() middleware.FinalizeHandler {
				num := 0
				reqsErrs := make([]error, constants.MaxNetworkRetryCount)
				for i := 0; i < constants.MaxNetworkRetryCount; i++ {
					reqsErrs[i] = &net.OpError{Op: "dial", Err: errors.New("no such host")}
				}
				return middleware.FinalizeHandlerFunc(func(ctx context.Context, in middleware.FinalizeInput) (out middleware.FinalizeOutput, metadata middleware.Metadata, err error) {
					if num >= len(reqsErrs) {
						err = fmt.Errorf("more requests than expected")
					} else {
						err = reqsErrs[num]
						num++
					}
					return out, metadata, err
				})
			},
			Err: fmt.Errorf("exceeded maximum number of attempts"),
			ExpectResults: func() retry.AttemptResults {
				results := retry.AttemptResults{
					Results: make([]retry.AttemptResult, constants.MaxNetworkRetryCount),
				}
				for i := 0; i < constants.MaxNetworkRetryCount-1; i++ {
					results.Results[i] = retry.AttemptResult{
						Err:       &net.OpError{Op: "dial", Err: errors.New("no such host")},
						Retryable: true,
						Retried:   true,
					}
				}
				results.Results[constants.MaxNetworkRetryCount-1] = retry.AttemptResult{
					Err:       &retry.MaxAttemptsError{Attempt: constants.MaxNetworkRetryCount, Err: &net.OpError{Op: "dial", Err: errors.New("no such host")}},
					Retryable: true,
				}
				return results
			}(),
		},
		"stops at MaxNetworkRetryCount for 'connection refused' errors": {
			NextHandler: func() middleware.FinalizeHandler {
				num := 0
				reqsErrs := make([]error, constants.MaxNetworkRetryCount)
				for i := 0; i < constants.MaxNetworkRetryCount; i++ {
					reqsErrs[i] = &net.OpError{Op: "dial", Err: errors.New("connection refused")}
				}
				return middleware.FinalizeHandlerFunc(func(ctx context.Context, in middleware.FinalizeInput) (out middleware.FinalizeOutput, metadata middleware.Metadata, err error) {
					if num >= len(reqsErrs) {
						err = fmt.Errorf("more requests than expected")
					} else {
						err = reqsErrs[num]
						num++
					}
					return out, metadata, err
				})
			},
			Err: fmt.Errorf("exceeded maximum number of attempts"),
			ExpectResults: func() retry.AttemptResults {
				results := retry.AttemptResults{
					Results: make([]retry.AttemptResult, constants.MaxNetworkRetryCount),
				}
				for i := 0; i < constants.MaxNetworkRetryCount-1; i++ {
					results.Results[i] = retry.AttemptResult{
						Err:       &net.OpError{Op: "dial", Err: errors.New("connection refused")},
						Retryable: true,
						Retried:   true,
					}
				}
				results.Results[constants.MaxNetworkRetryCount-1] = retry.AttemptResult{
					Err:       &retry.MaxAttemptsError{Attempt: constants.MaxNetworkRetryCount, Err: &net.OpError{Op: "dial", Err: errors.New("connection refused")}},
					Retryable: true,
				}
				return results
			}(),
		},
		"stops at maxRetries for other network errors": {
			NextHandler: func() middleware.FinalizeHandler {
				num := 0
				reqsErrs := make([]error, maxRetries)
				for i := 0; i < maxRetries; i++ {
					reqsErrs[i] = &net.OpError{Op: "dial", Err: errors.New("other error")}
				}
				return middleware.FinalizeHandlerFunc(func(ctx context.Context, in middleware.FinalizeInput) (out middleware.FinalizeOutput, metadata middleware.Metadata, err error) {
					if num >= len(reqsErrs) {
						err = fmt.Errorf("more requests than expected")
					} else {
						err = reqsErrs[num]
						num++
					}
					return out, metadata, err
				})
			},
			Err: fmt.Errorf("exceeded maximum number of attempts"),
			ExpectResults: func() retry.AttemptResults {
				results := retry.AttemptResults{
					Results: make([]retry.AttemptResult, maxRetries),
				}
				for i := 0; i < maxRetries-1; i++ {
					results.Results[i] = retry.AttemptResult{
						Err:       &net.OpError{Op: "dial", Err: errors.New("other error")},
						Retryable: true,
						Retried:   true,
					}
				}
				results.Results[maxRetries-1] = retry.AttemptResult{
					Err:       &retry.MaxAttemptsError{Attempt: maxRetries, Err: &net.OpError{Op: "dial", Err: errors.New("other error")}},
					Retryable: true,
				}
				return results
			}(),
		},
	}

	for name, testcase := range testcases {
		testcase := testcase

		t.Run(name, func(t *testing.T) {
			oldEnv := servicemocks.InitSessionTestEnv()
			defer servicemocks.PopEnv(oldEnv)

			config := &Config{
				AccessKey:           servicemocks.MockStaticAccessKey,
				Region:              "us-east-1",
				MaxRetries:          maxRetries,
				SecretKey:           servicemocks.MockStaticSecretKey,
				SkipCredsValidation: true,
			}
			awsConfig, err := GetAwsConfig(context.Background(), config)
			if err != nil {
				t.Fatalf("unexpected error from GetAwsConfig(): %s", err)
			}
			if awsConfig.Retryer == nil {
				t.Fatal("No Retryer configured on awsConfig")
			}

			am := retry.NewAttemptMiddleware(&withNoDelay{
				Retryer: awsConfig.Retryer(),
			}, func(i interface{}) interface{} {
				return i
			})
			_, metadata, err := am.HandleFinalize(context.Background(), middleware.FinalizeInput{Request: nil}, testcase.NextHandler())
			if err != nil && testcase.Err == nil {
				t.Errorf("expect no error, got %v", err)
			} else if err == nil && testcase.Err != nil {
				t.Errorf("expect error, got none")
			} else if err != nil && testcase.Err != nil {
				if !strings.Contains(err.Error(), testcase.Err.Error()) {
					t.Errorf("expect %v, got %v", testcase.Err, err)
				}
			}

			attemptResults, ok := retry.GetAttemptResults(metadata)
			if !ok {
				t.Fatalf("expected metadata to contain attempt results, got none")
			}
			if e, a := testcase.ExpectResults, attemptResults; !reflect.DeepEqual(e, a) {
				t.Fatalf("expected %v, got %v", e, a)
			}

			for i, attempt := range attemptResults.Results {
				_, ok := retry.GetAttemptResults(attempt.ResponseMetadata)
				if ok {
					t.Errorf("expect no attempt to include AttemptResults metadata, %v does, %#v", i, attempt)
				}
			}
		})
	}
}

type withNoDelay struct {
	aws.Retryer
}

func (r *withNoDelay) RetryDelay(attempt int, err error) (time.Duration, error) {
	delay, delayErr := r.Retryer.RetryDelay(attempt, err)
	if delayErr != nil {
		return delay, delayErr
	}

	return 0 * time.Second, nil
}
