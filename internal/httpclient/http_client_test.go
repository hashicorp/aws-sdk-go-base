package httpclient_test

import (
	"crypto/tls"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws/transport/http"
	"github.com/hashicorp/aws-sdk-go-base/v2/internal/config"
	"github.com/hashicorp/aws-sdk-go-base/v2/internal/httpclient"
)

func TestHTTPClientConfiguration_basic(t *testing.T) {
	client, err := httpclient.DefaultHttpClient(&config.Config{})
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}

	transport := client.GetTransport()

	if a, e := transport.MaxIdleConns, http.DefaultHTTPTransportMaxIdleConns; a != e {
		t.Errorf("expected MaxIdleConns to be %d, got %d", e, a)
	}
	if a, e := transport.MaxIdleConnsPerHost, http.DefaultHTTPTransportMaxIdleConnsPerHost; a != e {
		t.Errorf("expected MaxIdleConnsPerHost to be %d, got %d", e, a)
	}
	if a, e := transport.IdleConnTimeout, http.DefaultHTTPTransportIdleConnTimeout; a != e {
		t.Errorf("expected IdleConnTimeout to be %s, got %s", e, a)
	}
	if a, e := transport.TLSHandshakeTimeout, http.DefaultHTTPTransportTLSHandleshakeTimeout; a != e {
		t.Errorf("expected TLSHandshakeTimeout to be %s, got %s", e, a)
	}
	if a, e := transport.ExpectContinueTimeout, http.DefaultHTTPTransportExpectContinueTimeout; a != e {
		t.Errorf("expected ExpectContinueTimeout to be %s, got %s", e, a)
	}
	if !transport.ForceAttemptHTTP2 {
		t.Error("expected ForceAttemptHTTP2 to be true, got false")
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
	client, err := httpclient.DefaultHttpClient(&config.Config{
		Insecure: true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}

	transport := client.GetTransport()

	tlsConfig := transport.TLSClientConfig
	if !tlsConfig.InsecureSkipVerify {
		t.Error("expected InsecureSkipVerify to be true, got false")
	}
}
