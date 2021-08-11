package awsbase

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials/stscreds"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/aws/aws-sdk-go-v2/service/sts/types"
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
		config.WithEndpointResolver(endpointResolver(c)),
	)

	if c.AssumeRoleARN == "" {
		return cfg, err
	}

	// When assuming a role, we need to first authenticate the base credentials above, then assume the desired role
	log.Printf("[INFO] Attempting to AssumeRole %s (SessionName: %q, ExternalId: %q)",
		c.AssumeRoleARN, c.AssumeRoleSessionName, c.AssumeRoleExternalID)

	client := sts.NewFromConfig(cfg)

	appCreds := stscreds.NewAssumeRoleProvider(client, c.AssumeRoleARN, func(opts *stscreds.AssumeRoleOptions) {
		opts.RoleSessionName = c.AssumeRoleSessionName
		opts.Duration = time.Duration(c.AssumeRoleDurationSeconds) * time.Second

		if c.AssumeRoleExternalID != "" {
			opts.ExternalID = aws.String(c.AssumeRoleExternalID)
		}

		if c.AssumeRolePolicy != "" {
			opts.Policy = aws.String(c.AssumeRolePolicy)
		}

		if len(c.AssumeRolePolicyARNs) > 0 {
			var policyDescriptorTypes []types.PolicyDescriptorType

			for _, policyARN := range c.AssumeRolePolicyARNs {
				policyDescriptorType := types.PolicyDescriptorType{
					Arn: aws.String(policyARN),
				}
				policyDescriptorTypes = append(policyDescriptorTypes, policyDescriptorType)
			}

			opts.PolicyARNs = policyDescriptorTypes
		}

		if len(c.AssumeRoleTags) > 0 {
			log.Println("[WARN] Assume role tags are not currently supported by stscreds.AssumeRoleProvider")
		}
	})
	_, err = appCreds.Retrieve(ctx)
	if err != nil {
		return aws.Config{}, fmt.Errorf("error assuming role: %w", err)
	}

	cfg.Credentials = aws.NewCredentialsCache(appCreds)

	return cfg, err
}
