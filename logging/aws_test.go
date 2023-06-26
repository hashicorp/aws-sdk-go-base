// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package logging

import (
	"testing"
)

func TestMaskAWSSensitiveValues(t *testing.T) {
	t.Parallel()

	type testCase struct {
		input    string
		expected string
	}

	tests := map[string]testCase{
		"mask_simple": {
			input:    "4skd4lTSLVBMG/asedterGLKSNMSAlsxiLGfjt=ssD",
			expected: "4skd**********************************=ssD",
		},
		"mask_complex_json": {
			input: `
{
	"AWSSecretKey": "4skd4lTSLVBMG/asedterGLKSNMSAlsxiLGfjt=ssD",
	"BucketName": "test-bucket",
	"AWSKeyId": "AIDACKCEVSQ6C2EXAMPLE",
}
`,
			expected: `
{
	"AWSSecretKey": "4skd**********************************=ssD",
	"BucketName": "test-bucket",
	"AWSKeyId": "AIDA*************MPLE",
}
`,
		},
		"no_mask": {
			input:    "<BucketName>test-bucket</BucketName>",
			expected: "<BucketName>test-bucket</BucketName>",
		},
		"mask_xml": {
			input: `
<AWSSecretKey>4skd4lTSLVBMG/asedterGLKSNMSAlsxiLGfjt=ssD</AWSSecretKey>
<BucketName>test-bucket</BucketName>
<AWSKeyId>AIDACKCEVSQ6C2EXAMPLE</AWSKeyId>
`,
			expected: `
<AWSSecretKey>4skd**********************************=ssD</AWSSecretKey>
<BucketName>test-bucket</BucketName>
<AWSKeyId>AIDA*************MPLE</AWSKeyId>
`,
		},
	}

	for name, test := range tests {
		name, test := name, test
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			got := MaskAWSSensitiveValues(test.input)

			if got != test.expected {
				t.Errorf("unexpected diff +wanted: %s, -got: %s", test.expected, got)
			}
		})
	}
}
