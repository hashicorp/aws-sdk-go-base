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
			input:    "MfP3tIG15gibzIx7CSbhSNkgD5sSV4k2tWXgN8U8",
			expected: "MfP3********************************N8U8",
		},
		"mask_complex_json": {
			input: `
{
	"AWSSecretKey": "LEfH8nZmFN4BGIJnku6lkChHydRN5B/YlWCIjOte",
	"BucketName": "test-bucket",
	"AWSKeyId": "AIDACKCEVSQ6C2EXAMPLE",
}
`,
			expected: `
{
	"AWSSecretKey": "LEfH********************************jOte",
	"BucketName": "test-bucket",
	"AWSKeyId": "AIDA*************MPLE",
}
`,
		},
		"mask_multiple_json": {
			input: `
{
	"AWSSecretKey": "LEfH8nZmFN4BGIJnku6lkChHydRN5B/YlWCIjOte",
	"BucketName": "test-bucket-1",
	"AWSKeyId": "AIDACKCEVSQ6C2EXAMPLE",
},
{
	"Key": "ABCDEFGH!JKLMNOPQRSTUVWXYZ012345678901234567890123456789",
},
{
	"AWSSecretKey": "MfP3tIG15gibzIx7CSbhSNkgD5sSV4k2tWXgN8U8",
	"BucketName": "test-bucket-2",
	"AWSKeyId": "AKIA5PX2H2S3LHEXAMPLE",
}
`,
			expected: `
{
	"AWSSecretKey": "LEfH********************************jOte",
	"BucketName": "test-bucket-1",
	"AWSKeyId": "AIDA*************MPLE",
},
{
	"Key": "ABCDEFGH!JKLMNOPQRSTUVWXYZ012345678901234567890123456789",
},
{
	"AWSSecretKey": "MfP3********************************N8U8",
	"BucketName": "test-bucket-2",
	"AWSKeyId": "AKIA*************MPLE",
}
`,
		},
		"no_mask": {
			input:    "<BucketName>test-bucket</BucketName>",
			expected: "<BucketName>test-bucket</BucketName>",
		},
		"mask_xml": {
			input: `
<AWSSecretKey>8/AiP0ofCD/YOAqXWrungQt/Y4BkTj1UOjZ0MqBs</AWSSecretKey>
<BucketName>test-bucket</BucketName>
<AWSKeyId>AIDACKCEVSQ6C2EXAMPLE</AWSKeyId>
`,
			expected: `
<AWSSecretKey>8/Ai********************************MqBs</AWSSecretKey>
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
