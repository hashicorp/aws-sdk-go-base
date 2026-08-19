// Copyright IBM Corp. 2015, 2026
// SPDX-License-Identifier: MPL-2.0

package tfawserr

import (
	"errors"
	"fmt"
	"net/http"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sts/types"
	smithy "github.com/aws/smithy-go"
	smithyhttp "github.com/aws/smithy-go/transport/http"
)

func TestErrCodeEquals(t *testing.T) {
	testCases := map[string]struct {
		Err      error
		Codes    []string
		Expected bool
	}{
		"nil error": {
			Err:      nil,
			Expected: false,
		},
		"other error": {
			Err:      fmt.Errorf("other error"),
			Expected: false,
		},
		"Top-level smithy.GenericAPIError matching first code": {
			Err:      &smithy.GenericAPIError{Code: "TestCode", Message: "TestMessage"},
			Codes:    []string{"TestCode"},
			Expected: true,
		},
		"Top-level smithy.GenericAPIError matching last code": {
			Err:      &smithy.GenericAPIError{Code: "TestCode", Message: "TestMessage"},
			Codes:    []string{"NotMatching", "TestCode"},
			Expected: true,
		},
		"Top-level smithy.GenericAPIError no code": {
			Err: &smithy.GenericAPIError{Code: "TestCode", Message: "TestMessage"},
		},
		"Top-level smithy.GenericAPIError non-matching codes": {
			Err:   &smithy.GenericAPIError{Code: "TestCode", Message: "TestMessage"},
			Codes: []string{"NotMatching", "AlsoNotMatching"},
		},
		"Wrapped smithy.GenericAPIError matching first code": {
			Err:      fmt.Errorf("test: %w", &smithy.GenericAPIError{Code: "TestCode", Message: "TestMessage"}),
			Codes:    []string{"TestCode"},
			Expected: true,
		},
		"Wrapped smithy.GenericAPIError matching last code": {
			Err:      fmt.Errorf("test: %w", &smithy.GenericAPIError{Code: "TestCode", Message: "TestMessage"}),
			Codes:    []string{"NotMatching", "TestCode"},
			Expected: true,
		},
		"Wrapped smithy.GenericAPIError non-matching codes": {
			Err:   fmt.Errorf("test: %w", &smithy.GenericAPIError{Code: "TestCode", Message: "TestMessage"}),
			Codes: []string{"NotMatching", "AlsoNotMatching"},
		},
		"Top-level sts ExpiredTokenException matching first code": {
			Err:      &types.ExpiredTokenException{ErrorCodeOverride: aws.String("TestCode"), Message: aws.String("TestMessage")},
			Codes:    []string{"TestCode"},
			Expected: true,
		},
		"Top-level sts ExpiredTokenException matching last code": {
			Err:      &types.ExpiredTokenException{ErrorCodeOverride: aws.String("TestCode"), Message: aws.String("TestMessage")},
			Codes:    []string{"NotMatching", "TestCode"},
			Expected: true,
		},
		"Wrapped sts ExpiredTokenException matching first code": {
			Err:      fmt.Errorf("test: %w", &types.ExpiredTokenException{ErrorCodeOverride: aws.String("TestCode"), Message: aws.String("TestMessage")}),
			Codes:    []string{"TestCode"},
			Expected: true,
		},
		"Wrapped sts ExpiredTokenException matching last code": {
			Err:      fmt.Errorf("test: %w", &types.ExpiredTokenException{ErrorCodeOverride: aws.String("TestCode"), Message: aws.String("TestMessage")}),
			Codes:    []string{"NotMatching", "TestCode"},
			Expected: true,
		},
	}

	for name, testCase := range testCases {
		t.Run(name, func(t *testing.T) {
			got := ErrCodeEquals(testCase.Err, testCase.Codes...)

			if got != testCase.Expected {
				t.Errorf("got %t, expected %t", got, testCase.Expected)
			}
		})
	}
}

func TestErrCodeContains(t *testing.T) {
	testCases := map[string]struct {
		Err      error
		Code     string
		Expected bool
	}{
		"nil error": {
			Err:      nil,
			Expected: false,
		},
		"other error": {
			Err:      fmt.Errorf("other error"),
			Expected: false,
		},
		"Top-level smithy.GenericAPIError contains": {
			Err:      &smithy.GenericAPIError{Code: "TestCoder", Message: "TestMessage"},
			Code:     "TestCode",
			Expected: true,
		},
		"Top-level smithy.GenericAPIError does not contain": {
			Err:  &smithy.GenericAPIError{Code: "TestCode", Message: "TestMessage"},
			Code: "NotMatching",
		},
		"Wrapped smithy.GenericAPIError contains": {
			Err:      fmt.Errorf("test: %w", &smithy.GenericAPIError{Code: "ATestCode", Message: "TestMessage"}),
			Code:     "TestCode",
			Expected: true,
		},
		"Wrapped smithy.GenericAPIError does not contain": {
			Err:  fmt.Errorf("test: %w", &smithy.GenericAPIError{Code: "TestCode", Message: "TestMessage"}),
			Code: "AlsoNotMatching",
		},
		"Top-level sts ExpiredTokenException contains": {
			Err:      &types.ExpiredTokenException{ErrorCodeOverride: aws.String("ATestCoder"), Message: aws.String("TestMessage")},
			Code:     "TestCode",
			Expected: true,
		},
	}

	for name, testCase := range testCases {
		t.Run(name, func(t *testing.T) {
			got := ErrCodeContains(testCase.Err, testCase.Code)

			if got != testCase.Expected {
				t.Errorf("got %t, expected %t", got, testCase.Expected)
			}
		})
	}
}

func TestErrMessageContains(t *testing.T) {
	testCases := map[string]struct {
		Err      error
		Code     string
		Message  string
		Expected bool
	}{
		"nil error": {
			Err:      nil,
			Expected: false,
		},
		"nil error code": {
			Err:      nil,
			Code:     "test",
			Expected: false,
		},
		"nil error message": {
			Err:     nil,
			Message: "test",
		},
		"nil error code and message": {
			Err:     nil,
			Code:    "test",
			Message: "test",
		},
		"other error": {
			Err:      fmt.Errorf("other error"),
			Expected: false,
		},
		"other error code and message": {
			Err:     fmt.Errorf("other error"),
			Code:    "test",
			Message: "test",
		},
		"Top-level smithy.GenericAPIError no code": {
			Err: &smithy.GenericAPIError{Code: "TestCode", Message: "TestMessage"},
		},
		"Top-level smithy.GenericAPIError matching code and no message": {
			Err:      &smithy.GenericAPIError{Code: "TestCode", Message: "TestMessage"},
			Code:     "TestCode",
			Expected: true,
		},
		"Top-level smithy.GenericAPIError matching code and matching message exact": {
			Err:      &smithy.GenericAPIError{Code: "TestCode", Message: "TestMessage"},
			Code:     "TestCode",
			Message:  "TestMessage",
			Expected: true,
		},
		"Top-level smithy.GenericAPIError non-matching code and matching message exact": {
			Err:     &smithy.GenericAPIError{Code: "TestCode", Message: "TestMessage"},
			Code:    "NotMatching",
			Message: "TestMessage",
		},
		"Top-level smithy.GenericAPIError matching code and matching message contains": {
			Err:      &smithy.GenericAPIError{Code: "TestCode", Message: "TestMessage"},
			Code:     "TestCode",
			Message:  "estMess",
			Expected: true,
		},
		"Top-level smithy.GenericAPIError matching code and non-matching message": {
			Err:     &smithy.GenericAPIError{Code: "TestCode", Message: "TestMessage"},
			Code:    "TestCode",
			Message: "NotMatching",
		},
		"Wrapped smithy.GenericAPIError matching code and no message": {
			Err:      fmt.Errorf("test: %w", &smithy.GenericAPIError{Code: "TestCode", Message: "TestMessage"}),
			Code:     "TestCode",
			Expected: true,
		},
		"Wrapped smithy.GenericAPIError matching code and matching message exact": {
			Err:      fmt.Errorf("test: %w", &smithy.GenericAPIError{Code: "TestCode", Message: "TestMessage"}),
			Code:     "TestCode",
			Message:  "TestMessage",
			Expected: true,
		},
		"Wrapped smithy.GenericAPIError non-matching code and matching message exact": {
			Err:     fmt.Errorf("test: %w", &smithy.GenericAPIError{Code: "TestCode", Message: "TestMessage"}),
			Code:    "NotMatching",
			Message: "TestMessage",
		},
		"Wrapped smithy.GenericAPIError matching code and matching message contains": {
			Err:      fmt.Errorf("test: %w", &smithy.GenericAPIError{Code: "TestCode", Message: "TestMessage"}),
			Code:     "TestCode",
			Message:  "estMess",
			Expected: true,
		},
		"Wrapped smithy.GenericAPIError matching code and non-matching message": {
			Err:     fmt.Errorf("test: %w", &smithy.GenericAPIError{Code: "TestCode", Message: "TestMessage"}),
			Code:    "TestCode",
			Message: "NotMatching",
		},
		"Top-level sts ExpiredTokenException matching code and no message": {
			Err:      &types.ExpiredTokenException{ErrorCodeOverride: aws.String("TestCode"), Message: aws.String("TestMessage")},
			Code:     "TestCode",
			Expected: true,
		},
		"Top-level sts ExpiredTokenException matching code and matching message exact": {
			Err:      &types.ExpiredTokenException{ErrorCodeOverride: aws.String("TestCode"), Message: aws.String("TestMessage")},
			Code:     "TestCode",
			Message:  "TestMessage",
			Expected: true,
		},
		"Top-level sts ExpiredTokenException non-matching code and matching message exact": {
			Err:     &types.ExpiredTokenException{ErrorCodeOverride: aws.String("TestCode"), Message: aws.String("TestMessage")},
			Code:    "NotMatching",
			Message: "TestMessage",
		},
		"Top-level sts ExpiredTokenException matching code and matching message contains": {
			Err:      &types.ExpiredTokenException{ErrorCodeOverride: aws.String("TestCode"), Message: aws.String("TestMessage")},
			Code:     "TestCode",
			Message:  "estMess",
			Expected: true,
		},
		"Top-level sts ExpiredTokenException matching code and non-matching message": {
			Err:     &types.ExpiredTokenException{ErrorCodeOverride: aws.String("TestCode"), Message: aws.String("TestMessage")},
			Code:    "TestCode",
			Message: "NotMatching",
		},
	}

	for name, testCase := range testCases {
		t.Run(name, func(t *testing.T) {
			got := ErrMessageContains(testCase.Err, testCase.Code, testCase.Message)

			if got != testCase.Expected {
				t.Errorf("got %t, expected %t", got, testCase.Expected)
			}
		})
	}
}

func TestErrMessageContainsAny(t *testing.T) {
	testCases := map[string]struct {
		Err      error
		Code     string
		Messages []string
		Expected bool
	}{
		"nil error": {
			Err:      nil,
			Expected: false,
		},
		"other error": {
			Err:      fmt.Errorf("other error"),
			Code:     "TestCode",
			Messages: []string{"TestMessage"},
			Expected: false,
		},
		"matching code and one matching message": {
			Err:      &smithy.GenericAPIError{Code: "TestCode", Message: "TestMessage"},
			Code:     "TestCode",
			Messages: []string{"NotMatching", "estMess"},
			Expected: true,
		},
		"matching code and no matching messages": {
			Err:      &smithy.GenericAPIError{Code: "TestCode", Message: "TestMessage"},
			Code:     "TestCode",
			Messages: []string{"NotMatching", "AlsoNotMatching"},
			Expected: false,
		},
		"matching message and non-matching code": {
			Err:      &smithy.GenericAPIError{Code: "TestCode", Message: "TestMessage"},
			Code:     "NotMatching",
			Messages: []string{"TestMessage", "estMess"},
			Expected: false,
		},
		"matching code and no messages": {
			Err:      &smithy.GenericAPIError{Code: "TestCode", Message: "TestMessage"},
			Code:     "TestCode",
			Messages: nil,
			Expected: false,
		},
		"wrapped error matches": {
			Err:      fmt.Errorf("test: %w", &smithy.GenericAPIError{Code: "TestCode", Message: "TestMessage"}),
			Code:     "TestCode",
			Messages: []string{"NotMatching", "TestMessage"},
			Expected: true,
		},
		"sts error matches": {
			Err:      &types.ExpiredTokenException{ErrorCodeOverride: aws.String("TestCode"), Message: aws.String("TestMessage")},
			Code:     "TestCode",
			Messages: []string{"NotMatching", "estMess"},
			Expected: true,
		},
	}

	for name, testCase := range testCases {
		t.Run(name, func(t *testing.T) {
			got := ErrMessageContainsAny(testCase.Err, testCase.Code, testCase.Messages...)

			if got != testCase.Expected {
				t.Errorf("got %t, expected %t", got, testCase.Expected)
			}
		})
	}
}

func TestErrHTTPStatusCodeEquals(t *testing.T) {
	testCases := map[string]struct {
		Err      error
		Codes    []int
		Expected bool
	}{
		"nil error": {
			Err:      nil,
			Expected: false,
		},
		"other error": {
			Err:      fmt.Errorf("other error"),
			Expected: false,
		},
		"Top-level smithyhttp.ResponseError matching first code": {
			Err:      &smithyhttp.ResponseError{Response: &smithyhttp.Response{Response: &http.Response{StatusCode: http.StatusNotFound}}},
			Codes:    []int{http.StatusNotFound},
			Expected: true,
		},
		"Top-level smithyhttp.ResponseError matching last code": {
			Err:      &smithyhttp.ResponseError{Response: &smithyhttp.Response{Response: &http.Response{StatusCode: http.StatusOK}}},
			Codes:    []int{http.StatusNotFound, http.StatusOK},
			Expected: true,
		},
		"Top-level smithyhttp.ResponseError no code": {
			Err: &smithyhttp.ResponseError{Response: &smithyhttp.Response{Response: &http.Response{StatusCode: http.StatusOK}}},
		},
		"Top-level smithyhttp.ResponseError non-matching codes": {
			Err:   &smithyhttp.ResponseError{Response: &smithyhttp.Response{Response: &http.Response{StatusCode: http.StatusOK}}},
			Codes: []int{http.StatusNotFound, http.StatusNoContent},
		},
		"Wrapped smithyhttp.ResponseError matching first code": {
			Err:      &smithy.OperationError{Err: &smithyhttp.ResponseError{Response: &smithyhttp.Response{Response: &http.Response{StatusCode: http.StatusNotFound}}}},
			Codes:    []int{http.StatusNotFound},
			Expected: true,
		},
		"Wrapped smithyhttp.ResponseError matching last code": {
			Err:      &smithy.OperationError{Err: &smithyhttp.ResponseError{Response: &smithyhttp.Response{Response: &http.Response{StatusCode: http.StatusOK}}}},
			Codes:    []int{http.StatusNotFound, http.StatusOK},
			Expected: true,
		},
		"Wrapped smithyhttp.ResponseError non-matching codes": {
			Err:   &smithy.OperationError{Err: &smithyhttp.ResponseError{Response: &smithyhttp.Response{Response: &http.Response{StatusCode: http.StatusOK}}}},
			Codes: []int{http.StatusNotFound, http.StatusNoContent},
		},
	}

	for name, testCase := range testCases {
		t.Run(name, func(t *testing.T) {
			got := ErrHTTPStatusCodeEquals(testCase.Err, testCase.Codes...)

			if got != testCase.Expected {
				t.Errorf("got %t, expected %t", got, testCase.Expected)
			}
		})
	}
}

// nestedAPIError is a smithy.APIError which wraps another error, the shape used
// by AWS APIs that return a generic error code with a specific one inside it.
type nestedAPIError struct {
	code    string
	message string
	err     error
}

func (e *nestedAPIError) Error() string {
	return fmt.Sprintf("%s: %s", e.code, e.message)
}

func (e *nestedAPIError) ErrorCode() string { return e.code }

func (e *nestedAPIError) ErrorMessage() string { return e.message }

func (e *nestedAPIError) ErrorFault() smithy.ErrorFault { return smithy.FaultClient }

func (e *nestedAPIError) Unwrap() error { return e.err }

func TestErrCodeEqualsNested(t *testing.T) {
	testCases := map[string]struct {
		Err      error
		Codes    []string
		Expected bool
	}{
		"nil error": {
			Err:      nil,
			Codes:    []string{"TestCode"},
			Expected: false,
		},
		"other error": {
			Err:   fmt.Errorf("other error"),
			Codes: []string{"TestCode"},
		},
		"no codes": {
			Err: &smithy.GenericAPIError{Code: "TestCode", Message: "TestMessage"},
		},
		"top-level smithy.GenericAPIError matching": {
			Err:      &smithy.GenericAPIError{Code: "TestCode", Message: "TestMessage"},
			Codes:    []string{"TestCode"},
			Expected: true,
		},
		"top-level smithy.GenericAPIError non-matching": {
			Err:   &smithy.GenericAPIError{Code: "TestCode", Message: "TestMessage"},
			Codes: []string{"NotMatching"},
		},
		"outer code of a nested error": {
			Err: &nestedAPIError{
				code:    "OuterCode",
				message: "OuterMessage",
				err:     &smithy.GenericAPIError{Code: "InnerCode", Message: "InnerMessage"},
			},
			Codes:    []string{"OuterCode"},
			Expected: true,
		},
		"inner code of a nested error": {
			Err: &nestedAPIError{
				code:    "OuterCode",
				message: "OuterMessage",
				err:     &smithy.GenericAPIError{Code: "InnerCode", Message: "InnerMessage"},
			},
			Codes:    []string{"InnerCode"},
			Expected: true,
		},
		"inner code of a nested error matching last code": {
			Err: &nestedAPIError{
				code:    "OuterCode",
				message: "OuterMessage",
				err:     &smithy.GenericAPIError{Code: "InnerCode", Message: "InnerMessage"},
			},
			Codes:    []string{"NotMatching", "InnerCode"},
			Expected: true,
		},
		"nested error non-matching": {
			Err: &nestedAPIError{
				code:    "OuterCode",
				message: "OuterMessage",
				err:     &smithy.GenericAPIError{Code: "InnerCode", Message: "InnerMessage"},
			},
			Codes: []string{"NotMatching", "AlsoNotMatching"},
		},
		"inner code below a plain wrapper": {
			Err: &nestedAPIError{
				code:    "OuterCode",
				message: "OuterMessage",
				err:     fmt.Errorf("test: %w", &smithy.GenericAPIError{Code: "InnerCode", Message: "InnerMessage"}),
			},
			Codes:    []string{"InnerCode"},
			Expected: true,
		},
		"innermost code of three levels": {
			Err: &nestedAPIError{
				code:    "OuterCode",
				message: "OuterMessage",
				err: &nestedAPIError{
					code:    "MiddleCode",
					message: "MiddleMessage",
					err:     &smithy.GenericAPIError{Code: "InnerCode", Message: "InnerMessage"},
				},
			},
			Codes:    []string{"InnerCode"},
			Expected: true,
		},
		"nested error wrapped in a plain error": {
			Err: fmt.Errorf("test: %w", &nestedAPIError{
				code:    "OuterCode",
				message: "OuterMessage",
				err:     &smithy.GenericAPIError{Code: "InnerCode", Message: "InnerMessage"},
			}),
			Codes:    []string{"InnerCode"},
			Expected: true,
		},
		"second error of a joined tree": {
			Err: errors.Join(
				fmt.Errorf("other error"),
				&smithy.GenericAPIError{Code: "InnerCode", Message: "InnerMessage"},
			),
			Codes:    []string{"InnerCode"},
			Expected: true,
		},
		"joined tree non-matching": {
			Err: errors.Join(
				fmt.Errorf("other error"),
				&smithy.GenericAPIError{Code: "InnerCode", Message: "InnerMessage"},
			),
			Codes: []string{"NotMatching"},
		},
		"nested error with no inner error": {
			Err: &nestedAPIError{
				code:    "OuterCode",
				message: "OuterMessage",
			},
			Codes: []string{"InnerCode"},
		},
	}

	for name, testCase := range testCases {
		t.Run(name, func(t *testing.T) {
			got := ErrCodeEqualsNested(testCase.Err, testCase.Codes...)

			if got != testCase.Expected {
				t.Errorf("got %t, expected %t", got, testCase.Expected)
			}
		})
	}
}

// TestErrCodeEqualsNestedDiffersFromErrCodeEquals documents the reason the
// nested variant exists.
func TestErrCodeEqualsNestedDiffersFromErrCodeEquals(t *testing.T) {
	err := &nestedAPIError{
		code:    "OuterCode",
		message: "OuterMessage",
		err:     &smithy.GenericAPIError{Code: "InnerCode", Message: "InnerMessage"},
	}

	if ErrCodeEquals(err, "InnerCode") {
		t.Error("ErrCodeEquals matched a nested code, expected it not to")
	}

	if !ErrCodeEqualsNested(err, "InnerCode") {
		t.Error("ErrCodeEqualsNested did not match a nested code")
	}
}
