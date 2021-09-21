package awsbase

type Config struct {
	AccessKey              string
	AssumeRole             *AssumeRole
	CallerDocumentationURL string
	CallerName             string
	DebugLogging           bool
	IamEndpoint            string
	Insecure               bool
	MaxRetries             int
	Profile                string
	Region                 string
	SecretKey              string
	SharedCredentialsFiles []string
	SharedConfigFiles      []string
	SkipCredsValidation    bool
	SkipMetadataApiCheck   bool
	StsEndpoint            string
	Token                  string
	UserAgentProducts      []*UserAgentProduct
}

type AssumeRole struct {
	RoleARN           string
	DurationSeconds   int
	ExternalID        string
	Policy            string
	PolicyARNs        []string
	SessionName       string
	Tags              map[string]string
	TransitiveTagKeys []string
}

type UserAgentProduct struct {
	Extra   []string
	Name    string
	Version string
}
