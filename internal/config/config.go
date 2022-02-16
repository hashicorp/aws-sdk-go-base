package config

import "time"

type Config struct {
	AccessKey                      string
	APNInfo                        *APNInfo
	AssumeRole                     *AssumeRole
	CallerDocumentationURL         string
	CallerName                     string
	CustomCABundle                 string
	EC2MetadataServiceEndpoint     string
	EC2MetadataServiceEndpointMode string
	HTTPProxy                      string
	IamEndpoint                    string
	Insecure                       bool
	MaxRetries                     int
	Profile                        string
	Region                         string
	SecretKey                      string
	SharedCredentialsFiles         []string
	SharedConfigFiles              []string
	SkipCredsValidation            bool
	SkipEC2MetadataApiCheck        bool
	SkipRequestingAccountId        bool
	StsEndpoint                    string
	StsRegion                      string
	Token                          string
	UseDualStackEndpoint           bool
	UseFIPSEndpoint                bool
	UserAgent                      UserAgentProducts
}

type APNInfo struct {
	PartnerName string
	Products    []UserAgentProduct
}

type UserAgentProduct struct {
	Name    string
	Version string
	Comment string
}

type UserAgentProducts []UserAgentProduct

type AssumeRole struct {
	RoleARN           string
	Duration          time.Duration
	ExternalID        string
	Policy            string
	PolicyARNs        []string
	SessionName       string
	Tags              map[string]string
	TransitiveTagKeys []string
}
