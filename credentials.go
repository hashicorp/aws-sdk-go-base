package awsbase

import (
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
)

func credentialsProvider(c *Config) (aws.CredentialsProvider, error) {
	return credentials.NewStaticCredentialsProvider(
		c.AccessKey,
		c.SecretKey,
		c.Token,
	), nil
}
