// Copyright IBM Corp. 2015, 2026
// SPDX-License-Identifier: MPL-2.0

package servicemocks

import (
	"os"
	"testing"
)

// The mocks set an environment variable to point the AWS SDK at their test
// server. The variable must not outlive the returned close function, or every
// later test in the package sees an endpoint whose server has been closed.
func TestApiMocksRestoreEnvironment(t *testing.T) {
	tests := map[string]struct {
		key  string
		mock func() func()
	}{
		"AwsMetadataApiMock": {
			key:  "AWS_EC2_METADATA_SERVICE_ENDPOINT",
			mock: func() func() { return AwsMetadataApiMock(Ec2metadata_securityCredentialsEndpoints) },
		},
		"EcsCredentialsApiMock": {
			key:  "AWS_CONTAINER_CREDENTIALS_FULL_URI",
			mock: EcsCredentialsApiMock,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			t.Run("restores a previous value", func(t *testing.T) {
				const previous = "http://previous.example.test"
				t.Setenv(test.key, previous)

				closeMock := test.mock()
				if v := os.Getenv(test.key); v == previous {
					t.Fatalf("expected %s to point at the mock server, still %q", test.key, v)
				}
				closeMock()

				if a, e := os.Getenv(test.key), previous; a != e {
					t.Errorf("expected %s to be restored to %q, got %q", test.key, e, a)
				}
			})

			t.Run("unsets a variable that was not set", func(t *testing.T) {
				t.Setenv(test.key, "placeholder")
				os.Unsetenv(test.key)

				closeMock := test.mock()
				closeMock()

				if v, ok := os.LookupEnv(test.key); ok {
					t.Errorf("expected %s to be unset, got %q", test.key, v)
				}
			})
		})
	}
}
