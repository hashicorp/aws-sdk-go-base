package awsbase

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/credentials/ec2rolecreds"
	"github.com/aws/aws-sdk-go-v2/credentials/stscreds"
	"github.com/hashicorp/aws-sdk-go-base/v2/servicemocks"
)

func TestAWSGetCredentials_static(t *testing.T) {
	testCases := []struct {
		Key, Secret, Token string
	}{
		{
			Key:    "test",
			Secret: "secret",
		}, {
			Key:    "test",
			Secret: "secret",
			Token:  "token",
		},
	}

	for _, testCase := range testCases {
		c := testCase

		cfg := Config{
			AccessKey: c.Key,
			SecretKey: c.Secret,
			Token:     c.Token,
		}

		creds, source, err := getCredentialsProvider(context.Background(), &cfg)
		if err != nil {
			t.Fatalf("unexpected '%[1]T' error getting credentials provider: %[1]s", err)
		}

		if a, e := source, credentials.StaticCredentialsName; a != e {
			t.Errorf("Expected initial source to be %q, %q given", e, a)
		}

		validateCredentialsProvider(creds, c.Key, c.Secret, c.Token, credentials.StaticCredentialsName, t)
		testCredentialsProviderWrappedWithCache(creds, t)
	}
}

// TestAWSGetCredentials_ec2Imds is designed to test the scenario of running Terraform
// from an EC2 instance, without environment variables or manually supplied
// credentials.
func TestAWSGetCredentials_ec2Imds(t *testing.T) {
	// clear AWS_* environment variables
	resetEnv := servicemocks.UnsetEnv(t)
	defer resetEnv()

	// capture the test server's close method, to call after the test returns
	ts := servicemocks.AwsMetadataApiMock(append(
		servicemocks.Ec2metadata_securityCredentialsEndpoints,
		servicemocks.Ec2metadata_instanceIdEndpoint,
		servicemocks.Ec2metadata_iamInfoEndpoint,
	))
	defer ts()

	// An empty config, no key supplied
	cfg := Config{}

	creds, source, err := getCredentialsProvider(context.Background(), &cfg)
	if err != nil {
		t.Fatalf("unexpected '%[1]T' error getting credentials provider: %[1]s", err)
	}

	if a, e := source, ec2rolecreds.ProviderName; a != e {
		t.Errorf("Expected initial source to be %q, %q given", e, a)
	}

	validateCredentialsProvider(creds, "Ec2MetadataAccessKey", "Ec2MetadataSecretKey", "Ec2MetadataSessionToken", ec2rolecreds.ProviderName, t)
	testCredentialsProviderWrappedWithCache(creds, t)

}

func TestAWSGetCredentials_configShouldOverrideEc2IMDS(t *testing.T) {
	resetEnv := servicemocks.UnsetEnv(t)
	defer resetEnv()
	// capture the test server's close method, to call after the test returns
	ts := servicemocks.AwsMetadataApiMock(append(
		servicemocks.Ec2metadata_securityCredentialsEndpoints,
		servicemocks.Ec2metadata_instanceIdEndpoint,
		servicemocks.Ec2metadata_iamInfoEndpoint,
	))
	defer ts()
	testCases := []struct {
		Key, Secret, Token string
	}{
		{
			Key:    "test",
			Secret: "secret",
		}, {
			Key:    "test",
			Secret: "secret",
			Token:  "token",
		},
	}

	for _, testCase := range testCases {
		c := testCase

		cfg := Config{
			AccessKey: c.Key,
			SecretKey: c.Secret,
			Token:     c.Token,
		}

		creds, _, err := getCredentialsProvider(context.Background(), &cfg)
		if err != nil {
			t.Fatalf("unexpected '%[1]T' error: %[1]s", err)
		}

		validateCredentialsProvider(creds, c.Key, c.Secret, c.Token, credentials.StaticCredentialsName, t)
		testCredentialsProviderWrappedWithCache(creds, t)
	}
}

func TestAWSGetCredentials_shouldErrorWithInvalidEc2ImdsEndpoint(t *testing.T) {
	resetEnv := servicemocks.UnsetEnv(t)
	defer resetEnv()
	// capture the test server's close method, to call after the test returns
	ts := servicemocks.InvalidEC2MetadataEndpoint()
	defer ts()

	// An empty config, no key supplied
	cfg := Config{}

	_, _, err := getCredentialsProvider(context.Background(), &cfg)
	if err == nil {
		t.Fatal("expected error returned when getting creds w/ invalid EC2 IMDS endpoint")
	}
	if !IsNoValidCredentialSourcesError(err) {
		t.Fatalf("expected NoValidCredentialSourcesError, got '%[1]T': %[1]s", err)
	}
}

func TestAWSGetCredentials_sharedCredentialsFile(t *testing.T) {
	resetEnv := servicemocks.UnsetEnv(t)
	defer resetEnv()

	if err := os.Setenv("AWS_PROFILE", "myprofile"); err != nil {
		t.Fatalf("Error resetting env var AWS_PROFILE: %s", err)
	}

	fileEnvName := writeCredentialsFile(credentialsFileContentsEnv, t)
	defer os.Remove(fileEnvName)

	fileParamName := writeCredentialsFile(credentialsFileContentsParam, t)
	defer os.Remove(fileParamName)

	if err := os.Setenv("AWS_SHARED_CREDENTIALS_FILE", fileEnvName); err != nil {
		t.Fatalf("Error resetting env var AWS_SHARED_CREDENTIALS_FILE: %s", err)
	}

	// Confirm AWS_SHARED_CREDENTIALS_FILE is working
	credsEnv, source, err := getCredentialsProvider(context.Background(), &Config{
		Profile: "myprofile",
	})
	if err != nil {
		t.Fatalf("unexpected '%[1]T' error getting credentials provider from environment: %[1]s", err)
	}
	if a, e := source, sharedConfigCredentialsSource(fileEnvName); a != e {
		t.Errorf("Expected initial source to be %q, %q given", e, a)
	}
	validateCredentialsProvider(credsEnv, "accesskey1", "secretkey1", "", sharedConfigCredentialsSource(fileEnvName), t)

	// Confirm CredsFilename overrides AWS_SHARED_CREDENTIALS_FILE
	credsParam, source, err := getCredentialsProvider(context.Background(), &Config{
		Profile:                "myprofile",
		SharedCredentialsFiles: []string{fileParamName},
	})
	if err != nil {
		t.Fatalf("unexpected '%[1]T' error getting credentials provider from configuration: %[1]s", err)
	}
	if a, e := source, sharedConfigCredentialsSource(fileParamName); a != e {
		t.Errorf("Expected initial source to be %q, %q given", e, a)
	}
	validateCredentialsProvider(credsParam, "accesskey2", "secretkey2", "", sharedConfigCredentialsSource(fileParamName), t)
}

func TestAWSGetCredentials_webIdentityToken(t *testing.T) {
	cfg := Config{
		WebIdentityToken: servicemocks.MockWebIdentityToken,
		AssumeRole: &AssumeRole{
			RoleARN:     servicemocks.MockStsAssumeRoleWithWebIdentityArn,
			SessionName: servicemocks.MockStsAssumeRoleWithWebIdentitySessionName,
		},
	}

	ts := servicemocks.MockAwsApiServer("STS", []*servicemocks.MockEndpoint{
		servicemocks.MockStsAssumeRoleWithWebIdentityValidEndpoint,
		servicemocks.MockStsGetCallerIdentityValidAssumedRoleEndpoint,
	})
	defer ts.Close()
	cfg.StsEndpoint = ts.URL

	creds, source, err := getCredentialsProvider(context.Background(), &cfg)
	if err != nil {
		t.Fatalf("unexpected '%[1]T' error getting credentials provider: %[1]s", err)
	}

	if a, e := source, ""; a != e {
		t.Errorf("Expected initial source to be %q, %q given", e, a)
	}

	validateCredentialsProvider(creds,
		servicemocks.MockStsAssumeRoleWithWebIdentityAccessKey,
		servicemocks.MockStsAssumeRoleWithWebIdentitySecretKey,
		servicemocks.MockStsAssumeRoleWithWebIdentitySessionToken,
		stscreds.WebIdentityProviderName, t)
	testCredentialsProviderWrappedWithCache(creds, t)
}

func TestAWSGetCredentials_assumeRole(t *testing.T) {
	key := "test"
	secret := "secret"

	cfg := Config{
		AccessKey: key,
		SecretKey: secret,
		AssumeRole: &AssumeRole{
			RoleARN:     servicemocks.MockStsAssumeRoleArn,
			SessionName: servicemocks.MockStsAssumeRoleSessionName,
		},
	}

	ts := servicemocks.MockAwsApiServer("STS", []*servicemocks.MockEndpoint{
		servicemocks.MockStsAssumeRoleValidEndpoint,
		servicemocks.MockStsGetCallerIdentityValidAssumedRoleEndpoint,
	})
	defer ts.Close()
	cfg.StsEndpoint = ts.URL

	creds, source, err := getCredentialsProvider(context.Background(), &cfg)
	if err != nil {
		t.Fatalf("unexpected '%[1]T' error getting credentials provider: %[1]s", err)
	}

	if a, e := source, credentials.StaticCredentialsName; a != e {
		t.Errorf("Expected initial source to be %q, %q given", e, a)
	}

	validateCredentialsProvider(creds,
		servicemocks.MockStsAssumeRoleAccessKey,
		servicemocks.MockStsAssumeRoleSecretKey,
		servicemocks.MockStsAssumeRoleSessionToken,
		stscreds.ProviderName, t)
	testCredentialsProviderWrappedWithCache(creds, t)
}

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

func validateCredentialsProvider(creds aws.CredentialsProvider, accesskey, secretkey, token, source string, t *testing.T) {
	v, err := creds.Retrieve(context.Background())
	if err != nil {
		t.Fatalf("Error retrieving credentials: %s", err)
	}

	if v.AccessKeyID != accesskey {
		t.Errorf("AccessKeyID mismatch, expected: %q, got %q", accesskey, v.AccessKeyID)
	}
	if v.SecretAccessKey != secretkey {
		t.Errorf("SecretAccessKey mismatch, expected: %q, got %q", secretkey, v.SecretAccessKey)
	}
	if v.SessionToken != token {
		t.Errorf("SessionToken mismatch, expected: %q, got %q", token, v.SessionToken)
	}
	if v.Source != source {
		t.Errorf("Expected provider name to be %q, %q given", source, v.Source)
	}
}

func testCredentialsProviderWrappedWithCache(creds aws.CredentialsProvider, t *testing.T) {
	switch creds.(type) {
	case *aws.CredentialsCache:
		break
	default:
		t.Error("expected credentials provider to be wrapped with aws.CredentialsCache")
	}
}

func sharedConfigCredentialsSource(filename string) string {
	return fmt.Sprintf(sharedConfigCredentialsProvider+": %s", filename)
}
