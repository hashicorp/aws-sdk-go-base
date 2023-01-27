package test

import (
	"context"
	"testing"

	"github.com/hashicorp/aws-sdk-go-base/v2/logging"
)

func Context(t *testing.T) context.Context {
	return logging.RegisterLogger(context.Background(), logging.TfLogger(t.Name()))
}
