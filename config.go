package awsbase

type Config struct {
	AccessKey             string
	AssumeRoleARN         string
	AssumeRoleExternalID  string
	AssumeRolePolicy      string
	AssumeRoleSessionName string
	CredsFilename         string
	MaxRetries            int
	Profile               string
	Region                string
	SecretKey             string
	SkipMetadataApiCheck  bool
	Token                 string
}
