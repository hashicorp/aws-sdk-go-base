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
	Description          string
	EnvironmentVariables map[string]string
	ExpectedUserAgent    string
}

func TestUserAgentProducts(t *testing.T, awsSdkGoUserAgent func() string, testUserAgentProducts func(t *testing.T, testCase UserAgentTestCase)) {
	testCases := []UserAgentTestCase{
		{
			Config: &config.Config{
				AccessKey: servicemocks.MockStaticAccessKey,
				Region:    "us-east-1",
				SecretKey: servicemocks.MockStaticSecretKey,
			},
			Description:       "standard User-Agent",
			ExpectedUserAgent: awsSdkGoUserAgent(),
		},
		{
			Config: &config.Config{
				AccessKey: servicemocks.MockStaticAccessKey,
				Region:    "us-east-1",
				SecretKey: servicemocks.MockStaticSecretKey,
			},
			Description: "customized User-Agent TF_APPEND_USER_AGENT",
			EnvironmentVariables: map[string]string{
				constants.AppendUserAgentEnvVar: "Last",
			},
			ExpectedUserAgent: awsSdkGoUserAgent() + " Last",
		},
		{
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
			Description:       "APN User-Agent Products",
			ExpectedUserAgent: "APN/1.0 partner/1.0 first/1.2.3 second/1.0.2 (a comment) " + awsSdkGoUserAgent(),
		},
		{
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
			Description: "APN User-Agent Products and TF_APPEND_USER_AGENT",
			EnvironmentVariables: map[string]string{
				constants.AppendUserAgentEnvVar: "Last",
			},
			ExpectedUserAgent: "APN/1.0 partner/1.0 first/1.2.3 second/1.0.2 " + awsSdkGoUserAgent() + " Last",
		},
		{
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
			Description:       "User-Agent Products",
			ExpectedUserAgent: awsSdkGoUserAgent() + " first/1.2.3 second/1.0.2 (a comment)",
		},
		{
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
			Description:       "APN and User-Agent Products",
			ExpectedUserAgent: "APN/1.0 partner/1.0 first/1.2.3 second/1.0.2 (a comment) " + awsSdkGoUserAgent() + " third/4.5.6 fourth/2.1",
		},
	}

	for _, testCase := range testCases {
		testCase := testCase

		t.Run(testCase.Description, func(t *testing.T) {
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
