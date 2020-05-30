package awsbase

import (
	"os"
	"strings"
	"testing"
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

func TestGetSessionWithBlankConfig(t *testing.T) {
	oldEnv := initSessionTestEnv()
	defer PopEnv(oldEnv)

	_, err := GetSession(&Config{})
	if err == nil {
		t.Fatal("GetSession(&Config{}) with an empty config should result in an error")
	}
}

func TestGetSessionWithCreds(t *testing.T) {
	oldEnv := initSessionTestEnv()
	defer PopEnv(oldEnv)

	sess, err := GetSession(&Config{
		AccessKey:            "MockAccessKey",
		SecretKey:            "MockSecretKey",
		SkipCredsValidation:  true,
		SkipMetadataApiCheck: true,
		MaxRetries:           1,
		UserAgentProducts:    []*UserAgentProduct{{}},
	})
	if err != nil {
		t.Fatalf("GetSession(&Config{...}) should return a valid session, but got the error %s", err)
	}

	if sess == nil {
		t.Error("GetSession(...) resulted in a nil session")
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
			"", "", "No valid credential sources found for AWS Provider."},
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
