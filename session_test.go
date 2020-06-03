package awsbase

import (
	"errors"
	"io/ioutil"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/credentials/ec2rolecreds"
	"github.com/aws/aws-sdk-go/aws/credentials/endpointcreds"
	"github.com/aws/aws-sdk-go/aws/credentials/stscreds"
)

func TestGetSessionOptions(t *testing.T) {
	oldEnv := initSessionTestEnv()
	defer PopEnv(oldEnv)

	testCases := []struct {
		desc        string
		config      *Config
		expectError bool
	}{
		{"BlankConfig",
			&Config{},
			true,
		},
		{"ConfigWithCredentials",
			&Config{AccessKey: "MockAccessKey", SecretKey: "MockSecretKey"},
			false,
		},
		{"ConfigWithCredsAndOptions",
			&Config{AccessKey: "MockAccessKey", SecretKey: "MockSecretKey", Insecure: true, DebugLogging: true},
			false,
		},
	}

	for _, testCase := range testCases {
		tc := testCase

		t.Run(tc.desc, func(t *testing.T) {
			opts, err := GetSessionOptions(tc.config)
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
		Config                     *Config
		Description                string
		EnableEc2MetadataServer    bool
		EnableEcsCredentialsServer bool
		EnvironmentVariables       map[string]string
		ExpectedCredentialsValue   credentials.Value
		ExpectedRegion             string
		ExpectedError              func(err error) bool
		MockStsEndpoints           []*MockEndpoint
		SharedConfigurationFile    string
		SharedCredentialsFile      string
	}{
		{
			Config:      &Config{},
			Description: "no configuration or credentials",
			ExpectedError: func(err error) bool {
				return errors.Is(err, ErrNoValidCredentialSources)
			},
		},
		{
			Config: &Config{
				AccessKey: "StaticAccessKey",
				Region:    "us-east-1",
				SecretKey: "StaticSecretKey",
			},
			Description: "config AccessKey",
			ExpectedCredentialsValue: credentials.Value{
				AccessKeyID:     "StaticAccessKey",
				ProviderName:    credentials.StaticProviderName,
				SecretAccessKey: "StaticSecretKey",
			},
			ExpectedRegion: "us-east-1",
			MockStsEndpoints: []*MockEndpoint{
				{
					Request:  &MockRequest{"POST", "/", "Action=GetCallerIdentity&Version=2011-06-15"},
					Response: &MockResponse{200, stsResponse_GetCallerIdentity_valid, "text/xml"},
				},
			},
		},
		{
			Config: &Config{
				AccessKey:             "StaticAccessKey",
				AssumeRoleARN:         "arn:aws:iam::555555555555:role/AssumeRole",
				AssumeRoleSessionName: "AssumeRoleSessionName",
				Region:                "us-east-1",
				SecretKey:             "StaticSecretKey",
			},
			Description: "config AccessKey config AssumeRoleARN access key",
			ExpectedCredentialsValue: credentials.Value{
				AccessKeyID:     "AssumeRoleAccessKey",
				ProviderName:    stscreds.ProviderName,
				SecretAccessKey: "AssumeRoleSecretKey",
				SessionToken:    "AssumeRoleSessionToken",
			},
			ExpectedRegion: "us-east-1",
			MockStsEndpoints: []*MockEndpoint{
				{
					Request:  &MockRequest{"POST", "/", "Action=AssumeRole&DurationSeconds=900&RoleArn=arn%3Aaws%3Aiam%3A%3A555555555555%3Arole%2FAssumeRole&RoleSessionName=AssumeRoleSessionName&Version=2011-06-15"},
					Response: &MockResponse{200, stsResponse_AssumeRole_valid, "text/xml"},
				},
				{
					Request:  &MockRequest{"POST", "/", "Action=GetCallerIdentity&Version=2011-06-15"},
					Response: &MockResponse{200, stsResponse_GetCallerIdentity_valid, "text/xml"},
				},
			},
		},
		{
			Config: &Config{
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
			MockStsEndpoints: []*MockEndpoint{
				{
					Request:  &MockRequest{"POST", "/", "Action=GetCallerIdentity&Version=2011-06-15"},
					Response: &MockResponse{200, stsResponse_GetCallerIdentity_valid, "text/xml"},
				},
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
			Description: "config Profile shared configuration credential_source Ec2InstanceMetadata",
			EnvironmentVariables: map[string]string{
				"AWS_SDK_LOAD_CONFIG": "1",
			},
			EnableEc2MetadataServer: true,
			ExpectedCredentialsValue: credentials.Value{
				AccessKeyID:     "AssumeRoleAccessKey",
				ProviderName:    stscreds.ProviderName,
				SecretAccessKey: "AssumeRoleSecretKey",
				SessionToken:    "AssumeRoleSessionToken",
			},
			ExpectedRegion: "us-east-1",
			MockStsEndpoints: []*MockEndpoint{
				{
					Request:  &MockRequest{"POST", "/", "Action=AssumeRole&DurationSeconds=900&RoleArn=arn%3Aaws%3Aiam%3A%3A555555555555%3Arole%2FAssumeRole&RoleSessionName=AssumeRoleSessionName&Version=2011-06-15"},
					Response: &MockResponse{200, stsResponse_AssumeRole_valid, "text/xml"},
				},
				{
					Request:  &MockRequest{"POST", "/", "Action=GetCallerIdentity&Version=2011-06-15"},
					Response: &MockResponse{200, stsResponse_GetCallerIdentity_valid, "text/xml"},
				},
			},
			SharedConfigurationFile: `
[profile SharedConfigurationProfile]
credential_source = Ec2InstanceMetadata
role_arn = arn:aws:iam::555555555555:role/AssumeRole
role_session_name = AssumeRoleSessionName
`,
		},
		{
			Config: &Config{
				Profile: "SharedConfigurationProfile",
				Region:  "us-east-1",
			},
			Description: "config Profile shared configuration credential_source EcsContainer",
			EnvironmentVariables: map[string]string{
				"AWS_CONTAINER_CREDENTIALS_RELATIVE_URI": "/creds",
				"AWS_SDK_LOAD_CONFIG":                    "1",
			},
			EnableEc2MetadataServer:    true,
			EnableEcsCredentialsServer: true,
			ExpectedCredentialsValue: credentials.Value{
				AccessKeyID:     "AssumeRoleAccessKey",
				ProviderName:    stscreds.ProviderName,
				SecretAccessKey: "AssumeRoleSecretKey",
				SessionToken:    "AssumeRoleSessionToken",
			},
			ExpectedRegion: "us-east-1",
			MockStsEndpoints: []*MockEndpoint{
				{
					Request:  &MockRequest{"POST", "/", "Action=AssumeRole&DurationSeconds=900&RoleArn=arn%3Aaws%3Aiam%3A%3A555555555555%3Arole%2FAssumeRole&RoleSessionName=AssumeRoleSessionName&Version=2011-06-15"},
					Response: &MockResponse{200, stsResponse_AssumeRole_valid, "text/xml"},
				},
				{
					Request:  &MockRequest{"POST", "/", "Action=GetCallerIdentity&Version=2011-06-15"},
					Response: &MockResponse{200, stsResponse_GetCallerIdentity_valid, "text/xml"},
				},
			},
			SharedConfigurationFile: `
[profile SharedConfigurationProfile]
credential_source = EcsContainer
role_arn = arn:aws:iam::555555555555:role/AssumeRole
role_session_name = AssumeRoleSessionName
`,
		},
		{
			Config: &Config{
				Profile: "SharedConfigurationProfile",
				Region:  "us-east-1",
			},
			Description: "config Profile shared configuration source_profile",
			EnvironmentVariables: map[string]string{
				"AWS_SDK_LOAD_CONFIG": "1",
			},
			ExpectedCredentialsValue: credentials.Value{
				AccessKeyID:     "AssumeRoleAccessKey",
				ProviderName:    stscreds.ProviderName,
				SecretAccessKey: "AssumeRoleSecretKey",
				SessionToken:    "AssumeRoleSessionToken",
			},
			ExpectedRegion: "us-east-1",
			MockStsEndpoints: []*MockEndpoint{
				{
					Request:  &MockRequest{"POST", "/", "Action=AssumeRole&DurationSeconds=900&RoleArn=arn%3Aaws%3Aiam%3A%3A555555555555%3Arole%2FAssumeRole&RoleSessionName=AssumeRoleSessionName&Version=2011-06-15"},
					Response: &MockResponse{200, stsResponse_AssumeRole_valid, "text/xml"},
				},
				{
					Request:  &MockRequest{"POST", "/", "Action=GetCallerIdentity&Version=2011-06-15"},
					Response: &MockResponse{200, stsResponse_GetCallerIdentity_valid, "text/xml"},
				},
			},
			SharedConfigurationFile: `
[profile SharedConfigurationProfile]
role_arn = arn:aws:iam::555555555555:role/AssumeRole
role_session_name = AssumeRoleSessionName
source_profile = SharedConfigurationSourceProfile

[profile SharedConfigurationSourceProfile]
aws_access_key_id = SharedConfigurationSourceAccessKey
aws_secret_access_key = SharedConfigurationSourceSecretKey
`,
		},
		{
			Config: &Config{
				Region: "us-east-1",
			},
			Description: "environment AWS_ACCESS_KEY_ID",
			EnvironmentVariables: map[string]string{
				"AWS_ACCESS_KEY_ID":     "EnvAccessKey",
				"AWS_SECRET_ACCESS_KEY": "EnvSecretKey",
			},
			ExpectedCredentialsValue: credentials.Value{
				AccessKeyID:     "EnvAccessKey",
				ProviderName:    credentials.EnvProviderName,
				SecretAccessKey: "EnvSecretKey",
			},
			ExpectedRegion: "us-east-1",
			MockStsEndpoints: []*MockEndpoint{
				{
					Request:  &MockRequest{"POST", "/", "Action=GetCallerIdentity&Version=2011-06-15"},
					Response: &MockResponse{200, stsResponse_GetCallerIdentity_valid, "text/xml"},
				},
			},
		},
		{
			Config: &Config{
				AssumeRoleARN:         "arn:aws:iam::555555555555:role/AssumeRole",
				AssumeRoleSessionName: "AssumeRoleSessionName",
				Region:                "us-east-1",
			},
			Description: "environment AWS_ACCESS_KEY_ID config AssumeRoleARN access key",
			EnvironmentVariables: map[string]string{
				"AWS_ACCESS_KEY_ID":     "EnvAccessKey",
				"AWS_SECRET_ACCESS_KEY": "EnvSecretKey",
			},
			ExpectedCredentialsValue: credentials.Value{
				AccessKeyID:     "AssumeRoleAccessKey",
				ProviderName:    stscreds.ProviderName,
				SecretAccessKey: "AssumeRoleSecretKey",
				SessionToken:    "AssumeRoleSessionToken",
			},
			ExpectedRegion: "us-east-1",
			MockStsEndpoints: []*MockEndpoint{
				{
					Request:  &MockRequest{"POST", "/", "Action=AssumeRole&DurationSeconds=900&RoleArn=arn%3Aaws%3Aiam%3A%3A555555555555%3Arole%2FAssumeRole&RoleSessionName=AssumeRoleSessionName&Version=2011-06-15"},
					Response: &MockResponse{200, stsResponse_AssumeRole_valid, "text/xml"},
				},
				{
					Request:  &MockRequest{"POST", "/", "Action=GetCallerIdentity&Version=2011-06-15"},
					Response: &MockResponse{200, stsResponse_GetCallerIdentity_valid, "text/xml"},
				},
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
			ExpectedCredentialsValue: credentials.Value{
				AccessKeyID:     "ProfileSharedCredentialsAccessKey",
				ProviderName:    credentials.SharedCredsProviderName,
				SecretAccessKey: "ProfileSharedCredentialsSecretKey",
			},
			ExpectedRegion: "us-east-1",
			MockStsEndpoints: []*MockEndpoint{
				{
					Request:  &MockRequest{"POST", "/", "Action=GetCallerIdentity&Version=2011-06-15"},
					Response: &MockResponse{200, stsResponse_GetCallerIdentity_valid, "text/xml"},
				},
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
				"AWS_PROFILE":         "SharedConfigurationProfile",
				"AWS_SDK_LOAD_CONFIG": "1",
			},
			ExpectedCredentialsValue: credentials.Value{
				AccessKeyID:     "AssumeRoleAccessKey",
				ProviderName:    stscreds.ProviderName,
				SecretAccessKey: "AssumeRoleSecretKey",
				SessionToken:    "AssumeRoleSessionToken",
			},
			ExpectedRegion: "us-east-1",
			MockStsEndpoints: []*MockEndpoint{
				{
					Request:  &MockRequest{"POST", "/", "Action=AssumeRole&DurationSeconds=900&RoleArn=arn%3Aaws%3Aiam%3A%3A555555555555%3Arole%2FAssumeRole&RoleSessionName=AssumeRoleSessionName&Version=2011-06-15"},
					Response: &MockResponse{200, stsResponse_AssumeRole_valid, "text/xml"},
				},
				{
					Request:  &MockRequest{"POST", "/", "Action=GetCallerIdentity&Version=2011-06-15"},
					Response: &MockResponse{200, stsResponse_GetCallerIdentity_valid, "text/xml"},
				},
			},
			SharedConfigurationFile: `
[profile SharedConfigurationProfile]
credential_source = Ec2InstanceMetadata
role_arn = arn:aws:iam::555555555555:role/AssumeRole
role_session_name = AssumeRoleSessionName
`,
		},
		{
			Config: &Config{
				Region: "us-east-1",
			},
			Description:                "environment AWS_PROFILE shared configuration credential_source EcsContainer",
			EnableEc2MetadataServer:    true,
			EnableEcsCredentialsServer: true,
			EnvironmentVariables: map[string]string{
				"AWS_CONTAINER_CREDENTIALS_RELATIVE_URI": "/creds",
				"AWS_PROFILE":                            "SharedConfigurationProfile",
				"AWS_SDK_LOAD_CONFIG":                    "1",
			},
			ExpectedCredentialsValue: credentials.Value{
				AccessKeyID:     "AssumeRoleAccessKey",
				ProviderName:    stscreds.ProviderName,
				SecretAccessKey: "AssumeRoleSecretKey",
				SessionToken:    "AssumeRoleSessionToken",
			},
			ExpectedRegion: "us-east-1",
			MockStsEndpoints: []*MockEndpoint{
				{
					Request:  &MockRequest{"POST", "/", "Action=AssumeRole&DurationSeconds=900&RoleArn=arn%3Aaws%3Aiam%3A%3A555555555555%3Arole%2FAssumeRole&RoleSessionName=AssumeRoleSessionName&Version=2011-06-15"},
					Response: &MockResponse{200, stsResponse_AssumeRole_valid, "text/xml"},
				},
				{
					Request:  &MockRequest{"POST", "/", "Action=GetCallerIdentity&Version=2011-06-15"},
					Response: &MockResponse{200, stsResponse_GetCallerIdentity_valid, "text/xml"},
				},
			},
			SharedConfigurationFile: `
[profile SharedConfigurationProfile]
credential_source = EcsContainer
role_arn = arn:aws:iam::555555555555:role/AssumeRole
role_session_name = AssumeRoleSessionName
`,
		},
		{
			Config: &Config{
				Region: "us-east-1",
			},
			Description: "environment AWS_PROFILE shared configuration source_profile",
			EnvironmentVariables: map[string]string{
				"AWS_PROFILE":         "SharedConfigurationProfile",
				"AWS_SDK_LOAD_CONFIG": "1",
			},
			ExpectedCredentialsValue: credentials.Value{
				AccessKeyID:     "AssumeRoleAccessKey",
				ProviderName:    stscreds.ProviderName,
				SecretAccessKey: "AssumeRoleSecretKey",
				SessionToken:    "AssumeRoleSessionToken",
			},
			ExpectedRegion: "us-east-1",
			MockStsEndpoints: []*MockEndpoint{
				{
					Request:  &MockRequest{"POST", "/", "Action=AssumeRole&DurationSeconds=900&RoleArn=arn%3Aaws%3Aiam%3A%3A555555555555%3Arole%2FAssumeRole&RoleSessionName=AssumeRoleSessionName&Version=2011-06-15"},
					Response: &MockResponse{200, stsResponse_AssumeRole_valid, "text/xml"},
				},
				{
					Request:  &MockRequest{"POST", "/", "Action=GetCallerIdentity&Version=2011-06-15"},
					Response: &MockResponse{200, stsResponse_GetCallerIdentity_valid, "text/xml"},
				},
			},
			SharedConfigurationFile: `
[profile SharedConfigurationProfile]
role_arn = arn:aws:iam::555555555555:role/AssumeRole
role_session_name = AssumeRoleSessionName
source_profile = SharedConfigurationSourceProfile

[profile SharedConfigurationSourceProfile]
aws_access_key_id = SharedConfigurationSourceAccessKey
aws_secret_access_key = SharedConfigurationSourceSecretKey
`,
		},
		{
			Config: &Config{
				Region: "us-east-1",
			},
			Description: "environment AWS_SESSION_TOKEN",
			EnvironmentVariables: map[string]string{
				"AWS_ACCESS_KEY_ID":     "EnvAccessKey",
				"AWS_SECRET_ACCESS_KEY": "EnvSecretKey",
				"AWS_SESSION_TOKEN":     "EnvSessionToken",
			},
			ExpectedCredentialsValue: credentials.Value{
				AccessKeyID:     "EnvAccessKey",
				ProviderName:    credentials.EnvProviderName,
				SecretAccessKey: "EnvSecretKey",
				SessionToken:    "EnvSessionToken",
			},
			ExpectedRegion: "us-east-1",
			MockStsEndpoints: []*MockEndpoint{
				{
					Request:  &MockRequest{"POST", "/", "Action=GetCallerIdentity&Version=2011-06-15"},
					Response: &MockResponse{200, stsResponse_GetCallerIdentity_valid, "text/xml"},
				},
			},
		},
		{
			Config: &Config{
				Region: "us-east-1",
			},
			Description: "shared credentials default aws_access_key_id",
			ExpectedCredentialsValue: credentials.Value{
				AccessKeyID:     "DefaultSharedCredentialsAccessKey",
				ProviderName:    credentials.SharedCredsProviderName,
				SecretAccessKey: "DefaultSharedCredentialsSecretKey",
			},
			ExpectedRegion: "us-east-1",
			MockStsEndpoints: []*MockEndpoint{
				{
					Request:  &MockRequest{"POST", "/", "Action=GetCallerIdentity&Version=2011-06-15"},
					Response: &MockResponse{200, stsResponse_GetCallerIdentity_valid, "text/xml"},
				},
			},
			SharedCredentialsFile: `
[default]
aws_access_key_id = DefaultSharedCredentialsAccessKey
aws_secret_access_key = DefaultSharedCredentialsSecretKey
`,
		},
		{
			Config: &Config{
				AssumeRoleARN:         "arn:aws:iam::555555555555:role/AssumeRole",
				AssumeRoleSessionName: "AssumeRoleSessionName",
				Region:                "us-east-1",
			},
			Description: "shared credentials default aws_access_key_id config AssumeRoleARN access key",
			ExpectedCredentialsValue: credentials.Value{
				AccessKeyID:     "AssumeRoleAccessKey",
				ProviderName:    stscreds.ProviderName,
				SecretAccessKey: "AssumeRoleSecretKey",
				SessionToken:    "AssumeRoleSessionToken",
			},
			ExpectedRegion: "us-east-1",
			MockStsEndpoints: []*MockEndpoint{
				{
					Request:  &MockRequest{"POST", "/", "Action=AssumeRole&DurationSeconds=900&RoleArn=arn%3Aaws%3Aiam%3A%3A555555555555%3Arole%2FAssumeRole&RoleSessionName=AssumeRoleSessionName&Version=2011-06-15"},
					Response: &MockResponse{200, stsResponse_AssumeRole_valid, "text/xml"},
				},
				{
					Request:  &MockRequest{"POST", "/", "Action=GetCallerIdentity&Version=2011-06-15"},
					Response: &MockResponse{200, stsResponse_GetCallerIdentity_valid, "text/xml"},
				},
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
			Description:             "EC2 metadata access key",
			EnableEc2MetadataServer: true,
			ExpectedCredentialsValue: credentials.Value{
				AccessKeyID:     "Ec2MetadataAccessKey",
				ProviderName:    ec2rolecreds.ProviderName,
				SecretAccessKey: "Ec2MetadataSecretKey",
				SessionToken:    "Ec2MetadataSessionToken",
			},
			ExpectedRegion: "us-east-1",
			MockStsEndpoints: []*MockEndpoint{
				{
					Request:  &MockRequest{"POST", "/", "Action=GetCallerIdentity&Version=2011-06-15"},
					Response: &MockResponse{200, stsResponse_GetCallerIdentity_valid, "text/xml"},
				},
			},
		},
		{
			Config: &Config{
				AssumeRoleARN:         "arn:aws:iam::555555555555:role/AssumeRole",
				AssumeRoleSessionName: "AssumeRoleSessionName",
				Region:                "us-east-1",
			},
			Description:             "EC2 metadata access key config AssumeRoleARN access key",
			EnableEc2MetadataServer: true,
			ExpectedCredentialsValue: credentials.Value{
				AccessKeyID:     "AssumeRoleAccessKey",
				ProviderName:    stscreds.ProviderName,
				SecretAccessKey: "AssumeRoleSecretKey",
				SessionToken:    "AssumeRoleSessionToken",
			},
			ExpectedRegion: "us-east-1",
			MockStsEndpoints: []*MockEndpoint{
				{
					Request:  &MockRequest{"POST", "/", "Action=AssumeRole&DurationSeconds=900&RoleArn=arn%3Aaws%3Aiam%3A%3A555555555555%3Arole%2FAssumeRole&RoleSessionName=AssumeRoleSessionName&Version=2011-06-15"},
					Response: &MockResponse{200, stsResponse_AssumeRole_valid, "text/xml"},
				},
				{
					Request:  &MockRequest{"POST", "/", "Action=GetCallerIdentity&Version=2011-06-15"},
					Response: &MockResponse{200, stsResponse_GetCallerIdentity_valid, "text/xml"},
				},
			},
		},
		{
			Config: &Config{
				Region: "us-east-1",
			},
			Description:                "ECS credentials access key",
			EnableEc2MetadataServer:    true,
			EnableEcsCredentialsServer: true,
			ExpectedCredentialsValue: credentials.Value{
				AccessKeyID:     "EcsCredentialsAccessKey",
				ProviderName:    endpointcreds.ProviderName,
				SecretAccessKey: "EcsCredentialsSecretKey",
				SessionToken:    "EcsCredentialsSessionToken",
			},
			ExpectedRegion: "us-east-1",
			MockStsEndpoints: []*MockEndpoint{
				{
					Request:  &MockRequest{"POST", "/", "Action=GetCallerIdentity&Version=2011-06-15"},
					Response: &MockResponse{200, stsResponse_GetCallerIdentity_valid, "text/xml"},
				},
			},
		},
		{
			Config: &Config{
				AssumeRoleARN:         "arn:aws:iam::555555555555:role/AssumeRole",
				AssumeRoleSessionName: "AssumeRoleSessionName",
				Region:                "us-east-1",
			},
			Description:                "ECS credentials access key config AssumeRoleARN access key",
			EnableEc2MetadataServer:    true,
			EnableEcsCredentialsServer: true,
			ExpectedCredentialsValue: credentials.Value{
				AccessKeyID:     "AssumeRoleAccessKey",
				ProviderName:    stscreds.ProviderName,
				SecretAccessKey: "AssumeRoleSecretKey",
				SessionToken:    "AssumeRoleSessionToken",
			},
			ExpectedRegion: "us-east-1",
			MockStsEndpoints: []*MockEndpoint{
				{
					Request:  &MockRequest{"POST", "/", "Action=AssumeRole&DurationSeconds=900&RoleArn=arn%3Aaws%3Aiam%3A%3A555555555555%3Arole%2FAssumeRole&RoleSessionName=AssumeRoleSessionName&Version=2011-06-15"},
					Response: &MockResponse{200, stsResponse_AssumeRole_valid, "text/xml"},
				},
				{
					Request:  &MockRequest{"POST", "/", "Action=GetCallerIdentity&Version=2011-06-15"},
					Response: &MockResponse{200, stsResponse_GetCallerIdentity_valid, "text/xml"},
				},
			},
		},
		{
			Config: &Config{
				AccessKey: "StaticAccessKey",
				Region:    "us-east-1",
				SecretKey: "StaticSecretKey",
			},
			Description: "config AccessKey over environment AWS_ACCESS_KEY_ID",
			EnvironmentVariables: map[string]string{
				"AWS_ACCESS_KEY_ID":     "EnvAccessKey",
				"AWS_SECRET_ACCESS_KEY": "EnvSecretKey",
			},
			ExpectedCredentialsValue: credentials.Value{
				AccessKeyID:     "StaticAccessKey",
				ProviderName:    credentials.StaticProviderName,
				SecretAccessKey: "StaticSecretKey",
			},
			ExpectedRegion: "us-east-1",
			MockStsEndpoints: []*MockEndpoint{
				{
					Request:  &MockRequest{"POST", "/", "Action=GetCallerIdentity&Version=2011-06-15"},
					Response: &MockResponse{200, stsResponse_GetCallerIdentity_valid, "text/xml"},
				},
			},
		},
		{
			Config: &Config{
				AccessKey: "StaticAccessKey",
				Region:    "us-east-1",
				SecretKey: "StaticSecretKey",
			},
			Description: "config AccessKey over shared credentials default aws_access_key_id",
			ExpectedCredentialsValue: credentials.Value{
				AccessKeyID:     "StaticAccessKey",
				ProviderName:    credentials.StaticProviderName,
				SecretAccessKey: "StaticSecretKey",
			},
			ExpectedRegion: "us-east-1",
			MockStsEndpoints: []*MockEndpoint{
				{
					Request:  &MockRequest{"POST", "/", "Action=GetCallerIdentity&Version=2011-06-15"},
					Response: &MockResponse{200, stsResponse_GetCallerIdentity_valid, "text/xml"},
				},
			},
			SharedCredentialsFile: `
[default]
aws_access_key_id = DefaultSharedCredentialsAccessKey
aws_secret_access_key = DefaultSharedCredentialsSecretKey
`,
		},
		{
			Config: &Config{
				AccessKey: "StaticAccessKey",
				Region:    "us-east-1",
				SecretKey: "StaticSecretKey",
			},
			Description:             "config AccessKey over EC2 metadata access key",
			EnableEc2MetadataServer: true,
			ExpectedCredentialsValue: credentials.Value{
				AccessKeyID:     "StaticAccessKey",
				ProviderName:    credentials.StaticProviderName,
				SecretAccessKey: "StaticSecretKey",
			},
			ExpectedRegion: "us-east-1",
			MockStsEndpoints: []*MockEndpoint{
				{
					Request:  &MockRequest{"POST", "/", "Action=GetCallerIdentity&Version=2011-06-15"},
					Response: &MockResponse{200, stsResponse_GetCallerIdentity_valid, "text/xml"},
				},
			},
		},
		{
			Config: &Config{
				AccessKey: "StaticAccessKey",
				Region:    "us-east-1",
				SecretKey: "StaticSecretKey",
			},
			Description:                "config AccessKey over ECS credentials access key",
			EnableEc2MetadataServer:    true,
			EnableEcsCredentialsServer: true,
			ExpectedCredentialsValue: credentials.Value{
				AccessKeyID:     "StaticAccessKey",
				ProviderName:    credentials.StaticProviderName,
				SecretAccessKey: "StaticSecretKey",
			},
			ExpectedRegion: "us-east-1",
			MockStsEndpoints: []*MockEndpoint{
				{
					Request:  &MockRequest{"POST", "/", "Action=GetCallerIdentity&Version=2011-06-15"},
					Response: &MockResponse{200, stsResponse_GetCallerIdentity_valid, "text/xml"},
				},
			},
		},
		{
			Config: &Config{
				Region: "us-east-1",
			},
			Description: "environment AWS_ACCESS_KEY_ID over shared credentials default aws_access_key_id",
			EnvironmentVariables: map[string]string{
				"AWS_ACCESS_KEY_ID":     "EnvAccessKey",
				"AWS_SECRET_ACCESS_KEY": "EnvSecretKey",
			},
			ExpectedCredentialsValue: credentials.Value{
				AccessKeyID:     "EnvAccessKey",
				ProviderName:    credentials.EnvProviderName,
				SecretAccessKey: "EnvSecretKey",
			},
			ExpectedRegion: "us-east-1",
			MockStsEndpoints: []*MockEndpoint{
				{
					Request:  &MockRequest{"POST", "/", "Action=GetCallerIdentity&Version=2011-06-15"},
					Response: &MockResponse{200, stsResponse_GetCallerIdentity_valid, "text/xml"},
				},
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
				"AWS_ACCESS_KEY_ID":     "EnvAccessKey",
				"AWS_SECRET_ACCESS_KEY": "EnvSecretKey",
			},
			EnableEc2MetadataServer: true,
			ExpectedCredentialsValue: credentials.Value{
				AccessKeyID:     "EnvAccessKey",
				ProviderName:    credentials.EnvProviderName,
				SecretAccessKey: "EnvSecretKey",
			},
			ExpectedRegion: "us-east-1",
			MockStsEndpoints: []*MockEndpoint{
				{
					Request:  &MockRequest{"POST", "/", "Action=GetCallerIdentity&Version=2011-06-15"},
					Response: &MockResponse{200, stsResponse_GetCallerIdentity_valid, "text/xml"},
				},
			},
		},
		{
			Config: &Config{
				Region: "us-east-1",
			},
			Description: "environment AWS_ACCESS_KEY_ID over ECS credentials access key",
			EnvironmentVariables: map[string]string{
				"AWS_ACCESS_KEY_ID":     "EnvAccessKey",
				"AWS_SECRET_ACCESS_KEY": "EnvSecretKey",
			},
			EnableEc2MetadataServer:    true,
			EnableEcsCredentialsServer: true,
			ExpectedCredentialsValue: credentials.Value{
				AccessKeyID:     "EnvAccessKey",
				ProviderName:    credentials.EnvProviderName,
				SecretAccessKey: "EnvSecretKey",
			},
			ExpectedRegion: "us-east-1",
			MockStsEndpoints: []*MockEndpoint{
				{
					Request:  &MockRequest{"POST", "/", "Action=GetCallerIdentity&Version=2011-06-15"},
					Response: &MockResponse{200, stsResponse_GetCallerIdentity_valid, "text/xml"},
				},
			},
		},
		{
			Config: &Config{
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
			MockStsEndpoints: []*MockEndpoint{
				{
					Request:  &MockRequest{"POST", "/", "Action=GetCallerIdentity&Version=2011-06-15"},
					Response: &MockResponse{200, stsResponse_GetCallerIdentity_valid, "text/xml"},
				},
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
			EnableEc2MetadataServer:    true,
			EnableEcsCredentialsServer: true,
			ExpectedCredentialsValue: credentials.Value{
				AccessKeyID:     "DefaultSharedCredentialsAccessKey",
				ProviderName:    credentials.SharedCredsProviderName,
				SecretAccessKey: "DefaultSharedCredentialsSecretKey",
			},
			ExpectedRegion: "us-east-1",
			MockStsEndpoints: []*MockEndpoint{
				{
					Request:  &MockRequest{"POST", "/", "Action=GetCallerIdentity&Version=2011-06-15"},
					Response: &MockResponse{200, stsResponse_GetCallerIdentity_valid, "text/xml"},
				},
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
			EnableEc2MetadataServer:    true,
			EnableEcsCredentialsServer: true,
			ExpectedCredentialsValue: credentials.Value{
				AccessKeyID:     "EcsCredentialsAccessKey",
				ProviderName:    endpointcreds.ProviderName,
				SecretAccessKey: "EcsCredentialsSecretKey",
				SessionToken:    "EcsCredentialsSessionToken",
			},
			ExpectedRegion: "us-east-1",
			MockStsEndpoints: []*MockEndpoint{
				{
					Request:  &MockRequest{"POST", "/", "Action=GetCallerIdentity&Version=2011-06-15"},
					Response: &MockResponse{200, stsResponse_GetCallerIdentity_valid, "text/xml"},
				},
			},
		},
		{
			Config: &Config{
				AccessKey:             "StaticAccessKey",
				AssumeRoleARN:         "arn:aws:iam::555555555555:role/AssumeRole",
				AssumeRoleSessionName: "AssumeRoleSessionName",
				DebugLogging:          true,
				Region:                "us-east-1",
				SecretKey:             "StaticSecretKey",
			},
			Description: "assume role error",
			ExpectedError: func(err error) bool {
				return strings.Contains(err.Error(), "The role \"arn:aws:iam::555555555555:role/AssumeRole\" cannot be assumed.")
			},
			ExpectedRegion: "us-east-1",
			MockStsEndpoints: []*MockEndpoint{
				{
					Request:  &MockRequest{"POST", "/", "Action=AssumeRole&DurationSeconds=900&RoleArn=arn%3Aaws%3Aiam%3A%3A555555555555%3Arole%2FAssumeRole&RoleSessionName=AssumeRoleSessionName&Version=2011-06-15"},
					Response: &MockResponse{403, stsResponse_AssumeRole_InvalidClientTokenId, "text/xml"},
				},
				{
					Request:  &MockRequest{"POST", "/", "Action=GetCallerIdentity&Version=2011-06-15"},
					Response: &MockResponse{200, stsResponse_GetCallerIdentity_valid, "text/xml"},
				},
			},
		},
		{
			Config: &Config{
				AccessKey: "StaticAccessKey",
				Region:    "us-east-1",
				SecretKey: "StaticSecretKey",
			},
			Description: "credential validation error",
			ExpectedError: func(err error) bool {
				return IsAWSErr(err, "AccessDenied", "")
			},
			MockStsEndpoints: []*MockEndpoint{
				{
					Request:  &MockRequest{"POST", "/", "Action=GetCallerIdentity&Version=2011-06-15"},
					Response: &MockResponse{403, stsResponse_GetCallerIdentity_unauthorized, "text/xml"},
				},
			},
		},
		{
			Config: &Config{
				Profile: "SharedConfigurationProfile",
				Region:  "us-east-1",
			},
			Description: "session creation error",
			EnvironmentVariables: map[string]string{
				"AWS_SDK_LOAD_CONFIG": "1",
			},
			ExpectedError: func(err error) bool {
				// TODO: Return wrapped error
				//return IsAWSErr(err, "CredentialRequiresARNError", "")
				return err.Error() == errMsgNoValidCredentialSources
			},
			SharedConfigurationFile: `
[profile SharedConfigurationProfile]
source_profile = SourceSharedCredentials
`,
		},
		{
			Config: &Config{
				AccessKey:           "StaticAccessKey",
				Region:              "us-east-1",
				SecretKey:           "StaticSecretKey",
				SkipCredsValidation: true,
			},
			Description: "skip credentials validation",
			ExpectedCredentialsValue: credentials.Value{
				AccessKeyID:     "StaticAccessKey",
				ProviderName:    credentials.StaticProviderName,
				SecretAccessKey: "StaticSecretKey",
			},
			ExpectedRegion: "us-east-1",
		},
	}

	for _, testCase := range testCases {
		testCase := testCase

		t.Run(testCase.Description, func(t *testing.T) {
			oldEnv := initSessionTestEnv()
			defer PopEnv(oldEnv)

			if testCase.EnableEc2MetadataServer {
				closeEc2Metadata := awsMetadataApiMock(append(ec2metadata_securityCredentialsEndpoints, ec2metadata_instanceIdEndpoint, ec2metadata_iamInfoEndpoint))
				defer closeEc2Metadata()
			}

			if testCase.EnableEcsCredentialsServer {
				closeEcsCredentials := ecsCredentialsApiMock()
				defer closeEcsCredentials()
			}

			closeSts, mockStsSession, err := GetMockedAwsApiSession("STS", testCase.MockStsEndpoints)
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
		})
	}
}

func TestGetSessionWithAccountIDAndPartition(t *testing.T) {
	oldEnv := initSessionTestEnv()
	defer PopEnv(oldEnv)

	ts := MockAwsApiServer("STS", []*MockEndpoint{
		{
			Request:  &MockRequest{"POST", "/", "Action=GetCallerIdentity&Version=2011-06-15"},
			Response: &MockResponse{200, stsResponse_GetCallerIdentity_valid, "text/xml"},
		},
	})
	defer ts.Close()

	testCases := []struct {
		desc              string
		config            *Config
		expectedAcctID    string
		expectedPartition string
		expectedError     string
	}{
		{"StandardProvider_Config", &Config{
			AccessKey:         "MockAccessKey",
			SecretKey:         "MockSecretKey",
			Region:            "us-west-2",
			UserAgentProducts: []*UserAgentProduct{{}},
			StsEndpoint:       ts.URL},
			"222222222222", "aws", ""},
		{"SkipCredsValidation_Config", &Config{
			AccessKey:           "MockAccessKey",
			SecretKey:           "MockSecretKey",
			Region:              "us-west-2",
			SkipCredsValidation: true,
			UserAgentProducts:   []*UserAgentProduct{{}},
			StsEndpoint:         ts.URL},
			"222222222222", "aws", ""},
		{"SkipRequestingAccountId_Config", &Config{
			AccessKey:               "MockAccessKey",
			SecretKey:               "MockSecretKey",
			Region:                  "us-west-2",
			SkipCredsValidation:     true,
			SkipRequestingAccountId: true,
			UserAgentProducts:       []*UserAgentProduct{{}},
			StsEndpoint:             ts.URL},
			"", "aws", ""},
		// {"WithAssumeRole", &Config{
		// 		AccessKey: "MockAccessKey",
		// 		SecretKey: "MockSecretKey",
		// 		Region: "us-west-2",
		// 		UserAgentProducts: []*UserAgentProduct{{}},
		// 		AssumeRoleARN: "arn:aws:iam::222222222222:user/Alice"},
		// 	"222222222222", "aws"},
		{"NoCredentialProviders_Config", &Config{
			AccessKey:         "",
			SecretKey:         "",
			Region:            "us-west-2",
			UserAgentProducts: []*UserAgentProduct{{}},
			StsEndpoint:       ts.URL},
			"", "", "No valid credential sources found for AWS."},
	}

	for _, testCase := range testCases {
		tc := testCase

		t.Run(tc.desc, func(t *testing.T) {
			sess, acctID, part, err := GetSessionWithAccountIDAndPartition(tc.config)
			if err != nil {
				if tc.expectedError == "" {
					t.Fatalf("GetSessionWithAccountIDAndPartition(&Config{...}) should return a valid session, but got the error %s", err)
				} else {
					if !strings.Contains(err.Error(), tc.expectedError) {
						t.Fatalf("GetSession(c) expected error %q and got %q", tc.expectedError, err)
					} else {
						t.Logf("Found error message %q", err)
					}
				}
			} else {
				if sess == nil {
					t.Error("GetSession(c) resulted in a nil session")
				}

				if acctID != tc.expectedAcctID {
					t.Errorf("GetSession(c) returned an incorrect AWS account ID, expected %q but got %q", tc.expectedAcctID, acctID)
				}

				if part != tc.expectedPartition {
					t.Errorf("GetSession(c) returned an incorrect AWS partition, expected %q but got %q", tc.expectedPartition, part)
				}
			}
		})
	}
}

func StashEnv() []string {
	env := os.Environ()
	os.Clearenv()
	return env
}

func PopEnv(env []string) {
	os.Clearenv()

	for _, e := range env {
		p := strings.SplitN(e, "=", 2)
		k, v := p[0], ""
		if len(p) > 1 {
			v = p[1]
		}
		os.Setenv(k, v)
	}
}

func initSessionTestEnv() (oldEnv []string) {
	oldEnv = StashEnv()
	os.Setenv("AWS_CONFIG_FILE", "file_not_exists")
	os.Setenv("AWS_SHARED_CREDENTIALS_FILE", "file_not_exists")

	return oldEnv
}
