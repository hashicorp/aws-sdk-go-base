// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package awsv1shim

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	awsv2 "github.com/aws/aws-sdk-go-v2/aws" // nosemgrep: no-sdkv2-imports-in-awsv1shim
	retryv2 "github.com/aws/aws-sdk-go-v2/aws/retry"
	configv2 "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/feature/ec2/imds" // nosemgrep: no-sdkv2-imports-in-awsv1shim
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
	"github.com/hashicorp/aws-sdk-go-base/v2/diag"
	"github.com/hashicorp/aws-sdk-go-base/v2/internal/constants"
	"github.com/hashicorp/aws-sdk-go-base/v2/internal/test"
	"github.com/hashicorp/aws-sdk-go-base/v2/logging"
	"github.com/hashicorp/aws-sdk-go-base/v2/servicemocks"
	"github.com/hashicorp/aws-sdk-go-base/v2/useragent"
	"github.com/hashicorp/terraform-plugin-log/tflogtest"
)

func TestGetSessionOptions(t *testing.T) {
	servicemocks.InitSessionTestEnv(t)

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

			ctx, awsConfig, diags := awsbase.GetAwsConfig(ctx, tc.config)
			if diags.HasError() {
				t.Fatalf("GetAwsConfig() resulted in an error %v", diags)
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
		ValidateDiags              test.DiagsValidator
		MockStsEndpoints           []*servicemocks.MockEndpoint
		SharedConfigurationFile    string
		SharedCredentialsFile      string
	}{
		{
			Config:        &awsbase.Config{},
			Description:   "no configuration or credentials",
			ValidateDiags: test.ExpectDiagValidator("NoValidCredentialSourcesError", awsbase.IsNoValidCredentialSourcesError),
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
			Description:    "assume role error",
			ValidateDiags:  test.ExpectDiagValidator("CannotAssumeRoleError", awsbase.IsCannotAssumeRoleError),
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
			ValidateDiags: test.ExpectErrDiagValidator("ExpiredToken", func(err error) bool {
				return strings.Contains(err.Error(), "ExpiredToken")
			}),
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
			ValidateDiags: test.ExpectErrDiagValidator("ExpiredToken", func(err error) bool {
				return strings.Contains(err.Error(), "ExpiredToken")
			}),
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
			ValidateDiags: test.ExpectErrDiagValidator("ExpiredTokenException", func(err error) bool {
				return strings.Contains(err.Error(), "ExpiredTokenException")
			}),
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
			ValidateDiags: test.ExpectErrDiagValidator("ExpiredTokenException", func(err error) bool {
				return strings.Contains(err.Error(), "ExpiredTokenException")
			}),
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
			ValidateDiags: test.ExpectErrDiagValidator("RequestExpired", func(err error) bool {
				return strings.Contains(err.Error(), "RequestExpired")
			}),
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
			ValidateDiags: test.ExpectErrDiagValidator("RequestExpired", func(err error) bool {
				return strings.Contains(err.Error(), "RequestExpired")
			}),
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
			Description:    "skip EC2 Metadata API check",
			ValidateDiags:  test.ExpectDiagValidator("NoValidCredentialSourcesError", awsbase.IsNoValidCredentialSourcesError),
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
			ValidateDiags: test.ExpectErrDiagValidator("SharedConfigProfileNotExistError", func(err error) bool {
				var e configv2.SharedConfigProfileNotExistError
				return errors.As(err, &e)
			}),
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
			ValidateDiags: test.ExpectErrDiagValidator("SharedConfigProfileNotExistError", func(err error) bool {
				var e configv2.SharedConfigProfileNotExistError
				return errors.As(err, &e)
			}),
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
			ValidateDiags: test.ExpectErrDiagValidator("SharedConfigProfileNotExistError", func(err error) bool {
				var e configv2.SharedConfigProfileNotExistError
				return errors.As(err, &e)
			}),
			SharedCredentialsFile: `
[some-profile]
aws_access_key_id = DefaultSharedCredentialsAccessKey
aws_secret_access_key = DefaultSharedCredentialsSecretKey
`,
		},
	}

	for _, testCase := range testCases {
		testCase := testCase

		if testCase.ValidateDiags == nil {
			testCase.ValidateDiags = test.ExpectNoDiags
		}

		t.Run(testCase.Description, func(t *testing.T) {
			ctx := test.Context(t)

			servicemocks.InitSessionTestEnv(t)

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
					t.Setenv("AWS_ROLE_ARN", servicemocks.MockStsAssumeRoleWithWebIdentityArn)
					t.Setenv("AWS_ROLE_SESSION_NAME", servicemocks.MockStsAssumeRoleWithWebIdentitySessionName)
					t.Setenv("AWS_WEB_IDENTITY_TOKEN_FILE", file.Name())
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
				t.Setenv(k, v)
			}

			ctx, awsConfig, diags := awsbase.GetAwsConfig(ctx, testCase.Config)

			testCase.ValidateDiags(t, diags)
			if diags.HasError() {
				return
			}

			actualSession, ds := GetSession(ctx, &awsConfig, testCase.Config)

			diags = diags.Append(ds...)

			if diags.HasError() {
				t.Fatalf("expected no errors from GetSession(), got : %v", diags)
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

	ctx, awsConfig, diags := awsbase.GetAwsConfig(ctx, testCase.Config)
	if diags.HasError() {
		t.Fatalf("error in GetAwsConfig(): %v", diags)
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
			ExpectedMaxAttempts: retryv2.DefaultMaxAttempts,
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

			servicemocks.InitSessionTestEnv(t)

			for k, v := range testCase.EnvironmentVariables {
				t.Setenv(k, v)
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

			ctx, awsConfig, diags := awsbase.GetAwsConfig(ctx, testCase.Config)
			if diags.HasError() {
				t.Fatalf("error in GetAwsConfig(): %v", diags)
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

func TestRetryMode(t *testing.T) {
	testCases := map[string]struct {
		Config                  *awsbase.Config
		EnvironmentVariables    map[string]string
		SharedConfigurationFile string
		ExpectedRetryMode       awsv2.RetryMode
	}{
		"no configuration": {
			Config: &awsbase.Config{
				AccessKey: servicemocks.MockStaticAccessKey,
				SecretKey: servicemocks.MockStaticSecretKey,
			},
			ExpectedRetryMode: "",
		},

		"config": {
			Config: &awsbase.Config{
				AccessKey: servicemocks.MockStaticAccessKey,
				SecretKey: servicemocks.MockStaticSecretKey,
				RetryMode: awsv2.RetryModeStandard,
			},
			ExpectedRetryMode: awsv2.RetryModeStandard,
		},

		"AWS_RETRY_MODE": {
			Config: &awsbase.Config{
				AccessKey: servicemocks.MockStaticAccessKey,
				SecretKey: servicemocks.MockStaticSecretKey,
			},
			EnvironmentVariables: map[string]string{
				"AWS_RETRY_MODE": "adaptive",
			},
			ExpectedRetryMode: awsv2.RetryModeAdaptive,
		},

		"shared configuration file": {
			Config: &awsbase.Config{
				AccessKey: servicemocks.MockStaticAccessKey,
				SecretKey: servicemocks.MockStaticSecretKey,
			},
			SharedConfigurationFile: `
		[default]
		retry_mode = standard
		`,
			ExpectedRetryMode: awsv2.RetryModeStandard,
		},

		"config overrides AWS_RETRY_MODE": {
			Config: &awsbase.Config{
				AccessKey: servicemocks.MockStaticAccessKey,
				SecretKey: servicemocks.MockStaticSecretKey,
				RetryMode: awsv2.RetryModeStandard,
			},
			EnvironmentVariables: map[string]string{
				"AWS_RETRY_MODE": "adaptive",
			},
			ExpectedRetryMode: awsv2.RetryModeStandard,
		},

		"AWS_RETRY_MODE overrides shared configuration": {
			Config: &awsbase.Config{
				AccessKey: servicemocks.MockStaticAccessKey,
				SecretKey: servicemocks.MockStaticSecretKey,
			},
			EnvironmentVariables: map[string]string{
				"AWS_RETRY_MODE": "standard",
			},
			SharedConfigurationFile: `
		[default]
		retry_mode = adaptive
		`,
			ExpectedRetryMode: awsv2.RetryModeStandard,
		},
	}

	for testName, testCase := range testCases {
		testCase := testCase

		t.Run(testName, func(t *testing.T) {
			servicemocks.InitSessionTestEnv(t)

			for k, v := range testCase.EnvironmentVariables {
				t.Setenv(k, v)
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

			_, awsConfig, err := awsbase.GetAwsConfig(context.Background(), testCase.Config)
			if err != nil {
				t.Fatalf("error in GetAwsConfig() '%[1]T': %[1]s", err)
			}

			retryMode := awsConfig.RetryMode
			if a, e := retryMode, testCase.ExpectedRetryMode; a != e {
				t.Errorf(`expected RetryMode "%s", got: "%s"`, e.String(), a.String())
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

			servicemocks.InitSessionTestEnv(t)

			for k, v := range testCase.EnvironmentVariables {
				t.Setenv(k, v)
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

			ctx, awsConfig, diags := awsbase.GetAwsConfig(ctx, testCase.Config)
			if diags.HasError() {
				t.Fatalf("error in GetAwsConfig(): %v", diags)
			}

			actualSession, ds := GetSession(ctx, &awsConfig, testCase.Config)
			diags = diags.Append(ds...)
			if diags.HasError() {
				t.Fatalf("expected no errors from GetSession(), got : %v", diags)
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

			servicemocks.InitSessionTestEnv(t)

			for k, v := range testCase.EnvironmentVariables {
				t.Setenv(k, v)
			}

			tempdir, err := os.MkdirTemp("", "temp")
			if err != nil {
				t.Fatalf("error creating temp dir: %s", err)
			}
			defer os.Remove(tempdir)
			t.Setenv("TMPDIR", tempdir)

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
				t.Setenv("AWS_CA_BUNDLE", pemFile)
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

			ctx, awsConfig, diags := awsbase.GetAwsConfig(ctx, testCase.Config)
			if diags.HasError() {
				t.Fatalf("error in GetAwsConfig(): %v", diags)
			}

			actualSession, ds := GetSession(ctx, &awsConfig, testCase.Config)

			diags = diags.Append(ds...)

			if diags.HasError() {
				t.Fatalf("expected no errors from GetSession(), got : %v", diags)
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
		ValidateDiags            test.DiagsValidator
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
			ValidateDiags: test.ExpectDiagValidator(`"role ARN not set" error`, func(d diag.Diagnostic) bool {
				return d.Equal(diag.NewErrorDiagnostic(
					"Cannot assume IAM Role",
					"IAM Role ARN not set",
				))
			}),
		},
	}

	for testName, testCase := range testCases {
		testCase := testCase

		if testCase.ValidateDiags == nil {
			testCase.ValidateDiags = test.ExpectNoDiags
		}

		t.Run(testName, func(t *testing.T) {
			ctx := test.Context(t)

			servicemocks.InitSessionTestEnv(t)

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
			t.Setenv("TMPDIR", tempdir)

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

			ctx, awsConfig, diags := awsbase.GetAwsConfig(ctx, testCase.Config)

			testCase.ValidateDiags(t, diags)
			if diags.HasError() {
				return
			}

			actualSession, ds := GetSession(ctx, &awsConfig, testCase.Config)

			diags = diags.Append(ds...)

			if diags.HasError() {
				t.Fatalf("expected no errors from GetSession(), got : %v", diags)
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
		ValidateDiags              test.DiagsValidator
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
			ValidateDiags: test.ExpectWarningDiagValidator(diag.NewErrorDiagnostic(
				"Assume Role With Web Identity",
				"Role ARN was not set",
			)),
		},

		"invalid no token": {
			Config: &awsbase.Config{
				AssumeRoleWithWebIdentity: &awsbase.AssumeRoleWithWebIdentity{
					RoleARN: servicemocks.MockStsAssumeRoleWithWebIdentityArn,
				},
			},
			ExpectedCredentialsValue: mockdata.MockStsAssumeRoleWithWebIdentityCredentials,
			ValidateDiags: test.ExpectWarningDiagValidator(diag.NewErrorDiagnostic(
				"Assume Role With Web Identity",
				"One of WebIdentityToken, WebIdentityTokenFile must be set",
			)),
		},
	}

	for testName, testCase := range testCases {
		testCase := testCase

		if testCase.ValidateDiags == nil {
			testCase.ValidateDiags = test.ExpectNoDiags
		}

		t.Run(testName, func(t *testing.T) {
			ctx := test.Context(t)

			servicemocks.InitSessionTestEnv(t)

			for k, v := range testCase.EnvironmentVariables {
				t.Setenv(k, v)
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
			t.Setenv("TMPDIR", tempdir)

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
				t.Setenv("AWS_WEB_IDENTITY_TOKEN_FILE", tokenFileName)
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

			ctx, awsConfig, diags := awsbase.GetAwsConfig(ctx, testCase.Config)

			testCase.ValidateDiags(t, diags)
			if diags.HasError() {
				return
			}

			actualSession, ds := GetSession(ctx, &awsConfig, testCase.Config)

			diags = diags.Append(ds...)

			if diags.HasError() {
				t.Fatalf("expected no errors from GetSession(), got : %v", diags)
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
			Description:              "other error under maxRetries",
			RetryCount:               maxRetries - 1,
			Error:                    errors.New("some error"),
			ExpectedRetryableValue:   true, // defaults to true for non-AWS errors
			ExpectRetryToBeAttempted: true,
		},
		{
			Description:              "other error over maxRetries",
			RetryCount:               maxRetries,
			Error:                    errors.New("some error"),
			ExpectedRetryableValue:   true,  // defaults to true for non-AWS errors
			ExpectRetryToBeAttempted: false, // Does not actually get retried, because over max retry limit
		},
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
		{
			Description:              "send request no such host failed under MaxNetworkRetryCount",
			RetryCount:               constants.MaxNetworkRetryCount - 1,
			Error:                    awserr.New(request.ErrCodeRequestError, "send request failed", &net.OpError{Op: "dial", Err: errors.New("no such host")}),
			ExpectedRetryableValue:   true,
			ExpectRetryToBeAttempted: true,
		},
		{
			Description:              "send request no such host failed over MaxNetworkRetryCount",
			RetryCount:               constants.MaxNetworkRetryCount,
			Error:                    awserr.New(request.ErrCodeRequestError, "send request failed", &net.OpError{Op: "dial", Err: errors.New("no such host")}),
			ExpectedRetryableValue:   false,
			ExpectRetryToBeAttempted: false,
		},
		{
			Description:              "send request connection refused failed under MaxNetworkRetryCount",
			RetryCount:               constants.MaxNetworkRetryCount - 1,
			Error:                    awserr.New(request.ErrCodeRequestError, "send request failed", &net.OpError{Op: "dial", Err: errors.New("connection refused")}),
			ExpectedRetryableValue:   true,
			ExpectRetryToBeAttempted: true,
		},
		{
			Description:              "send request connection refused failed over MaxNetworkRetryCount",
			RetryCount:               constants.MaxNetworkRetryCount,
			Error:                    awserr.New(request.ErrCodeRequestError, "send request failed", &net.OpError{Op: "dial", Err: errors.New("connection refused")}),
			ExpectedRetryableValue:   false,
			ExpectRetryToBeAttempted: false,
		},
		{
			Description:              "send request other error failed under MaxNetworkRetryCount",
			RetryCount:               constants.MaxNetworkRetryCount - 1,
			Error:                    awserr.New(request.ErrCodeRequestError, "send request failed", &net.OpError{Op: "dial", Err: errors.New("other error")}),
			ExpectedRetryableValue:   true,
			ExpectRetryToBeAttempted: true,
		},
		{
			Description:              "send request other error failed over MaxNetworkRetryCount",
			RetryCount:               constants.MaxNetworkRetryCount,
			Error:                    awserr.New(request.ErrCodeRequestError, "send request failed", &net.OpError{Op: "dial", Err: errors.New("other error")}),
			ExpectedRetryableValue:   true,
			ExpectRetryToBeAttempted: true,
		},
	}
	for _, testcase := range testcases {
		testcase := testcase

		t.Run(testcase.Description, func(t *testing.T) {
			ctx := test.Context(t)

			servicemocks.InitSessionTestEnv(t)

			config := &awsbase.Config{
				AccessKey:           servicemocks.MockStaticAccessKey,
				MaxRetries:          maxRetries,
				SecretKey:           servicemocks.MockStaticSecretKey,
				SkipCredsValidation: true,
			}
			ctx, awsConfig, diags := awsbase.GetAwsConfig(ctx, config)
			if diags.HasError() {
				t.Fatalf("error in GetAwsConfig(): %v", diags)
			}

			session, err := GetSession(ctx, &awsConfig, config)
			if err != nil {
				t.Fatalf("unexpected error from GetSession(): %s", err)
			}

			iamconn := iam.New(session)

			request, _ := iamconn.GetUserRequest(&iam.GetUserInput{})
			request.RetryCount = testcase.RetryCount
			request.Error = testcase.Error
			request.SetContext(ctx)

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

	servicemocks.InitSessionTestEnv(t)

	ctx, logger := logging.NewTfLogger(ctx)

	config := &awsbase.Config{
		AccessKey: servicemocks.MockStaticAccessKey,
		Logger:    logger,
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

	ctx, awsConfig, diags := awsbase.GetAwsConfig(ctx, config)
	if diags.HasError() {
		t.Fatalf("error in GetAwsConfig(): %v", diags)
	}

	_, err := tflogtest.MultilineJSONDecode(&buf)
	if err != nil {
		t.Fatalf("GetAwsConfig: decoding log lines: %s", err)
	}

	// Ignore log lines from GetAwsConfig()

	_, ds := GetSession(ctx, &awsConfig, config)

	diags = diags.Append(ds...)

	if diags.HasError() {
		t.Fatalf("expected no errors from GetSession(), got : %v", diags)
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

func TestS3UsEast1RegionalEndpoint(t *testing.T) {
	testCases := map[string]struct {
		Config                            *awsbase.Config
		EnvironmentVariables              map[string]string
		SharedConfigurationFile           string
		ExpectedS3UsEast1RegionalEndpoint endpoints.S3UsEast1RegionalEndpoint
	}{
		"no configuration": {
			Config: &awsbase.Config{
				AccessKey: servicemocks.MockStaticAccessKey,
				SecretKey: servicemocks.MockStaticSecretKey,
			},
			ExpectedS3UsEast1RegionalEndpoint: endpoints.LegacyS3UsEast1Endpoint,
		},

		"AWS_S3_US_EAST_1_REGIONAL_ENDPOINT legacy": {
			Config: &awsbase.Config{
				AccessKey: servicemocks.MockStaticAccessKey,
				SecretKey: servicemocks.MockStaticSecretKey,
			},
			EnvironmentVariables: map[string]string{
				"AWS_S3_US_EAST_1_REGIONAL_ENDPOINT": "legacy",
			},
			ExpectedS3UsEast1RegionalEndpoint: endpoints.LegacyS3UsEast1Endpoint,
		},

		"AWS_S3_US_EAST_1_REGIONAL_ENDPOINT regional": {
			Config: &awsbase.Config{
				AccessKey: servicemocks.MockStaticAccessKey,
				SecretKey: servicemocks.MockStaticSecretKey,
			},
			EnvironmentVariables: map[string]string{
				"AWS_S3_US_EAST_1_REGIONAL_ENDPOINT": "regional",
			},
			ExpectedS3UsEast1RegionalEndpoint: endpoints.RegionalS3UsEast1Endpoint,
		},

		"shared configuration file legacy": {
			Config: &awsbase.Config{
				AccessKey: servicemocks.MockStaticAccessKey,
				SecretKey: servicemocks.MockStaticSecretKey,
			},
			SharedConfigurationFile: `
[default]
s3_us_east_1_regional_endpoint = legacy
`,
			ExpectedS3UsEast1RegionalEndpoint: endpoints.LegacyS3UsEast1Endpoint,
		},

		"shared configuration file regional": {
			Config: &awsbase.Config{
				AccessKey: servicemocks.MockStaticAccessKey,
				SecretKey: servicemocks.MockStaticSecretKey,
			},
			SharedConfigurationFile: `
[default]
s3_us_east_1_regional_endpoint = regional
`,
			ExpectedS3UsEast1RegionalEndpoint: endpoints.RegionalS3UsEast1Endpoint,
		},

		"AWS_RETRY_MODE overrides shared configuration": {
			Config: &awsbase.Config{
				AccessKey: servicemocks.MockStaticAccessKey,
				SecretKey: servicemocks.MockStaticSecretKey,
			},
			EnvironmentVariables: map[string]string{
				"AWS_S3_US_EAST_1_REGIONAL_ENDPOINT": "regional",
			},
			SharedConfigurationFile: `
[default]
s3_us_east_1_regional_endpoint = legacy
`,
			ExpectedS3UsEast1RegionalEndpoint: endpoints.RegionalS3UsEast1Endpoint,
		},
	}

	for testName, testCase := range testCases {
		testCase := testCase

		t.Run(testName, func(t *testing.T) {
			servicemocks.InitSessionTestEnv(t)

			for k, v := range testCase.EnvironmentVariables {
				t.Setenv(k, v)
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

			ctx, awsConfig, diags := awsbase.GetAwsConfig(context.Background(), testCase.Config)
			if diags.HasError() {
				t.Fatalf("error in GetAwsConfig(): %v", diags)
			}

			actualSession, ds := GetSession(ctx, &awsConfig, testCase.Config)
			diags = diags.Append(ds...)
			if diags.HasError() {
				t.Fatalf("expected no errors from GetSession(), got : %v", diags)
			}

			if a, e := actualSession.Config.S3UsEast1RegionalEndpoint, testCase.ExpectedS3UsEast1RegionalEndpoint; a != e {
				t.Errorf(`expected S3UsEast1RegionalEndpoin "%s", got: "%s"`, e.String(), a.String())
			}
		})
	}
}
