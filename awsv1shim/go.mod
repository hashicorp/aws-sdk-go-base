module github.com/hashicorp/aws-sdk-go-base/awsv1shim

require (
	github.com/aws/aws-sdk-go v1.40.43
	github.com/aws/aws-sdk-go-v2 v1.9.0
	github.com/google/go-cmp v0.5.6
	github.com/hashicorp/aws-sdk-go-base v0.0.0-00010101000000-000000000000
	github.com/hashicorp/go-cleanhttp v0.5.2
	github.com/hashicorp/go-multierror v1.1.1
)

go 1.16

replace github.com/hashicorp/aws-sdk-go-base => ../
