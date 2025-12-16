// Copyright IBM Corp. 2015, 2025
// SPDX-License-Identifier: MPL-2.0

package awsv1shim

import (
	"github.com/aws/aws-sdk-go/aws/request"
	"github.com/hashicorp/aws-sdk-go-base/v2/useragent"
)

func userAgentFromContextHandler(r *request.Request) {
	ctx := r.Context()

	if v := useragent.BuildFromContext(ctx); v != "" {
		request.AddToUserAgent(r, v)
	}
}
