package awsbase

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
)

func GetAwsConfig(ctx context.Context, c *Config) (aws.Config, error) {
	credentialsProvider, err := credentialsProvider(c)
	if err != nil {
		return aws.Config{}, err
	}

	cfg, err := config.LoadDefaultConfig(ctx,
		config.WithCredentialsProvider(credentialsProvider),
		config.WithRegion(c.Region),
		config.WithSharedCredentialsFiles([]string{c.CredsFilename}),
		config.WithSharedConfigProfile(c.Profile),
	)
	return cfg, err
}
