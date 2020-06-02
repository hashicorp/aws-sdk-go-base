package awsbase

import (
	"errors"
	"fmt"
	"testing"

	"github.com/aws/aws-sdk-go/aws/awserr"
)

func TestIsAwsErr(t *testing.T) {
	testCases := []struct {
		Name     string
		Err      error
		Code     string
		Message  string
		Expected bool
	}{
		{
			Name: "nil error",
			Err:  nil,
		},
		{
			Name: "nil error code",
			Err:  nil,
			Code: "test",
		},
		{
			Name:    "nil error message",
			Err:     nil,
			Message: "test",
		},
		{
			Name:    "nil error code and message",
			Err:     nil,
			Code:    "test",
			Message: "test",
		},
		{
			Name: "other error",
			Err:  errors.New("test"),
		},
		{
			Name: "other error code",
			Err:  errors.New("test"),
			Code: "test",
		},
		{
			Name:    "other error message",
			Err:     errors.New("test"),
			Message: "test",
		},
		{
			Name:    "other error code and message",
			Err:     errors.New("test"),
			Code:    "test",
			Message: "test",
		},
		{
			Name:     "awserr error matching code and no message",
			Err:      awserr.New("TestCode", "TestMessage", nil),
			Code:     "TestCode",
			Expected: true,
		},
		{
			Name:     "awserr error matching code and matching message exact",
			Err:      awserr.New("TestCode", "TestMessage", nil),
			Code:     "TestCode",
			Message:  "TestMessage",
			Expected: true,
		},
		{
			Name:     "awserr error matching code and matching message contains",
			Err:      awserr.New("TestCode", "TestMessage", nil),
			Code:     "TestCode",
			Message:  "Message",
			Expected: true,
		},
		{
			Name:    "awserr error matching code and non-matching message",
			Err:     awserr.New("TestCode", "TestMessage", nil),
			Code:    "TestCode",
			Message: "NotMatching",
		},
		{
			Name: "awserr error no code",
			Err:  awserr.New("TestCode", "TestMessage", nil),
		},
		{
			Name:    "awserr error no code and matching message exact",
			Err:     awserr.New("TestCode", "TestMessage", nil),
			Message: "TestMessage",
		},
		{
			Name: "awserr error non-matching code",
			Err:  awserr.New("TestCode", "TestMessage", nil),
			Code: "NotMatching",
		},
		{
			Name:    "awserr error non-matching code and message exact",
			Err:     awserr.New("TestCode", "TestMessage", nil),
			Message: "TestMessage",
		},
		{
			Name: "wrapped other error",
			Err:  fmt.Errorf("test: %w", errors.New("test")),
		},
		{
			Name: "wrapped other error code",
			Err:  fmt.Errorf("test: %w", errors.New("test")),
			Code: "test",
		},
		{
			Name:    "wrapped other error message",
			Err:     fmt.Errorf("test: %w", errors.New("test")),
			Message: "test",
		},
		{
			Name:    "wrapped other error code and message",
			Err:     fmt.Errorf("test: %w", errors.New("test")),
			Code:    "test",
			Message: "test",
		},
		{
			Name:     "wrapped awserr error matching code and no message",
			Err:      fmt.Errorf("test: %w", awserr.New("TestCode", "TestMessage", nil)),
			Code:     "TestCode",
			Expected: true,
		},
		{
			Name:     "wrapped awserr error matching code and matching message exact",
			Err:      fmt.Errorf("test: %w", awserr.New("TestCode", "TestMessage", nil)),
			Code:     "TestCode",
			Message:  "TestMessage",
			Expected: true,
		},
		{
			Name:     "wrapped awserr error matching code and matching message contains",
			Err:      fmt.Errorf("test: %w", awserr.New("TestCode", "TestMessage", nil)),
			Code:     "TestCode",
			Message:  "Message",
			Expected: true,
		},
		{
			Name:    "wrapped awserr error matching code and non-matching message",
			Err:     fmt.Errorf("test: %w", awserr.New("TestCode", "TestMessage", nil)),
			Code:    "TestCode",
			Message: "NotMatching",
		},
		{
			Name: "wrapped awserr error no code",
			Err:  fmt.Errorf("test: %w", awserr.New("TestCode", "TestMessage", nil)),
		},
		{
			Name:    "wrapped awserr error no code and matching message exact",
			Err:     fmt.Errorf("test: %w", awserr.New("TestCode", "TestMessage", nil)),
			Message: "TestMessage",
		},
		{
			Name: "wrapped awserr error non-matching code",
			Err:  fmt.Errorf("test: %w", awserr.New("TestCode", "TestMessage", nil)),
			Code: "NotMatching",
		},
		{
			Name:    "wrapped awserr error non-matching code and message exact",
			Err:     fmt.Errorf("test: %w", awserr.New("TestCode", "TestMessage", nil)),
			Message: "TestMessage",
		},
	}

	for _, testCase := range testCases {
		testCase := testCase

		t.Run(testCase.Name, func(t *testing.T) {
			got := IsAWSErr(testCase.Err, testCase.Code, testCase.Message)

			if got != testCase.Expected {
				t.Errorf("got %t, expected %t", got, testCase.Expected)
			}
		})
	}
}

func TestIsAwsErrExtended(t *testing.T) {
	testCases := []struct {
		Name            string
		Err             error
		Code            string
		Message         string
		ExtendedMessage string
		Expected        bool
	}{
		{
			Name: "nil error",
			Err:  nil,
		},
		{
			Name: "nil error code",
			Err:  nil,
			Code: "test",
		},
		{
			Name:    "nil error message",
			Err:     nil,
			Message: "test",
		},
		{
			Name:    "nil error code and message",
			Err:     nil,
			Code:    "test",
			Message: "test",
		},
		{
			Name:            "nil error code, message, and extended message",
			Err:             nil,
			Code:            "test",
			Message:         "test",
			ExtendedMessage: "test",
		},
		{
			Name: "other error",
			Err:  errors.New("test"),
		},
		{
			Name: "other error code",
			Err:  errors.New("test"),
			Code: "test",
		},
		{
			Name:    "other error message",
			Err:     errors.New("test"),
			Message: "test",
		},
		{
			Name:    "other error code and message",
			Err:     errors.New("test"),
			Code:    "test",
			Message: "test",
		},
		{
			Name:            "other error code, message, and extended message",
			Err:             errors.New("test"),
			Code:            "test",
			Message:         "test",
			ExtendedMessage: "test",
		},
		{
			Name:     "awserr non-extended error matching code, no message, and no extended message",
			Err:      awserr.New("TestCode", "TestMessage", nil),
			Code:     "TestCode",
			Expected: true,
		},
		{
			Name:            "awserr non-extended error matching code, no message, and extended message",
			Err:             awserr.New("TestCode", "TestMessage", nil),
			Code:            "TestCode",
			ExtendedMessage: "TestExtendedMessage",
		},
		{
			Name:     "awserr non-extended error matching code, matching message exact, and no extended message",
			Err:      awserr.New("TestCode", "TestMessage", nil),
			Code:     "TestCode",
			Message:  "TestMessage",
			Expected: true,
		},
		{
			Name:            "awserr non-extended error matching code, matching message exact, and extended message",
			Err:             awserr.New("TestCode", "TestMessage", nil),
			Code:            "TestCode",
			Message:         "TestMessage",
			ExtendedMessage: "TestExtendedMessage",
		},
		{
			Name:     "awserr non-extended error matching code, matching message contains, and no extended message",
			Err:      awserr.New("TestCode", "TestMessage", nil),
			Code:     "TestCode",
			Message:  "Message",
			Expected: true,
		},
		{
			Name:            "awserr non-extended error matching code, matching message contains, and extended message",
			Err:             awserr.New("TestCode", "TestMessage", nil),
			Code:            "TestCode",
			Message:         "Message",
			ExtendedMessage: "Message",
		},
		{
			Name:    "awserr non-extended error matching code, non-matching message, and no extended message",
			Err:     awserr.New("TestCode", "TestMessage", nil),
			Code:    "TestCode",
			Message: "NotMatching",
		},
		{
			Name: "awserr non-extended error no code, no message, and no extended message",
			Err:  awserr.New("TestCode", "TestMessage", nil),
		},
		{
			Name:    "awserr non-extended error no code, matching message exact, and no extended message",
			Err:     awserr.New("TestCode", "TestMessage", nil),
			Message: "TestMessage",
		},
		{
			Name: "awserr non-extended error non-matching code, no message, and no extended message",
			Err:  awserr.New("TestCode", "TestMessage", nil),
			Code: "NotMatching",
		},
		{
			Name:    "awserr non-extended error non-matching code, matching message exact, and no extended message",
			Err:     awserr.New("TestCode", "TestMessage", errors.New("TestExtendedMessage")),
			Code:    "NonMatching",
			Message: "TestMessage",
		},
		{
			Name:     "awserr extended error matching code, no message, and no extended message",
			Err:      awserr.New("TestCode", "TestMessage", errors.New("TestExtendedMessage")),
			Code:     "TestCode",
			Expected: true,
		},
		{
			Name:            "awserr extended error matching code, no message, and matching extended message exact",
			Err:             awserr.New("TestCode", "TestMessage", errors.New("TestExtendedMessage")),
			Code:            "TestCode",
			ExtendedMessage: "TestExtendedMessage",
			Expected:        true,
		},
		{
			Name:            "awserr extended error matching code, no message, and matching extended message contains",
			Err:             awserr.New("TestCode", "TestMessage", errors.New("TestExtendedMessage")),
			Code:            "TestCode",
			ExtendedMessage: "ExtendedMessage",
			Expected:        true,
		},
		{
			Name:     "awserr extended error matching code, matching message exact, and no extended message",
			Err:      awserr.New("TestCode", "TestMessage", errors.New("TestExtendedMessage")),
			Code:     "TestCode",
			Message:  "TestMessage",
			Expected: true,
		},
		{
			Name:            "awserr extended error matching code, matching message exact, and matching extended message exact",
			Err:             awserr.New("TestCode", "TestMessage", errors.New("TestExtendedMessage")),
			Code:            "TestCode",
			Message:         "TestMessage",
			ExtendedMessage: "TestExtendedMessage",
			Expected:        true,
		},
		{
			Name:            "awserr extended error matching code, matching message exact, and matching extended message contains",
			Err:             awserr.New("TestCode", "TestMessage", errors.New("TestExtendedMessage")),
			Code:            "TestCode",
			Message:         "TestMessage",
			ExtendedMessage: "ExtendedMessage",
			Expected:        true,
		},
		{
			Name:     "awserr extended error matching code, matching message contains, and no extended message",
			Err:      awserr.New("TestCode", "TestMessage", errors.New("TestExtendedMessage")),
			Code:     "TestCode",
			Message:  "Message",
			Expected: true,
		},
		{
			Name:            "awserr extended error matching code, matching message contains, and matching extended message contains",
			Err:             awserr.New("TestCode", "TestMessage", errors.New("TestExtendedMessage")),
			Code:            "TestCode",
			Message:         "Message",
			ExtendedMessage: "ExtendedMessage",
			Expected:        true,
		},
		{
			Name:    "awserr extended error matching code, non-matching message, and no extended message",
			Err:     awserr.New("TestCode", "TestMessage", errors.New("TestExtendedMessage")),
			Code:    "TestCode",
			Message: "NotMatching",
		},
		{
			Name: "awserr extended error no code, no message, and no extended message",
			Err:  awserr.New("TestCode", "TestMessage", errors.New("TestExtendedMessage")),
		},
		{
			Name:    "awserr extended error no code, matching message exact, and no extended message",
			Err:     awserr.New("TestCode", "TestMessage", errors.New("TestExtendedMessage")),
			Message: "TestMessage",
		},
		{
			Name: "awserr extended error non-matching code, no message, and no extended message",
			Err:  awserr.New("TestCode", "TestMessage", errors.New("TestExtendedMessage")),
			Code: "NotMatching",
		},
		{
			Name:    "awserr extended error non-matching code, matching message exact, and no extended message",
			Err:     awserr.New("TestCode", "TestMessage", errors.New("TestExtendedMessage")),
			Code:    "NonMatching",
			Message: "TestMessage",
		},
	}

	for _, testCase := range testCases {
		testCase := testCase

		t.Run(testCase.Name, func(t *testing.T) {
			got := IsAWSErrExtended(testCase.Err, testCase.Code, testCase.Message, testCase.ExtendedMessage)

			if got != testCase.Expected {
				t.Errorf("got %t, expected %t", got, testCase.Expected)
			}
		})
	}
}

func TestIsAwsErrCode(t *testing.T) {
	testCases := []struct {
		Name     string
		Err      error
		Code     string
		Expected bool
	}{
		{
			Name: "nil error",
			Err:  nil,
		},
		{
			Name: "nil error code",
			Err:  nil,
			Code: "test",
		},
		{
			Name: "other error",
			Err:  errors.New("test"),
		},
		{
			Name: "other error code",
			Err:  errors.New("test"),
			Code: "test",
		},
		{
			Name:     "awserr error matching code",
			Err:      awserr.New("TestCode", "TestMessage", nil),
			Code:     "TestCode",
			Expected: true,
		},
		{
			Name: "awserr error no code",
			Err:  awserr.New("TestCode", "TestMessage", nil),
		},
		{
			Name: "awserr error non-matching code",
			Err:  awserr.New("TestCode", "TestMessage", nil),
			Code: "NotMatching",
		},
		{
			Name: "wrapped other error",
			Err:  fmt.Errorf("test: %w", errors.New("test")),
		},
		{
			Name: "wrapped other error code",
			Err:  fmt.Errorf("test: %w", errors.New("test")),
			Code: "test",
		},
		{
			Name:     "wrapped awserr error matching code",
			Err:      fmt.Errorf("test: %w", awserr.New("TestCode", "TestMessage", nil)),
			Code:     "TestCode",
			Expected: true,
		},
		{
			Name: "wrapped awserr error no code",
			Err:  fmt.Errorf("test: %w", awserr.New("TestCode", "TestMessage", nil)),
		},
		{
			Name: "wrapped awserr error non-matching code",
			Err:  fmt.Errorf("test: %w", awserr.New("TestCode", "TestMessage", nil)),
			Code: "NotMatching",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.Name, func(t *testing.T) {
			got := IsAWSErrCode(testCase.Err, testCase.Code)

			if got != testCase.Expected {
				t.Errorf("got %t, expected %t", got, testCase.Expected)
			}
		})
	}
}

func TestIsAwsErrCodeContains(t *testing.T) {
	testCases := []struct {
		Name     string
		Err      error
		Code     string
		Expected bool
	}{
		{
			Name: "nil error",
			Err:  nil,
		},
		{
			Name: "nil error code",
			Err:  nil,
			Code: "test",
		},
		{
			Name: "other error",
			Err:  errors.New("test"),
		},
		{
			Name: "other error code",
			Err:  errors.New("test"),
			Code: "test",
		},
		{
			Name:     "awserr error matching code",
			Err:      awserr.New("TestCode", "TestMessage", nil),
			Code:     "TestCode",
			Expected: true,
		},
		{
			Name:     "awserr error contains code",
			Err:      awserr.New("TestCode", "TestMessage", nil),
			Code:     "Test",
			Expected: true,
		},
		{
			Name:     "awserr error no code",
			Err:      awserr.New("TestCode", "TestMessage", nil),
			Expected: true,
		},
		{
			Name: "awserr error non-matching code",
			Err:  awserr.New("TestCode", "TestMessage", nil),
			Code: "NotMatching",
		},
		{
			Name: "wrapped other error",
			Err:  fmt.Errorf("test: %w", errors.New("test")),
		},
		{
			Name: "wrapped other error code",
			Err:  fmt.Errorf("test: %w", errors.New("test")),
			Code: "test",
		},
		{
			Name:     "wrapped awserr error matching code",
			Err:      fmt.Errorf("test: %w", awserr.New("TestCode", "TestMessage", nil)),
			Code:     "TestCode",
			Expected: true,
		},
		{
			Name:     "wrapped awserr error contains code",
			Err:      fmt.Errorf("test: %w", awserr.New("TestCode", "TestMessage", nil)),
			Code:     "Test",
			Expected: true,
		},
		{
			Name:     "wrapped awserr error no code",
			Err:      fmt.Errorf("test: %w", awserr.New("TestCode", "TestMessage", nil)),
			Expected: true,
		},
		{
			Name: "wrapped awserr error non-matching code",
			Err:  fmt.Errorf("test: %w", awserr.New("TestCode", "TestMessage", nil)),
			Code: "NotMatching",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.Name, func(t *testing.T) {
			got := IsAWSErrCodeContains(testCase.Err, testCase.Code)

			if got != testCase.Expected {
				t.Errorf("got %t, expected %t", got, testCase.Expected)
			}
		})
	}
}

func TestIsAWSErrCodeMessageContains(t *testing.T) {
	testCases := []struct {
		Name     string
		Err      error
		Code     string
		Message  string
		Expected bool
	}{
		{
			Name: "nil error",
			Err:  nil,
		},
		{
			Name: "nil error code",
			Err:  nil,
			Code: "test",
		},
		{
			Name:    "nil error message",
			Err:     nil,
			Message: "test",
		},
		{
			Name:    "nil error code and message",
			Err:     nil,
			Code:    "test",
			Message: "test",
		},
		{
			Name: "other error",
			Err:  errors.New("test"),
		},
		{
			Name: "other error code",
			Err:  errors.New("test"),
			Code: "test",
		},
		{
			Name:    "other error message",
			Err:     errors.New("test"),
			Message: "test",
		},
		{
			Name:    "other error code and message",
			Err:     errors.New("test"),
			Code:    "test",
			Message: "test",
		},
		{
			Name:     "awserr error matching code and no message",
			Err:      awserr.New("TestCode", "TestMessage", nil),
			Code:     "TestCode",
			Expected: true,
		},
		{
			Name:     "awserr error matching code and matching message exact",
			Err:      awserr.New("TestCode", "TestMessage", nil),
			Code:     "TestCode",
			Message:  "TestMessage",
			Expected: true,
		},
		{
			Name:     "awserr error matching code and matching message contains",
			Err:      awserr.New("TestCode", "TestMessage", nil),
			Code:     "TestCode",
			Message:  "Message",
			Expected: true,
		},
		{
			Name:    "awserr error matching code and non-matching message",
			Err:     awserr.New("TestCode", "TestMessage", nil),
			Code:    "TestCode",
			Message: "NotMatching",
		},
		{
			Name: "awserr error no code",
			Err:  awserr.New("TestCode", "TestMessage", nil),
		},
		{
			Name:    "awserr error no code and matching message exact",
			Err:     awserr.New("TestCode", "TestMessage", nil),
			Message: "TestMessage",
		},
		{
			Name: "awserr error non-matching code",
			Err:  awserr.New("TestCode", "TestMessage", nil),
			Code: "NotMatching",
		},
		{
			Name:    "awserr error non-matching code and message exact",
			Err:     awserr.New("TestCode", "TestMessage", nil),
			Message: "TestMessage",
		},
		{
			Name: "wrapped other error",
			Err:  fmt.Errorf("test: %w", errors.New("test")),
		},
		{
			Name: "wrapped other error code",
			Err:  fmt.Errorf("test: %w", errors.New("test")),
			Code: "test",
		},
		{
			Name:    "wrapped other error message",
			Err:     fmt.Errorf("test: %w", errors.New("test")),
			Message: "test",
		},
		{
			Name:    "wrapped other error code and message",
			Err:     fmt.Errorf("test: %w", errors.New("test")),
			Code:    "test",
			Message: "test",
		},
		{
			Name:     "wrapped awserr error matching code and no message",
			Err:      fmt.Errorf("test: %w", awserr.New("TestCode", "TestMessage", nil)),
			Code:     "TestCode",
			Expected: true,
		},
		{
			Name:     "wrapped awserr error matching code and matching message exact",
			Err:      fmt.Errorf("test: %w", awserr.New("TestCode", "TestMessage", nil)),
			Code:     "TestCode",
			Message:  "TestMessage",
			Expected: true,
		},
		{
			Name:     "wrapped awserr error matching code and matching message contains",
			Err:      fmt.Errorf("test: %w", awserr.New("TestCode", "TestMessage", nil)),
			Code:     "TestCode",
			Message:  "Message",
			Expected: true,
		},
		{
			Name:    "wrapped awserr error matching code and non-matching message",
			Err:     fmt.Errorf("test: %w", awserr.New("TestCode", "TestMessage", nil)),
			Code:    "TestCode",
			Message: "NotMatching",
		},
		{
			Name: "wrapped awserr error no code",
			Err:  fmt.Errorf("test: %w", awserr.New("TestCode", "TestMessage", nil)),
		},
		{
			Name:    "wrapped awserr error no code and matching message exact",
			Err:     fmt.Errorf("test: %w", awserr.New("TestCode", "TestMessage", nil)),
			Message: "TestMessage",
		},
		{
			Name: "wrapped awserr error non-matching code",
			Err:  fmt.Errorf("test: %w", awserr.New("TestCode", "TestMessage", nil)),
			Code: "NotMatching",
		},
		{
			Name:    "wrapped awserr error non-matching code and message exact",
			Err:     fmt.Errorf("test: %w", awserr.New("TestCode", "TestMessage", nil)),
			Message: "TestMessage",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.Name, func(t *testing.T) {
			got := IsAWSErrCodeMessageContains(testCase.Err, testCase.Code, testCase.Message)

			if got != testCase.Expected {
				t.Errorf("got %t, expected %t", got, testCase.Expected)
			}
		})
	}
}

func TestIsAWSErrRequestFailureStatusCode(t *testing.T) {
	testCases := []struct {
		Name       string
		Err        error
		StatusCode int
		Expected   bool
	}{
		{
			Name: "nil error",
			Err:  nil,
		},
		{
			Name:       "nil error status code",
			Err:        nil,
			StatusCode: 42,
		},
		{
			Name: "other error",
			Err:  errors.New("test"),
		},
		{
			Name:       "other error status code",
			Err:        errors.New("test"),
			StatusCode: 42,
		},
		{
			Name:       "awserr error matching status code",
			Err:        awserr.NewRequestFailure(awserr.New("TestCode", "TestMessage", nil), 42, ""),
			StatusCode: 42,
			Expected:   true,
		},
		{
			Name:       "awserr error non-matching statuc code",
			Err:        awserr.NewRequestFailure(awserr.New("TestCode", "TestMessage", nil), 404, ""),
			StatusCode: 42,
		},
		{
			Name: "wrapped other error",
			Err:  fmt.Errorf("test: %w", errors.New("test")),
		},
		{
			Name:       "wrapped other status code",
			Err:        fmt.Errorf("test: %w", errors.New("test")),
			StatusCode: 42,
		},
		{
			Name:       "wrapped awserr error matching status code",
			Err:        fmt.Errorf("test: %w", awserr.NewRequestFailure(awserr.New("TestCode", "TestMessage", nil), 42, "")),
			StatusCode: 42,
			Expected:   true,
		},
		{
			Name:       "wrapped awserr error non-matching status code",
			Err:        fmt.Errorf("test: %w", awserr.NewRequestFailure(awserr.New("TestCode", "TestMessage", nil), 404, "")),
			StatusCode: 42,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.Name, func(t *testing.T) {
			got := IsAWSErrRequestFailureStatusCode(testCase.Err, testCase.StatusCode)

			if got != testCase.Expected {
				t.Errorf("got %t, expected %t", got, testCase.Expected)
			}
		})
	}
}
