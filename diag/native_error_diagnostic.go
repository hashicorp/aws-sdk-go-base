package diag

var _ DiagnosticWithErr = NativeErrorDiagnostic{}

// NativeErrorDiagnostic is a generic diagnostic with error severity.
type NativeErrorDiagnostic struct {
	// detail  string
	// summary string
	err error
}

// NewErrorDiagnostic returns a new error severity diagnostic with the given summary and detail.
func NewNativeErrorDiagnostic( /*summary string detail string,*/ err error) NativeErrorDiagnostic {
	return NativeErrorDiagnostic{
		// detail:  detail,
		// summary: summary,
		err: err,
	}
}

// Severity returns the diagnostic severity.
func (d NativeErrorDiagnostic) Severity() Severity {
	return SeverityError
}

// Summary returns the diagnostic summary.
func (d NativeErrorDiagnostic) Summary() string {
	return d.err.Error()
}

// Detail returns the diagnostic detail.
func (d NativeErrorDiagnostic) Detail() string {
	return ""
}

func (d NativeErrorDiagnostic) Err() error {
	return d.err
}

// Equal returns true if the other diagnostic is wholly equivalent.
func (d NativeErrorDiagnostic) Equal(other Diagnostic) bool {
	ed, ok := other.(NativeErrorDiagnostic)

	if !ok {
		return false
	}

	return ed.Summary() == d.Summary() && ed.Detail() == d.Detail()
}
