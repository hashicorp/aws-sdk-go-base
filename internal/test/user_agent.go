package test

import (
	"os"
	"testing"

	"github.com/hashicorp/aws-sdk-go-base/v2/internal/config"
	"github.com/hashicorp/aws-sdk-go-base/v2/internal/constants"
	"github.com/hashicorp/aws-sdk-go-base/v2/servicemocks"
)

type UserAgentTestCase struct {
	Config               *config.Config
	EnvironmentVariables map[string]string
	ExpectedUserAgent    string
}

func TestUserAgentProducts(t *testing.T, awsSdkGoUserAgent func() string, testUserAgentProducts func(t *testing.T, testCase UserAgentTestCase)) {
	testCases := map[string]UserAgentTestCase{
		"standard User-Agent": {
			Config: &config.Config{
				AccessKey: servicemocks.MockStaticAccessKey,
				Region:    "us-east-1",
				SecretKey: servicemocks.MockStaticSecretKey,
			},
			ExpectedUserAgent: awsSdkGoUserAgent(),
		},
		"customized User-Agent TF_APPEND_USER_AGENT": {
			Config: &config.Config{
				AccessKey: servicemocks.MockStaticAccessKey,
				Region:    "us-east-1",
				SecretKey: servicemocks.MockStaticSecretKey,
			},
			EnvironmentVariables: map[string]string{
				constants.AppendUserAgentEnvVar: "Last",
			},
			ExpectedUserAgent: awsSdkGoUserAgent() + " Last",
		},
		"APN User-Agent Products": {
			Config: &config.Config{
				AccessKey: servicemocks.MockStaticAccessKey,
				Region:    "us-east-1",
				SecretKey: servicemocks.MockStaticSecretKey,
				APNInfo: &config.APNInfo{
					PartnerName: "partner",
					Products: []config.UserAgentProduct{
						{
							Name:    "first",
							Version: "1.2.3",
						},
						{
							Name:    "second",
							Version: "1.0.2",
							Comment: "a comment",
						},
					},
				},
			},
			ExpectedUserAgent: "APN/1.0 partner/1.0 first/1.2.3 second/1.0.2 (a comment) " + awsSdkGoUserAgent(),
		},
		"APN User-Agent Products and TF_APPEND_USER_AGENT": {
			Config: &config.Config{
				AccessKey: servicemocks.MockStaticAccessKey,
				Region:    "us-east-1",
				SecretKey: servicemocks.MockStaticSecretKey,
				APNInfo: &config.APNInfo{
					PartnerName: "partner",
					Products: []config.UserAgentProduct{
						{
							Name:    "first",
							Version: "1.2.3",
						},
						{
							Name:    "second",
							Version: "1.0.2",
						},
					},
				},
			},
			EnvironmentVariables: map[string]string{
				constants.AppendUserAgentEnvVar: "Last",
			},
			ExpectedUserAgent: "APN/1.0 partner/1.0 first/1.2.3 second/1.0.2 " + awsSdkGoUserAgent() + " Last",
		},
		"User-Agent Products": {
			Config: &config.Config{
				AccessKey: servicemocks.MockStaticAccessKey,
				Region:    "us-east-1",
				SecretKey: servicemocks.MockStaticSecretKey,
				UserAgent: []config.UserAgentProduct{
					{
						Name:    "first",
						Version: "1.2.3",
					},
					{
						Name:    "second",
						Version: "1.0.2",
						Comment: "a comment",
					},
				},
			},
			ExpectedUserAgent: awsSdkGoUserAgent() + " first/1.2.3 second/1.0.2 (a comment)",
		},
		"APN and User-Agent Products": {
			Config: &config.Config{
				AccessKey: servicemocks.MockStaticAccessKey,
				Region:    "us-east-1",
				SecretKey: servicemocks.MockStaticSecretKey,
				APNInfo: &config.APNInfo{
					PartnerName: "partner",
					Products: []config.UserAgentProduct{
						{
							Name:    "first",
							Version: "1.2.3",
						},
						{
							Name:    "second",
							Version: "1.0.2",
							Comment: "a comment",
						},
					},
				},
				UserAgent: []config.UserAgentProduct{
					{
						Name:    "third",
						Version: "4.5.6",
					},
					{
						Name:    "fourth",
						Version: "2.1",
					},
				},
			},
			ExpectedUserAgent: "APN/1.0 partner/1.0 first/1.2.3 second/1.0.2 (a comment) " + awsSdkGoUserAgent() + " third/4.5.6 fourth/2.1",
		},
	}

	for name, testCase := range testCases {
		testCase := testCase

		t.Run(name, func(t *testing.T) {
			oldEnv := servicemocks.InitSessionTestEnv()
			defer servicemocks.PopEnv(oldEnv)

			for k, v := range testCase.EnvironmentVariables {
				os.Setenv(k, v)
			}

			testCase.Config.SkipCredsValidation = true

			testUserAgentProducts(t, testCase)
		})
	}
}