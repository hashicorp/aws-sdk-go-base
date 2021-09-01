package awsv1shim

import (
	"errors"
	"fmt"
	"log"
	"os"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/arn"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/credentials/ec2rolecreds"
	"github.com/aws/aws-sdk-go/aws/ec2metadata"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/iam"
	"github.com/aws/aws-sdk-go/service/sts"
	"github.com/hashicorp/go-multierror"
)

// getAccountIDAndPartition gets the account ID and associated partition.
func getAccountIDAndPartition(iamconn *iam.IAM, stsconn *sts.STS, authProviderName string) (string, string, error) {
	var accountID, partition string
	var err, errors error

	if authProviderName == ec2rolecreds.ProviderName {
		accountID, partition, err = getAccountIDAndPartitionFromEC2Metadata()
	} else {
		accountID, partition, err = getAccountIDAndPartitionFromIAMGetUser(iamconn)
	}
	if accountID != "" {
		return accountID, partition, nil
	}
	errors = multierror.Append(errors, err)

	accountID, partition, err = getAccountIDAndPartitionFromSTSGetCallerIdentity(stsconn)
	if accountID != "" {
		return accountID, partition, nil
	}
	errors = multierror.Append(errors, err)

	accountID, partition, err = getAccountIDAndPartitionFromIAMListRoles(iamconn)
	if accountID != "" {
		return accountID, partition, nil
	}
	errors = multierror.Append(errors, err)

	return accountID, partition, errors
}

// getAccountIDAndPartitionFromEC2Metadata gets the account ID and associated
// partition from EC2 metadata.
func getAccountIDAndPartitionFromEC2Metadata() (string, string, error) {
	log.Println("[DEBUG] Trying to get account information via EC2 Metadata")

	cfg := &aws.Config{}
	setOptionalEndpoint(cfg)
	sess, err := session.NewSession(cfg)
	if err != nil {
		return "", "", fmt.Errorf("error creating EC2 Metadata session: %w", err)
	}

	metadataClient := ec2metadata.New(sess)
	info, err := metadataClient.IAMInfo()
	if err != nil {
		// We can end up here if there's an issue with the instance metadata service
		// or if we're getting credentials from AdRoll's Hologram (in which case IAMInfo will
		// error out).
		err = fmt.Errorf("failed getting account information via EC2 Metadata IAM information: %w", err)
		log.Printf("[DEBUG] %s", err)
		return "", "", err
	}

	return parseAccountIDAndPartitionFromARN(info.InstanceProfileArn)
}

// getAccountIDAndPartitionFromIAMGetUser gets the account ID and associated
// partition from IAM.
func getAccountIDAndPartitionFromIAMGetUser(iamconn *iam.IAM) (string, string, error) {
	log.Println("[DEBUG] Trying to get account information via iam:GetUser")

	output, err := iamconn.GetUser(&iam.GetUserInput{})
	if err != nil {
		// AccessDenied and ValidationError can be raised
		// if credentials belong to federated profile, so we ignore these
		if awsErr, ok := err.(awserr.Error); ok {
			switch awsErr.Code() {
			case "AccessDenied", "InvalidClientTokenId", "ValidationError":
				return "", "", nil
			}
		}
		err = fmt.Errorf("failed getting account information via iam:GetUser: %w", err)
		log.Printf("[DEBUG] %s", err)
		return "", "", err
	}

	if output == nil || output.User == nil {
		err = errors.New("empty iam:GetUser response")
		log.Printf("[DEBUG] %s", err)
		return "", "", err
	}

	return parseAccountIDAndPartitionFromARN(aws.StringValue(output.User.Arn))
}

// getAccountIDAndPartitionFromIAMListRoles gets the account ID and associated
// partition from listing IAM roles.
func getAccountIDAndPartitionFromIAMListRoles(iamconn *iam.IAM) (string, string, error) {
	log.Println("[DEBUG] Trying to get account information via iam:ListRoles")

	output, err := iamconn.ListRoles(&iam.ListRolesInput{
		MaxItems: aws.Int64(int64(1)),
	})
	if err != nil {
		err = fmt.Errorf("failed getting account information via iam:ListRoles: %w", err)
		log.Printf("[DEBUG] %s", err)
		return "", "", err
	}

	if output == nil || len(output.Roles) < 1 {
		err = fmt.Errorf("empty iam:ListRoles response")
		log.Printf("[DEBUG] %s", err)
		return "", "", err
	}

	return parseAccountIDAndPartitionFromARN(aws.StringValue(output.Roles[0].Arn))
}

// getAccountIDAndPartitionFromSTSGetCallerIdentity gets the account ID and associated
// partition from STS caller identity.
func getAccountIDAndPartitionFromSTSGetCallerIdentity(stsconn *sts.STS) (string, string, error) {
	log.Println("[DEBUG] Trying to get account information via sts:GetCallerIdentity")

	output, err := stsconn.GetCallerIdentity(&sts.GetCallerIdentityInput{})
	if err != nil {
		return "", "", fmt.Errorf("error calling sts:GetCallerIdentity: %w", err)
	}

	if output == nil || output.Arn == nil {
		err = errors.New("empty sts:GetCallerIdentity response")
		log.Printf("[DEBUG] %s", err)
		return "", "", err
	}

	return parseAccountIDAndPartitionFromARN(aws.StringValue(output.Arn))
}

func parseAccountIDAndPartitionFromARN(inputARN string) (string, string, error) {
	arn, err := arn.Parse(inputARN)
	if err != nil {
		return "", "", fmt.Errorf("error parsing ARN (%s): %s", inputARN, err)
	}
	return arn.AccountID, arn.Partition, nil
}

func setOptionalEndpoint(cfg *aws.Config) string {
	endpoint := os.Getenv("AWS_METADATA_URL")
	if endpoint != "" {
		log.Printf("[INFO] Setting custom metadata endpoint: %q", endpoint)
		cfg.Endpoint = aws.String(endpoint)
		return endpoint
	}
	return ""
}
