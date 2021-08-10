package awsmocks

import (
	"os"
	"strings"
)

func InitSessionTestEnv() (oldEnv []string) {
	oldEnv = StashEnv()
	os.Setenv("AWS_CONFIG_FILE", "file_not_exists")
	os.Setenv("AWS_SHARED_CREDENTIALS_FILE", "file_not_exists")

	return oldEnv
}

func StashEnv() []string {
	env := os.Environ()
	os.Clearenv()
	return env
}

func PopEnv(env []string) {
	os.Clearenv()

	for _, e := range env {
		p := strings.SplitN(e, "=", 2)
		k, v := p[0], ""
		if len(p) > 1 {
			v = p[1]
		}
		os.Setenv(k, v)
	}
}
