package endpoints

import (
	"regexp"
)

type partition struct {
	id          string
	regionRegex *regexp.Regexp
}

// TODO: this should be generated from the AWS SDK source data
var partitions = []partition{
	{
		id:          "aws",
		regionRegex: regexp.MustCompile(`^(us|eu|ap|sa|ca|me|af)\-\w+\-\d+$`),
	},
	{
		id:          "aws-cn",
		regionRegex: regexp.MustCompile(`^cn\-\w+\-\d+$`),
	},
	{
		id:          "aws-us-gov",
		regionRegex: regexp.MustCompile(`^us\-gov\-\w+\-\d+$`),
	},
	{
		id:          "aws-iso",
		regionRegex: regexp.MustCompile(`^us\-iso\-\w+\-\d+$`),
	},
	{
		id:          "aws-iso-b",
		regionRegex: regexp.MustCompile(`^us\-isob\-\w+\-\d+$`),
	},
}

func PartitionForRegion(regionID string) string {
	for _, p := range partitions {
		if p.regionRegex.MatchString(regionID) {
			return p.id
		}
	}

	return ""
}
