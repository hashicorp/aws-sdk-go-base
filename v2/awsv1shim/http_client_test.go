package awsv1shim

import (
	"crypto/tls"
	"net/http"
	"testing"

	awshttp "github.com/aws/aws-sdk-go-v2/aws/transport/http"
	"github.com/hashicorp/aws-sdk-go-base/v2/internal/config"
)

func TestHTTPClientConfiguration_basic(t *testing.T) {
	client, err := defaultHttpClient(&config.Config{})
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}

	transport, ok := client.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("Unexpected type for HTTP client transport: %T", client.Transport)
	}

	if a, e := transport.MaxIdleConns, awshttp.DefaultHTTPTransportMaxIdleConns; a != e {
		t.Errorf("expected MaxIdleConns to be %d, got %d", e, a)
	}
	if a, e := transport.MaxIdleConnsPerHost, awshttp.DefaultHTTPTransportMaxIdleConnsPerHost; a != e {
		t.Errorf("expected MaxIdleConnsPerHost to be %d, got %d", e, a)
	}
	if a, e := transport.IdleConnTimeout, awshttp.DefaultHTTPTransportIdleConnTimeout; a != e {
		t.Errorf("expected IdleConnTimeout to be %s, got %s", e, a)
	}
	if a, e := transport.TLSHandshakeTimeout, awshttp.DefaultHTTPTransportTLSHandleshakeTimeout; a != e {
		t.Errorf("expected TLSHandshakeTimeout to be %s, got %s", e, a)
	}
	if a, e := transport.ExpectContinueTimeout, awshttp.DefaultHTTPTransportExpectContinueTimeout; a != e {
		t.Errorf("expected ExpectContinueTimeout to be %s, got %s", e, a)
	}
	if !transport.ForceAttemptHTTP2 {
		t.Error("expected ForceAttemptHTTP2 to be true, got false")
	}
	if transport.DisableKeepAlives {
		t.Error("expected DisableKeepAlives to be false, got true")
	}

	tlsConfig := transport.TLSClientConfig
	if a, e := int(tlsConfig.MinVersion), tls.VersionTLS12; a != e {
		t.Errorf("expected tlsConfig.MinVersion to be %d, got %d", e, a)
	}
	if tlsConfig.InsecureSkipVerify {
		t.Error("expected InsecureSkipVerify to be false, got true")
	}
}

func TestHTTPClientConfiguration_insecureHTTPS(t *testing.T) {
	client, err := defaultHttpClient(&config.Config{
		Insecure: true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}

	transport, ok := client.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("Unexpected type for HTTP client transport: %T", client.Transport)
	}

	tlsConfig := transport.TLSClientConfig
	if !tlsConfig.InsecureSkipVerify {
		t.Error("expected InsecureSkipVerify to be true, got false")
	}
}
