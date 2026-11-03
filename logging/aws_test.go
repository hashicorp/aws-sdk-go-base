// Copyright IBM Corp. 2015, 2025
// SPDX-License-Identifier: MPL-2.0

package logging

import (
	"testing"
	"unsafe"
)

func TestMaskAWSAccessKey(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		input    string
		expected string
	}{
		// One case per IAM unique-ID prefix (prefix + 17 chars = 21).
		"prefix_ABIA": {
			input:    "ABIACKCEVSQ6C2EXAMPLE1",
			expected: "ABIA**************PLE1",
		},
		"prefix_ACCA": {
			input:    "ACCACKCEVSQ6C2EXAMPLE1",
			expected: "ACCA**************PLE1",
		},
		"prefix_AGPA": {
			input:    "AGPACKCEVSQ6C2EXAMPLE1",
			expected: "AGPA**************PLE1",
		},
		"prefix_AIDA": {
			input:    "AIDACKCEVSQ6C2EXAMPLE1",
			expected: "AIDA**************PLE1",
		},
		"prefix_AIPA": {
			input:    "AIPACKCEVSQ6C2EXAMPLE1",
			expected: "AIPA**************PLE1",
		},
		"prefix_AKIA": {
			input:    "AKIACKCEVSQ6C2EXAMPLE1",
			expected: "AKIA**************PLE1",
		},
		"prefix_ANPA": {
			input:    "ANPACKCEVSQ6C2EXAMPLE1",
			expected: "ANPA**************PLE1",
		},
		"prefix_ANVA": {
			input:    "ANVACKCEVSQ6C2EXAMPLE1",
			expected: "ANVA**************PLE1",
		},
		"prefix_APKA": {
			input:    "APKACKCEVSQ6C2EXAMPLE1",
			expected: "APKA**************PLE1",
		},
		"prefix_AROA": {
			input:    "AROACKCEVSQ6C2EXAMPLE1",
			expected: "AROA**************PLE1",
		},
		"prefix_ASCA": {
			input:    "ASCACKCEVSQ6C2EXAMPLE1",
			expected: "ASCA**************PLE1",
		},
		"prefix_ASIA": {
			input:    "ASIACKCEVSQ6C2EXAMPLE1",
			expected: "ASIA**************PLE1",
		},

		// A3T is a 3-char prefix followed by one [A-Z0-9].
		"prefix_A3T": {
			input:    "A3TXABCDEFGHIJKLMNOPQR",
			expected: "A3TX**************OPQR",
		},

		// Length boundary: 19 total (4+15) is too short; 20 (4+16) matches.
		"too_short_19": {
			input:    "AKIA123456789012345",
			expected: "AKIA123456789012345",
		},
		"minimum_length_20": {
			input:    "AKIA1234567890123456",
			expected: "AKIA************3456",
		},

		// Prefix appears mid-run; preceding alnum is preserved.
		"prefix_mid_run": {
			input:    "XXAKIACKCEVSQ6C2EXAMPLE1",
			expected: "XXAKIA**************PLE1",
		},

		// Back-to-back prefixes: leftmost match greedily spans the whole run.
		"back_to_back": {
			input:    "AKIAAKIACKCEVSQ6C2EXAMPLE1",
			expected: "AKIA******************PLE1",
		},

		// Leading 'A' that is not a prefix, immediately followed by a valid
		// prefix: the match starts at the second 'A'. Guards the next-'A' search.
		"leading_a_then_prefix": {
			input:    "AAKIACKCEVSQ6C2EXAMPLE1",
			expected: "AAKIA**************PLE1",
		},
		"two_leading_a_then_prefix": {
			input:    "AAAKIACKCEVSQ6C2EXAMPLE1",
			expected: "AAAKIA**************PLE1",
		},

		// Multiple keys separated by a non-alnum boundary.
		"multiple_keys": {
			input:    "AKIACKCEVSQ6C2EXAMPLE1 AROACKCEVSQ6C2EXAMPLE1",
			expected: "AKIA**************PLE1 AROA**************PLE1",
		},

		// Near-miss prefix (AKIB is not a valid prefix): unchanged.
		"near_miss_prefix": {
			input:    "AKIB1234567890123456789",
			expected: "AKIB1234567890123456789",
		},

		// A3T not followed by [A-Z0-9]: no match.
		"a3t_non_alnum": {
			input:    "A3T-1234567890123456789",
			expected: "A3T-1234567890123456789",
		},

		"no_key": {
			input:    "no keys here at all",
			expected: "no keys here at all",
		},
		"empty": {
			input:    "",
			expected: "",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			got := []byte(tc.input)
			MaskAWSAccessKey(got)
			if string(got) != tc.expected {
				t.Errorf("MaskAWSAccessKey(%q) = %q, want %q", tc.input, string(got), tc.expected)
			}
		})
	}
}

type maskAWSSensitiveValuesTestCase struct {
	input    string
	expected string
}

var maskAWSSensitiveValuesTests = map[string]maskAWSSensitiveValuesTestCase{
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

func TestMaskAWSSensitiveValues(t *testing.T) {
	t.Parallel()

	for name, test := range maskAWSSensitiveValuesTests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			got := MaskAWSSensitiveValues(test.input)
			if got != test.expected {
				t.Errorf("unexpected diff +wanted: %s, -got: %s", test.expected, got)
			}
		})
	}
}

func TestMaskAWSSensitiveValuesBytes(t *testing.T) {
	t.Parallel()

	for name, test := range maskAWSSensitiveValuesTests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			input := []byte(test.input)
			got := MaskAWSSensitiveValuesBytes(input)
			if string(got) != test.expected {
				t.Errorf("unexpected diff +wanted: %s, -got: %s", test.expected, got)
			}
			if len(input) > 0 && unsafe.SliceData(got) != unsafe.SliceData(input) {
				t.Error("MaskAWSSensitiveValuesBytes should mask in place and return the same slice")
			}
		})
	}
}

func BenchmarkMaskAWSAccessKey(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		MaskAWSAccessKey([]byte(`
{
	"AWSSecretKey": "LEfH8nZmFN4BGIJnku6lkChHydRN5B/YlWCIjOte",
	"BucketName": "test-bucket",
	"AWSKeyId": "AIDACKCEVSQ6C2EXAMPLE",
}
`))
	}
}

func BenchmarkPartialMaskString(b *testing.B) {
	var s []byte
	b.ReportAllocs()
	for b.Loop() {
		s = partialMaskString([]byte("AIDACKCEVSQ6C2EXAMPLE"), 4, 4)
	}
	dump = unsafe.String(unsafe.SliceData(s), len(s))
}

func BenchmarkMaskAWSSecretKeys(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		MaskAWSSecretKeys([]byte(`
{
	"AWSSecretKey": "LEfH8nZmFN4BGIJnku6lkChHydRN5B/YlWCIjOte",
	"BucketName": "test-bucket",
	"AWSKeyId": "AIDACKCEVSQ6C2EXAMPLE",
}
`))
	}
}

func BenchmarkMaskAWSSensitiveValues(b *testing.B) {
	var s string
	b.ReportAllocs()
	for b.Loop() {
		s = MaskAWSSensitiveValues(`
{
	"AWSSecretKey": "LEfH8nZmFN4BGIJnku6lkChHydRN5B/YlWCIjOte",
	"BucketName": "test-bucket",
	"AWSKeyId": "AIDACKCEVSQ6C2EXAMPLE",
}
`)
	}
	dump = s
}

func BenchmarkMaskAWSSensitiveValuesBytes(b *testing.B) {
	source := []byte(`
{
	"AWSSecretKey": "LEfH8nZmFN4BGIJnku6lkChHydRN5B/YlWCIjOte",
	"BucketName": "test-bucket",
	"AWSKeyId": "AIDACKCEVSQ6C2EXAMPLE",
}
`)
	field := make([]byte, len(source))
	b.ReportAllocs()
	for b.Loop() {
		copy(field, source)
		field = MaskAWSSensitiveValuesBytes(field)
	}
	dump = string(field)
}

var dump string
