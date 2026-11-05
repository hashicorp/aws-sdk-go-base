// Copyright IBM Corp. 2015, 2025
// SPDX-License-Identifier: MPL-2.0

package logging

import (
	"regexp"
	"unsafe"
)

// IAM Unique ID prefixes from
// https://docs.aws.amazon.com/IAM/latest/UserGuide/reference_identifiers.html#identifiers-unique-ids
var UniqueIDRegex = regexp.MustCompile(`(A3T[A-Z0-9]` +
	`|ABIA` + // STS service bearer token
	`|ACCA` + // Context-specific credential
	`|AGPA` + // User group
	`|AIDA` + // IAM user
	`|AIPA` + // EC2 instance profile
	`|AKIA` + // Access key
	`|ANPA` + // Managed policy
	`|ANVA` + // Version in a managed policy
	`|APKA` + // Public key
	`|AROA` + // Role
	`|ASCA` + // Certificate
	`|ASIA` + // STS temporary access key
	`)[A-Z0-9]{16,}`)

const (
	unmaskedFirst = 4
	unmaskedLast  = 4
)

// awsAccessKeyPrefixes are the four-character IAM unique-ID prefixes that precede an access
// key's 16+ trailing [A-Z0-9] characters.
// Note: "A3T[A-Z0-9];" is handled separately in hasAccessKeyPrefix.
var awsAccessKeyPrefixes = [...]string{
	"ABIA", "ACCA", "AGPA", "AIDA", "AIPA", "AKIA",
	"ANPA", "ANVA", "APKA", "AROA", "ASCA", "ASIA",
}

// isUpperAlnum reports whether b is in [A-Z0-9].
func isUpperAlnum(b byte) bool {
	return (b >= 'A' && b <= 'Z') || (b >= '0' && b <= '9')
}

// hasAccessKeyPrefix reports whether in[i:] begins with a valid access-key prefix.
// Every prefix starts with 'A', so callers should gate on that first.
func hasAccessKeyPrefix(in []byte, i int) bool {
	if i+4 > len(in) {
		return false
	}
	// in[i] == 'A' is guaranteed by the caller's gate. "A3T" is the one prefix
	// followed by a variable [A-Z0-9]; compare its three fixed bytes at once.
	if string(in[i:i+3]) == "A3T" && isUpperAlnum(in[i+3]) {
		return true
	}
	p := in[i : i+4 : i+4]
	for _, prefix := range awsAccessKeyPrefixes {
		if p[0] == prefix[0] && p[1] == prefix[1] && p[2] == prefix[2] && p[3] == prefix[3] {
			return true
		}
	}
	return false
}

// MaskAWSAccessKey masks likely AWS access keys in field in place.
// It is equivalent to the regexp `UniqueIDRegex`, masking the match's middle,
// but scans byte-wise without allocating.
func MaskAWSAccessKey(field []byte) {
	const (
		accessKeyPrefixLen = 4
		minLen             = accessKeyPrefixLen + 16 // prefix + [A-Z0-9]{16,}
	)
	i := 0
	for i < len(field) {
		// Every access-key prefix starts with 'A'; skip other bytes cheaply.
		if field[i] != 'A' || !hasAccessKeyPrefix(field, i) {
			i++
			continue
		}
		// Extend to the end of the contiguous [A-Z0-9] run (greedy match).
		end := i + accessKeyPrefixLen
		for end < len(field) && isUpperAlnum(field[end]) {
			end++
		}
		if end-i < minLen {
			// Not enough trailing characters; advance past this position.
			i++
			continue
		}
		// Mask the middle, preserving the first and last unmasked bytes.
		for j := i + unmaskedFirst; j < end-unmaskedLast; j++ {
			field[j] = '*'
		}
		i = end
	}
}

func MaskAWSSensitiveValues(field string) string {
	b := []byte(field)
	MaskAWSAccessKey(b)
	MaskAWSSecretKeys(b)
	return unsafe.String(unsafe.SliceData(b), len(b))
}

// MaskAWSSecretKeys masks likely AWS secret access keys in the input.
// See https://aws.amazon.com/blogs/security/a-safer-way-to-distribute-aws-credentials-to-ec2/:
// "Find me 40-character, base-64 strings that don’t have any base 64 characters immediately before or after".
func MaskAWSSecretKeys(in []byte) {
	const (
		secretKeyLen = 40
	)
	len := len(in)
	base64Characters := 0

	for i := range len {
		b := in[i]

		if (b >= 'A' && b <= 'Z') || (b >= 'a' && b <= 'z') || (b >= '0' && b <= '9') || b == '/' || b == '+' || b == '=' {
			// base64 character.
			base64Characters++
		} else {
			if base64Characters == secretKeyLen {
				for j := (i - secretKeyLen) + unmaskedFirst; j < i-unmaskedLast; j++ {
					in[j] = '*'
				}
			}

			base64Characters = 0
		}
	}

	if base64Characters == secretKeyLen {
		for j := (len - secretKeyLen) + unmaskedFirst; j < len-unmaskedLast; j++ {
			in[j] = '*'
		}
	}
}
