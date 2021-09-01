package awsv1shim

import (
	"net/http"
	"testing"

	"github.com/aws/aws-sdk-go/aws/credentials/ec2rolecreds"
	"github.com/aws/aws-sdk-go/service/iam"
	"github.com/aws/aws-sdk-go/service/sts"
	"github.com/hashicorp/aws-sdk-go-base/awsmocks"
)

func TestGetAccountIDAndPartition(t *testing.T) {
	var testCases = []struct {
		Description          string
		AuthProviderName     string
		EC2MetadataEndpoints []*awsmocks.MetadataResponse
		IAMEndpoints         []*awsmocks.MockEndpoint
		STSEndpoints         []*awsmocks.MockEndpoint
		ErrCount             int
		ExpectedAccountID    string
		ExpectedPartition    string
	}{
		{
			Description:          "EC2 Metadata over iam:GetUser when using EC2 Instance Profile",
			AuthProviderName:     ec2rolecreds.ProviderName,
			EC2MetadataEndpoints: append(awsmocks.Ec2metadata_securityCredentialsEndpoints, awsmocks.Ec2metadata_instanceIdEndpoint, awsmocks.Ec2metadata_iamInfoEndpoint),

			IAMEndpoints: []*awsmocks.MockEndpoint{
				{
					Request:  &awsmocks.MockRequest{Method: "POST", Uri: "/", Body: "Action=GetUser&Version=2010-05-08"},
					Response: &awsmocks.MockResponse{StatusCode: http.StatusOK, Body: awsmocks.IamResponse_GetUser_valid, ContentType: "text/xml"},
				},
			},
			ExpectedAccountID: awsmocks.Ec2metadata_iamInfoEndpoint_expectedAccountID,
			ExpectedPartition: awsmocks.Ec2metadata_iamInfoEndpoint_expectedPartition,
		},
		{
			Description:          "Mimic the metadata service mocked by Hologram (https://github.com/AdRoll/hologram)",
			AuthProviderName:     ec2rolecreds.ProviderName,
			EC2MetadataEndpoints: awsmocks.Ec2metadata_securityCredentialsEndpoints,
			IAMEndpoints: []*awsmocks.MockEndpoint{
				{
					Request:  &awsmocks.MockRequest{Method: "POST", Uri: "/", Body: "Action=GetUser&Version=2010-05-08"},
					Response: &awsmocks.MockResponse{StatusCode: http.StatusForbidden, Body: awsmocks.IamResponse_GetUser_unauthorized, ContentType: "text/xml"},
				},
			},
			STSEndpoints: []*awsmocks.MockEndpoint{
				awsmocks.MockStsGetCallerIdentityValidEndpoint,
			},
			ExpectedAccountID: awsmocks.MockStsGetCallerIdentityAccountID,
			ExpectedPartition: awsmocks.MockStsGetCallerIdentityPartition,
		},
		{
			Description: "iam:ListRoles if iam:GetUser AccessDenied and sts:GetCallerIdentity fails",
			IAMEndpoints: []*awsmocks.MockEndpoint{
				{
					Request:  &awsmocks.MockRequest{Method: "POST", Uri: "/", Body: "Action=GetUser&Version=2010-05-08"},
					Response: &awsmocks.MockResponse{StatusCode: http.StatusForbidden, Body: awsmocks.IamResponse_GetUser_unauthorized, ContentType: "text/xml"},
				},
				{
					Request:  &awsmocks.MockRequest{Method: "POST", Uri: "/", Body: "Action=ListRoles&MaxItems=1&Version=2010-05-08"},
					Response: &awsmocks.MockResponse{StatusCode: http.StatusOK, Body: awsmocks.IamResponse_ListRoles_valid, ContentType: "text/xml"},
				},
			},
			STSEndpoints: []*awsmocks.MockEndpoint{
				awsmocks.MockStsGetCallerIdentityInvalidEndpointAccessDenied,
			},
			ExpectedAccountID: awsmocks.IamResponse_ListRoles_valid_expectedAccountID,
			ExpectedPartition: awsmocks.IamResponse_ListRoles_valid_expectedPartition,
		},
		{
			Description: "iam:ListRoles if iam:GetUser ValidationError and sts:GetCallerIdentity fails",
			IAMEndpoints: []*awsmocks.MockEndpoint{
				{
					Request:  &awsmocks.MockRequest{Method: "POST", Uri: "/", Body: "Action=GetUser&Version=2010-05-08"},
					Response: &awsmocks.MockResponse{StatusCode: http.StatusBadRequest, Body: awsmocks.IamResponse_GetUser_federatedFailure, ContentType: "text/xml"},
				},
				{
					Request:  &awsmocks.MockRequest{Method: "POST", Uri: "/", Body: "Action=ListRoles&MaxItems=1&Version=2010-05-08"},
					Response: &awsmocks.MockResponse{StatusCode: http.StatusOK, Body: awsmocks.IamResponse_ListRoles_valid, ContentType: "text/xml"},
				},
			},
			STSEndpoints: []*awsmocks.MockEndpoint{
				awsmocks.MockStsGetCallerIdentityInvalidEndpointAccessDenied,
			},
			ExpectedAccountID: awsmocks.IamResponse_ListRoles_valid_expectedAccountID,
			ExpectedPartition: awsmocks.IamResponse_ListRoles_valid_expectedPartition,
		},
		{
			Description: "Error when all endpoints fail",
			IAMEndpoints: []*awsmocks.MockEndpoint{
				{
					Request:  &awsmocks.MockRequest{Method: "POST", Uri: "/", Body: "Action=GetUser&Version=2010-05-08"},
					Response: &awsmocks.MockResponse{StatusCode: http.StatusBadRequest, Body: awsmocks.IamResponse_GetUser_federatedFailure, ContentType: "text/xml"},
				},
				{
					Request:  &awsmocks.MockRequest{Method: "POST", Uri: "/", Body: "Action=ListRoles&MaxItems=1&Version=2010-05-08"},
					Response: &awsmocks.MockResponse{StatusCode: http.StatusForbidden, Body: awsmocks.IamResponse_ListRoles_unauthorized, ContentType: "text/xml"},
				},
			},
			STSEndpoints: []*awsmocks.MockEndpoint{
				awsmocks.MockStsGetCallerIdentityInvalidEndpointAccessDenied,
			},
			ErrCount: 1,
		},
	}

	for _, testCase := range testCases {
		testCase := testCase

		t.Run(testCase.Description, func(t *testing.T) {
			resetEnv := awsmocks.UnsetEnv(t)
			defer resetEnv()
			// capture the test server's close method, to call after the test returns
			awsTs := awsmocks.AwsMetadataApiMock(testCase.EC2MetadataEndpoints)
			defer awsTs()

			closeIam, iamSess, err := awsmocks.GetMockedAwsApiSession("IAM", testCase.IAMEndpoints)
			defer closeIam()
			if err != nil {
				t.Fatal(err)
			}

			closeSts, stsSess, err := awsmocks.GetMockedAwsApiSession("STS", testCase.STSEndpoints)
			defer closeSts()
			if err != nil {
				t.Fatal(err)
			}

			iamConn := iam.New(iamSess)
			stsConn := sts.New(stsSess)

			accountID, partition, err := getAccountIDAndPartition(iamConn, stsConn, testCase.AuthProviderName)
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
		resetEnv := awsmocks.UnsetEnv(t)
		defer resetEnv()
		// capture the test server's close method, to call after the test returns
		awsTs := awsmocks.AwsMetadataApiMock(append(awsmocks.Ec2metadata_securityCredentialsEndpoints, awsmocks.Ec2metadata_instanceIdEndpoint, awsmocks.Ec2metadata_iamInfoEndpoint))
		defer awsTs()

		id, partition, err := getAccountIDAndPartitionFromEC2Metadata()
		if err != nil {
			t.Fatalf("Getting account ID from EC2 metadata API failed: %s", err)
		}

		if id != awsmocks.Ec2metadata_iamInfoEndpoint_expectedAccountID {
			t.Fatalf("Expected account ID: %s, given: %s", awsmocks.Ec2metadata_iamInfoEndpoint_expectedAccountID, id)
		}
		if partition != awsmocks.Ec2metadata_iamInfoEndpoint_expectedPartition {
			t.Fatalf("Expected partition: %s, given: %s", awsmocks.Ec2metadata_iamInfoEndpoint_expectedPartition, partition)
		}
	})
}

func TestGetAccountIDAndPartitionFromIAMGetUser(t *testing.T) {
	var testCases = []struct {
		Description       string
		MockEndpoints     []*awsmocks.MockEndpoint
		ErrCount          int
		ExpectedAccountID string
		ExpectedPartition string
	}{
		{
			Description: "Ignore iam:GetUser failure with federated user",
			MockEndpoints: []*awsmocks.MockEndpoint{
				{
					Request:  &awsmocks.MockRequest{Method: "POST", Uri: "/", Body: "Action=GetUser&Version=2010-05-08"},
					Response: &awsmocks.MockResponse{StatusCode: http.StatusBadRequest, Body: awsmocks.IamResponse_GetUser_federatedFailure, ContentType: "text/xml"},
				},
			},
			ErrCount: 0,
		},
		{
			Description: "Ignore iam:GetUser failure with unauthorized user",
			MockEndpoints: []*awsmocks.MockEndpoint{
				{
					Request:  &awsmocks.MockRequest{Method: "POST", Uri: "/", Body: "Action=GetUser&Version=2010-05-08"},
					Response: &awsmocks.MockResponse{StatusCode: http.StatusForbidden, Body: awsmocks.IamResponse_GetUser_unauthorized, ContentType: "text/xml"},
				},
			},
			ErrCount: 0,
		},
		{
			Description: "iam:GetUser success",
			MockEndpoints: []*awsmocks.MockEndpoint{
				{
					Request:  &awsmocks.MockRequest{Method: "POST", Uri: "/", Body: "Action=GetUser&Version=2010-05-08"},
					Response: &awsmocks.MockResponse{StatusCode: http.StatusOK, Body: awsmocks.IamResponse_GetUser_valid, ContentType: "text/xml"},
				},
			},
			ExpectedAccountID: awsmocks.IamResponse_GetUser_valid_expectedAccountID,
			ExpectedPartition: awsmocks.IamResponse_GetUser_valid_expectedPartition,
		},
	}

	for _, testCase := range testCases {
		testCase := testCase

		t.Run(testCase.Description, func(t *testing.T) {
			closeIam, iamSess, err := awsmocks.GetMockedAwsApiSession("IAM", testCase.MockEndpoints)
			defer closeIam()
			if err != nil {
				t.Fatal(err)
			}

			iamConn := iam.New(iamSess)

			accountID, partition, err := getAccountIDAndPartitionFromIAMGetUser(iamConn)
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
		MockEndpoints     []*awsmocks.MockEndpoint
		ErrCount          int
		ExpectedAccountID string
		ExpectedPartition string
	}{
		{
			Description: "iam:ListRoles unauthorized",
			MockEndpoints: []*awsmocks.MockEndpoint{
				{
					Request:  &awsmocks.MockRequest{Method: "POST", Uri: "/", Body: "Action=ListRoles&MaxItems=1&Version=2010-05-08"},
					Response: &awsmocks.MockResponse{StatusCode: http.StatusForbidden, Body: awsmocks.IamResponse_ListRoles_unauthorized, ContentType: "text/xml"},
				},
			},
			ErrCount: 1,
		},
		{
			Description: "iam:ListRoles success",
			MockEndpoints: []*awsmocks.MockEndpoint{
				{
					Request:  &awsmocks.MockRequest{Method: "POST", Uri: "/", Body: "Action=ListRoles&MaxItems=1&Version=2010-05-08"},
					Response: &awsmocks.MockResponse{StatusCode: http.StatusOK, Body: awsmocks.IamResponse_ListRoles_valid, ContentType: "text/xml"},
				},
			},
			ExpectedAccountID: awsmocks.IamResponse_ListRoles_valid_expectedAccountID,
			ExpectedPartition: awsmocks.IamResponse_ListRoles_valid_expectedPartition,
		},
	}

	for _, testCase := range testCases {
		testCase := testCase

		t.Run(testCase.Description, func(t *testing.T) {
			closeIam, iamSess, err := awsmocks.GetMockedAwsApiSession("IAM", testCase.MockEndpoints)
			defer closeIam()
			if err != nil {
				t.Fatal(err)
			}

			iamConn := iam.New(iamSess)

			accountID, partition, err := getAccountIDAndPartitionFromIAMListRoles(iamConn)
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
		MockEndpoints     []*awsmocks.MockEndpoint
		ErrCount          int
		ExpectedAccountID string
		ExpectedPartition string
	}{
		{
			Description: "sts:GetCallerIdentity unauthorized",
			MockEndpoints: []*awsmocks.MockEndpoint{
				awsmocks.MockStsGetCallerIdentityInvalidEndpointAccessDenied,
			},
			ErrCount: 1,
		},
		{
			Description: "sts:GetCallerIdentity success",
			MockEndpoints: []*awsmocks.MockEndpoint{
				awsmocks.MockStsGetCallerIdentityValidEndpoint,
			},
			ExpectedAccountID: awsmocks.MockStsGetCallerIdentityAccountID,
			ExpectedPartition: awsmocks.MockStsGetCallerIdentityPartition,
		},
	}

	for _, testCase := range testCases {
		testCase := testCase

		t.Run(testCase.Description, func(t *testing.T) {
			closeSts, stsSess, err := awsmocks.GetMockedAwsApiSession("STS", testCase.MockEndpoints)
			defer closeSts()
			if err != nil {
				t.Fatal(err)
			}

			stsConn := sts.New(stsSess)

			accountID, partition, err := getAccountIDAndPartitionFromSTSGetCallerIdentity(stsConn)
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
