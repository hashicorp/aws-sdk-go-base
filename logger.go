package awsbase

import (
	"log"
	"strings"
)

type DebugLogger struct{}

func (l DebugLogger) Log(args ...interface{}) {
	tokens := make([]string, 0, len(args))
	for _, arg := range args {
		if token, ok := arg.(string); ok {
			tokens = append(tokens, token)
		}
	}
	s := strings.Join(tokens, " ")
	s = strings.ReplaceAll(s, "\r", "") // Works around https://github.com/jen20/teamcity-go-test/pull/2
	log.Printf("[DEBUG] [aws-sdk-go] %s", s)
}
