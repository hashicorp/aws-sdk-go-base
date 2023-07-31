module github.com/hashicorp/aws-sdk-go-base/v2/awsv1shim/v2

go 1.19

require (
	github.com/aws/aws-sdk-go v1.44.313
	github.com/aws/aws-sdk-go-v2 v1.20.0
	github.com/aws/aws-sdk-go-v2/config v1.18.31
	github.com/aws/aws-sdk-go-v2/credentials v1.13.30
	github.com/aws/aws-sdk-go-v2/feature/ec2/imds v1.13.7
	github.com/aws/aws-sdk-go-v2/service/sts v1.21.0
	github.com/google/go-cmp v0.5.9
	github.com/hashicorp/aws-sdk-go-base/v2 v2.0.0-beta.32
	github.com/hashicorp/go-cleanhttp v0.5.2
	github.com/hashicorp/terraform-plugin-log v0.9.0
	go.opentelemetry.io/otel v1.16.0
)

require (
	github.com/aws/aws-sdk-go-v2/internal/configsources v1.1.37 // indirect
	github.com/aws/aws-sdk-go-v2/internal/endpoints/v2 v2.4.31 // indirect
	github.com/aws/aws-sdk-go-v2/internal/ini v1.3.38 // indirect
	github.com/aws/aws-sdk-go-v2/service/iam v1.22.0 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/presigned-url v1.9.31 // indirect
	github.com/aws/aws-sdk-go-v2/service/sso v1.13.0 // indirect
	github.com/aws/aws-sdk-go-v2/service/ssooidc v1.15.0 // indirect
	github.com/aws/smithy-go v1.14.0 // indirect
	github.com/fatih/color v1.15.0 // indirect
	github.com/hashicorp/errwrap v1.1.0 // indirect
	github.com/hashicorp/go-hclog v1.5.0 // indirect
	github.com/hashicorp/go-multierror v1.1.1 // indirect
	github.com/jmespath/go-jmespath v0.4.0 // indirect
	github.com/mattn/go-colorable v0.1.13 // indirect
	github.com/mattn/go-isatty v0.0.17 // indirect
	github.com/mitchellh/go-homedir v1.1.0 // indirect
	github.com/mitchellh/go-testing-interface v1.14.1 // indirect
	go.opentelemetry.io/otel/trace v1.16.0 // indirect
	golang.org/x/exp v0.0.0-20230510235704-dd950f8aeaea // indirect
	golang.org/x/sys v0.8.0 // indirect
	gopkg.in/yaml.v2 v2.4.0 // indirect
)

replace github.com/hashicorp/aws-sdk-go-base/v2 => ../..
