package servicemocks

import (
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
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

// InvalidEC2MetadataEndpoint establishes a httptest server to simulate behaviour
// when endpoint doesn't respond as expected
func InvalidEC2MetadataEndpoint() func() {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.Printf("[DEBUG] Received EC2 IMDS API %q request to %q", r.Method, r.RequestURI)
		w.WriteHeader(http.StatusBadRequest)
	}))

	os.Setenv("AWS_EC2_METADATA_SERVICE_ENDPOINT", ts.URL+"/latest")
	return ts.Close
}

// UnsetEnv unsets environment variables for testing a "clean slate" with no
// credentials in the environment
func UnsetEnv(t *testing.T) func() {
	// Grab any existing AWS keys and preserve. In some tests we'll unset these, so
	// we need to have them and restore them after
	e := getEnv()
	if err := os.Unsetenv("AWS_ACCESS_KEY_ID"); err != nil {
		t.Fatalf("Error unsetting env var AWS_ACCESS_KEY_ID: %s", err)
	}
	if err := os.Unsetenv("AWS_SECRET_ACCESS_KEY"); err != nil {
		t.Fatalf("Error unsetting env var AWS_SECRET_ACCESS_KEY: %s", err)
	}
	if err := os.Unsetenv("AWS_SESSION_TOKEN"); err != nil {
		t.Fatalf("Error unsetting env var AWS_SESSION_TOKEN: %s", err)
	}
	if err := os.Unsetenv("AWS_PROFILE"); err != nil {
		t.Fatalf("Error unsetting env var AWS_PROFILE: %s", err)
	}
	if err := os.Unsetenv("AWS_SHARED_CREDENTIALS_FILE"); err != nil {
		t.Fatalf("Error unsetting env var AWS_SHARED_CREDENTIALS_FILE: %s", err)
	}
	// The Shared Credentials Provider has a very reasonable fallback option of
	// checking the user's home directory for credentials, which may create
	// unexpected results for users running these tests
	os.Setenv("HOME", "/dev/null")

	return func() {
		// re-set all the envs we unset above
		if err := os.Setenv("AWS_ACCESS_KEY_ID", e.Key); err != nil {
			t.Fatalf("Error resetting env var AWS_ACCESS_KEY_ID: %s", err)
		}
		if err := os.Setenv("AWS_SECRET_ACCESS_KEY", e.Secret); err != nil {
			t.Fatalf("Error resetting env var AWS_SECRET_ACCESS_KEY: %s", err)
		}
		if err := os.Setenv("AWS_SESSION_TOKEN", e.Token); err != nil {
			t.Fatalf("Error resetting env var AWS_SESSION_TOKEN: %s", err)
		}
		if err := os.Setenv("AWS_PROFILE", e.Profile); err != nil {
			t.Fatalf("Error resetting env var AWS_PROFILE: %s", err)
		}
		if err := os.Setenv("AWS_SHARED_CREDENTIALS_FILE", e.CredsFilename); err != nil {
			t.Fatalf("Error resetting env var AWS_SHARED_CREDENTIALS_FILE: %s", err)
		}
		if err := os.Setenv("HOME", e.Home); err != nil {
			t.Fatalf("Error resetting env var HOME: %s", err)
		}
	}
}

func SetEnv(s string, t *testing.T) func() {
	e := getEnv()
	// Set all the envs to a dummy value
	if err := os.Setenv("AWS_ACCESS_KEY_ID", s); err != nil {
		t.Fatalf("Error setting env var AWS_ACCESS_KEY_ID: %s", err)
	}
	if err := os.Setenv("AWS_SECRET_ACCESS_KEY", s); err != nil {
		t.Fatalf("Error setting env var AWS_SECRET_ACCESS_KEY: %s", err)
	}
	if err := os.Setenv("AWS_SESSION_TOKEN", s); err != nil {
		t.Fatalf("Error setting env var AWS_SESSION_TOKEN: %s", err)
	}
	if err := os.Setenv("AWS_PROFILE", s); err != nil {
		t.Fatalf("Error setting env var AWS_PROFILE: %s", err)
	}
	if err := os.Setenv("AWS_SHARED_CREDENTIALS_FILE", s); err != nil {
		t.Fatalf("Error setting env var AWS_SHARED_CREDENTIALS_FLE: %s", err)
	}

	return func() {
		// re-set all the envs we unset above
		if err := os.Setenv("AWS_ACCESS_KEY_ID", e.Key); err != nil {
			t.Fatalf("Error resetting env var AWS_ACCESS_KEY_ID: %s", err)
		}
		if err := os.Setenv("AWS_SECRET_ACCESS_KEY", e.Secret); err != nil {
			t.Fatalf("Error resetting env var AWS_SECRET_ACCESS_KEY: %s", err)
		}
		if err := os.Setenv("AWS_SESSION_TOKEN", e.Token); err != nil {
			t.Fatalf("Error resetting env var AWS_SESSION_TOKEN: %s", err)
		}
		if err := os.Setenv("AWS_PROFILE", e.Profile); err != nil {
			t.Fatalf("Error setting env var AWS_PROFILE: %s", err)
		}
		if err := os.Setenv("AWS_SHARED_CREDENTIALS_FILE", s); err != nil {
			t.Fatalf("Error setting env var AWS_SHARED_CREDENTIALS_FLE: %s", err)
		}
	}
}

func getEnv() *currentEnv {
	// Grab any existing AWS keys and preserve. In some tests we'll unset these, so
	// we need to have them and restore them after
	return &currentEnv{
		Key:           os.Getenv("AWS_ACCESS_KEY_ID"),
		Secret:        os.Getenv("AWS_SECRET_ACCESS_KEY"),
		Token:         os.Getenv("AWS_SESSION_TOKEN"),
		Profile:       os.Getenv("AWS_PROFILE"),
		CredsFilename: os.Getenv("AWS_SHARED_CREDENTIALS_FILE"),
		Home:          os.Getenv("HOME"),
	}
}

// struct to preserve the current environment
type currentEnv struct {
	Key, Secret, Token, Profile, CredsFilename, Home string
}
