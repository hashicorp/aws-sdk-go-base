module github.com/hashicorp/aws-sdk-go-base/v2/awsv1shim/v2

require (
	github.com/aws/aws-sdk-go v1.42.48
	github.com/aws/aws-sdk-go-v2 v1.13.0
	github.com/google/go-cmp v0.5.7
	github.com/hashicorp/aws-sdk-go-base/v2 v2.0.0-beta.5
)

go 1.16

replace github.com/hashicorp/aws-sdk-go-base/v2 => ../..
