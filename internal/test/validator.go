package test

import (
	"testing"

	"github.com/hashicorp/aws-sdk-go-base/v2/diag"
)

type DiagsValidator func(*testing.T, diag.Diagnostics)

type ErrValidator func(error) bool

func ExpectErrDiag(diags diag.Diagnostics, ev ErrValidator) bool {
	for _, d := range diags.Errors() {
		if e, ok := d.(diag.DiagnosticWithErr); ok {
			if ev(e.Err()) {
				return true
			}
		}
	}
	return false
}

func ExpectNoDiags(t *testing.T, diags diag.Diagnostics) {
	expectDiagsCount(t, diags, 0)
}

func expectDiagsCount(t *testing.T, diags diag.Diagnostics, c int) {
	if l := diags.Count(); l != c {
		t.Fatalf("expected %d Diagnostics, got %d", c, l)
	}
}
