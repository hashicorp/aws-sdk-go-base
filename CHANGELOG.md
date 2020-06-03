# v0.5.0 (Unreleased)

BREAKING CHANGES

* Credential ordering has changed from static, environment, shared credentials, EC2 metadata, default AWS Go SDK (shared configuration, web identity, ECS, etc.) to static, environment, shared credentials, default AWS Go SDK (shared configuration, web identity, ECS, etc.), ECS metadata, EC2 metadata. #20

ENHANCEMENTS

* Always enable AWS shared configuration file support (no longer require `AWS_SDK_LOAD_CONFIG` environment variable) #38
* Automatically expand `~` prefix for home directories in shared credentials filename handling #40

BUG FIXES

* Properly use custom STS endpoint during AssumeRole API calls triggered by Terraform AWS Provider and S3 Backend configurations #32
* Properly use custom EC2 metadata endpoint during API calls triggered by fallback credentials lookup #32
* Prefer shared configuration handling over EC2 metadata #20
* Prefer ECS credentials over EC2 metadata #20

# v0.4.0 (October 3, 2019)

BUG FIXES

* awsauth: fixed credentials retrieval, validation, and error handling

# v0.3.0 (February 26, 2019)

BUG FIXES

* session: Return error instead of logging with AWS Account ID lookup failure [GH-3]

# v0.2.0 (February 20, 2019)

ENHANCEMENTS

* validation: Add `ValidateAccountID` and `ValidateRegion` functions [GH-1]

# v0.1.0 (February 18, 2019)

* Initial release after split from github.com/terraform-providers/terraform-provider-aws
