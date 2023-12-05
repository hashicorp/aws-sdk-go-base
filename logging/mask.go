// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package logging

import (
	"strings"
)

func partialMaskString(s string, first, last int) string {
	l := len(s)
	result := strings.Builder{}
	result.Grow(l)
	result.WriteString(s[0:first])
	for i := 0; i < l-first-last; i++ {
		result.WriteByte('*')
	}
	result.WriteString(s[l-last:])
	return result.String()
}
