// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package awsv1shim

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	configv2 "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/feature/ec2/imds"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/client"
	"github.com/aws/aws-sdk-go/aws/client/metadata"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/endpoints"
	"github.com/aws/aws-sdk-go/aws/request"
	"github.com/aws/aws-sdk-go/service/iam"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	awsbase "github.com/hashicorp/aws-sdk-go-base/v2"
	"github.com/hashicorp/aws-sdk-go-base/v2/awsv1shim/v2/mockdata"
	"github.com/hashicorp/aws-sdk-go-base/v2/internal/test"
	"github.com/hashicorp/aws-sdk-go-base/v2/servicemocks"
	"github.com/hashicorp/aws-sdk-go-base/v2/useragent"
	"github.com/hashicorp/terraform-plugin-log/tflogtest"
)

func TestGetSessionOptions(t *testing.T) {
	oldEnv := servicemocks.InitSessionTestEnv()
	defer servicemocks.PopEnv(oldEnv)

	testCases := []struct {
		desc        string
		config      *awsbase.Config
		expectError bool
	}{
		{"ConfigWithCredentials",
			&awsbase.Config{AccessKey: "MockAccessKey", SecretKey: "MockSecretKey"},
			false,
		},
		{"ConfigWithCredsAndOptions",
			&awsbase.Config{AccessKey: "MockAccessKey", SecretKey: "MockSecretKey", Insecure: true},
			false,
		},
	}

	for _, testCase := range testCases {
		tc := testCase

		t.Run(tc.desc, func(t *testing.T) {
			ctx := test.Context(t)

			tc.config.SkipCredsValidation = true

			ctx, awsConfig, err := awsbase.GetAwsConfig(ctx, tc.config)
			if err != nil {
				t.Fatalf("GetAwsConfig() resulted in an error %s", err)
			}

			opts, err := getSessionOptions(ctx, &awsConfig, tc.config)
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
		EnableWebIdentityEnvVars   bool
		EnableWebIdentityConfig    bool
		EnvironmentVariables       map[string]string
		ExpectedCredentialsValue   credentials.Value
		ExpectedRegion             string
		ExpectedError              func(err error) bool
		MockStsEndpoints           []*servicemocks.MockEndpoint
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
			Config: &awsbase.Config{
				AccessKey: servicemocks.MockStaticAccessKey,
				AssumeRole: &awsbase.AssumeRole{
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
			Config: &awsbase.Config{
				AccessKey: servicemocks.MockStaticAccessKey,
				AssumeRole: &awsbase.AssumeRole{
					RoleARN:     servicemocks.MockStsAssumeRoleArn,
					Duration:    1 * time.Hour,
					SessionName: servicemocks.MockStsAssumeRoleSessionName,
				},
				Region:    "us-east-1",
				SecretKey: servicemocks.MockStaticSecretKey,
			},
			Description:              "config AssumeRoleDuration",
			ExpectedCredentialsValue: mockdata.MockStsAssumeRoleCredentials,
			ExpectedRegion:           "us-east-1",
			MockStsEndpoints: []*servicemocks.MockEndpoint{
				servicemocks.MockStsAssumeRoleValidEndpointWithOptions(map[string]string{"DurationSeconds": "3600"}),
				servicemocks.MockStsGetCallerIdentityValidEndpoint,
			},
		},
		{
			Config: &awsbase.Config{
				AccessKey: servicemocks.MockStaticAccessKey,
				AssumeRole: &awsbase.AssumeRole{
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
			Config: &awsbase.Config{
				AccessKey: servicemocks.MockStaticAccessKey,
				AssumeRole: &awsbase.AssumeRole{
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
			Config: &awsbase.Config{
				AccessKey: servicemocks.MockStaticAccessKey,
				AssumeRole: &awsbase.AssumeRole{
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
			Config: &awsbase.Config{
				AccessKey: servicemocks.MockStaticAccessKey,
				AssumeRole: &awsbase.AssumeRole{
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
			Config: &awsbase.Config{
				AccessKey: servicemocks.MockStaticAccessKey,
				AssumeRole: &awsbase.AssumeRole{
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
			Config: &awsbase.Config{
				AccessKey: servicemocks.MockStaticAccessKey,
				AssumeRole: &awsbase.AssumeRole{
					RoleARN:        servicemocks.MockStsAssumeRoleArn,
					SessionName:    servicemocks.MockStsAssumeRoleSessionName,
					SourceIdentity: servicemocks.MockStsAssumeRoleSourceIdentity,
				},
				Region:    "us-east-1",
				SecretKey: servicemocks.MockStaticSecretKey,
			},
			Description:              "config AssumeRoleSourceIdentity",
			ExpectedCredentialsValue: mockdata.MockStsAssumeRoleCredentials,
			ExpectedRegion:           "us-east-1",
			MockStsEndpoints: []*servicemocks.MockEndpoint{
				servicemocks.MockStsAssumeRoleValidEndpointWithOptions(map[string]string{"SourceIdentity": servicemocks.MockStsAssumeRoleSourceIdentity}),
				servicemocks.MockStsGetCallerIdentityValidEndpoint,
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
			Config: &awsbase.Config{
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
		// 			ExpectedCredentialsValue:   mockdata.MockStsAssumeRoleCredentials,
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
			Config: &awsbase.Config{
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
			Config: &awsbase.Config{
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
			Config: &awsbase.Config{
				AssumeRole: &awsbase.AssumeRole{
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
			Config: &awsbase.Config{
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
		// 			ExpectedCredentialsValue: mockdata.MockStsAssumeRoleCredentials,
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
			Config: &awsbase.Config{
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
			Config: &awsbase.Config{
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
			Config: &awsbase.Config{
				AssumeRole: &awsbase.AssumeRole{
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
			Config: &awsbase.Config{
				Region: "us-east-1",
			},
			Description:              "web identity token access key",
			EnableWebIdentityEnvVars: true,
			ExpectedCredentialsValue: mockdata.MockStsAssumeRoleWithWebIdentityCredentials,
			ExpectedRegion:           "us-east-1",
			MockStsEndpoints: []*servicemocks.MockEndpoint{
				servicemocks.MockStsAssumeRoleWithWebIdentityValidEndpoint,
				servicemocks.MockStsGetCallerIdentityValidEndpoint,
			},
		},
		{
			Config: &awsbase.Config{
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
			Config: &awsbase.Config{
				AssumeRole: &awsbase.AssumeRole{
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
			Config: &awsbase.Config{
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
			Config: &awsbase.Config{
				AssumeRole: &awsbase.AssumeRole{
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
			Config: &awsbase.Config{
				AssumeRole: &awsbase.AssumeRole{
					RoleARN:     servicemocks.MockStsAssumeRoleArn,
					SessionName: servicemocks.MockStsAssumeRoleSessionName,
				},
				Region: "us-east-1",
			},
			Description:              "AssumeWebIdentity envvar AssumeRoleARN access key",
			EnableWebIdentityEnvVars: true,
			ExpectedCredentialsValue: mockdata.MockStsAssumeRoleCredentials,
			ExpectedRegion:           "us-east-1",
			MockStsEndpoints: []*servicemocks.MockEndpoint{
				servicemocks.MockStsAssumeRoleWithWebIdentityValidEndpoint,
				servicemocks.MockStsAssumeRoleValidEndpoint,
				servicemocks.MockStsGetCallerIdentityValidEndpoint,
			},
		},
		{
			Config: &awsbase.Config{
				AssumeRole: &awsbase.AssumeRole{
					RoleARN:     servicemocks.MockStsAssumeRoleArn,
					SessionName: servicemocks.MockStsAssumeRoleSessionName,
				},
				Region: "us-east-1",
			},
			Description:              "AssumeWebIdentity config AssumeRoleARN access key",
			EnableWebIdentityConfig:  true,
			ExpectedCredentialsValue: mockdata.MockStsAssumeRoleCredentials,
			ExpectedRegion:           "us-east-1",
			MockStsEndpoints: []*servicemocks.MockEndpoint{
				servicemocks.MockStsAssumeRoleWithWebIdentityValidEndpoint,
				servicemocks.MockStsAssumeRoleValidEndpoint,
				servicemocks.MockStsGetCallerIdentityValidEndpoint,
			},
		},
		{
			Config: &awsbase.Config{
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
			Config: &awsbase.Config{
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
			Config: &awsbase.Config{
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
			Config: &awsbase.Config{
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
			Config: &awsbase.Config{
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
			Config: &awsbase.Config{
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
			Config: &awsbase.Config{
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
			Config: &awsbase.Config{
				Region: "us-east-1",
			},
			Description: "shared credentials default aws_access_key_id over EC2 metadata access key",
			ExpectedCredentialsValue: credentials.Value{
				AccessKeyID:     "DefaultSharedCredentialsAccessKey",
				ProviderName:    credentials.SharedCredsProviderName,
				SecretAccessKey: "DefaultSharedCredentialsSecretKey",
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
			Config: &awsbase.Config{
				Region: "us-east-1",
			},
			Description:                "shared credentials default aws_access_key_id over ECS credentials access key",
			EnableEcsCredentialsServer: true,
			ExpectedCredentialsValue: credentials.Value{
				AccessKeyID:     "DefaultSharedCredentialsAccessKey",
				ProviderName:    credentials.SharedCredsProviderName,
				SecretAccessKey: "DefaultSharedCredentialsSecretKey",
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
			Config: &awsbase.Config{
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
			Config: &awsbase.Config{
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
			Config: &awsbase.Config{
				AccessKey: servicemocks.MockStaticAccessKey,
				AssumeRole: &awsbase.AssumeRole{
					RoleARN:     servicemocks.MockStsAssumeRoleArn,
					SessionName: servicemocks.MockStsAssumeRoleSessionName,
				},
				Region:    "us-east-1",
				SecretKey: servicemocks.MockStaticSecretKey,
			},
			Description: "assume role error",
			ExpectedError: func(err error) bool {
				return awsbase.IsCannotAssumeRoleError(err)
			},
			ExpectedRegion: "us-east-1",
			MockStsEndpoints: []*servicemocks.MockEndpoint{
				servicemocks.MockStsAssumeRoleInvalidEndpointInvalidClientTokenId,
				servicemocks.MockStsGetCallerIdentityValidEndpoint,
			},
		},
		{
			Config: &awsbase.Config{
				AccessKey: servicemocks.MockStaticAccessKey,
				Region:    "us-east-1",
				SecretKey: servicemocks.MockStaticSecretKey,
			},
			Description: "ExpiredToken invalid body",
			ExpectedError: func(err error) bool {
				return strings.Contains(err.Error(), "ExpiredToken")
			},
			MockStsEndpoints: []*servicemocks.MockEndpoint{
				servicemocks.MockStsGetCallerIdentityInvalidBodyExpiredToken,
			},
		},
		{
			Config: &awsbase.Config{
				AccessKey: servicemocks.MockStaticAccessKey,
				Region:    "us-east-1",
				SecretKey: servicemocks.MockStaticSecretKey,
			},
			Description: "ExpiredToken valid body", // in case they change it
			ExpectedError: func(err error) bool {
				return strings.Contains(err.Error(), "ExpiredToken")
			},
			MockStsEndpoints: []*servicemocks.MockEndpoint{
				servicemocks.MockStsGetCallerIdentityValidBodyExpiredToken,
			},
		},
		{
			Config: &awsbase.Config{
				AccessKey: servicemocks.MockStaticAccessKey,
				Region:    "us-east-1",
				SecretKey: servicemocks.MockStaticSecretKey,
			},
			Description: "ExpiredTokenException invalid body",
			ExpectedError: func(err error) bool {
				return strings.Contains(err.Error(), "ExpiredTokenException")
			},
			MockStsEndpoints: []*servicemocks.MockEndpoint{
				servicemocks.MockStsGetCallerIdentityInvalidBodyExpiredTokenException,
			},
		},
		{
			Config: &awsbase.Config{
				AccessKey: servicemocks.MockStaticAccessKey,
				Region:    "us-east-1",
				SecretKey: servicemocks.MockStaticSecretKey,
			},
			Description: "ExpiredTokenException valid body", // in case they change it
			ExpectedError: func(err error) bool {
				return strings.Contains(err.Error(), "ExpiredTokenException")
			},
			MockStsEndpoints: []*servicemocks.MockEndpoint{
				servicemocks.MockStsGetCallerIdentityValidBodyExpiredTokenException,
			},
		},
		{
			Config: &awsbase.Config{
				AccessKey: servicemocks.MockStaticAccessKey,
				Region:    "us-east-1",
				SecretKey: servicemocks.MockStaticSecretKey,
			},
			Description: "RequestExpired invalid body",
			ExpectedError: func(err error) bool {
				return strings.Contains(err.Error(), "RequestExpired")
			},
			MockStsEndpoints: []*servicemocks.MockEndpoint{
				servicemocks.MockStsGetCallerIdentityInvalidBodyRequestExpired,
			},
		},
		{
			Config: &awsbase.Config{
				AccessKey: servicemocks.MockStaticAccessKey,
				Region:    "us-east-1",
				SecretKey: servicemocks.MockStaticSecretKey,
			},
			Description: "RequestExpired valid body", // in case they change it
			ExpectedError: func(err error) bool {
				return strings.Contains(err.Error(), "RequestExpired")
			},
			MockStsEndpoints: []*servicemocks.MockEndpoint{
				servicemocks.MockStsGetCallerIdentityValidBodyRequestExpired,
			},
		},
		// 		{
		// 			Config: &awsbase.Config{
		// 				AccessKey: servicemocks.MockStaticAccessKey,
		// 				Region:    "us-east-1",
		// 				SecretKey: servicemocks.MockStaticSecretKey,
		// 			},
		// 			Description: "credential validation error",
		// 			ExpectedError: func(err error) bool {
		// 				return tfawserr.ErrCodeEquals(err, "AccessDenied")
		// 			},
		// 			MockStsEndpoints: []*servicemocks.MockEndpoint{
		// 				servicemocks.MockStsGetCallerIdentityInvalidEndpointAccessDenied,
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
			Config: &awsbase.Config{
				Region:                        "us-east-1",
				EC2MetadataServiceEnableState: imds.ClientDisabled,
			},
			Description: "skip EC2 Metadata API check",
			ExpectedError: func(err error) bool {
				return awsbase.IsNoValidCredentialSourcesError(err)
			},
			ExpectedRegion: "us-east-1",
			// The IMDS server must be enabled so that auth will succeed if the IMDS is called
			EnableEc2MetadataServer: true,
			MockStsEndpoints: []*servicemocks.MockEndpoint{
				servicemocks.MockStsGetCallerIdentityValidEndpoint,
			},
		},
		{
			Config: &awsbase.Config{
				Region: "us-east-1",
			},
			Description: "invalid profile name from envvar",
			EnvironmentVariables: map[string]string{
				"AWS_PROFILE": "no-such-profile",
			},
			ExpectedError: func(err error) bool {
				var e configv2.SharedConfigProfileNotExistError
				return errors.As(err, &e)
			},
			SharedCredentialsFile: `
[some-profile]
aws_access_key_id = DefaultSharedCredentialsAccessKey
aws_secret_access_key = DefaultSharedCredentialsSecretKey
`,
		},
		{
			Config: &awsbase.Config{
				Profile: "no-such-profile",
				Region:  "us-east-1",
			},
			Description: "invalid profile name from config",
			ExpectedError: func(err error) bool {
				var e configv2.SharedConfigProfileNotExistError
				return errors.As(err, &e)
			},
			SharedCredentialsFile: `
[some-profile]
aws_access_key_id = DefaultSharedCredentialsAccessKey
aws_secret_access_key = DefaultSharedCredentialsSecretKey
`,
		},
		{
			Config:      &awsbase.Config{},
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
			Config: &awsbase.Config{
				Region: "us-east-1",
			},
			Description: "AWS_ACCESS_KEY_ID does not override invalid profile name from envvar",
			EnvironmentVariables: map[string]string{
				"AWS_ACCESS_KEY_ID":     servicemocks.MockEnvAccessKey,
				"AWS_SECRET_ACCESS_KEY": servicemocks.MockEnvSecretKey,
				"AWS_PROFILE":           "no-such-profile",
			},
			ExpectedError: func(err error) bool {
				var e configv2.SharedConfigProfileNotExistError
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
			ctx := test.Context(t)

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

			if testCase.EnableWebIdentityEnvVars || testCase.EnableWebIdentityConfig {
				file, err := os.CreateTemp("", "aws-sdk-go-base-web-identity-token-file")
				if err != nil {
					t.Fatalf("unexpected error creating temporary web identity token file: %s", err)
				}

				defer os.Remove(file.Name())

				err = os.WriteFile(file.Name(), []byte(servicemocks.MockWebIdentityToken), 0600)

				if err != nil {
					t.Fatalf("unexpected error writing web identity token file: %s", err)
				}

				if testCase.EnableWebIdentityEnvVars {
					os.Setenv("AWS_ROLE_ARN", servicemocks.MockStsAssumeRoleWithWebIdentityArn)
					os.Setenv("AWS_ROLE_SESSION_NAME", servicemocks.MockStsAssumeRoleWithWebIdentitySessionName)
					os.Setenv("AWS_WEB_IDENTITY_TOKEN_FILE", file.Name())
				} else if testCase.EnableWebIdentityConfig {
					testCase.Config.AssumeRoleWithWebIdentity = &awsbase.AssumeRoleWithWebIdentity{
						RoleARN:              servicemocks.MockStsAssumeRoleWithWebIdentityArn,
						SessionName:          servicemocks.MockStsAssumeRoleWithWebIdentitySessionName,
						WebIdentityTokenFile: file.Name(),
					}
				}
			}

			closeSts, mockStsSession, err := mockdata.GetMockedAwsApiSession("STS", testCase.MockStsEndpoints)
			defer closeSts()

			if err != nil {
				t.Fatalf("unexpected error creating mock STS server: %s", err)
			}

			if mockStsSession != nil && mockStsSession.Config != nil {
				testCase.Config.StsEndpoint = aws.StringValue(mockStsSession.Config.Endpoint)
			}

			if testCase.SharedConfigurationFile != "" {
				file, err := os.CreateTemp("", "aws-sdk-go-base-shared-configuration-file")

				if err != nil {
					t.Fatalf("unexpected error creating temporary shared configuration file: %s", err)
				}

				defer os.Remove(file.Name())

				err = os.WriteFile(file.Name(), []byte(testCase.SharedConfigurationFile), 0600)

				if err != nil {
					t.Fatalf("unexpected error writing shared configuration file: %s", err)
				}

				testCase.Config.SharedConfigFiles = []string{file.Name()}
			}

			if testCase.SharedCredentialsFile != "" {
				file, err := os.CreateTemp("", "aws-sdk-go-base-shared-credentials-file")

				if err != nil {
					t.Fatalf("unexpected error creating temporary shared credentials file: %s", err)
				}

				defer os.Remove(file.Name())

				err = os.WriteFile(file.Name(), []byte(testCase.SharedCredentialsFile), 0600)

				if err != nil {
					t.Fatalf("unexpected error writing shared credentials file: %s", err)
				}

				testCase.Config.SharedCredentialsFiles = []string{file.Name()}
			}

			for k, v := range testCase.EnvironmentVariables {
				os.Setenv(k, v)
			}

			ctx, awsConfig, err := awsbase.GetAwsConfig(ctx, testCase.Config)
			if err != nil {
				if testCase.ExpectedError == nil {
					t.Fatalf("expected no error from GetAwsConfig(), got '%[1]T' error: %[1]s", err)
				}

				if !testCase.ExpectedError(err) {
					t.Fatalf("unexpected GetAwsConfig() '%[1]T' error: %[1]s", err)
				}

				t.Logf("received expected error (awsbase.GetAwsConfig): %s", err)
				return
			}
			actualSession, err := GetSession(ctx, &awsConfig, testCase.Config)
			if err != nil {
				if testCase.ExpectedError == nil {
					t.Fatalf("expected no error from GetSession(), got '%[1]T' error: %[1]s", err)
				}

				if !testCase.ExpectedError(err) {
					t.Fatalf("unexpected GetSession() '%[1]T' error: %[1]s", err)
				}

				t.Logf("received expected error (GetSession): %s", err)
				return
			}

			if err == nil && testCase.ExpectedError != nil {
				t.Fatalf("expected error, got no error")
			}

			credentialsValue, err := actualSession.Config.Credentials.GetWithContext(ctx)

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
	test.TestUserAgentProducts(t, awsSdkGoUserAgent, testUserAgentProducts)
}

func testUserAgentProducts(t *testing.T, testCase test.UserAgentTestCase) {
	ctx := test.Context(t)

	ctx, awsConfig, err := awsbase.GetAwsConfig(ctx, testCase.Config)
	if err != nil {
		t.Fatalf("GetAwsConfig() returned error: %s", err)
	}
	actualSession, err := GetSession(ctx, &awsConfig, testCase.Config)
	if err != nil {
		t.Fatalf("error in GetSession() '%[1]T': %[1]s", err)
	}

	clientInfo := metadata.ClientInfo{
		Endpoint:    "http://endpoint",
		SigningName: "",
	}
	conn := client.New(*actualSession.Config, clientInfo, actualSession.Handlers)

	req := conn.NewRequest(&request.Operation{Name: "Operation"}, nil, nil)

	if testCase.Context != nil {
		ctx := useragent.Context(ctx, testCase.Context)
		req.SetContext(ctx)
	}

	if err := req.Build(); err != nil {
		t.Fatalf("expect no Request.Build() error, got %s", err)
	}

	if e, a := testCase.ExpectedUserAgent, req.HTTPRequest.Header.Get("User-Agent"); e != a {
		t.Errorf("expected User-Agent %q, got: %q", e, a)
	}
}

func awsSdkGoUserAgent() string {
	return fmt.Sprintf("%s/%s (%s; %s; %s)", aws.SDKName, aws.SDKVersion, runtime.Version(), runtime.GOOS, runtime.GOARCH)
}

func TestMaxAttempts(t *testing.T) {
	testCases := map[string]struct {
		Config                  *awsbase.Config
		EnvironmentVariables    map[string]string
		SharedConfigurationFile string
		ExpectedMaxAttempts     int
	}{
		"no configuration": {
			Config: &awsbase.Config{
				AccessKey: servicemocks.MockStaticAccessKey,
				SecretKey: servicemocks.MockStaticSecretKey,
			},
			ExpectedMaxAttempts: -1,
		},

		"config": {
			Config: &awsbase.Config{
				AccessKey:  servicemocks.MockStaticAccessKey,
				SecretKey:  servicemocks.MockStaticSecretKey,
				MaxRetries: 5,
			},
			ExpectedMaxAttempts: 5,
		},

		"AWS_MAX_ATTEMPTS": {
			Config: &awsbase.Config{
				AccessKey: servicemocks.MockStaticAccessKey,
				SecretKey: servicemocks.MockStaticSecretKey,
			},
			EnvironmentVariables: map[string]string{
				"AWS_MAX_ATTEMPTS": "5",
			},
			ExpectedMaxAttempts: 5,
		},

		"shared configuration file": {
			Config: &awsbase.Config{
				AccessKey: servicemocks.MockStaticAccessKey,
				SecretKey: servicemocks.MockStaticSecretKey,
			},
			SharedConfigurationFile: `
[default]
max_attempts = 5
`,
			ExpectedMaxAttempts: 5,
		},

		"config overrides AWS_MAX_ATTEMPTS": {
			Config: &awsbase.Config{
				AccessKey:  servicemocks.MockStaticAccessKey,
				SecretKey:  servicemocks.MockStaticSecretKey,
				MaxRetries: 10,
			},
			EnvironmentVariables: map[string]string{
				"AWS_MAX_ATTEMPTS": "5",
			},
			ExpectedMaxAttempts: 10,
		},

		"AWS_MAX_ATTEMPTS overrides shared configuration": {
			Config: &awsbase.Config{
				AccessKey: servicemocks.MockStaticAccessKey,
				SecretKey: servicemocks.MockStaticSecretKey,
			},
			EnvironmentVariables: map[string]string{
				"AWS_MAX_ATTEMPTS": "5",
			},
			SharedConfigurationFile: `
[default]
max_attempts = 10
`,
			ExpectedMaxAttempts: 5,
		},
	}

	for testName, testCase := range testCases {
		testCase := testCase

		t.Run(testName, func(t *testing.T) {
			ctx := test.Context(t)

			oldEnv := servicemocks.InitSessionTestEnv()
			defer servicemocks.PopEnv(oldEnv)

			for k, v := range testCase.EnvironmentVariables {
				os.Setenv(k, v)
			}

			if testCase.SharedConfigurationFile != "" {
				file, err := os.CreateTemp("", "aws-sdk-go-base-shared-configuration-file")

				if err != nil {
					t.Fatalf("unexpected error creating temporary shared configuration file: %s", err)
				}

				defer os.Remove(file.Name())

				err = os.WriteFile(file.Name(), []byte(testCase.SharedConfigurationFile), 0600)

				if err != nil {
					t.Fatalf("unexpected error writing shared configuration file: %s", err)
				}

				testCase.Config.SharedConfigFiles = []string{file.Name()}
			}

			testCase.Config.SkipCredsValidation = true

			ctx, awsConfig, err := awsbase.GetAwsConfig(ctx, testCase.Config)
			if err != nil {
				t.Fatalf("GetAwsConfig() returned error: %s", err)
			}
			actualSession, err := GetSession(ctx, &awsConfig, testCase.Config)
			if err != nil {
				t.Fatalf("error in GetSession() '%[1]T': %[1]s", err)
			}

			if a, e := *actualSession.Config.MaxRetries, testCase.ExpectedMaxAttempts; a != e {
				t.Errorf(`expected MaxAttempts "%d", got: "%d"`, e, a)
			}
		})
	}
}

func TestServiceEndpointTypes(t *testing.T) {
	testCases := map[string]struct {
		Config                       *awsbase.Config
		EnvironmentVariables         map[string]string
		SharedConfigurationFile      string
		ExpectedUseFIPSEndpoint      endpoints.FIPSEndpointState
		ExpectedUseDualStackEndpoint endpoints.DualStackEndpointState
	}{
		"normal endpoint": {
			Config: &awsbase.Config{
				AccessKey: servicemocks.MockStaticAccessKey,
				SecretKey: servicemocks.MockStaticSecretKey,
			},
			ExpectedUseFIPSEndpoint:      endpoints.FIPSEndpointStateUnset,
			ExpectedUseDualStackEndpoint: endpoints.DualStackEndpointStateUnset,
		},

		// FIPS Endpoint
		"FIPS endpoint config": {
			Config: &awsbase.Config{
				AccessKey:       servicemocks.MockStaticAccessKey,
				SecretKey:       servicemocks.MockStaticSecretKey,
				UseFIPSEndpoint: true,
			},
			ExpectedUseFIPSEndpoint: endpoints.FIPSEndpointStateEnabled,
		},
		"FIPS endpoint envvar": {
			Config: &awsbase.Config{
				AccessKey: servicemocks.MockStaticAccessKey,
				SecretKey: servicemocks.MockStaticSecretKey,
			},
			EnvironmentVariables: map[string]string{
				"AWS_USE_FIPS_ENDPOINT": "true",
			},
			ExpectedUseFIPSEndpoint: endpoints.FIPSEndpointStateEnabled,
		},
		"FIPS endpoint shared configuration file": {
			Config: &awsbase.Config{
				AccessKey: servicemocks.MockStaticAccessKey,
				SecretKey: servicemocks.MockStaticSecretKey,
			},
			SharedConfigurationFile: `
		[default]
		use_fips_endpoint = true
		`,
			ExpectedUseFIPSEndpoint: endpoints.FIPSEndpointStateEnabled,
		},

		// DualStack Endpoint
		"DualStack endpoint config": {
			Config: &awsbase.Config{
				AccessKey:            servicemocks.MockStaticAccessKey,
				SecretKey:            servicemocks.MockStaticSecretKey,
				UseDualStackEndpoint: true,
			},
			ExpectedUseDualStackEndpoint: endpoints.DualStackEndpointStateEnabled,
		},
		"DualStack endpoint envvar": {
			Config: &awsbase.Config{
				AccessKey: servicemocks.MockStaticAccessKey,
				SecretKey: servicemocks.MockStaticSecretKey,
			},
			EnvironmentVariables: map[string]string{
				"AWS_USE_DUALSTACK_ENDPOINT": "true",
			},
			ExpectedUseDualStackEndpoint: endpoints.DualStackEndpointStateEnabled,
		},
		"DualStack endpoint shared configuration file": {
			Config: &awsbase.Config{
				AccessKey: servicemocks.MockStaticAccessKey,
				SecretKey: servicemocks.MockStaticSecretKey,
			},
			SharedConfigurationFile: `
		[default]
		use_dualstack_endpoint = true
		`,
			ExpectedUseDualStackEndpoint: endpoints.DualStackEndpointStateEnabled,
		},

		// FIPS and DualStack Endpoint
		"Both endpoints config": {
			Config: &awsbase.Config{
				AccessKey:            servicemocks.MockStaticAccessKey,
				SecretKey:            servicemocks.MockStaticSecretKey,
				UseDualStackEndpoint: true,
				UseFIPSEndpoint:      true,
			},
			ExpectedUseDualStackEndpoint: endpoints.DualStackEndpointStateEnabled,
			ExpectedUseFIPSEndpoint:      endpoints.FIPSEndpointStateEnabled,
		},
		"Both endpoints FIPS config DualStack envvar": {
			Config: &awsbase.Config{
				AccessKey:       servicemocks.MockStaticAccessKey,
				SecretKey:       servicemocks.MockStaticSecretKey,
				UseFIPSEndpoint: true,
			},
			EnvironmentVariables: map[string]string{
				"AWS_USE_DUALSTACK_ENDPOINT": "true",
			},
			ExpectedUseDualStackEndpoint: endpoints.DualStackEndpointStateEnabled,
			ExpectedUseFIPSEndpoint:      endpoints.FIPSEndpointStateEnabled,
		},
		"Both endpoints FIPS shared configuration file DualStack config": {
			Config: &awsbase.Config{
				AccessKey:            servicemocks.MockStaticAccessKey,
				SecretKey:            servicemocks.MockStaticSecretKey,
				UseDualStackEndpoint: true,
			},
			SharedConfigurationFile: `
[default]
use_fips_endpoint = true
`,
			ExpectedUseDualStackEndpoint: endpoints.DualStackEndpointStateEnabled,
			ExpectedUseFIPSEndpoint:      endpoints.FIPSEndpointStateEnabled,
		},
	}

	for testName, testCase := range testCases {
		testCase := testCase

		t.Run(testName, func(t *testing.T) {
			ctx := test.Context(t)

			oldEnv := servicemocks.InitSessionTestEnv()
			defer servicemocks.PopEnv(oldEnv)

			for k, v := range testCase.EnvironmentVariables {
				os.Setenv(k, v)
			}

			closeSts, mockStsSession, err := mockdata.GetMockedAwsApiSession("STS", []*servicemocks.MockEndpoint{
				servicemocks.MockStsGetCallerIdentityValidEndpoint,
			})
			defer closeSts()

			if err != nil {
				t.Fatalf("unexpected error creating mock STS server: %s", err)
			}

			if mockStsSession != nil && mockStsSession.Config != nil {
				testCase.Config.StsEndpoint = aws.StringValue(mockStsSession.Config.Endpoint)
			}

			if testCase.SharedConfigurationFile != "" {
				file, err := os.CreateTemp("", "aws-sdk-go-base-shared-configuration-file")

				if err != nil {
					t.Fatalf("unexpected error creating temporary shared configuration file: %s", err)
				}

				defer os.Remove(file.Name())

				err = os.WriteFile(file.Name(), []byte(testCase.SharedConfigurationFile), 0600)

				if err != nil {
					t.Fatalf("unexpected error writing shared configuration file: %s", err)
				}

				testCase.Config.SharedConfigFiles = []string{file.Name()}
			}

			testCase.Config.SkipCredsValidation = true

			ctx, awsConfig, err := awsbase.GetAwsConfig(ctx, testCase.Config)
			if err != nil {
				t.Fatalf("GetAwsConfig() returned error: %s", err)
			}
			actualSession, err := GetSession(ctx, &awsConfig, testCase.Config)
			if err != nil {
				t.Fatalf("error in GetSession() '%[1]T': %[1]s", err)
			}

			if e, a := testCase.ExpectedUseFIPSEndpoint, actualSession.Config.UseFIPSEndpoint; e != a {
				t.Errorf("expected UseFIPSEndpoint %q, got: %q", FIPSEndpointStateString(e), FIPSEndpointStateString(a))
			}

			if e, a := testCase.ExpectedUseDualStackEndpoint, actualSession.Config.UseDualStackEndpoint; e != a {
				t.Errorf("expected UseDualStackEndpoint %q, got: %q", DualStackEndpointStateString(e), DualStackEndpointStateString(a))
			}
		})
	}
}

func FIPSEndpointStateString(state endpoints.FIPSEndpointState) string {
	switch state {
	case endpoints.FIPSEndpointStateUnset:
		return "FIPSEndpointStateUnset"
	case endpoints.FIPSEndpointStateEnabled:
		return "FIPSEndpointStateEnabled"
	case endpoints.FIPSEndpointStateDisabled:
		return "FIPSEndpointStateDisabled"
	}
	return fmt.Sprintf("unknown endpoints.FIPSEndpointState (%d)", state)
}

func DualStackEndpointStateString(state endpoints.DualStackEndpointState) string {
	switch state {
	case endpoints.DualStackEndpointStateUnset:
		return "DualStackEndpointStateUnset"
	case endpoints.DualStackEndpointStateEnabled:
		return "DualStackEndpointStateEnabled"
	case endpoints.DualStackEndpointStateDisabled:
		return "DualStackEndpointStateDisabled"
	}
	return fmt.Sprintf("unknown endpoints.FIPSEndpointStateUnset (%d)", state)
}

func TestCustomCABundle(t *testing.T) {
	testCases := map[string]struct {
		Config                              *awsbase.Config
		SetConfig                           bool
		SetEnvironmentVariable              bool
		SetSharedConfigurationFile          bool
		SetSharedConfigurationFileToInvalid bool
		ExpandEnvVars                       bool
		EnvironmentVariables                map[string]string
		ExpectTLSClientConfigRootCAsSet     bool
	}{
		"no configuration": {
			Config: &awsbase.Config{
				AccessKey: servicemocks.MockStaticAccessKey,
				SecretKey: servicemocks.MockStaticSecretKey,
			},
			ExpectTLSClientConfigRootCAsSet: false,
		},

		"config": {
			Config: &awsbase.Config{
				AccessKey: servicemocks.MockStaticAccessKey,
				SecretKey: servicemocks.MockStaticSecretKey,
			},
			SetConfig:                       true,
			ExpectTLSClientConfigRootCAsSet: true,
		},

		"expanded config": {
			Config: &awsbase.Config{
				AccessKey: servicemocks.MockStaticAccessKey,
				SecretKey: servicemocks.MockStaticSecretKey,
			},
			SetConfig:                       true,
			ExpandEnvVars:                   true,
			ExpectTLSClientConfigRootCAsSet: true,
		},

		"envvar": {
			Config: &awsbase.Config{
				AccessKey: servicemocks.MockStaticAccessKey,
				SecretKey: servicemocks.MockStaticSecretKey,
			},
			SetEnvironmentVariable:          true,
			ExpectTLSClientConfigRootCAsSet: true,
		},

		"shared configuration file": {
			Config: &awsbase.Config{
				AccessKey: servicemocks.MockStaticAccessKey,
				SecretKey: servicemocks.MockStaticSecretKey,
			},
			SetSharedConfigurationFile:      true,
			ExpectTLSClientConfigRootCAsSet: true,
		},

		"config overrides envvar": {
			Config: &awsbase.Config{
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
			Config: &awsbase.Config{
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
			ctx := test.Context(t)

			oldEnv := servicemocks.InitSessionTestEnv()
			defer servicemocks.PopEnv(oldEnv)

			for k, v := range testCase.EnvironmentVariables {
				os.Setenv(k, v)
			}

			tempdir, err := os.MkdirTemp("", "temp")
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
				file, err := os.CreateTemp("", "aws-sdk-go-base-shared-configuration-file")

				if err != nil {
					t.Fatalf("unexpected error creating temporary shared configuration file: %s", err)
				}

				defer os.Remove(file.Name())

				err = os.WriteFile(
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
				file, err := os.CreateTemp("", "aws-sdk-go-base-shared-configuration-file")

				if err != nil {
					t.Fatalf("unexpected error creating temporary shared configuration file: %s", err)
				}

				defer os.Remove(file.Name())

				err = os.WriteFile(
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

			ctx, awsConfig, err := awsbase.GetAwsConfig(ctx, testCase.Config)
			if err != nil {
				t.Fatalf("GetAwsConfig() returned error: %s", err)
			}
			actualSession, err := GetSession(ctx, &awsConfig, testCase.Config)
			if err != nil {
				t.Fatalf("error in GetSession() '%[1]T': %[1]s", err)
			}

			roundTripper := actualSession.Config.HTTPClient.Transport
			tr, ok := roundTripper.(*http.Transport)
			if !ok {
				t.Fatalf("Unexpected type for HTTP client transport: %T", roundTripper)
			}

			if a, e := tr.TLSClientConfig.RootCAs != nil, testCase.ExpectTLSClientConfigRootCAsSet; a != e {
				t.Errorf("expected(%t) CA Bundle, got: %t", e, a)
			}
		})
	}
}

func TestAssumeRole(t *testing.T) {
	testCases := map[string]struct {
		Config                   *awsbase.Config
		SharedConfigurationFile  string
		ExpectedCredentialsValue credentials.Value
		ExpectedError            func(err error) bool
		MockStsEndpoints         []*servicemocks.MockEndpoint
	}{
		"config": {
			Config: &awsbase.Config{
				AssumeRole: &awsbase.AssumeRole{
					RoleARN:     servicemocks.MockStsAssumeRoleArn,
					SessionName: servicemocks.MockStsAssumeRoleSessionName,
				},
				AccessKey: servicemocks.MockStaticAccessKey,
				SecretKey: servicemocks.MockStaticSecretKey,
			},
			ExpectedCredentialsValue: mockdata.MockStsAssumeRoleCredentials,
			MockStsEndpoints: []*servicemocks.MockEndpoint{
				servicemocks.MockStsAssumeRoleValidEndpoint,
			},
		},

		"shared configuration file": {
			Config: &awsbase.Config{},
			SharedConfigurationFile: fmt.Sprintf(`
[default]
role_arn = %[1]s
role_session_name = %[2]s
source_profile = SharedConfigurationSourceProfile

[profile SharedConfigurationSourceProfile]
aws_access_key_id = SharedConfigurationSourceAccessKey
aws_secret_access_key = SharedConfigurationSourceSecretKey
`, servicemocks.MockStsAssumeRoleArn, servicemocks.MockStsAssumeRoleSessionName),
			ExpectedCredentialsValue: mockdata.MockStsAssumeRoleCredentials,
			MockStsEndpoints: []*servicemocks.MockEndpoint{
				servicemocks.MockStsAssumeRoleValidEndpoint,
			},
		},

		"config overrides shared configuration": {
			Config: &awsbase.Config{
				AssumeRole: &awsbase.AssumeRole{
					RoleARN:     servicemocks.MockStsAssumeRoleArn,
					SessionName: servicemocks.MockStsAssumeRoleSessionName,
				},
				AccessKey: servicemocks.MockStaticAccessKey,
				SecretKey: servicemocks.MockStaticSecretKey,
			},
			SharedConfigurationFile: fmt.Sprintf(`
[default]
role_arn = %[1]s
role_session_name = %[2]s
source_profile = SharedConfigurationSourceProfile

[profile SharedConfigurationSourceProfile]
aws_access_key_id = SharedConfigurationSourceAccessKey
aws_secret_access_key = SharedConfigurationSourceSecretKey
`, servicemocks.MockStsAssumeRoleArn, servicemocks.MockStsAssumeRoleSessionName),
			ExpectedCredentialsValue: mockdata.MockStsAssumeRoleCredentials,
			MockStsEndpoints: []*servicemocks.MockEndpoint{
				servicemocks.MockStsAssumeRoleValidEndpoint,
			},
		},

		"with duration": {
			Config: &awsbase.Config{
				AssumeRole: &awsbase.AssumeRole{
					RoleARN:     servicemocks.MockStsAssumeRoleArn,
					SessionName: servicemocks.MockStsAssumeRoleSessionName,
					Duration:    1 * time.Hour,
				},
				AccessKey: servicemocks.MockStaticAccessKey,
				SecretKey: servicemocks.MockStaticSecretKey,
			},
			ExpectedCredentialsValue: mockdata.MockStsAssumeRoleCredentials,
			MockStsEndpoints: []*servicemocks.MockEndpoint{
				servicemocks.MockStsAssumeRoleValidEndpointWithOptions(map[string]string{"DurationSeconds": "3600"}),
			},
		},

		"with policy": {
			Config: &awsbase.Config{
				AssumeRole: &awsbase.AssumeRole{
					RoleARN:     servicemocks.MockStsAssumeRoleArn,
					SessionName: servicemocks.MockStsAssumeRoleSessionName,
					Policy:      "{}",
				},
				AccessKey: servicemocks.MockStaticAccessKey,
				SecretKey: servicemocks.MockStaticSecretKey,
			},
			ExpectedCredentialsValue: mockdata.MockStsAssumeRoleCredentials,
			MockStsEndpoints: []*servicemocks.MockEndpoint{
				servicemocks.MockStsAssumeRoleValidEndpointWithOptions(map[string]string{"Policy": "{}"}),
			},
		},

		"invalid empty config": {
			Config: &awsbase.Config{
				AssumeRole: &awsbase.AssumeRole{},
				AccessKey:  servicemocks.MockStaticAccessKey,
				SecretKey:  servicemocks.MockStaticSecretKey,
			},
			ExpectedCredentialsValue: mockdata.MockStsAssumeRoleCredentials,
			ExpectedError: func(err error) bool {
				return strings.Contains(err.Error(), "role ARN not set")
			},
		},
	}

	for testName, testCase := range testCases {
		testCase := testCase

		t.Run(testName, func(t *testing.T) {
			ctx := test.Context(t)

			oldEnv := servicemocks.InitSessionTestEnv()
			defer servicemocks.PopEnv(oldEnv)

			closeSts, mockStsSession, err := mockdata.GetMockedAwsApiSession("STS", testCase.MockStsEndpoints)
			defer closeSts()

			if err != nil {
				t.Fatalf("unexpected error creating mock STS server: %s", err)
			}

			if mockStsSession != nil && mockStsSession.Config != nil {
				testCase.Config.StsEndpoint = aws.StringValue(mockStsSession.Config.Endpoint)
			}

			tempdir, err := os.MkdirTemp("", "temp")
			if err != nil {
				t.Fatalf("error creating temp dir: %s", err)
			}
			defer os.Remove(tempdir)
			os.Setenv("TMPDIR", tempdir)

			if testCase.SharedConfigurationFile != "" {
				file, err := os.CreateTemp("", "aws-sdk-go-base-shared-configuration-file")

				if err != nil {
					t.Fatalf("unexpected error creating temporary shared configuration file: %s", err)
				}

				defer os.Remove(file.Name())

				err = os.WriteFile(file.Name(), []byte(testCase.SharedConfigurationFile), 0600)

				if err != nil {
					t.Fatalf("unexpected error writing shared configuration file: %s", err)
				}

				testCase.Config.SharedConfigFiles = []string{file.Name()}
			}

			testCase.Config.SkipCredsValidation = true

			ctx, awsConfig, err := awsbase.GetAwsConfig(ctx, testCase.Config)
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
			actualSession, err := GetSession(ctx, &awsConfig, testCase.Config)
			if err != nil {
				if testCase.ExpectedError == nil {
					t.Fatalf("expected no error, got '%[1]T' error: %[1]s", err)
				}

				if !testCase.ExpectedError(err) {
					t.Fatalf("unexpected GetSession() '%[1]T' error: %[1]s", err)
				}

				t.Logf("received expected '%[1]T' error: %[1]s", err)
				return
			}

			credentialsValue, err := actualSession.Config.Credentials.GetWithContext(ctx)

			if err != nil {
				t.Fatalf("unexpected credentials Get() error: %s", err)
			}

			if diff := cmp.Diff(credentialsValue, testCase.ExpectedCredentialsValue, cmpopts.IgnoreFields(credentials.Value{}, "ProviderName")); diff != "" {
				t.Fatalf("unexpected credentials: (- got, + expected)\n%s", diff)
			}
		})
	}
}

func TestAssumeRoleWithWebIdentity(t *testing.T) {
	testCases := map[string]struct {
		Config                     *awsbase.Config
		SetConfig                  bool
		ExpandEnvVars              bool
		EnvironmentVariables       map[string]string
		SetEnvironmentVariable     bool
		SharedConfigurationFile    string
		SetSharedConfigurationFile bool
		ExpectedCredentialsValue   credentials.Value
		ExpectedError              func(err error) bool
		MockStsEndpoints           []*servicemocks.MockEndpoint
	}{
		"config with inline token": {
			Config: &awsbase.Config{
				AssumeRoleWithWebIdentity: &awsbase.AssumeRoleWithWebIdentity{
					RoleARN:          servicemocks.MockStsAssumeRoleWithWebIdentityArn,
					SessionName:      servicemocks.MockStsAssumeRoleWithWebIdentitySessionName,
					WebIdentityToken: servicemocks.MockWebIdentityToken,
				},
			},
			ExpectedCredentialsValue: mockdata.MockStsAssumeRoleWithWebIdentityCredentials,
			MockStsEndpoints: []*servicemocks.MockEndpoint{
				servicemocks.MockStsAssumeRoleWithWebIdentityValidEndpoint,
			},
		},

		"config with token file": {
			Config: &awsbase.Config{
				AssumeRoleWithWebIdentity: &awsbase.AssumeRoleWithWebIdentity{
					RoleARN:     servicemocks.MockStsAssumeRoleWithWebIdentityArn,
					SessionName: servicemocks.MockStsAssumeRoleWithWebIdentitySessionName,
				},
			},
			SetConfig:                true,
			ExpectedCredentialsValue: mockdata.MockStsAssumeRoleWithWebIdentityCredentials,
			MockStsEndpoints: []*servicemocks.MockEndpoint{
				servicemocks.MockStsAssumeRoleWithWebIdentityValidEndpoint,
			},
		},

		"config with expanded path": {
			Config: &awsbase.Config{
				AssumeRoleWithWebIdentity: &awsbase.AssumeRoleWithWebIdentity{
					RoleARN:     servicemocks.MockStsAssumeRoleWithWebIdentityArn,
					SessionName: servicemocks.MockStsAssumeRoleWithWebIdentitySessionName,
				},
			},
			SetConfig:                true,
			ExpandEnvVars:            true,
			ExpectedCredentialsValue: mockdata.MockStsAssumeRoleWithWebIdentityCredentials,
			MockStsEndpoints: []*servicemocks.MockEndpoint{
				servicemocks.MockStsAssumeRoleWithWebIdentityValidEndpoint,
			},
		},

		"envvar": {
			Config: &awsbase.Config{},
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
			Config: &awsbase.Config{},
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

		"config overrides envvar": {
			Config: &awsbase.Config{
				AssumeRoleWithWebIdentity: &awsbase.AssumeRoleWithWebIdentity{
					RoleARN:          servicemocks.MockStsAssumeRoleWithWebIdentityArn,
					SessionName:      servicemocks.MockStsAssumeRoleWithWebIdentitySessionName,
					WebIdentityToken: servicemocks.MockWebIdentityToken,
				},
			},
			EnvironmentVariables: map[string]string{
				"AWS_ROLE_ARN":                servicemocks.MockStsAssumeRoleWithWebIdentityAlternateArn,
				"AWS_ROLE_SESSION_NAME":       servicemocks.MockStsAssumeRoleWithWebIdentityAlternateSessionName,
				"AWS_WEB_IDENTITY_TOKEN_FILE": "no-such-file",
			},
			ExpectedCredentialsValue: mockdata.MockStsAssumeRoleWithWebIdentityCredentials,
			MockStsEndpoints: []*servicemocks.MockEndpoint{
				servicemocks.MockStsAssumeRoleWithWebIdentityValidEndpoint,
			},
		},

		// "config with file envvar": {
		// 	Config: &awsbase.Config{
		// 		AssumeRoleWithWebIdentity: &awsbase.AssumeRoleWithWebIdentity{
		// 			RoleARN:     servicemocks.MockStsAssumeRoleWithWebIdentityArn,
		// 			SessionName: servicemocks.MockStsAssumeRoleWithWebIdentitySessionName,
		// 		},
		// 	},
		// 	SetEnvironmentVariable:   true,
		// 	ExpectedCredentialsValue: mockdata.MockStsAssumeRoleWithWebIdentityCredentials,
		// 	MockStsEndpoints: []*servicemocks.MockEndpoint{
		// 		servicemocks.MockStsAssumeRoleWithWebIdentityValidEndpoint,
		// 	},
		// },

		"envvar overrides shared configuration": {
			Config: &awsbase.Config{},
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
`, servicemocks.MockStsAssumeRoleWithWebIdentityAlternateArn, servicemocks.MockStsAssumeRoleWithWebIdentityAlternateSessionName),
			ExpectedCredentialsValue: mockdata.MockStsAssumeRoleWithWebIdentityCredentials,
			MockStsEndpoints: []*servicemocks.MockEndpoint{
				servicemocks.MockStsAssumeRoleWithWebIdentityValidEndpoint,
			},
		},

		"config overrides shared configuration": {
			Config: &awsbase.Config{
				AssumeRoleWithWebIdentity: &awsbase.AssumeRoleWithWebIdentity{
					RoleARN:          servicemocks.MockStsAssumeRoleWithWebIdentityArn,
					SessionName:      servicemocks.MockStsAssumeRoleWithWebIdentitySessionName,
					WebIdentityToken: servicemocks.MockWebIdentityToken,
				},
			},
			SharedConfigurationFile: fmt.Sprintf(`
[default]
role_arn = %[1]s
role_session_name = %[2]s
web_identity_token_file = no-such-file
`, servicemocks.MockStsAssumeRoleWithWebIdentityAlternateArn, servicemocks.MockStsAssumeRoleWithWebIdentityAlternateSessionName),
			ExpectedCredentialsValue: mockdata.MockStsAssumeRoleWithWebIdentityCredentials,
			MockStsEndpoints: []*servicemocks.MockEndpoint{
				servicemocks.MockStsAssumeRoleWithWebIdentityValidEndpoint,
			},
		},

		"with duration": {
			Config: &awsbase.Config{
				AssumeRoleWithWebIdentity: &awsbase.AssumeRoleWithWebIdentity{
					RoleARN:          servicemocks.MockStsAssumeRoleWithWebIdentityArn,
					SessionName:      servicemocks.MockStsAssumeRoleWithWebIdentitySessionName,
					WebIdentityToken: servicemocks.MockWebIdentityToken,
					Duration:         1 * time.Hour,
				},
			},
			ExpectedCredentialsValue: mockdata.MockStsAssumeRoleWithWebIdentityCredentials,
			MockStsEndpoints: []*servicemocks.MockEndpoint{
				servicemocks.MockStsAssumeRoleWithWebIdentityValidWithOptions(map[string]string{"DurationSeconds": "3600"}),
			},
		},

		"with policy": {
			Config: &awsbase.Config{
				AssumeRoleWithWebIdentity: &awsbase.AssumeRoleWithWebIdentity{
					RoleARN:          servicemocks.MockStsAssumeRoleWithWebIdentityArn,
					SessionName:      servicemocks.MockStsAssumeRoleWithWebIdentitySessionName,
					WebIdentityToken: servicemocks.MockWebIdentityToken,
					Policy:           "{}",
				},
			},
			ExpectedCredentialsValue: mockdata.MockStsAssumeRoleWithWebIdentityCredentials,
			MockStsEndpoints: []*servicemocks.MockEndpoint{
				servicemocks.MockStsAssumeRoleWithWebIdentityValidWithOptions(map[string]string{"Policy": "{}"}),
			},
		},

		"invalid empty config": {
			Config: &awsbase.Config{
				AssumeRoleWithWebIdentity: &awsbase.AssumeRoleWithWebIdentity{},
			},
			ExpectedCredentialsValue: mockdata.MockStsAssumeRoleWithWebIdentityCredentials,
			ExpectedError: func(err error) bool {
				return strings.Contains(err.Error(), "role ARN not set")
			},
		},

		"invalid no token": {
			Config: &awsbase.Config{
				AssumeRoleWithWebIdentity: &awsbase.AssumeRoleWithWebIdentity{
					RoleARN: servicemocks.MockStsAssumeRoleWithWebIdentityArn,
				},
			},
			ExpectedCredentialsValue: mockdata.MockStsAssumeRoleWithWebIdentityCredentials,
			ExpectedError: func(err error) bool {
				return strings.Contains(err.Error(), "one of WebIdentityToken, WebIdentityTokenFile must be set")
			},
		},
	}

	for testName, testCase := range testCases {
		testCase := testCase

		t.Run(testName, func(t *testing.T) {
			ctx := test.Context(t)

			oldEnv := servicemocks.InitSessionTestEnv()
			defer servicemocks.PopEnv(oldEnv)

			for k, v := range testCase.EnvironmentVariables {
				os.Setenv(k, v)
			}

			closeSts, mockStsSession, err := mockdata.GetMockedAwsApiSession("STS", testCase.MockStsEndpoints)
			defer closeSts()

			if err != nil {
				t.Fatalf("unexpected error creating mock STS server: %s", err)
			}

			if mockStsSession != nil && mockStsSession.Config != nil {
				testCase.Config.StsEndpoint = aws.StringValue(mockStsSession.Config.Endpoint)
			}

			tempdir, err := os.MkdirTemp("", "temp")
			if err != nil {
				t.Fatalf("error creating temp dir: %s", err)
			}
			defer os.Remove(tempdir)
			os.Setenv("TMPDIR", tempdir)

			tokenFile, err := os.CreateTemp("", "aws-sdk-go-base-web-identity-token-file")
			if err != nil {
				t.Fatalf("unexpected error creating temporary web identity token file: %s", err)
			}
			tokenFileName := tokenFile.Name()

			defer os.Remove(tokenFileName)

			err = os.WriteFile(tokenFileName, []byte(servicemocks.MockWebIdentityToken), 0600)

			if err != nil {
				t.Fatalf("unexpected error writing web identity token file: %s", err)
			}

			if testCase.ExpandEnvVars {
				tmpdir := os.Getenv("TMPDIR")
				rel, err := filepath.Rel(tmpdir, tokenFileName)
				if err != nil {
					t.Fatalf("error making path relative: %s", err)
				}
				t.Logf("relative: %s", rel)
				tokenFileName = filepath.Join("$TMPDIR", rel)
				t.Logf("env tempfile: %s", tokenFileName)
			}

			if testCase.SetConfig {
				testCase.Config.AssumeRoleWithWebIdentity.WebIdentityTokenFile = tokenFileName
			}

			if testCase.SetEnvironmentVariable {
				os.Setenv("AWS_WEB_IDENTITY_TOKEN_FILE", tokenFileName)
			}

			if testCase.SharedConfigurationFile != "" {
				file, err := os.CreateTemp("", "aws-sdk-go-base-shared-configuration-file")

				if err != nil {
					t.Fatalf("unexpected error creating temporary shared configuration file: %s", err)
				}

				defer os.Remove(file.Name())

				if testCase.SetSharedConfigurationFile {
					testCase.SharedConfigurationFile += fmt.Sprintf("web_identity_token_file = %s\n", tokenFileName)
				}

				err = os.WriteFile(file.Name(), []byte(testCase.SharedConfigurationFile), 0600)

				if err != nil {
					t.Fatalf("unexpected error writing shared configuration file: %s", err)
				}

				testCase.Config.SharedConfigFiles = []string{file.Name()}
			}

			testCase.Config.SkipCredsValidation = true

			ctx, awsConfig, err := awsbase.GetAwsConfig(ctx, testCase.Config)
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
			actualSession, err := GetSession(ctx, &awsConfig, testCase.Config)
			if err != nil {
				if testCase.ExpectedError == nil {
					t.Fatalf("expected no error, got '%[1]T' error: %[1]s", err)
				}

				if !testCase.ExpectedError(err) {
					t.Fatalf("unexpected GetSession() '%[1]T' error: %[1]s", err)
				}

				t.Logf("received expected '%[1]T' error: %[1]s", err)
				return
			}

			credentialsValue, err := actualSession.Config.Credentials.GetWithContext(ctx)

			if err != nil {
				t.Fatalf("unexpected credentials Get() error: %s", err)
			}

			if diff := cmp.Diff(credentialsValue, testCase.ExpectedCredentialsValue, cmpopts.IgnoreFields(credentials.Value{}, "ProviderName")); diff != "" {
				t.Fatalf("unexpected credentials: (- got, + expected)\n%s", diff)
			}
		})
	}
}

func TestSessionRetryHandlers(t *testing.T) {
	const maxRetries = 25

	testcases := []struct {
		Description              string
		RetryCount               int
		Error                    error
		ExpectedRetryableValue   bool
		ExpectRetryToBeAttempted bool
	}{
		{
			Description:              "ExpiredToken error no retries",
			RetryCount:               maxRetries,
			Error:                    awserr.New("ExpiredToken", "The security token included in the request is expired", nil),
			ExpectedRetryableValue:   false,
			ExpectRetryToBeAttempted: false,
		},
		{
			Description:              "ExpiredTokenException error no retries",
			RetryCount:               maxRetries,
			Error:                    awserr.New("ExpiredTokenException", "The security token included in the request is expired", nil),
			ExpectedRetryableValue:   false,
			ExpectRetryToBeAttempted: false,
		},
		{
			Description:              "RequestExpired error no retries",
			RetryCount:               maxRetries,
			Error:                    awserr.New("RequestExpired", "The security token included in the request is expired", nil),
			ExpectedRetryableValue:   false,
			ExpectRetryToBeAttempted: false,
		},
	}
	for _, testcase := range testcases {
		testcase := testcase

		t.Run(testcase.Description, func(t *testing.T) {
			ctx := test.Context(t)

			oldEnv := servicemocks.InitSessionTestEnv()
			defer servicemocks.PopEnv(oldEnv)

			config := &awsbase.Config{
				AccessKey:           servicemocks.MockStaticAccessKey,
				MaxRetries:          maxRetries,
				SecretKey:           servicemocks.MockStaticSecretKey,
				SkipCredsValidation: true,
			}
			ctx, awsConfig, err := awsbase.GetAwsConfig(ctx, config)
			if err != nil {
				t.Fatalf("unexpected error from GetAwsConfig(): %s", err)
			}
			session, err := GetSession(ctx, &awsConfig, config)
			if err != nil {
				t.Fatalf("unexpected error from GetSession(): %s", err)
			}

			iamconn := iam.New(session)

			request, _ := iamconn.GetUserRequest(&iam.GetUserInput{})
			request.RetryCount = testcase.RetryCount
			request.Error = testcase.Error

			// Prevent the retryer from using the default retry delay
			retryer := request.Retryer.(client.DefaultRetryer)
			retryer.MinRetryDelay = 1 * time.Microsecond
			retryer.MaxRetryDelay = 1 * time.Microsecond
			request.Retryer = retryer

			request.Handlers.Retry.Run(request)
			request.Handlers.AfterRetry.Run(request)

			if request.Retryable == nil {
				t.Fatal("retryable is nil")
			}
			if actual, expected := aws.BoolValue(request.Retryable), testcase.ExpectedRetryableValue; actual != expected {
				t.Errorf("expected Retryable to be %t, got %t", expected, actual)
			}

			expectedRetryCount := testcase.RetryCount
			if testcase.ExpectRetryToBeAttempted {
				expectedRetryCount++
			}
			if actual, expected := request.RetryCount, expectedRetryCount; actual != expected {
				t.Errorf("expected RetryCount to be %d, got %d", expected, actual)
			}
		})
	}
}

func TestLogger(t *testing.T) {
	var buf bytes.Buffer
	ctx := tflogtest.RootLogger(context.Background(), &buf)

	oldEnv := servicemocks.InitSessionTestEnv()
	defer servicemocks.PopEnv(oldEnv)

	config := &awsbase.Config{
		AccessKey: servicemocks.MockStaticAccessKey,
		Region:    "us-east-1",
		SecretKey: servicemocks.MockStaticSecretKey,
	}

	// config.SkipCredsValidation = true
	ts := servicemocks.MockAwsApiServer("STS", []*servicemocks.MockEndpoint{
		servicemocks.MockStsGetCallerIdentityValidEndpoint,
	})
	defer ts.Close()
	config.StsEndpoint = ts.URL

	expectedName := fmt.Sprintf("provider.%s", loggerName)

	ctx, awsConfig, err := awsbase.GetAwsConfig(ctx, config)
	if err != nil {
		t.Fatalf("GetAwsConfig: unexpected '%[1]T': %[1]s", err)
	}

	_, err = tflogtest.MultilineJSONDecode(&buf)
	if err != nil {
		t.Fatalf("GetAwsConfig: decoding log lines: %s", err)
	}

	// Ignore log lines from GetAwsConfig()

	_, err = GetSession(ctx, &awsConfig, config)
	if err != nil {
		t.Fatalf("GetSession: unexpected '%[1]T': %[1]s", err)
	}

	lines, err := tflogtest.MultilineJSONDecode(&buf)
	if err != nil {
		t.Fatalf("GetSession: decoding log lines: %s", err)
	}

	for i, line := range lines {
		if a, e := line["@module"], expectedName; a != e {
			t.Errorf("GetSession: line %d: expected module %q, got %q", i+1, e, a)
		}
	}
}
