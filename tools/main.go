// Copyright IBM Corp. 2015, 2025
// SPDX-License-Identifier: MPL-2.0

//go:build tools
// +build tools

package main

import (
	_ "github.com/golangci/golangci-lint/v2/cmd/golangci-lint"
	_ "github.com/pavius/impi/cmd/impi"
)
