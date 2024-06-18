module github.com/hashicorp/aws-sdk-go-base/v2/awsv1shim/v2

go 1.21

require (
	github.com/aws/aws-sdk-go v1.54.3
	github.com/aws/aws-sdk-go-v2 v1.28.0
	github.com/aws/aws-sdk-go-v2/config v1.27.19
	github.com/aws/aws-sdk-go-v2/credentials v1.17.19
	github.com/aws/aws-sdk-go-v2/feature/ec2/imds v1.16.6
	github.com/aws/aws-sdk-go-v2/service/sts v1.28.13
	github.com/google/go-cmp v0.6.0
	github.com/hashicorp/aws-sdk-go-base/v2 v2.0.0-beta.53
	github.com/hashicorp/go-cleanhttp v0.5.2
	github.com/hashicorp/terraform-plugin-log v0.9.0
	go.opentelemetry.io/contrib/instrumentation/github.com/aws/aws-sdk-go-v2/otelaws v0.52.0
	go.opentelemetry.io/otel v1.27.0
)

require (
	github.com/aws/aws-sdk-go-v2/aws/protocol/eventstream v1.6.2 // indirect
	github.com/aws/aws-sdk-go-v2/internal/configsources v1.3.10 // indirect
	github.com/aws/aws-sdk-go-v2/internal/endpoints/v2 v2.6.10 // indirect
	github.com/aws/aws-sdk-go-v2/internal/ini v1.8.0 // indirect
	github.com/aws/aws-sdk-go-v2/internal/v4a v1.3.10 // indirect
	github.com/aws/aws-sdk-go-v2/service/dynamodb v1.32.9 // indirect
	github.com/aws/aws-sdk-go-v2/service/iam v1.32.7 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/accept-encoding v1.11.2 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/checksum v1.3.12 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/endpoint-discovery v1.9.11 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/presigned-url v1.11.12 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/s3shared v1.17.10 // indirect
	github.com/aws/aws-sdk-go-v2/service/s3 v1.55.2 // indirect
	github.com/aws/aws-sdk-go-v2/service/sqs v1.32.7 // indirect
	github.com/aws/aws-sdk-go-v2/service/sso v1.20.12 // indirect
	github.com/aws/aws-sdk-go-v2/service/ssooidc v1.24.6 // indirect
	github.com/aws/smithy-go v1.20.2 // indirect
	github.com/fatih/color v1.17.0 // indirect
	github.com/go-logr/logr v1.4.1 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/hashicorp/errwrap v1.1.0 // indirect
	github.com/hashicorp/go-hclog v1.6.3 // indirect
	github.com/hashicorp/go-multierror v1.1.1 // indirect
	github.com/jmespath/go-jmespath v0.4.0 // indirect
	github.com/mattn/go-colorable v0.1.13 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/mitchellh/go-homedir v1.1.0 // indirect
	github.com/mitchellh/go-testing-interface v1.14.1 // indirect
	go.opentelemetry.io/otel/metric v1.27.0 // indirect
	go.opentelemetry.io/otel/trace v1.27.0 // indirect
	golang.org/x/net v0.26.0 // indirect
	golang.org/x/sys v0.21.0 // indirect
	golang.org/x/text v0.16.0 // indirect
)

replace github.com/hashicorp/aws-sdk-go-base/v2 => ../..
