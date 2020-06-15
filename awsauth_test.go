package awsbase

import (
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/aws/aws-sdk-go/aws/credentials/ec2rolecreds"
	"github.com/aws/aws-sdk-go/service/iam"
	"github.com/aws/aws-sdk-go/service/sts"
)

func TestGetAccountIDAndPartition(t *testing.T) {
	var testCases = []struct {
		Description          string
		AuthProviderName     string
		EC2MetadataEndpoints []*MetadataResponse
		IAMEndpoints         []*MockEndpoint
		STSEndpoints         []*MockEndpoint
		ErrCount             int
		ExpectedAccountID    string
		ExpectedPartition    string
	}{
		{
			Description:          "EC2 Metadata over iam:GetUser when using EC2 Instance Profile",
			AuthProviderName:     ec2rolecreds.ProviderName,
			EC2MetadataEndpoints: append(ec2metadata_securityCredentialsEndpoints, ec2metadata_instanceIdEndpoint, ec2metadata_iamInfoEndpoint),
			IAMEndpoints: []*MockEndpoint{
				{
					Request:  &MockRequest{"POST", "/", "Action=GetUser&Version=2010-05-08"},
					Response: &MockResponse{200, iamResponse_GetUser_valid, "text/xml"},
				},
			},
			ExpectedAccountID: ec2metadata_iamInfoEndpoint_expectedAccountID,
			ExpectedPartition: ec2metadata_iamInfoEndpoint_expectedPartition,
		},
		{
			Description:          "Mimic the metadata service mocked by Hologram (https://github.com/AdRoll/hologram)",
			AuthProviderName:     ec2rolecreds.ProviderName,
			EC2MetadataEndpoints: ec2metadata_securityCredentialsEndpoints,
			IAMEndpoints: []*MockEndpoint{
				{
					Request:  &MockRequest{"POST", "/", "Action=GetUser&Version=2010-05-08"},
					Response: &MockResponse{403, iamResponse_GetUser_unauthorized, "text/xml"},
				},
			},
			STSEndpoints: []*MockEndpoint{
				MockStsGetCallerIdentityValidEndpoint,
			},
			ExpectedAccountID: MockStsGetCallerIdentityAccountID,
			ExpectedPartition: MockStsGetCallerIdentityPartition,
		},
		{
			Description: "iam:ListRoles if iam:GetUser AccessDenied and sts:GetCallerIdentity fails",
			IAMEndpoints: []*MockEndpoint{
				{
					Request:  &MockRequest{"POST", "/", "Action=GetUser&Version=2010-05-08"},
					Response: &MockResponse{403, iamResponse_GetUser_unauthorized, "text/xml"},
				},
				{
					Request:  &MockRequest{"POST", "/", "Action=ListRoles&MaxItems=1&Version=2010-05-08"},
					Response: &MockResponse{200, iamResponse_ListRoles_valid, "text/xml"},
				},
			},
			STSEndpoints: []*MockEndpoint{
				MockStsGetCallerIdentityInvalidEndpointAccessDenied,
			},
			ExpectedAccountID: iamResponse_ListRoles_valid_expectedAccountID,
			ExpectedPartition: iamResponse_ListRoles_valid_expectedPartition,
		},
		{
			Description: "iam:ListRoles if iam:GetUser ValidationError and sts:GetCallerIdentity fails",
			IAMEndpoints: []*MockEndpoint{
				{
					Request:  &MockRequest{"POST", "/", "Action=GetUser&Version=2010-05-08"},
					Response: &MockResponse{400, iamResponse_GetUser_federatedFailure, "text/xml"},
				},
				{
					Request:  &MockRequest{"POST", "/", "Action=ListRoles&MaxItems=1&Version=2010-05-08"},
					Response: &MockResponse{200, iamResponse_ListRoles_valid, "text/xml"},
				},
			},
			STSEndpoints: []*MockEndpoint{
				MockStsGetCallerIdentityInvalidEndpointAccessDenied,
			},
			ExpectedAccountID: iamResponse_ListRoles_valid_expectedAccountID,
			ExpectedPartition: iamResponse_ListRoles_valid_expectedPartition,
		},
		{
			Description: "Error when all endpoints fail",
			IAMEndpoints: []*MockEndpoint{
				{
					Request:  &MockRequest{"POST", "/", "Action=GetUser&Version=2010-05-08"},
					Response: &MockResponse{400, iamResponse_GetUser_federatedFailure, "text/xml"},
				},
				{
					Request:  &MockRequest{"POST", "/", "Action=ListRoles&MaxItems=1&Version=2010-05-08"},
					Response: &MockResponse{403, iamResponse_ListRoles_unauthorized, "text/xml"},
				},
			},
			STSEndpoints: []*MockEndpoint{
				MockStsGetCallerIdentityInvalidEndpointAccessDenied,
			},
			ErrCount: 1,
		},
	}

	for _, testCase := range testCases {
		testCase := testCase

		t.Run(testCase.Description, func(t *testing.T) {
			resetEnv := unsetEnv(t)
			defer resetEnv()
			// capture the test server's close method, to call after the test returns
			awsTs := awsMetadataApiMock(testCase.EC2MetadataEndpoints)
			defer awsTs()

			closeIam, iamSess, err := GetMockedAwsApiSession("IAM", testCase.IAMEndpoints)
			defer closeIam()
			if err != nil {
				t.Fatal(err)
			}

			closeSts, stsSess, err := GetMockedAwsApiSession("STS", testCase.STSEndpoints)
			defer closeSts()
			if err != nil {
				t.Fatal(err)
			}

			iamConn := iam.New(iamSess)
			stsConn := sts.New(stsSess)

			accountID, partition, err := GetAccountIDAndPartition(iamConn, stsConn, testCase.AuthProviderName)
			if err != nil && testCase.ErrCount == 0 {
				t.Fatalf("Expected no error, received error: %s", err)
			}
			if err == nil && testCase.ErrCount > 0 {
				t.Fatalf("Expected %d error(s), received none", testCase.ErrCount)
			}
			if accountID != testCase.ExpectedAccountID {
				t.Fatalf("Parsed account ID doesn't match with expected (%q != %q)", accountID, testCase.ExpectedAccountID)
			}
			if partition != testCase.ExpectedPartition {
				t.Fatalf("Parsed partition doesn't match with expected (%q != %q)", partition, testCase.ExpectedPartition)
			}
		})
	}
}

func TestGetAccountIDAndPartitionFromEC2Metadata(t *testing.T) {
	t.Run("EC2 metadata success", func(t *testing.T) {
		resetEnv := unsetEnv(t)
		defer resetEnv()
		// capture the test server's close method, to call after the test returns
		awsTs := awsMetadataApiMock(append(ec2metadata_securityCredentialsEndpoints, ec2metadata_instanceIdEndpoint, ec2metadata_iamInfoEndpoint))
		defer awsTs()

		id, partition, err := GetAccountIDAndPartitionFromEC2Metadata()
		if err != nil {
			t.Fatalf("Getting account ID from EC2 metadata API failed: %s", err)
		}

		if id != ec2metadata_iamInfoEndpoint_expectedAccountID {
			t.Fatalf("Expected account ID: %s, given: %s", ec2metadata_iamInfoEndpoint_expectedAccountID, id)
		}
		if partition != ec2metadata_iamInfoEndpoint_expectedPartition {
			t.Fatalf("Expected partition: %s, given: %s", ec2metadata_iamInfoEndpoint_expectedPartition, partition)
		}
	})
}

func TestGetAccountIDAndPartitionFromIAMGetUser(t *testing.T) {
	var testCases = []struct {
		Description       string
		MockEndpoints     []*MockEndpoint
		ErrCount          int
		ExpectedAccountID string
		ExpectedPartition string
	}{
		{
			Description: "Ignore iam:GetUser failure with federated user",
			MockEndpoints: []*MockEndpoint{
				{
					Request:  &MockRequest{"POST", "/", "Action=GetUser&Version=2010-05-08"},
					Response: &MockResponse{400, iamResponse_GetUser_federatedFailure, "text/xml"},
				},
			},
			ErrCount: 0,
		},
		{
			Description: "Ignore iam:GetUser failure with unauthorized user",
			MockEndpoints: []*MockEndpoint{
				{
					Request:  &MockRequest{"POST", "/", "Action=GetUser&Version=2010-05-08"},
					Response: &MockResponse{403, iamResponse_GetUser_unauthorized, "text/xml"},
				},
			},
			ErrCount: 0,
		},
		{
			Description: "iam:GetUser success",
			MockEndpoints: []*MockEndpoint{
				{
					Request:  &MockRequest{"POST", "/", "Action=GetUser&Version=2010-05-08"},
					Response: &MockResponse{200, iamResponse_GetUser_valid, "text/xml"},
				},
			},
			ExpectedAccountID: iamResponse_GetUser_valid_expectedAccountID,
			ExpectedPartition: iamResponse_GetUser_valid_expectedPartition,
		},
	}

	for _, testCase := range testCases {
		testCase := testCase

		t.Run(testCase.Description, func(t *testing.T) {
			closeIam, iamSess, err := GetMockedAwsApiSession("IAM", testCase.MockEndpoints)
			defer closeIam()
			if err != nil {
				t.Fatal(err)
			}

			iamConn := iam.New(iamSess)

			accountID, partition, err := GetAccountIDAndPartitionFromIAMGetUser(iamConn)
			if err != nil && testCase.ErrCount == 0 {
				t.Fatalf("Expected no error, received error: %s", err)
			}
			if err == nil && testCase.ErrCount > 0 {
				t.Fatalf("Expected %d error(s), received none", testCase.ErrCount)
			}
			if accountID != testCase.ExpectedAccountID {
				t.Fatalf("Parsed account ID doesn't match with expected (%q != %q)", accountID, testCase.ExpectedAccountID)
			}
			if partition != testCase.ExpectedPartition {
				t.Fatalf("Parsed partition doesn't match with expected (%q != %q)", partition, testCase.ExpectedPartition)
			}
		})
	}
}

func TestGetAccountIDAndPartitionFromIAMListRoles(t *testing.T) {
	var testCases = []struct {
		Description       string
		MockEndpoints     []*MockEndpoint
		ErrCount          int
		ExpectedAccountID string
		ExpectedPartition string
	}{
		{
			Description: "iam:ListRoles unauthorized",
			MockEndpoints: []*MockEndpoint{
				{
					Request:  &MockRequest{"POST", "/", "Action=ListRoles&MaxItems=1&Version=2010-05-08"},
					Response: &MockResponse{403, iamResponse_ListRoles_unauthorized, "text/xml"},
				},
			},
			ErrCount: 1,
		},
		{
			Description: "iam:ListRoles success",
			MockEndpoints: []*MockEndpoint{
				{
					Request:  &MockRequest{"POST", "/", "Action=ListRoles&MaxItems=1&Version=2010-05-08"},
					Response: &MockResponse{200, iamResponse_ListRoles_valid, "text/xml"},
				},
			},
			ExpectedAccountID: iamResponse_ListRoles_valid_expectedAccountID,
			ExpectedPartition: iamResponse_ListRoles_valid_expectedPartition,
		},
	}

	for _, testCase := range testCases {
		testCase := testCase

		t.Run(testCase.Description, func(t *testing.T) {
			closeIam, iamSess, err := GetMockedAwsApiSession("IAM", testCase.MockEndpoints)
			defer closeIam()
			if err != nil {
				t.Fatal(err)
			}

			iamConn := iam.New(iamSess)

			accountID, partition, err := GetAccountIDAndPartitionFromIAMListRoles(iamConn)
			if err != nil && testCase.ErrCount == 0 {
				t.Fatalf("Expected no error, received error: %s", err)
			}
			if err == nil && testCase.ErrCount > 0 {
				t.Fatalf("Expected %d error(s), received none", testCase.ErrCount)
			}
			if accountID != testCase.ExpectedAccountID {
				t.Fatalf("Parsed account ID doesn't match with expected (%q != %q)", accountID, testCase.ExpectedAccountID)
			}
			if partition != testCase.ExpectedPartition {
				t.Fatalf("Parsed partition doesn't match with expected (%q != %q)", partition, testCase.ExpectedPartition)
			}
		})
	}
}

func TestGetAccountIDAndPartitionFromSTSGetCallerIdentity(t *testing.T) {
	var testCases = []struct {
		Description       string
		MockEndpoints     []*MockEndpoint
		ErrCount          int
		ExpectedAccountID string
		ExpectedPartition string
	}{
		{
			Description: "sts:GetCallerIdentity unauthorized",
			MockEndpoints: []*MockEndpoint{
				MockStsGetCallerIdentityInvalidEndpointAccessDenied,
			},
			ErrCount: 1,
		},
		{
			Description: "sts:GetCallerIdentity success",
			MockEndpoints: []*MockEndpoint{
				MockStsGetCallerIdentityValidEndpoint,
			},
			ExpectedAccountID: MockStsGetCallerIdentityAccountID,
			ExpectedPartition: MockStsGetCallerIdentityPartition,
		},
	}

	for _, testCase := range testCases {
		testCase := testCase

		t.Run(testCase.Description, func(t *testing.T) {
			closeSts, stsSess, err := GetMockedAwsApiSession("STS", testCase.MockEndpoints)
			defer closeSts()
			if err != nil {
				t.Fatal(err)
			}

			stsConn := sts.New(stsSess)

			accountID, partition, err := GetAccountIDAndPartitionFromSTSGetCallerIdentity(stsConn)
			if err != nil && testCase.ErrCount == 0 {
				t.Fatalf("Expected no error, received error: %s", err)
			}
			if err == nil && testCase.ErrCount > 0 {
				t.Fatalf("Expected %d error(s), received none", testCase.ErrCount)
			}
			if accountID != testCase.ExpectedAccountID {
				t.Fatalf("Parsed account ID doesn't match with expected (%q != %q)", accountID, testCase.ExpectedAccountID)
			}
			if partition != testCase.ExpectedPartition {
				t.Fatalf("Parsed partition doesn't match with expected (%q != %q)", partition, testCase.ExpectedPartition)
			}
		})
	}
}

func TestAWSParseAccountIDAndPartitionFromARN(t *testing.T) {
	var testCases = []struct {
		InputARN          string
		ErrCount          int
		ExpectedAccountID string
		ExpectedPartition string
	}{
		{
			InputARN: "invalid-arn",
			ErrCount: 1,
		},
		{
			InputARN:          "arn:aws:iam::123456789012:instance-profile/name",
			ExpectedAccountID: "123456789012",
			ExpectedPartition: "aws",
		},
		{
			InputARN:          "arn:aws:iam::123456789012:user/name",
			ExpectedAccountID: "123456789012",
			ExpectedPartition: "aws",
		},
		{
			InputARN:          "arn:aws:sts::123456789012:assumed-role/name",
			ExpectedAccountID: "123456789012",
			ExpectedPartition: "aws",
		},
		{
			InputARN:          "arn:aws-us-gov:sts::123456789012:assumed-role/name",
			ExpectedAccountID: "123456789012",
			ExpectedPartition: "aws-us-gov",
		},
	}

	for _, testCase := range testCases {
		testCase := testCase

		t.Run(testCase.InputARN, func(t *testing.T) {
			accountID, partition, err := parseAccountIDAndPartitionFromARN(testCase.InputARN)
			if err != nil && testCase.ErrCount == 0 {
				t.Fatalf("Expected no error when parsing ARN, received error: %s", err)
			}
			if err == nil && testCase.ErrCount > 0 {
				t.Fatalf("Expected %d error(s) when parsing ARN, received none", testCase.ErrCount)
			}
			if accountID != testCase.ExpectedAccountID {
				t.Fatalf("Parsed account ID doesn't match with expected (%q != %q)", accountID, testCase.ExpectedAccountID)
			}
			if partition != testCase.ExpectedPartition {
				t.Fatalf("Parsed partition doesn't match with expected (%q != %q)", partition, testCase.ExpectedPartition)
			}
		})
	}
}

func TestAWSGetCredentials_shouldErrorWhenBlank(t *testing.T) {
	resetEnv := unsetEnv(t)
	defer resetEnv()

	cfg := Config{}
	_, err := GetCredentials(&cfg)

	if !IsNoValidCredentialSourcesError(err) {
		t.Fatalf("Unexpected error: %s", err)
	}

	if err == nil {
		t.Fatal("Expected an error given empty env, keys, and IAM in AWS Config")
	}
}

func TestAWSGetCredentials_shouldBeStatic(t *testing.T) {
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

		creds, err := GetCredentials(&cfg)
		if err != nil {
			t.Fatalf("Error gettings creds: %s", err)
		}
		if creds == nil {
			t.Fatal("Expected a static creds provider to be returned")
		}

		v, err := creds.Get()
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

// TestAWSGetCredentials_shouldIAM is designed to test the scenario of running Terraform
// from an EC2 instance, without environment variables or manually supplied
// credentials.
func TestAWSGetCredentials_shouldIAM(t *testing.T) {
	// clear AWS_* environment variables
	resetEnv := unsetEnv(t)
	defer resetEnv()

	// capture the test server's close method, to call after the test returns
	ts := awsMetadataApiMock(append(ec2metadata_securityCredentialsEndpoints, ec2metadata_instanceIdEndpoint, ec2metadata_iamInfoEndpoint))
	defer ts()

	// An empty config, no key supplied
	cfg := Config{}

	creds, err := GetCredentials(&cfg)
	if err != nil {
		t.Fatalf("Error gettings creds: %s", err)
	}
	if creds == nil {
		t.Fatal("Expected a static creds provider to be returned")
	}

	v, err := creds.Get()
	if err != nil {
		t.Fatalf("Error gettings creds: %s", err)
	}
	if expected, actual := "Ec2MetadataAccessKey", v.AccessKeyID; expected != actual {
		t.Fatalf("expected access key (%s), got: %s", expected, actual)
	}
	if expected, actual := "Ec2MetadataSecretKey", v.SecretAccessKey; expected != actual {
		t.Fatalf("expected secret key (%s), got: %s", expected, actual)
	}
	if expected, actual := "Ec2MetadataSessionToken", v.SessionToken; expected != actual {
		t.Fatalf("expected session token (%s), got: %s", expected, actual)
	}
}

// TestAWSGetCredentials_shouldIAM is designed to test the scenario of running Terraform
// from an EC2 instance, without environment variables or manually supplied
// credentials.
func TestAWSGetCredentials_shouldIgnoreIAM(t *testing.T) {
	resetEnv := unsetEnv(t)
	defer resetEnv()
	// capture the test server's close method, to call after the test returns
	ts := awsMetadataApiMock(append(ec2metadata_securityCredentialsEndpoints, ec2metadata_instanceIdEndpoint, ec2metadata_iamInfoEndpoint))
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

		creds, err := GetCredentials(&cfg)
		if err != nil {
			t.Fatalf("Error gettings creds: %s", err)
		}
		if creds == nil {
			t.Fatal("Expected a static creds provider to be returned")
		}

		v, err := creds.Get()
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

func TestAWSGetCredentials_shouldErrorWithInvalidEndpoint(t *testing.T) {
	resetEnv := unsetEnv(t)
	defer resetEnv()
	// capture the test server's close method, to call after the test returns
	ts := invalidAwsEnv()
	defer ts()

	_, err := GetCredentials(&Config{})

	if !IsNoValidCredentialSourcesError(err) {
		t.Fatalf("Error gettings creds: %s", err)
	}

	if err == nil {
		t.Fatal("Expected error returned when getting creds w/ invalid EC2 endpoint")
	}
}

func TestAWSGetCredentials_shouldIgnoreInvalidEndpoint(t *testing.T) {
	resetEnv := unsetEnv(t)
	defer resetEnv()
	// capture the test server's close method, to call after the test returns
	ts := invalidAwsEnv()
	defer ts()

	creds, err := GetCredentials(&Config{AccessKey: "accessKey", SecretKey: "secretKey"})
	if err != nil {
		t.Fatalf("Error gettings creds: %s", err)
	}
	v, err := creds.Get()
	if err != nil {
		t.Fatalf("Getting static credentials w/ invalid EC2 endpoint failed: %s", err)
	}
	if creds == nil {
		t.Fatal("Expected a static creds provider to be returned")
	}

	if v.ProviderName != "StaticProvider" {
		t.Fatalf("Expected provider name to be %q, %q given", "StaticProvider", v.ProviderName)
	}

	if v.AccessKeyID != "accessKey" {
		t.Fatalf("Static Access Key %q doesn't match: %s", "accessKey", v.AccessKeyID)
	}

	if v.SecretAccessKey != "secretKey" {
		t.Fatalf("Static Secret Key %q doesn't match: %s", "secretKey", v.SecretAccessKey)
	}
}

func TestAWSGetCredentials_shouldCatchEC2RoleProvider(t *testing.T) {
	resetEnv := unsetEnv(t)
	defer resetEnv()
	// capture the test server's close method, to call after the test returns
	ts := awsMetadataApiMock(append(ec2metadata_securityCredentialsEndpoints, ec2metadata_instanceIdEndpoint, ec2metadata_iamInfoEndpoint))
	defer ts()

	creds, err := GetCredentials(&Config{})
	if err != nil {
		t.Fatalf("Error gettings creds: %s", err)
	}
	if creds == nil {
		t.Fatal("Expected an EC2Role creds provider to be returned")
	}

	v, err := creds.Get()
	if err != nil {
		t.Fatalf("Expected no error when getting creds: %s", err)
	}
	expectedProvider := "EC2RoleProvider"
	if v.ProviderName != expectedProvider {
		t.Fatalf("Expected provider name to be %q, %q given",
			expectedProvider, v.ProviderName)
	}
}

var credentialsFileContents = `[myprofile]
aws_access_key_id = accesskey
aws_secret_access_key = secretkey
`

func TestAWSGetCredentials_shouldBeShared(t *testing.T) {
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

	defer os.Remove(file.Name())

	resetEnv := unsetEnv(t)
	defer resetEnv()

	if err := os.Setenv("AWS_PROFILE", "myprofile"); err != nil {
		t.Fatalf("Error resetting env var AWS_PROFILE: %s", err)
	}
	if err := os.Setenv("AWS_SHARED_CREDENTIALS_FILE", file.Name()); err != nil {
		t.Fatalf("Error resetting env var AWS_SHARED_CREDENTIALS_FILE: %s", err)
	}

	creds, err := GetCredentials(&Config{Profile: "myprofile", CredsFilename: file.Name()})
	if err != nil {
		t.Fatalf("Error gettings creds: %s", err)
	}
	if creds == nil {
		t.Fatal("Expected a provider chain to be returned")
	}

	v, err := creds.Get()
	if err != nil {
		t.Fatalf("Error gettings creds: %s", err)
	}

	if v.AccessKeyID != "accesskey" {
		t.Fatalf("AccessKeyID mismatch, expected (%s), got (%s)", "accesskey", v.AccessKeyID)
	}

	if v.SecretAccessKey != "secretkey" {
		t.Fatalf("SecretAccessKey mismatch, expected (%s), got (%s)", "accesskey", v.AccessKeyID)
	}
}

func TestAWSGetCredentials_shouldBeENV(t *testing.T) {
	// need to set the environment variables to a dummy string, as we don't know
	// what they may be at runtime without hardcoding here
	s := "some_env"
	resetEnv := setEnv(s, t)

	defer resetEnv()

	cfg := Config{}
	creds, err := GetCredentials(&cfg)
	if err != nil {
		t.Fatalf("Error gettings creds: %s", err)
	}
	if creds == nil {
		t.Fatalf("Expected a static creds provider to be returned")
	}

	v, err := creds.Get()
	if err != nil {
		t.Fatalf("Error gettings creds: %s", err)
	}
	if v.AccessKeyID != s {
		t.Fatalf("AccessKeyID mismatch, expected: (%s), got (%s)", s, v.AccessKeyID)
	}
	if v.SecretAccessKey != s {
		t.Fatalf("SecretAccessKey mismatch, expected: (%s), got (%s)", s, v.SecretAccessKey)
	}
	if v.SessionToken != s {
		t.Fatalf("SessionToken mismatch, expected: (%s), got (%s)", s, v.SessionToken)
	}
}

// invalidAwsEnv establishes a httptest server to simulate behaviour
// when endpoint doesn't respond as expected
func invalidAwsEnv() func() {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(400)
	}))

	os.Setenv("AWS_METADATA_URL", ts.URL+"/latest")
	return ts.Close
}

// unsetEnv unsets environment variables for testing a "clean slate" with no
// credentials in the environment
func unsetEnv(t *testing.T) func() {
	// Grab any existing AWS keys and preserve. In some tests we'll unset these, so
	// we need to have them and restore them after
	e := getEnv()
	if err := os.Unsetenv("AWS_ACCESS_KEY_ID"); err != nil {
		t.Fatalf("Error unsetting env var AWS_ACCESS_KEY_ID: %s", err)
	}
	if err := os.Unsetenv("AWS_SECRET_ACCESS_KEY"); err != nil {
		t.Fatalf("Error unsetting env var AWS_SECRET_ACCESS_KEY: %s", err)
	}
	if err := os.Unsetenv("AWS_SESSION_TOKEN"); err != nil {
		t.Fatalf("Error unsetting env var AWS_SESSION_TOKEN: %s", err)
	}
	if err := os.Unsetenv("AWS_PROFILE"); err != nil {
		t.Fatalf("Error unsetting env var AWS_PROFILE: %s", err)
	}
	if err := os.Unsetenv("AWS_SHARED_CREDENTIALS_FILE"); err != nil {
		t.Fatalf("Error unsetting env var AWS_SHARED_CREDENTIALS_FILE: %s", err)
	}
	// The Shared Credentials Provider has a very reasonable fallback option of
	// checking the user's home directory for credentials, which may create
	// unexpected results for users running these tests
	os.Setenv("HOME", "/dev/null")

	return func() {
		// re-set all the envs we unset above
		if err := os.Setenv("AWS_ACCESS_KEY_ID", e.Key); err != nil {
			t.Fatalf("Error resetting env var AWS_ACCESS_KEY_ID: %s", err)
		}
		if err := os.Setenv("AWS_SECRET_ACCESS_KEY", e.Secret); err != nil {
			t.Fatalf("Error resetting env var AWS_SECRET_ACCESS_KEY: %s", err)
		}
		if err := os.Setenv("AWS_SESSION_TOKEN", e.Token); err != nil {
			t.Fatalf("Error resetting env var AWS_SESSION_TOKEN: %s", err)
		}
		if err := os.Setenv("AWS_PROFILE", e.Profile); err != nil {
			t.Fatalf("Error resetting env var AWS_PROFILE: %s", err)
		}
		if err := os.Setenv("AWS_SHARED_CREDENTIALS_FILE", e.CredsFilename); err != nil {
			t.Fatalf("Error resetting env var AWS_SHARED_CREDENTIALS_FILE: %s", err)
		}
		if err := os.Setenv("HOME", e.Home); err != nil {
			t.Fatalf("Error resetting env var HOME: %s", err)
		}
	}
}

func setEnv(s string, t *testing.T) func() {
	e := getEnv()
	// Set all the envs to a dummy value
	if err := os.Setenv("AWS_ACCESS_KEY_ID", s); err != nil {
		t.Fatalf("Error setting env var AWS_ACCESS_KEY_ID: %s", err)
	}
	if err := os.Setenv("AWS_SECRET_ACCESS_KEY", s); err != nil {
		t.Fatalf("Error setting env var AWS_SECRET_ACCESS_KEY: %s", err)
	}
	if err := os.Setenv("AWS_SESSION_TOKEN", s); err != nil {
		t.Fatalf("Error setting env var AWS_SESSION_TOKEN: %s", err)
	}
	if err := os.Setenv("AWS_PROFILE", s); err != nil {
		t.Fatalf("Error setting env var AWS_PROFILE: %s", err)
	}
	if err := os.Setenv("AWS_SHARED_CREDENTIALS_FILE", s); err != nil {
		t.Fatalf("Error setting env var AWS_SHARED_CREDENTIALS_FLE: %s", err)
	}

	return func() {
		// re-set all the envs we unset above
		if err := os.Setenv("AWS_ACCESS_KEY_ID", e.Key); err != nil {
			t.Fatalf("Error resetting env var AWS_ACCESS_KEY_ID: %s", err)
		}
		if err := os.Setenv("AWS_SECRET_ACCESS_KEY", e.Secret); err != nil {
			t.Fatalf("Error resetting env var AWS_SECRET_ACCESS_KEY: %s", err)
		}
		if err := os.Setenv("AWS_SESSION_TOKEN", e.Token); err != nil {
			t.Fatalf("Error resetting env var AWS_SESSION_TOKEN: %s", err)
		}
		if err := os.Setenv("AWS_PROFILE", e.Profile); err != nil {
			t.Fatalf("Error setting env var AWS_PROFILE: %s", err)
		}
		if err := os.Setenv("AWS_SHARED_CREDENTIALS_FILE", s); err != nil {
			t.Fatalf("Error setting env var AWS_SHARED_CREDENTIALS_FLE: %s", err)
		}
	}
}

func getEnv() *currentEnv {
	// Grab any existing AWS keys and preserve. In some tests we'll unset these, so
	// we need to have them and restore them after
	return &currentEnv{
		Key:           os.Getenv("AWS_ACCESS_KEY_ID"),
		Secret:        os.Getenv("AWS_SECRET_ACCESS_KEY"),
		Token:         os.Getenv("AWS_SESSION_TOKEN"),
		Profile:       os.Getenv("AWS_PROFILE"),
		CredsFilename: os.Getenv("AWS_SHARED_CREDENTIALS_FILE"),
		Home:          os.Getenv("HOME"),
	}
}

// struct to preserve the current environment
type currentEnv struct {
	Key, Secret, Token, Profile, CredsFilename, Home string
}
