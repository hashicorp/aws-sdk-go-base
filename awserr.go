package awsbase

import (
	"errors"
	"strings"

	"github.com/aws/aws-sdk-go/aws/awserr"
)

var (
	// IsAWSErr returns true if the error matches all these conditions:
	//  * err is of type awserr.Error
	//  * Error.Code() matches code
	//  * Error.Message() contains message
	IsAWSErr = IsAWSErrCodeMessageContains
)

// IsAWSErrExtended returns true if the error matches all these conditions:
//  * err is of type awserr.Error
//  * Error.Code() matches code
//  * Error.Message() contains message
//  * Error.OrigErr() contains origErrMessage
func IsAWSErrExtended(err error, code string, message string, origErrMessage string) bool {
	if !IsAWSErr(err, code, message) {
		return false
	}

	if origErrMessage == "" {
		return true
	}

	// Ensure OrigErr() is non-nil, to prevent panics
	if origErr := err.(awserr.Error).OrigErr(); origErr != nil {
		return strings.Contains(origErr.Error(), origErrMessage)
	}

	return false
}

// IsAWSErrCode returns true if the error matches all these conditions:
//  * err is of type awserr.Error
//  * Error.Code() equals code
func IsAWSErrCode(err error, code string) bool {
	var awsErr awserr.Error
	if errors.As(err, &awsErr) {
		return awsErr.Code() == code
	}
	return false
}

// IsAWSErrCodeContains returns true if the error matches all these conditions:
//  * err is of type awserr.Error
//  * Error.Code() contains code
func IsAWSErrCodeContains(err error, code string) bool {
	var awsErr awserr.Error
	if errors.As(err, &awsErr) {
		return strings.Contains(awsErr.Code(), code)
	}
	return false
}

// IsAWSErrCodeMessageContains returns true if the error matches all these conditions:
//  * err is of type awserr.Error
//  * Error.Code() equals code
//  * Error.Message() contains message
func IsAWSErrCodeMessageContains(err error, code string, message string) bool {
	var awsErr awserr.Error
	if errors.As(err, &awsErr) {
		return awsErr.Code() == code && strings.Contains(awsErr.Message(), message)
	}
	return false
}

// IsAWSErrRequestFailureStatusCode returns true if the error matches all these conditions:
//  * err is of type awserr.RequestFailure
//  * RequestFailure.StatusCode() equals statusCode
// It is always preferable to use IsAWSErr() except in older APIs (e.g. S3)
// that sometimes only respond with status codes.
func IsAWSErrRequestFailureStatusCode(err error, statusCode int) bool {
	var awsErr awserr.RequestFailure
	if errors.As(err, &awsErr) {
		return awsErr.StatusCode() == statusCode
	}
	return false
}
