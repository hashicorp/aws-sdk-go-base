package awsv1shim

import (
	"github.com/aws/aws-sdk-go/aws/request"
	"github.com/hashicorp/aws-sdk-go-base/v2/internal/useragent"
)

func userAgentFromContextHandler(r *request.Request) {
	ctx := r.Context()

	if v := useragent.FromContext(ctx); v != "" {
		request.AddToUserAgent(r, v)
	}
}
