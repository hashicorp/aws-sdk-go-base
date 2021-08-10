package awsbase

import (
	"context"
	"io/ioutil"
	"os"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/google/go-cmp/cmp"
	"github.com/hashicorp/aws-sdk-go-base/awsmocks"
)

func TestGetSession(t *testing.T) {
	testCases := []struct {
		Config                     *Config
		Description                string
		EnableEc2MetadataServer    bool
		EnableEcsCredentialsServer bool
		EnableWebIdentityToken     bool
		EnvironmentVariables       map[string]string
		ExpectedCredentialsValue   aws.Credentials
		ExpectedRegion             string
		ExpectedUserAgent          string
		ExpectedError              func(err error) bool
		MockStsEndpoints           []*awsmocks.MockEndpoint
		SharedConfigurationFile    string
		SharedCredentialsFile      string
	}{
		// {
		// 	Config:      &Config{},
		// 	Description: "no configuration or credentials",
		// 	ExpectedError: func(err error) bool {
		// 		return IsNoValidCredentialSourcesError(err)
		// 	},
		// },
		{
			Config: &Config{
				AccessKey: awsmocks.MockStaticAccessKey,
				Region:    "us-east-1",
				SecretKey: awsmocks.MockStaticSecretKey,
			},
			Description:              "config AccessKey",
			ExpectedCredentialsValue: awsmocks.MockStaticCredentialsV2,
			ExpectedRegion:           "us-east-1",
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
				testCase.Config.StsEndpoint = aws.ToString(mockStsSession.Config.Endpoint)
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

				// TODO: verify this
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

				// TODO: verify this
				// Config does not provide a passthrough for session.Options.SharedConfigFiles
				testCase.Config.CredsFilename = file.Name()
			}

			for k, v := range testCase.EnvironmentVariables {
				os.Setenv(k, v)
			}

			awsConfig, err := GetAwsConfig(context.Background(), testCase.Config)

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

			credentialsValue, err := awsConfig.Credentials.Retrieve(context.Background())

			if err != nil {
				t.Fatalf("unexpected credentials Retrieve() error: %s", err)
			}

			// if !reflect.DeepEqual(credentialsValue, testCase.ExpectedCredentialsValue) {
			// 	t.Fatalf("unexpected credentials: %#v", credentialsValue)
			// }
			if diff := cmp.Diff(credentialsValue, testCase.ExpectedCredentialsValue); diff != "" {
				t.Fatalf("unexpected credentials: %s", diff)
			}

			if expected, actual := testCase.ExpectedRegion, awsConfig.Region; expected != actual {
				t.Fatalf("expected region (%s), got: %s", expected, actual)
			}

			// if testCase.ExpectedUserAgent != "" {
			// 	clientInfo := metadata.ClientInfo{
			// 		Endpoint:    "http://endpoint",
			// 		SigningName: "",
			// 	}
			// 	conn := client.New(*actualSession.Config, clientInfo, actualSession.Handlers)

			// 	req := conn.NewRequest(&request.Operation{Name: "Operation"}, nil, nil)

			// 	if err := req.Build(); err != nil {
			// 		t.Fatalf("expect no Request.Build() error, got %s", err)
			// 	}

			// 	if e, a := testCase.ExpectedUserAgent, req.HTTPRequest.Header.Get("User-Agent"); e != a {
			// 		t.Errorf("expected User-Agent (%s), got: %s", e, a)
			// 	}
			// }
		})
	}
}
