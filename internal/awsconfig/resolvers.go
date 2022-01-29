package awsconfig

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
)

// Copied from https://github.com/aws/aws-sdk-go-v2/blob/main/internal/configsources/config.go
type UseFIPSEndpointProvider interface {
	GetUseFIPSEndpoint(context.Context) (value aws.FIPSEndpointState, found bool, err error)
}

// Copied from https://github.com/aws/aws-sdk-go-v2/blob/main/internal/configsources/config.go
func ResolveUseFIPSEndpoint(ctx context.Context, configSources []interface{}) (value aws.FIPSEndpointState, found bool, err error) {
	for _, cfg := range configSources {
		if p, ok := cfg.(UseFIPSEndpointProvider); ok {
			value, found, err = p.GetUseFIPSEndpoint(ctx)
			if err != nil || found {
				break
			}
		}
	}
	return
}

func FIPSEndpointStateString(state aws.FIPSEndpointState) string {
	switch state {
	case aws.FIPSEndpointStateUnset:
		return "FIPSEndpointStateUnset"
	case aws.FIPSEndpointStateEnabled:
		return "FIPSEndpointStateEnabled"
	case aws.FIPSEndpointStateDisabled:
		return "FIPSEndpointStateDisabled"
	}
	return fmt.Sprintf("unknown aws.FIPSEndpointState (%d)", state)
}

// Copied from https://github.com/aws/aws-sdk-go-v2/blob/main/internal/configsources/config.go
type UseDualStackEndpointProvider interface {
	GetUseDualStackEndpoint(context.Context) (value aws.DualStackEndpointState, found bool, err error)
}

// Copied from https://github.com/aws/aws-sdk-go-v2/blob/main/internal/configsources/config.go
func ResolveUseDualStackEndpoint(ctx context.Context, configSources []interface{}) (value aws.DualStackEndpointState, found bool, err error) {
	for _, cfg := range configSources {
		if p, ok := cfg.(UseDualStackEndpointProvider); ok {
			value, found, err = p.GetUseDualStackEndpoint(ctx)
			if err != nil || found {
				break
			}
		}
	}
	return
}

func DualStackEndpointStateString(state aws.DualStackEndpointState) string {
	switch state {
	case aws.DualStackEndpointStateUnset:
		return "DualStackEndpointStateUnset"
	case aws.DualStackEndpointStateEnabled:
		return "DualStackEndpointStateEnabled"
	case aws.DualStackEndpointStateDisabled:
		return "DualStackEndpointStateDisabled"
	}
	return fmt.Sprintf("unknown aws.FIPSEndpointStateUnset (%d)", state)
}

// Copied and renamed from https://github.com/aws/aws-sdk-go-v2/blob/main/feature/ec2/imds/internal/config/resolvers.go
type EC2IMDSEndpointResolver interface {
	GetEC2IMDSEndpoint() (value string, found bool, err error)
}

// Copied and renamed from https://github.com/aws/aws-sdk-go-v2/blob/main/feature/ec2/imds/internal/config/resolvers.go
func ResolveEC2IMDSEndpointConfig(configSources []interface{}) (value string, found bool, err error) {
	for _, cfg := range configSources {
		if p, ok := cfg.(EC2IMDSEndpointResolver); ok {
			value, found, err = p.GetEC2IMDSEndpoint()
			if err != nil || found {
				break
			}
		}
	}
	return
}
