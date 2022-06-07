package useragent

import (
	"context"

	"github.com/hashicorp/aws-sdk-go-base/v2/internal/config"
)

type userAgentKey string

const (
	ContextScopedUserAgent userAgentKey = "ContextScopedUserAgent"
)

func FromContext(ctx context.Context) string {
	var ps config.UserAgentProducts

	s := ctx.Value(ContextScopedUserAgent)
	switch v := s.(type) {
	case config.UserAgentProducts:
		ps = v

	case []config.UserAgentProduct:
		ps = config.UserAgentProducts(v)

	default:
		return ""
	}

	return ps.BuildUserAgentString()
}
