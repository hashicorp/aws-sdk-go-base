package awsbase

import (
	"context"
	"io/ioutil"
	"os"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/hashicorp/aws-sdk-go-base/awsmocks"
)

// func TestAWSGetCredentials_shouldErrorWhenBlank(t *testing.T) {
// 	resetEnv := awsmocks.UnsetEnv(t)
// 	defer resetEnv()

// 	cfg := Config{}
// 	_, err := credentialsProvider(&cfg)

// 	if err == nil {
// 		t.Fatal("Expected an error given empty env, keys, and IAM in AWS Config")
// 	}

// 	if !IsNoValidCredentialSourcesError(err) {
// 		t.Fatalf("Unexpected error: %s", err)
// 	}
// }

func TestAWSGetCredentials_static(t *testing.T) {
	testCases := []struct {
		Key, Secret, Token string
	}{
		{
			Key:    "test",
			Secret: "secret",
		}, {
			Key:    "test",
			Secret: "test",
			Token:  "test",
		},
	}

	for _, testCase := range testCases {
		c := testCase

		cfg := Config{
			AccessKey: c.Key,
			SecretKey: c.Secret,
			Token:     c.Token,
		}

		creds, err := credentialsProvider(&cfg)
		if err != nil {
			t.Fatalf("Error gettings creds: %s", err)
		}

		validateCredentials(creds, c.Key, c.Secret, c.Token, credentials.StaticCredentialsName, t)
	}
}

// // TestAWSGetCredentials_shouldIAM is designed to test the scenario of running Terraform
// // from an EC2 instance, without environment variables or manually supplied
// // credentials.
// func TestAWSGetCredentials_shouldIAM(t *testing.T) {
// 	// clear AWS_* environment variables
// 	resetEnv := awsmocks.UnsetEnv(t)
// 	defer resetEnv()

// 	// capture the test server's close method, to call after the test returns
// 	ts := awsmocks.AwsMetadataApiMock(append(awsmocks.Ec2metadata_securityCredentialsEndpoints, awsmocks.Ec2metadata_instanceIdEndpoint, awsmocks.Ec2metadata_iamInfoEndpoint))
// 	defer ts()

// 	// An empty config, no key supplied
// 	cfg := Config{}

// 	creds, err := credentialsProvider(&cfg)
// 	if err != nil {
// 		t.Fatalf("Error gettings creds: %s", err)
// 	}
// 	if creds == nil {
// 		t.Fatal("Expected a static creds provider to be returned")
// 	}

// 	v, err := creds.Retrieve(context.Background())
// 	if err != nil {
// 		t.Fatalf("Error gettings creds: %s", err)
// 	}
// 	if expected, actual := "Ec2MetadataAccessKey", v.AccessKeyID; expected != actual {
// 		t.Fatalf("expected access key (%s), got: %s", expected, actual)
// 	}
// 	if expected, actual := "Ec2MetadataSecretKey", v.SecretAccessKey; expected != actual {
// 		t.Fatalf("expected secret key (%s), got: %s", expected, actual)
// 	}
// 	if expected, actual := "Ec2MetadataSessionToken", v.SessionToken; expected != actual {
// 		t.Fatalf("expected session token (%s), got: %s", expected, actual)
// 	}
// }

// TestAWSGetCredentials_shouldIAM is designed to test the scenario of running Terraform
// from an EC2 instance, without environment variables or manually supplied
// credentials.
func TestAWSGetCredentials_shouldIgnoreIAM(t *testing.T) {
	resetEnv := awsmocks.UnsetEnv(t)
	defer resetEnv()
	// capture the test server's close method, to call after the test returns
	ts := awsmocks.AwsMetadataApiMock(append(awsmocks.Ec2metadata_securityCredentialsEndpoints, awsmocks.Ec2metadata_instanceIdEndpoint, awsmocks.Ec2metadata_iamInfoEndpoint))
	defer ts()
	testCases := []struct {
		Key, Secret, Token string
	}{
		{
			Key:    "test",
			Secret: "secret",
		}, {
			Key:    "test",
			Secret: "test",
			Token:  "test",
		},
	}

	for _, testCase := range testCases {
		c := testCase

		cfg := Config{
			AccessKey: c.Key,
			SecretKey: c.Secret,
			Token:     c.Token,
		}

		creds, err := credentialsProvider(&cfg)
		if err != nil {
			t.Fatalf("Error gettings creds: %s", err)
		}
		if creds == nil {
			t.Fatal("Expected a static creds provider to be returned")
		}

		v, err := creds.Retrieve(context.Background())
		if err != nil {
			t.Fatalf("Error gettings creds: %s", err)
		}
		if v.AccessKeyID != c.Key {
			t.Fatalf("AccessKeyID mismatch, expected: (%s), got (%s)", c.Key, v.AccessKeyID)
		}
		if v.SecretAccessKey != c.Secret {
			t.Fatalf("SecretAccessKey mismatch, expected: (%s), got (%s)", c.Secret, v.SecretAccessKey)
		}
		if v.SessionToken != c.Token {
			t.Fatalf("SessionToken mismatch, expected: (%s), got (%s)", c.Token, v.SessionToken)
		}
	}
}

// func TestAWSGetCredentials_shouldErrorWithInvalidEndpoint(t *testing.T) {
// 	resetEnv := awsmocks.UnsetEnv(t)
// 	defer resetEnv()
// 	// capture the test server's close method, to call after the test returns
// 	ts := awsmocks.InvalidAwsEnv()
// 	defer ts()

// 	_, err := credentialsProvider(&Config{})

// 	if !IsNoValidCredentialSourcesError(err) {
// 		t.Fatalf("Error gettings creds: %s", err)
// 	}

// 	if err == nil {
// 		t.Fatal("Expected error returned when getting creds w/ invalid EC2 endpoint")
// 	}
// }

// func TestAWSGetCredentials_shouldIgnoreInvalidEndpoint(t *testing.T) {
// 	resetEnv := awsmocks.UnsetEnv(t)
// 	defer resetEnv()
// 	// capture the test server's close method, to call after the test returns
// 	ts := awsmocks.InvalidAwsEnv()
// 	defer ts()

// 	creds, err := credentialsProvider(&Config{AccessKey: "accessKey", SecretKey: "secretKey"})
// 	if err != nil {
// 		t.Fatalf("Error gettings creds: %s", err)
// 	}
// 	v, err := creds.Retrieve(context.Background())
// 	if err != nil {
// 		t.Fatalf("Getting static credentials w/ invalid EC2 endpoint failed: %s", err)
// 	}
// 	if creds == nil {
// 		t.Fatal("Expected a static creds provider to be returned")
// 	}

// 	if v.Source != "StaticProvider" {
// 		t.Fatalf("Expected provider name to be %q, %q given", "StaticProvider", v.Source)
// 	}

// 	if v.AccessKeyID != "accessKey" {
// 		t.Fatalf("Static Access Key %q doesn't match: %s", "accessKey", v.AccessKeyID)
// 	}

// 	if v.SecretAccessKey != "secretKey" {
// 		t.Fatalf("Static Secret Key %q doesn't match: %s", "secretKey", v.SecretAccessKey)
// 	}
// }

// func TestAWSGetCredentials_shouldCatchEC2RoleProvider(t *testing.T) {
// 	resetEnv := awsmocks.UnsetEnv(t)
// 	defer resetEnv()
// 	// capture the test server's close method, to call after the test returns
// 	ts := awsmocks.AwsMetadataApiMock(append(awsmocks.Ec2metadata_securityCredentialsEndpoints, awsmocks.Ec2metadata_instanceIdEndpoint, awsmocks.Ec2metadata_iamInfoEndpoint))
// 	defer ts()

// 	creds, err := credentialsProvider(&Config{})
// 	if err != nil {
// 		t.Fatalf("Error gettings creds: %s", err)
// 	}
// 	if creds == nil {
// 		t.Fatal("Expected an EC2Role creds provider to be returned")
// 	}

// 	v, err := creds.Retrieve(context.Background())
// 	if err != nil {
// 		t.Fatalf("Expected no error when getting creds: %s", err)
// 	}
// 	expectedProvider := "EC2RoleProvider"
// 	if v.Source != expectedProvider {
// 		t.Fatalf("Expected provider name to be %q, %q given",
// 			expectedProvider, v.Source)
// 	}
// }

var credentialsFileContentsEnv = `[myprofile]
aws_access_key_id = accesskey1
aws_secret_access_key = secretkey1
`

var credentialsFileContentsParam = `[myprofile]
aws_access_key_id = accesskey2
aws_secret_access_key = secretkey2
`

func writeCredentialsFile(credentialsFileContents string, t *testing.T) string {
	file, err := ioutil.TempFile(os.TempDir(), "terraform_aws_cred")
	if err != nil {
		t.Fatalf("Error writing temporary credentials file: %s", err)
	}
	_, err = file.WriteString(credentialsFileContents)
	if err != nil {
		t.Fatalf("Error writing temporary credentials to file: %s", err)
	}
	err = file.Close()
	if err != nil {
		t.Fatalf("Error closing temporary credentials file: %s", err)
	}
	return file.Name()
}

func validateCredentials(creds aws.CredentialsProvider, accesskey, secretkey, token, source string, t *testing.T) {
	v, err := creds.Retrieve(context.Background())
	if err != nil {
		t.Fatalf("Error gettings creds: %s", err)
	}

	if v.AccessKeyID != accesskey {
		t.Fatalf("AccessKeyID mismatch, expected: %q, got %q", accesskey, v.AccessKeyID)
	}
	if v.SecretAccessKey != secretkey {
		t.Fatalf("SecretAccessKey mismatch, expected: %q, got %q", secretkey, v.SecretAccessKey)
	}
	if v.SessionToken != token {
		t.Fatalf("SessionToken mismatch, expected: %q, got %q", token, v.SessionToken)
	}
	if v.Source != source {
		t.Fatalf("Expected provider name to be %q, %q given", source, v.Source)
	}
}
