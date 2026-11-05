// SPDX-License-Identifier: MPL-2.0

package logging

import (
	"testing"
)

// referenceMaskAWSAccessKey is the original regexp-based implementation,
// retained as the oracle for FuzzMaskAWSAccessKey. It uses the exported
// UniqueIDRegex so the fuzz test verifies the in-place MaskAWSAccessKey stays
// equivalent to the documented pattern.
func referenceMaskAWSAccessKey(field []byte) []byte {
	return UniqueIDRegex.ReplaceAllFunc(field, func(s []byte) []byte {
		return partialMaskString(s, unmaskedFirst, unmaskedLast)
	})
}

func FuzzMaskAWSAccessKey(f *testing.F) {
	f.Add("AKIA5PX2H2S3LHEXAMPLE")
	f.Add("AIDACKCEVSQ6C2EXAMPLE")
	f.Add("A3TXABCDEFGHIJKLMNOPQRST")
	f.Add("XXAKIA5PX2H2S3LHEXAMPLE")
	f.Add("AKIA5PX2H2S3LHEXAMP")  // 19, too short
	f.Add("AKIA5PX2H2S3LHEXAMPL") // 20, minimum
	f.Add("AKIAAKIA5PX2H2S3LHEXAMPLE")
	f.Add("AAKIACKCEVSQ6C2EXAMPLE1")
	f.Add("prefix AKIA5PX2H2S3LHEXAMPLE and AROA1234567890ABCDEFGH suffix")
	f.Add("no keys here at all")
	f.Add("")

	f.Fuzz(func(t *testing.T, input string) {
		// Each implementation masks in place, so give each its own copy.
		got := []byte(input)
		MaskAWSAccessKey(got)
		want := referenceMaskAWSAccessKey([]byte(input))
		if string(got) != string(want) {
			t.Fatalf("MaskAWSAccessKey(%q) = %q, UniqueIDRegex reference = %q", input, string(got), string(want))
		}
	})
}
