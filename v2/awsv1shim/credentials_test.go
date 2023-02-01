// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package awsv1shim

import (
	"context"
	"fmt"
	"testing"
	"time"

	awsv2 "github.com/aws/aws-sdk-go-v2/aws"
	credentialsv2 "github.com/aws/aws-sdk-go-v2/credentials"
	stscredsv2 "github.com/aws/aws-sdk-go-v2/credentials/stscreds"
	stsv2 "github.com/aws/aws-sdk-go-v2/service/sts"
	ststypesv2 "github.com/aws/aws-sdk-go-v2/service/sts/types"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/hashicorp/aws-sdk-go-base/v2/internal/test"
)

func TestV2CredentialsProviderPassthrough(t *testing.T) {
	ctx := test.Context(t)

	v2creds := credentialsv2.NewStaticCredentialsProvider("key", "secret", "session")

	creds := newV2Credentials(v2creds)

	value, err := creds.GetWithContext(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}

	if a, e := value.AccessKeyID, "key"; a != e {
		t.Errorf("AccessKeyID: expected %q, got %q", e, a)
	}
	if a, e := value.SecretAccessKey, "secret"; a != e {
		t.Errorf("SecretAccessKey: expected %q, got %q", e, a)
	}
	if a, e := value.SessionToken, "session"; a != e {
		t.Errorf("SecretAccessKey: expected %q, got %q", e, a)
	}
	if a, e := value.ProviderName, fmt.Sprintf("v2Credentials(%s)", credentialsv2.StaticCredentialsName); a != e {
		t.Errorf("ProviderName: expected %q, got %q", e, a)
	}
}

func TestV2CredentialsProviderExpriry(t *testing.T) {
	testcases := map[string]struct {
		v2creds awsv2.CredentialsProvider
	}{
		credentialsv2.StaticCredentialsName: {
			v2creds: credentialsv2.NewStaticCredentialsProvider("key", "secret", "session"),
		},
	}

	for name, testcase := range testcases {
		t.Run(name, func(t *testing.T) {
			ctx := test.Context(t)

			creds := newV2Credentials(testcase.v2creds)

			// Credentials need to be retrieved before we can check
			_, err := creds.GetWithContext(ctx)
			if err != nil {
				t.Fatalf("unexpected error: %s", err)
			}
			if creds.IsExpired() {
				t.Fatalf("did not expect creds to be expired")
			}
			expiry, err := creds.ExpiresAt()
			if err != nil {
				t.Fatalf("unexpected error getting expiry: %s", err)
			}
			if !expiry.Equal(time.Time{}) {
				t.Fatalf("expected no expiry time, got %s", expiry)
			}

			creds.Expire()
			if !creds.IsExpired() {
				t.Fatalf("expected creds to be expired")
			}

			value, err := creds.GetWithContext(ctx)
			if err != nil {
				t.Fatalf("unexpected error: %s", err)
			}
			if value.AccessKeyID == "" {
				t.Error("AccessKeyID: expected a value")
			}
			if value.SecretAccessKey == "" {
				t.Error("SecretAccessKey: expected a value")
			}
			if value.SessionToken == "" {
				t.Error("SessionToken: expected a value")
			}
		})
	}
}

func TestV2CredentialsProviderExpriry_AssumeRole(t *testing.T) {
	ctx := test.Context(t)

	stsClient := &mockAssumeRole{}
	v2creds := stscredsv2.NewAssumeRoleProvider(stsClient, "role")

	creds := newV2Credentials(v2creds)

	// Credentials need to be retrieved before we can check expiry information
	_, err := creds.GetWithContext(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	if creds.IsExpired() {
		t.Fatalf("did not expect creds to be expired")
	}
	expiry, err := creds.ExpiresAt()
	if err != nil {
		t.Fatalf("unexpected error getting expiry: %s", err)
	}
	if expiry.Equal(time.Time{}) {
		t.Fatal("expected expiry time, got none")
	}

	creds.Expire()
	if !creds.IsExpired() {
		t.Fatalf("expected creds to be expired")
	}

	value, err := creds.GetWithContext(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	if value.AccessKeyID == "" {
		t.Error("AccessKeyID: expected a value")
	}
	if value.SecretAccessKey == "" {
		t.Error("SecretAccessKey: expected a value")
	}
	if value.SessionToken == "" {
		t.Error("SessionToken: expected a value")
	}
}

func TestV2CredentialsProviderCaching(t *testing.T) {
	ctx := test.Context(t)

	stsClientCalls := 0
	expectedStsClientCalls := 0
	stsClient := &mockAssumeRole{
		TestInput: func(in *stsv2.AssumeRoleInput) {
			stsClientCalls++
		},
	}
	v2creds := stscredsv2.NewAssumeRoleProvider(stsClient, "role")
	creds := newV2Credentials(v2creds)
	if stsClientCalls != expectedStsClientCalls {
		t.Errorf("did not expect call to STS client")
		expectedStsClientCalls = stsClientCalls
	}

	_, err := creds.GetWithContext(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	expectedStsClientCalls++
	if stsClientCalls != expectedStsClientCalls {
		t.Errorf("expected call to STS client")
		expectedStsClientCalls = stsClientCalls
	}

	_, err = creds.GetWithContext(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	if stsClientCalls != expectedStsClientCalls {
		t.Errorf("did not expect call to STS client")
		expectedStsClientCalls = stsClientCalls
	}

	creds.IsExpired()
	if stsClientCalls != expectedStsClientCalls {
		t.Errorf("did not expect call to STS client")
		expectedStsClientCalls = stsClientCalls
	}

	_, err = creds.ExpiresAt()
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	if stsClientCalls != expectedStsClientCalls {
		t.Errorf("did not expect call to STS client")
		expectedStsClientCalls = stsClientCalls
	}

	creds.Expire()
	if stsClientCalls != expectedStsClientCalls {
		t.Errorf("did not expect call to STS client")
		expectedStsClientCalls = stsClientCalls
	}

	creds.IsExpired()
	if stsClientCalls != expectedStsClientCalls {
		t.Errorf("did not expect call to STS client")
		expectedStsClientCalls = stsClientCalls
	}

	_, err = creds.ExpiresAt()
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	if stsClientCalls != expectedStsClientCalls {
		t.Errorf("did not expect call to STS client")
		expectedStsClientCalls = stsClientCalls
	}

	_, err = creds.GetWithContext(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	expectedStsClientCalls++
	if stsClientCalls != expectedStsClientCalls {
		t.Errorf("expected call to STS client")
	}
}

type mockAssumeRole struct {
	TestInput func(*stsv2.AssumeRoleInput)
}

func (s *mockAssumeRole) AssumeRole(ctx context.Context, params *stsv2.AssumeRoleInput, optFns ...func(*stsv2.Options)) (*stsv2.AssumeRoleOutput, error) {
	if s.TestInput != nil {
		s.TestInput(params)
	}
	expiry := time.Now().Add(60 * time.Minute)

	return &stsv2.AssumeRoleOutput{
		Credentials: &ststypesv2.Credentials{
			// Just reflect the role arn to the provider.
			AccessKeyId:     params.RoleArn,
			SecretAccessKey: aws.String("assumedSecretAccessKey"),
			SessionToken:    aws.String("assumedSessionToken"),
			Expiration:      &expiry,
		},
	}, nil
}
