module github.com/hashicorp/aws-sdk-go-base/v2/awsv1shim/v2

require (
	github.com/aws/aws-sdk-go v1.44.197
	github.com/aws/aws-sdk-go-v2 v1.17.4
	github.com/aws/aws-sdk-go-v2/config v1.18.12
	github.com/aws/aws-sdk-go-v2/credentials v1.13.12
	github.com/aws/aws-sdk-go-v2/feature/ec2/imds v1.12.22
	github.com/aws/aws-sdk-go-v2/service/sts v1.18.3
	github.com/google/go-cmp v0.5.9
	github.com/hashicorp/aws-sdk-go-base/v2 v2.0.0-beta.24
	github.com/hashicorp/go-cleanhttp v0.5.2
	github.com/hashicorp/terraform-plugin-log v0.8.0
	go.opentelemetry.io/otel v1.13.0
)

require (
	github.com/aws/aws-sdk-go-v2/internal/configsources v1.1.28 // indirect
	github.com/aws/aws-sdk-go-v2/internal/endpoints/v2 v2.4.22 // indirect
	github.com/aws/aws-sdk-go-v2/internal/ini v1.3.29 // indirect
	github.com/aws/aws-sdk-go-v2/service/iam v1.19.2 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/presigned-url v1.9.22 // indirect
	github.com/aws/aws-sdk-go-v2/service/sso v1.12.1 // indirect
	github.com/aws/aws-sdk-go-v2/service/ssooidc v1.14.1 // indirect
	github.com/aws/smithy-go v1.13.5 // indirect
	github.com/fatih/color v1.13.0 // indirect
	github.com/hashicorp/errwrap v1.1.0 // indirect
	github.com/hashicorp/go-hclog v1.4.0 // indirect
	github.com/hashicorp/go-multierror v1.1.1 // indirect
	github.com/jmespath/go-jmespath v0.4.0 // indirect
	github.com/mattn/go-colorable v0.1.12 // indirect
	github.com/mattn/go-isatty v0.0.14 // indirect
	github.com/mitchellh/go-homedir v1.1.0 // indirect
	github.com/mitchellh/go-testing-interface v1.14.1 // indirect
	go.opentelemetry.io/otel/trace v1.13.0 // indirect
	golang.org/x/exp v0.0.0-20230131160201-f062dba9d201 // indirect
	golang.org/x/sys v0.1.0 // indirect
)

go 1.18

replace github.com/hashicorp/aws-sdk-go-base/v2 => ../..
